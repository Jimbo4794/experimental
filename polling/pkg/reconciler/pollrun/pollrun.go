package pollrun

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/tektoncd/experimental/polling/pkg/apis/poll/v1alpha1"

	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	pipelineclientset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelinerunLister "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1beta1"

	poll "github.com/tektoncd/experimental/polling/pkg/client/clientset/versioned"
	pollrunListers "github.com/tektoncd/experimental/polling/pkg/client/listers/poll/v1alpha1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	pkgreconciler "knative.dev/pkg/reconciler"
)

const requeueTime = time.Second * 20

// Implements controller.Reconciler for the polling resources
type Reconciler struct {
	KubeClientSet      kubernetes.Interface
	PipelinesClientSet pipelineclientset.Interface
	PollClientSet      poll.Interface

	PollLister        pollrunListers.PollRunLister
	PipelineRunLister pipelinerunLister.PipelineRunLister
}

func (c *Reconciler) ReconcileKind(ctx context.Context, p *v1alpha1.PollRun) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	activeGroup := p.Status.ActiveGroup

	if activeGroup == "" {
		logger.Infof("PollRun %s just created, setting group 0 to active group", p.Name)
		p.Status.ActiveGroup = "0"
		c.PollClientSet.TektonV1alpha1().PollRuns(p.Namespace).Update(ctx, p, v1.UpdateOptions{})
		return controller.NewRequeueAfter(time.Second)
	}

	if activeGroup == "finished" {
		p.Status.Finished = true
		c.PollClientSet.TektonV1alpha1().PollRuns(p.Namespace).Update(ctx, p, v1.UpdateOptions{})
		return nil
	}

	logger.Infof("Current active group for pollrun %s is %s", p.Name, activeGroup)

	var groupedRuns []v1alpha1.Run
	group, _ := strconv.Atoi(activeGroup)
	for _, pr := range p.Spec.PollRuns {
		if pr.Group == group {
			groupedRuns = append(groupedRuns, pr)
		}
	}
	logger.Infof("There is %v run(s) found for group %s", len(groupedRuns), activeGroup)

	running := false
	for _, run := range groupedRuns {
		// Check it exists, submit if not, check completetion if does
		selector := labels.NewSelector().Add(
			mustNewRequirement("owningpollrun", selection.Equals, []string{p.Name}),
			mustNewRequirement("group", selection.Equals, []string{activeGroup}),
			mustNewRequirement("poll", selection.Equals, []string{run.Name}),
		)

		list, err := c.PipelineRunLister.List(selector)

		if err != nil {
			logger.Errorf("Failed to list pipelineruns in the the namespace %s: %s", p.Namespace, err.Error())
		}
		if len(list) < 1 {
			l := make(map[string]string)
			l["owningpollrun"] = p.Name
			l["group"] = activeGroup
			l["poll"] = run.Name

			pipelineRun := v1beta1.PipelineRun{
				ObjectMeta: v1.ObjectMeta{
					GenerateName: p.Name + "-",
					Labels:       l,
					Namespace:    p.Namespace,
				},
				Spec: run.Spec,
			}

			logger.Infof("Submitting pipelinerun %v", pipelineRun.GenerateName)
			_, err = c.PipelinesClientSet.TektonV1beta1().PipelineRuns(p.Namespace).Create(ctx, &pipelineRun, v1.CreateOptions{})
			running = true
			if err != nil {
				logger.Errorf(err.Error())
			}

		} else {
			logger.Infof("Running poll run found: %s", list[0].Name)
			if list[0].Status.GetCondition(apis.ConditionSucceeded).IsUnknown() {
				running = true
			}
		}
	}
	if running {
		return controller.NewRequeueAfter(requeueTime)
	}

	selector := labels.NewSelector().Add(
		mustNewRequirement("owningpollrun", selection.Equals, []string{p.Name}),
		mustNewRequirement("group", selection.Equals, []string{activeGroup}),
	)

	list, err := c.PipelineRunLister.List(selector)
	if err != nil {
		logger.Errorf("Failed to list pipelineruns in the the namespace %s: %s", p.Namespace, err.Error())
	}

	for _, run := range list {
		if run.Status.GetCondition(apis.ConditionSucceeded).IsTrue() {
			continue
		}
		return controller.NewPermanentError(fmt.Errorf("The pipelinerun %s failed, stopping the run", run.Name))
	}

	// Group completed, check if next group is required
	doContinue := false
	agi, _ := strconv.Atoi(activeGroup)
	for _, run := range p.Spec.PollRuns {
		if run.Group >= agi+1 {
			doContinue = true
		}
	}
	if doContinue {
		p.Status.ActiveGroup = strconv.Itoa(agi + 1)
		c.PollClientSet.TektonV1alpha1().PollRuns(p.Namespace).Update(ctx, p, v1.UpdateOptions{})
		return controller.NewRequeueAfter(requeueTime)
	} else {
		logger.Infof("No runs in group %v found. Pollrun %s completed", activeGroup, p.Name)
		p.Status.ActiveGroup = "finished"
		c.PollClientSet.TektonV1alpha1().PollRuns(p.Namespace).Update(ctx, p, v1.UpdateOptions{})
		return nil
	}
}

// mustNewRequirement panics if there are any errors constructing our selectors.
func mustNewRequirement(key string, op selection.Operator, vals []string) labels.Requirement {
	r, err := labels.NewRequirement(key, op, vals)
	if err != nil {
		panic(fmt.Sprintf("mustNewRequirement(%v, %v, %v) = %v", key, op, vals, err))
	}
	return *r
}
