package scmpollstate

import (
	"context"
	"fmt"
	"time"

	poll "github.com/tektoncd/experimental/polling/pkg/apis/scmpoll"
	"github.com/tektoncd/experimental/polling/pkg/apis/scmpoll/v1alpha1"

	scmpoll "github.com/tektoncd/experimental/polling/pkg/client/clientset/versioned"
	scmpollListers "github.com/tektoncd/experimental/polling/pkg/client/listers/scmpoll/v1alpha1"
	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	pipelineclientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelinerunLister "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/apis/duck/v1beta1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	pkgreconciler "knative.dev/pkg/reconciler"
)

type Reconciler struct {
	KubeClientSet kubernetes.Interface

	SCMPollClientSet   scmpoll.Interface
	SCMPollLister      scmpollListers.SCMPollLister
	SCMPollStateLister scmpollListers.SCMPollStateLister

	PipelineRunLister  pipelinerunLister.PipelineRunLister
	PipelinesClientSet pipelineclientset.Interface
}

func (c *Reconciler) ReconcileKind(ctx context.Context, p *v1alpha1.SCMPollState) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Debugf("Reconciling scmpollstate %s", p.Name)
	repos, err := getRepos(ctx, p.Spec.Repositories)
	if err != nil {
		return controller.NewPermanentError(fmt.Errorf("Failed to reach repos: %v", err))
	}
	c.AuthenticatePollRepos(ctx, p, repos)

	if p.Spec.Pending {
		logger.Debugf("%s is pending, init status being set", p.Name)
		err = c.updateRepos(p, repos, v1beta1.Conditions{
			{
				Type:   apis.ConditionSucceeded,
				Status: corev1.ConditionUnknown,
			},
		})
		if err != nil {
			logger.Errorf("Updating repos failed: %v", err)
		}
		return c.initialiseState(ctx, p)
	}

	// Need to wait for state to prevent unauthed runs
	if len(p.Status.SCMPollRepos) == 0 {
		return nil
	}

	for _, state := range p.Status.SCMPollRepos {
		if !state.Authorised {
			err = c.updateRepos(p, repos, v1beta1.Conditions{
				{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionUnknown,
				},
			})
			if err != nil {
				logger.Errorf("Updating repos failed: %v", err)
			}
			return nil
		}
	}

	if p.Status.PipelineRunning {
		logger.Debugf("Pipeline marked as running")
		selector := labels.NewSelector().
			Add(mustNewRequirement("scmpollstate", selection.Equals, []string{p.Name}))

		prs, _ := c.PipelineRunLister.List(selector)
		logger.Debugf("Found %v PRS, checking if any are still running", len(prs))
		for _, pr := range prs {
			if isPipelineRunning(pr) {
				logger.Debugf("Pipeline %v is still running, ending", pr)
				return nil
			}
		}
		latestPr := getLatest(prs)
		logger.Debugf("Latest pr is: %s", latestPr.Name)
		p, _ := c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Get(ctx, p.Name, v1.GetOptions{})
		p.Status.PipelineRunning = false
		p.Status.StateStatus = isPipelineSuccessful(latestPr)
		switch p.Status.StateStatus {
		case v1alpha1.StateFailed:
			logger.Debugf("PR failed")
			err = c.updateRepos(p, repos, v1beta1.Conditions{
				{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionFalse,
				},
			})
		case v1alpha1.StatePass:
			logger.Debugf("PR Passed")
			err = c.updateRepos(p, repos, v1beta1.Conditions{
				{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionTrue,
				},
			})
		case v1alpha1.StateUnknown:
			logger.Debugf("PR Unknown")
			err = c.updateRepos(p, repos, v1beta1.Conditions{
				{
					Type:   apis.ConditionSucceeded,
					Status: corev1.ConditionUnknown,
				},
			})
		}
		if err != nil {
			logger.Errorf("Updating repos failed: %v", err)
		}
		_, err = c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).UpdateStatus(ctx, p, v1.UpdateOptions{})
		if err != nil {
			return controller.NewPermanentError(fmt.Errorf("Failed to update state: %s", err))
		}
		return nil
	}

	if p.Status.Outdated {
		logger.Debugf("%s is outdated, submitting pipelinerun", p.Name)
		c.updateRepos(p, repos, v1beta1.Conditions{
			{
				Type:   apis.ConditionSucceeded,
				Status: corev1.ConditionUnknown,
			},
		})
		err := c.submitPipelineRun(ctx, p)
		if err != nil {
			return controller.NewPermanentError(fmt.Errorf("Failed to create pipelinerun: %s", err))
		}

		p.Status.Outdated = false
		p.Status.PipelineRunning = true
		p.Status.StateStatus = v1alpha1.StateUnknown
		p, _ := c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Get(ctx, p.Name, v1.GetOptions{})
		_, err = c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Update(ctx, p, v1.UpdateOptions{})
		if err != nil {
			return controller.NewPermanentError(fmt.Errorf("Failed to update state: %s", err))
		}
		return nil
	}

	return nil
}

