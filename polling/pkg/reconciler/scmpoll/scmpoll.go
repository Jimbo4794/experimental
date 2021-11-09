package scmpoll

import (
	"context"
	"fmt"
	"time"

	"github.com/tektoncd/experimental/polling/pkg/apis/scmpoll/v1alpha1"

	pipelineclientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelinerunLister "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"

	scmpollv1alpha1 "github.com/tektoncd/experimental/polling/pkg/apis/scmpoll"
	scmpoll "github.com/tektoncd/experimental/polling/pkg/client/clientset/versioned"
	scmpollInformers "github.com/tektoncd/experimental/polling/pkg/client/informers/externalversions/scmpoll/v1alpha1"
	scmpollListers "github.com/tektoncd/experimental/polling/pkg/client/listers/scmpoll/v1alpha1"
	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/apis/duck/v1beta1"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	pkgreconciler "knative.dev/pkg/reconciler"
)

// Implements controller.Reconciler for the polling resources
type Reconciler struct {
	KubeClientSet kubernetes.Interface

	SCMPollClientSet   scmpoll.Interface
	SCMPollLister      scmpollListers.SCMPollLister
	SCMPollStateLister scmpollListers.SCMPollStateLister
	SCMPollInformer    scmpollInformers.SCMPollInformer

	PipelineRunLister  pipelinerunLister.PipelineRunLister
	PipelinesClientSet pipelineclientset.Interface
}

/**
Basic Logic:
Run poll
If find change then create or update state

So when we run a poll the response needs to be enough information to identify state, scmpoll name and state id
*/

func (c *Reconciler) ReconcileKind(ctx context.Context, p *v1alpha1.SCMPoll) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Infof("Poll run for %s is starting", p.Name)

	if err := p.Validate(ctx); err != nil {
		return controller.NewPermanentError(fmt.Errorf("SCMPoll could not be validated: %v", err))
	}

	//Perform poll
	pollRepos, err := scmpollv1alpha1.FormRunList(p.Name, p)
	if err != nil {
		return controller.NewPermanentError(fmt.Errorf("Problem creating poll list %v", err))
	}
	// Provide any credentials if required
	err = c.AuthenticatePollRepos(ctx, p, pollRepos)
	if err != nil {
		return controller.NewPermanentError(fmt.Errorf("Problem authenticating poll repos: %v", err))
	}

	for _, repo := range pollRepos {
		logger.Infof("Peforming poll on %s", repo.GetName())
		responses, err := repo.Poll()
		if err != nil {
			return controller.NewPermanentError(fmt.Errorf("Problem performing poll on repo %s: %v", repo.GetName(), err))
		}

		err = c.removeStaleStates(ctx, p, responses)
		if err != nil {
			return controller.NewPermanentError(fmt.Errorf("Problem removing stale states"))
		}

		// Look for a state for each response
		for _, resp := range responses {
			selector := labels.NewSelector().
				Add(mustNewRequirement("scmpoll", selection.Equals, []string{p.Name})).
				Add(mustNewRequirement("stateid", selection.Equals, []string{resp.StateId}))

			state, err := c.findPollState(ctx, selector)
			c.updateNextPollStatus(ctx, state, int(p.Spec.SCMPollFrequency))
			if err != nil {
				return controller.NewPermanentError(fmt.Errorf("Problem listing states: %v", err))
			}

			if state == nil {
				logger.Infof("No state found - Generating new state and Pipelinerun")
				repo.Update(resp, v1beta1.Conditions{apis.Condition{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionUnknown,
				}})
				c.createNewPipelineAndState(ctx, p, resp.StateId, repo.GetName())
			} else {
				stateRepoList, err := scmpollv1alpha1.GetTypedRepoPollStates(state)
				if err != nil {
					return controller.NewPermanentError(fmt.Errorf("Problem getting typed repo states: %v", err))
				}

				// Find the PR's
				prs, err := c.PipelineRunLister.List(selector)
				if err != nil {
					return controller.NewPermanentError(fmt.Errorf("Failed to locate pipelineruns for  s: %v", state.Name, err))
				}
				latestPR := getLatest(prs)
				c.updateStateStatus(ctx, state, &resp, latestPR)
				if latestPR == nil {
					logger.Infof("Expected pipelinerun not found yet, requeue check")
					// PR not availble yet, requeue
					return controller.NewRequeueAfter(time.Second * 3)
				}
				latestPR.Status.GetConditions()

				repo.Update(resp, latestPR.Status.Conditions)
				err = c.addStateAsOwnerIfNotOwned(ctx, latestPR, state)
				if err != nil {
					return controller.NewPermanentError(fmt.Errorf("Failed to apply onwership for %s: %v", state.Name, err))
				}

				if len(stateRepoList) == 0 {
					return controller.NewRequeueImmediately()
				}

				if !p.Spec.ConcurrentPipelines {
					if state.Status.PipelineRunning {
						logger.Infof("Pipeline running and parrallel runs not allowed, requeuing %s", p.Name)
						return c.completePoll(ctx, p)
					}
				}

				for _, repoStatus := range stateRepoList {
					if repoStatus.GetName() == resp.Name {
						changed, err := repoStatus.HasStatusChanged(resp)
						if err != nil {
							return controller.NewPermanentError(fmt.Errorf("problem checking repostatus %s: %v", repoStatus.GetName(), err))
						}
						if changed {
							logger.Infof("Detected a change in %s, submitting pipelinerun", repoStatus.GetName())
							c.submitStatePipelineRun(ctx, p, resp.StateId)
							repo.Update(resp, v1beta1.Conditions{apis.Condition{
								Type:   apis.ConditionSucceeded,
								Status: corev1.ConditionUnknown,
							}})
						}
						logger.Infof("No changes detected in %s, doing nothing", repoStatus.GetName())
						break
					}
				}

			}

		}
	}
	logger.Infof("No actions taken for %s", p.Name)
	return c.completePoll(ctx, p)
}

