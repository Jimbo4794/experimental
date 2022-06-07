package scmpoll

import (
	"context"
	"fmt"
	"time"

	poll "github.com/tektoncd/experimental/polling/pkg/apis/scmpoll"
	"github.com/tektoncd/experimental/polling/pkg/apis/scmpoll/v1alpha1"

	scmpollv1alpha1 "github.com/tektoncd/experimental/polling/pkg/apis/scmpoll"
	scmpoll "github.com/tektoncd/experimental/polling/pkg/client/clientset/versioned"
	scmpollInformers "github.com/tektoncd/experimental/polling/pkg/client/informers/externalversions/scmpoll/v1alpha1"
	scmpollListers "github.com/tektoncd/experimental/polling/pkg/client/listers/scmpoll/v1alpha1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	pkgreconciler "knative.dev/pkg/reconciler"
)

// Implements controller.Reconciler for the polling resources
type Reconciler struct {
	KubeClientSet kubernetes.Interface

	SCMPollClientSet   scmpoll.Interface
	SCMPollLister      scmpollListers.SCMPollLister
	SCMPollStateLister scmpollListers.SCMPollStateLister
	SCMPollInformer    scmpollInformers.SCMPollInformer
}

/**
Basic Logic:
Run poll
If find change then create or update state

So when we run a poll the response needs to be enough information to identify state, scmpoll name and state id
*/

func (c *Reconciler) ReconcileKind(ctx context.Context, p *v1alpha1.SCMPoll) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Infof("Poll run for %s", p.Name)

	if err := p.Validate(ctx); err != nil {
		return controller.NewPermanentError(fmt.Errorf("SCMPoll could not be validated: %v", err))
	}

	// Get poll list
	pollRepos, err := scmpollv1alpha1.GetTypedRepoList(p.Spec.Repositories)
	if err != nil {
		return controller.NewPermanentError(fmt.Errorf("Problem creating poll list %v", err))
	}

	// Provide any credentials if required
	err = c.AuthenticatePollRepos(ctx, p, pollRepos)
	if err != nil {
		return controller.NewPermanentError(fmt.Errorf("Problem authenticating poll repos: %v", err))
	}

	// Poll
	desiredStates, err := c.PollRepos(ctx, pollRepos)
	if err != nil {
		return controller.NewPermanentError(err)
	}

	return c.reconcileStates(ctx, p, desiredStates)
}

func (c *Reconciler) reconcileStates(ctx context.Context, p *v1alpha1.SCMPoll, desiredStates map[v1alpha1.SCMPollRepositoryInteface][]v1alpha1.SCMRepositoryStatus) pkgreconciler.Event {
	logger := logging.FromContext(ctx)

	// Remove any stale states
	err := c.removeStaleStates(ctx, p, desiredStates)
	if err != nil {
		return controller.NewPermanentError(err)
	}

	for repo, states := range desiredStates {
		logger.Debugf("Looking at repo %s, has states: %v", repo.GetName(), states)
		for _, state := range states {
			logger.Debugf("Looking for state: %s, ID: %s, Params: %v", state.Name, state.StateId, state.StatusParams)
			selector := labels.NewSelector().
				Add(mustNewRequirement("scmpoll", selection.Equals, []string{p.Name})).
				Add(mustNewRequirement("stateid", selection.Equals, []string{state.StateId}))

			sysState, err := c.findSystemState(ctx, selector)
			if err != nil {
				return controller.NewPermanentError(fmt.Errorf("Problem listing states: %v", err))
			}

			// New state
			if sysState == nil {
				logger.Debugf("No state found - Generating new state")
				err = c.createState(ctx, p, repo.GetName(), state)
				if err != nil {
					return err
				}
				return controller.NewRequeueImmediately()
			}
			logger.Debugf("Found a state in the system: %v", sysState)

			if sysState.Spec.Pending {
				continue
			}

			// Translate the retrieved states into typed states
			currentRepos, err := poll.GetTypedRepoStates(sysState)
			if err != nil {
				return fmt.Errorf("failed to find scmpollstate states: %s", err)
			}

			if currentRepo, ok := currentRepos[repo.GetName()]; !ok {
				sysState.Status.SCMPollRepos[repo.GetName()] = &state
				_, err = c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).UpdateStatus(ctx, sysState, v1.UpdateOptions{})
				if err != nil {
					return controller.NewPermanentError(fmt.Errorf("Failed to update state status: %s", err))
				}
				continue
			} else {
				updated, err := currentRepo.HasStatusChanged(state)
				if err != nil {
					return fmt.Errorf("failed checking state status: %s", err)
				}

				// New update
				if updated {
					sysState.Status.Outdated = updated
					sysState.Status.SCMPollRepos[repo.GetName()] = &state
					_, err = c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).UpdateStatus(ctx, sysState, v1.UpdateOptions{})
					if err != nil {
						return controller.NewPermanentError(fmt.Errorf("Failed to update state status: %s", err))
					}
					continue
				}
			}
			sysState.Status.LastUpdate = &v1.Time{Time: time.Now()}
			c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).UpdateStatus(ctx, sysState, v1.UpdateOptions{})
		}
	}
	return c.completePoll(ctx, p)
}

