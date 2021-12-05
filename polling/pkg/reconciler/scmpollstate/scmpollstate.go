package scmpollstate

import (
	"context"
	"fmt"
	"time"

	"github.com/tektoncd/experimental/polling/pkg/apis/scmpoll/v1alpha1"

	pipelineclientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelinerunLister "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"

	scmpollv1alpha1 "github.com/tektoncd/experimental/polling/pkg/apis/scmpoll"
	scmpoll "github.com/tektoncd/experimental/polling/pkg/client/clientset/versioned"
	scmpollListers "github.com/tektoncd/experimental/polling/pkg/client/listers/scmpoll/v1alpha1"
	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
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
	if p.Spec.Pending {
		logger.Debugf("%s is pending, init status being set", p.Name)
		return c.initialiseState(ctx, p)
	}

	for _, state := range p.Status.SCMPollStates {
		if !state.Authorised {
			logger.Debugf("%s is not authorised to run", p.Name)
			return nil
		}
	}

	if p.Status.Outdated {
		logger.Debugf("%s is outdated, submitting pipelinerun", p.Name)
		pipelinerun := pipelines.PipelineRun{
			ObjectMeta: v1.ObjectMeta{
				GenerateName:    p.Name + "-",
				OwnerReferences: p.OwnerReferences,
			},
			Spec: p.Spec.PipelineSpec,
		}

		_, err := c.PipelinesClientSet.TektonV1beta1().PipelineRuns(p.Namespace).Create(ctx, &pipelinerun, v1.CreateOptions{})
		if err != nil {
			return controller.NewPermanentError(fmt.Errorf("Failed to create pipelinerun: %s", err))
		}
		p.Status.Outdated = false
		_, err = c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Update(ctx, p, v1.UpdateOptions{})
		if err != nil {
			return controller.NewPermanentError(fmt.Errorf("Failed to update state: %s", err))
		}
	}

	return nil
}

func (c *Reconciler) initialiseState(ctx context.Context, p *v1alpha1.SCMPollState) pkgreconciler.Event {
	status := v1alpha1.SCMPollStateStatus{
		SCMPollStateStatusFields: v1alpha1.SCMPollStateStatusFields{
			PipelineRunning: false,
			LastUpdate:      &v1.Time{time.Now()},
			Outdated:        true,
			SCMPollStates:   map[string]*v1alpha1.SCMRepositoryStatus{},
		},
	}

	p.Status = status
	_, err := c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).UpdateStatus(ctx, p, v1.UpdateOptions{})
	if err != nil {
		return controller.NewPermanentError(err)
	}

	p, err = c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Get(ctx, p.Name, v1.GetOptions{})
	if err != nil {
		return controller.NewPermanentError(err)
	}
	p.Spec.Pending = false
	_, err = c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Update(ctx, p, v1.UpdateOptions{})
	if err != nil {
		return controller.NewPermanentError(err)
	}

	return controller.NewRequeueImmediately()
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

// func (c *Reconciler) createNewPipelineAndState(ctx context.Context, p *v1alpha1.SCMPoll, stateId, repoName string) pkgreconciler.Event {
// 	err := c.submitStatePipelineRun(ctx, p, stateId)
// 	if err != nil {
// 		return controller.NewPermanentError(fmt.Errorf("problem submitting new pipelinerun: %v", err))
// 	}

// 	_, err = c.SCMPollClientSet.TektonV1alpha1().SCMPollStates(p.Namespace).Create(ctx, s, v1.CreateOptions{})
// 	if err != nil {
// 		return controller.NewPermanentError(fmt.Errorf("Problem create poll state: %v", err))
// 	}
// 	return controller.NewRequeueImmediately()
// }

// func (c *Reconciler) addStateAsOwnerIfNotOwned(ctx context.Context, pr *pipelines.PipelineRun, state *v1alpha1.SCMPollState) error {
// 	owners := pr.ObjectMeta.GetOwnerReferences()
// 	for _, owner := range owners {
// 		if owner.UID != state.GetUID() {
// 			continue
// 		}
// 		return nil
// 	}
// 	owners = append(owners, v1.OwnerReference{
// 		Kind:       "SCMPollState",
// 		APIVersion: "tekton.dev/v1alpha1",