func (c *Reconciler) updateRepos(p *v1alpha1.SCMPollState, repos map[string]v1alpha1.SCMPollRepositoryInteface, conds duckv1beta1.Conditions) error {
	for _, repo := range repos {
		if state, ok := p.Status.SCMPollRepos[repo.GetName()]; !ok {
			continue
		} else {
			err := repo.Update(*state, conds)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getRepos(ctx context.Context, r []v1alpha1.Repository) (map[string]v1alpha1.SCMPollRepositoryInteface, error) {
	repos, err := poll.GetTypedRepoList(r)
	if err != nil {
		return nil, err
	}
	return repos, nil
}

func (c *Reconciler) submitPipelineRun(ctx context.Context, p *v1alpha1.SCMPollState) error {
	pipelinerun := pipelines.PipelineRun{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: p.Name + "-",
			Labels: map[string]string{
				"scmpollstate": p.Name,
			},
			OwnerReferences: []v1.OwnerReference{
				{
					Kind:               "SCMPollState",
					APIVersion:         "tekton.dev/v1alpha1",
					BlockOwnerDeletion: &p.Spec.Tidy,
					Name:               p.Name,
					UID:                p.GetUID(),
				},
			},
		},
		Spec: p.Spec.PipelineSpec,
	}

	_, err := c.PipelinesClientSet.TektonV1beta1().PipelineRuns(p.Namespace).Create(ctx, &pipelinerun, v1.CreateOptions{})
	return err
}

func (c *Reconciler) initialiseState(ctx context.Context, p *v1alpha1.SCMPollState) pkgreconciler.Event {
	status := v1alpha1.SCMPollStateStatus{
		SCMPollStateStatusFields: v1alpha1.SCMPollStateStatusFields{
			PipelineRunning: false,
			LastUpdate:      &v1.Time{time.Now()},
			Outdated:        true,
			StateStatus:     v1alpha1.StateUnknown,
			SCMPollRepos:    map[string]*v1alpha1.SCMRepositoryStatus{},
		},
	}

	p.Status = status
	p.Spec.Pending = false
	_, err := c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Update(ctx, p, v1.UpdateOptions{})

	return err
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

func isPipelineRunning(pr *pipelines.PipelineRun) bool {
	return pr.Status.GetCondition(apis.ConditionSucceeded).IsUnknown()
}

func isPipelineSuccessful(pr *pipelines.PipelineRun) string {
	if pr.Status.GetCondition(apis.ConditionSucceeded).IsTrue() {
		return v1alpha1.StatePass
	}
	if pr.Status.GetCondition(apis.ConditionSucceeded).IsFalse() {
		return v1alpha1.StateFailed
	}
	return v1alpha1.StateUnknown
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

func (c *Reconciler) AuthenticatePollRepos(ctx context.Context, p *v1alpha1.SCMPollState, pollRepos map[string]v1alpha1.SCMPollRepositoryInteface) error {
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

func mustNewRequirement(key string, op selection.Operator, vals []string) labels.Requirement {
	r, err := labels.NewRequirement(key, op, vals)
	if err != nil {
		panic(fmt.Sprintf("mustNewRequirement(%v, %v, %v) = %v", key, op, vals, err))
	}
	return *r
}