func (c *Reconciler) PollRepos(ctx context.Context, repos map[string]v1alpha1.SCMPollRepositoryInteface) (map[v1alpha1.SCMPollRepositoryInteface][]v1alpha1.SCMRepositoryStatus, error) {
	logger := logging.FromContext(ctx)
	states := make(map[v1alpha1.SCMPollRepositoryInteface][]v1alpha1.SCMRepositoryStatus)
	for _, repo := range repos {
		logger.Debugf("Peforming poll on %s", repo.GetName())
		responses, err := repo.Poll()
		if err != nil {
			return nil, controller.NewPermanentError(fmt.Errorf("Problem performing poll on repo %s: %v", repo.GetName(), err))
		}
		logger.Debugf("Responses from poll %s: %v", repo.GetName(), responses)
		states[repo] = responses
	}
	return states, nil
}

func (c *Reconciler) createState(ctx context.Context, p *v1alpha1.SCMPoll, repoName string, polledState v1alpha1.SCMRepositoryStatus) error {
	logger := logging.FromContext(ctx)
	logger.Debugf("Creating state from %v", polledState)
	var parms []pipelines.Param
	pipelineSpec := p.Spec.PipelineRunSpec
	subs := make(map[string]string)
	for _, param := range polledState.StatusParams {
		logger.Debugf("Adding the value to be subsituted: %s:%s", polledState.Name+"."+param.Name, param.Value.StringVal)
		subs[polledState.Name+"."+param.Name] = param.Value.StringVal
	}
	logger.Debugf("Need to apply subsitutions to spec params %v:%v", subs, pipelineSpec.Params)
	for _, p := range pipelineSpec.Params {
		p.Value.ApplyReplacements(subs, nil)
		parms = append(parms, p)
	}
	pipelineSpec.Params = parms

	s := &v1alpha1.SCMPollState{
		ObjectMeta: v1.ObjectMeta{
			Name:      p.Name + "-" + polledState.StateId,
			Namespace: p.Namespace,
			Labels: map[string]string{
				"scmpoll": p.Name,
				"stateid": polledState.StateId,
			},
			OwnerReferences: []v1.OwnerReference{
				{
					Kind:               "SCMPoll",
					APIVersion:         "tekton.dev/v1alpha1",
					BlockOwnerDeletion: &p.Spec.Tidy,
					Name:               p.Name,
					UID:                p.GetUID(),
				},
			},
		},
		Spec: v1alpha1.SCMPollStateSpec{
			Name:         repoName,
			Repositories: p.Spec.Repositories,
			Tidy:         p.Spec.Tidy,
			Pending:      true,
			PipelineSpec: pipelineSpec,
		},
	}
	_, err := c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Create(ctx, s, v1.CreateOptions{})
	if err != nil {
		return controller.NewPermanentError(fmt.Errorf("Problem create poll state: %v", err))
	}
	return nil
}