func (c *Reconciler) updateNextPollStatus(ctx context.Context, state *v1alpha1.SCMPollState, pollFrequency int) {
	state.Status.NextPoll = &v1.Time{
		Time: time.Now().Add(time.Second * time.Duration(pollFrequency)),
	}
	c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(state.Namespace).UpdateStatus(ctx, state, v1.UpdateOptions{})
}

func (c *Reconciler) createNewPipelineAndState(ctx context.Context, p *v1alpha1.SCMPoll, stateId, repoName string) pkgreconciler.Event {
	err := c.submitStatePipelineRun(ctx, p, stateId)
	if err != nil {
		return controller.NewPermanentError(fmt.Errorf("problem submitting new pipelinerun: %v", err))
	}

	s := &v1alpha1.SCMPollState{
		ObjectMeta: v1.ObjectMeta{
			Name:      p.Name + "-" + stateId,
			Namespace: p.Namespace,
			Labels: map[string]string{
				"scmpoll": p.Name,
				"stateid": stateId,
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
			Name: repoName,
			Tidy: p.Spec.Tidy,
		},
	}
	_, err = c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Create(ctx, s, v1.CreateOptions{})
	if err != nil {
		return controller.NewPermanentError(fmt.Errorf("Problem create poll state: %v", err))
	}
	return controller.NewRequeueImmediately()
}

func (c *Reconciler) addStateAsOwnerIfNotOwned(ctx context.Context, pr *pipelines.PipelineRun, state *v1alpha1.SCMPollState) error {
	owners := pr.ObjectMeta.GetOwnerReferences()
	for _, owner := range owners {
		if owner.UID != state.GetUID() {
			continue
		}
		return nil
	}
	owners = append(owners, v1.OwnerReference{
		Kind:       "SCMPollState",
		APIVersion: "tekton.dev/v1alpha1",

		BlockOwnerDeletion: &state.Spec.Tidy,
		Name:               state.Name,
		UID:                state.GetUID(),
	})
	pr, _ = c.PipelinesClientSet.TektonV1beta1().PipelineRuns(pr.Namespace).Get(ctx, pr.Name, v1.GetOptions{})
	pr.ObjectMeta.OwnerReferences = owners
	_, err := c.PipelinesClientSet.TektonV1beta1().PipelineRuns(pr.Namespace).Update(ctx, pr, v1.UpdateOptions{})
	return err
}

func (c *Reconciler) removeStaleStates(ctx context.Context, p *v1alpha1.SCMPoll, polledStates []v1alpha1.SCMRepositoryStatus) error {
	logger := logging.FromContext(ctx)
	var states []string
	selector := labels.NewSelector().Add(mustNewRequirement("scmpoll", selection.Equals, []string{p.Name}))
	list, err := c.SCMPollStateLister.List(selector)
	if err != nil {
		return err
	}
	// Create the list of all polled states
	for _, polledState := range polledStates {
		states = append(states, p.Name+"-"+polledState.StateId)
	}

	// loop through all the states on k8s and check that state is still in the polled states
	for _, state := range list {
		if inStringList(state.Name, states) {
			continue
		}
		logger.Infof("State %s is not in the list from polled states: %v", state.Name, states)
		c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Delete(ctx, state.Name, v1.DeleteOptions{})
	}
	return nil
}

func inStringList(s string, sl []string) bool {
	for _, v := range sl {
		if v == s {
			return true
		}
	}
	return false
}

func (c *Reconciler) updateStateStatus(ctx context.Context, s *v1alpha1.SCMPollState, resp *v1alpha1.SCMRepositoryStatus, pr *pipelines.PipelineRun) error {
	// repos := p.Spec.Repositories
	var repoStates map[string]*v1alpha1.SCMRepositoryStatus

	if s.Status.SCMPollStates == nil {
		repoStates = make(map[string]*v1alpha1.SCMRepositoryStatus)
	} else {
		repoStates = s.Status.SCMPollStates
	}

	if pr == nil {
		s.Status = v1alpha1.SCMPollStateStatus{
			SCMPollStateStatusFields: v1alpha1.SCMPollStateStatusFields{
				PipelineRunning: false,
				Outdated:        false,
				LastUpdate:      &v1.Time{time.Now()},
				SCMPollStates:   repoStates,
			},
		}
	} else {
		s.Status = v1alpha1.SCMPollStateStatus{
			SCMPollStateStatusFields: v1alpha1.SCMPollStateStatusFields{
				PipelineRunning: isPipelineRunning(pr),
				Outdated:        false,
				LastUpdate:      &v1.Time{time.Now()},
				SCMPollStates:   repoStates,
			},
		}
	}

	s.Status.SCMPollStates[resp.Name] = resp
	_, err := c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(s.Namespace).UpdateStatus(ctx, s, v1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func getLatest(list []*pipelines.PipelineRun) *pipelines.PipelineRun {
	var latest *pipelines.PipelineRun
	if len(list) == 0 {
		return nil
	}
	latest = list[0]
	for _, pr := range list {
		if pr.CreationTimestamp.After(latest.CreationTimestamp.Time) {
			latest = pr
		}
	}
	return latest
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

func (c *Reconciler) completePoll(ctx context.Context, p *v1alpha1.SCMPoll) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Infof("Finishing poll - polling again in %v seconds", p.Spec.SCMPollFrequency)
	return controller.NewRequeueAfter(time.Duration(p.Spec.SCMPollFrequency * time.Second.Nanoseconds()))
}

// Looks for an exsisting pollstates
func (c *Reconciler) findPollState(ctx context.Context, selector labels.Selector) (*v1alpha1.SCMPollState, error) {
	logger := logging.FromContext(ctx)
	list, err := c.SCMPollStateLister.List(selector)
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

func (c *Reconciler) retrievePipelineRuns(ctx context.Context, p *v1alpha1.SCMPoll) ([]*pipelines.PipelineRun, error) {
	selector := labels.NewSelector().Add(
		mustNewRequirement("scmpoll", selection.Equals, []string{p.Name}),
	)
	prs, err := c.PipelineRunLister.List(selector)
	if err != nil {
		return nil, fmt.Errorf("problem requesting pipelineruns for poll %s: %v", p.Name, err)
	}

	return prs, nil
}

func (c *Reconciler) submitStatePipelineRun(ctx context.Context, p *v1alpha1.SCMPoll, stateid string) error {
	labels := make(map[string]string)
	labels["scmpoll"] = p.Name
	labels["stateid"] = stateid
	subs := make(map[string]string)

	pollRepos, err := scmpollv1alpha1.FormRunList(p.Name, p)
	for _, v := range pollRepos {
		// Need to update everything of the same stateid
		responses, err := v.Poll()
		for _, resp := range responses {
			for _, param := range resp.StatusParams {
				subs[resp.Name+"."+param.Name] = param.Value.StringVal
			}
		}
		if err != nil {
			return nil
		}
	}
	var parms []pipelines.Param
	for _, p := range p.Spec.PipelineRunSpec.Params {
		p.Value.ApplyReplacements(subs, nil)
		parms = append(parms, p)
	}
	p.Spec.PipelineRunSpec.Params = parms

	pr := &pipelines.PipelineRun{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: p.Name + "-" + stateid + "-",
			Namespace:    p.Namespace,
			Labels:       labels,
		},
		Spec: p.Spec.PipelineRunSpec,
	}

	_, err = c.PipelinesClientSet.TektonV1beta1().PipelineRuns(p.Namespace).Create(ctx, pr, v1.CreateOptions{})

	return err
}

func isPipelineRunning(pr *pipelines.PipelineRun) bool {
	return pr.Status.GetCondition(apis.ConditionSucceeded).IsUnknown()
}

// mustNewRequirement panics if there are any errors constructing our selectors.
func mustNewRequirement(key string, op selection.Operator, vals []string) labels.Requirement {
	r, err := labels.NewRequirement(key, op, vals)
	if err != nil {
		panic(fmt.Sprintf("mustNewRequirement(%v, %v, %v) = %v", key, op, vals, err))
	}
	return *r
}