// 		BlockOwnerDeletion: &state.Spec.Tidy,
// 		Name:               state.Name,
// 		UID:                state.GetUID(),
// 	})
// 	pr, _ = c.PipelinesClientSet.TektonV1beta1().PipelineRuns(pr.Namespace).Get(ctx, pr.Name, v1.GetOptions{})
// 	pr.ObjectMeta.OwnerReferences = owners
// 	c.PipelinesClientSet.TektonV1beta1().PipelineRuns(pr.Namespace).Update(ctx, pr, v1.UpdateOptions{})
// 	return nil
// }

func (c *Reconciler) updateStateStatus(ctx context.Context, ss *v1alpha1.SCMPoll, s *v1alpha1.SCMPollState, resp *v1alpha1.SCMRepositoryStatus, pr *pipelines.PipelineRun) error {
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
				LastUpdate:      &v1.Time{time.Now()},
				SCMPollStates:   repoStates,
				NextPoll:        &v1.Time{time.Now()},
			},
		}
	} else {
		s.Status = v1alpha1.SCMPollStateStatus{
			SCMPollStateStatusFields: v1alpha1.SCMPollStateStatusFields{
				PipelineRunning: isPipelineRunning(pr),
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

// 	stateRepoList, err := scmpollv1alpha1.GetTypedRepoPollStates(state)
// 	if err != nil {
// 		return controller.NewPermanentError(fmt.Errorf("Problem getting typed repo states: %v", err))
// 	}

// 	// Find the PR's
// 	prs, err := c.PipelineRunLister.List(selector)
// 	if err != nil {
// 		return controller.NewPermanentError(fmt.Errorf("Failed to locate pipelineruns for  s: %v", state.Name, err))
// 	}
// 	latestPR := getLatest(prs)
// 	c.updateStateStatus(ctx, p, state, &resp, latestPR)
// 	if latestPR == nil {
// 		logger.Debugf("Expected pipelinerun not found yet, requeue check")
// 		// PR not availble yet, requeue
// 		return controller.NewRequeueAfter(time.Second * 3)
// 	}
// 	latestPR.Status.GetConditions()

// 	repo.Update(resp, latestPR.Status.Conditions)
// 	err = c.addStateAsOwnerIfNotOwned(ctx, latestPR, state)
// 	if err != nil {
// 		return controller.NewPermanentError(fmt.Errorf("Failed to apply onwership for %s: %v", state.Name, err))
// 	}

// 	if len(stateRepoList) == 0 {
// 		return controller.NewRequeueImmediately()
// 	}

// 	if !p.Spec.ConcurrentPipelines {
// 		if state.Status.PipelineRunning {
// 			logger.Debugf("Pipeline running and parrallel runs not allowed, requeuing %s", p.Name)
// 			continue
// 			// return c.completePoll(ctx, p)
// 		}
// 	}

// 	for _, repoStatus := range stateRepoList {
// 		if repoStatus.GetName() == resp.Name {
// 			changed, err := repoStatus.HasStatusChanged(resp)
// 			if err != nil {
// 				return controller.NewPermanentError(fmt.Errorf("problem checking repostatus %s: %v", repoStatus.GetName(), err))
// 			}
// 			if changed {
// 				logger.Debugf("Detected a change in %s, submitting pipelinerun", repoStatus.GetName())
// 				c.submitStatePipelineRun(ctx, p, resp.StateId)
// 				repo.Update(resp, v1beta1.Conditions{apis.Condition{
// 					Type:   apis.ConditionSucceeded,
// 					Status: corev1.ConditionUnknown,
// 				}})
// 			}

// 			logger.Debugf("No changes detected in %s, doing nothing", repoStatus.GetName())
// 			break
// 		}
// 	}
// 	err = c.updateNextPollStatus(ctx, state, int(p.Spec.SCMPollFrequency))
// 	if err != nil {
// 		return controller.NewPermanentError(fmt.Errorf("Failed to updates status about poll: %s", err))
// 	}

func mustNewRequirement(key string, op selection.Operator, vals []string) labels.Requirement {
	r, err := labels.NewRequirement(key, op, vals)
	if err != nil {
		panic(fmt.Sprintf("mustNewRequirement(%v, %v, %v) = %v", key, op, vals, err))
	}
	return *r
}