func (c *Reconciler) removeStaleStates(ctx context.Context, p *v1alpha1.SCMPoll, desiredStates map[v1alpha1.SCMPollRepositoryInteface][]v1alpha1.SCMRepositoryStatus) error {
	logger := logging.FromContext(ctx)
	var states []string
	selector := labels.NewSelector().Add(mustNewRequirement("scmpoll", selection.Equals, []string{p.Name}))
	list, err := c.SCMPollStateLister.List(selector)
	if err != nil {
		return err
	}
	// Create the list of all polled states
	for _, statelist := range desiredStates {
		for _, state := range statelist {
			states = append(states, p.Name+"-"+state.StateId)
		}
	}

	// loop through all the states on k8s and check that state is still in the polled states
	for _, state := range list {
		if inStringList(state.Name, states) {
			continue
		}
		logger.Debugf("State %s is not in the list from required states: %v", state.Name, states)
		c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Delete(ctx, state.Name, v1.DeleteOptions{})
	}
	return nil
}

func (c *Reconciler) AuthenticatePollRepos(ctx context.Context, p *v1alpha1.SCMPoll, pollRepos map[string]v1alpha1.SCMPollRepositoryInteface) error {
	logger := logging.FromContext(ctx)
	for _, repo := range pollRepos {
		sa, err := c.KubeClientSet.CoreV1().ServiceAccounts(p.Namespace).Get(ctx, repo.GetServiceAccountName(), v1.GetOptions{})
		if err != nil {
			logger.Warnf("Failed to find any service accounts for %s with the name %s, using default", p.Name, repo.GetServiceAccountName())
		} else {
			for _, secretEntry := range sa.Secrets {
				if secretEntry.Name == "" {
					continue
				}

				secret, err := c.KubeClientSet.CoreV1().Secrets(p.Namespace).Get(ctx, secretEntry.Name, v1.GetOptions{})
				if err != nil {
					return fmt.Errorf("Failed to get Poling secrets")
				}
				annotations := secret.GetAnnotations()
				for _, v := range annotations {
					if v == repo.GetEndpoint() {
						if err = repo.Authenticate(secret); err != nil {
							logger.Errorf("Failed to authenticate to endpoint, continuing un-authenticated: %s", err.Error())
						} else {
							logger.Debug("Service account found for endpoint %v, authenticating", repo.GetName())
							break
						}
					}
				}
			}
		}
	}
	return nil
}

// Looks for an exsisting pollstates
func (c *Reconciler) findSystemState(ctx context.Context, selector labels.Selector) (*v1alpha1.SCMPollState, error) {
	logger := logging.FromContext(ctx)
	logger.Debugf("Listing selector: %s", selector)
	list, err := c.SCMPollStateLister.List(selector)
	logger.Debugf("Listing states: %s", list)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	if len(list) > 1 {
		logger.Warnf("Multiple states found, returning first: %v", list)
	}
	return list[0], nil
}

func (c *Reconciler) completePoll(ctx context.Context, p *v1alpha1.SCMPoll) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Debugf("Finishing poll - polling again in %v seconds", p.Spec.SCMPollFrequency)
	return controller.NewRequeueAfter(time.Duration(p.Spec.SCMPollFrequency * time.Second.Nanoseconds()))
}

// mustNewRequirement panics if there are any errors constructing our selectors.
func mustNewRequirement(key string, op selection.Operator, vals []string) labels.Requirement {
	r, err := labels.NewRequirement(key, op, vals)
	if err != nil {
		panic(fmt.Sprintf("mustNewRequirement(%v, %v, %v) = %v", key, op, vals, err))
	}
	return *r
}

func inStringList(s string, sl []string) bool {
	for _, v := range sl {
		if v == s {
			return true
		}
	}
	return false
}
