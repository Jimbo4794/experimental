package poll

import (
	"context"
	"fmt"
	"time"

	"github.com/tektoncd/experimental/polling/pkg/apis/poll/v1alpha1"

	pollv1alpha1 "github.com/tektoncd/experimental/polling/pkg/apis/poll"

	poll "github.com/tektoncd/experimental/polling/pkg/client/clientset/versioned"
	pollInformers "github.com/tektoncd/experimental/polling/pkg/client/informers/externalversions/poll/v1alpha1"
	pollListers "github.com/tektoncd/experimental/polling/pkg/client/listers/poll/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"

	pkgreconciler "knative.dev/pkg/reconciler"
)

// Implements controller.Reconciler for the polling resources
type Reconciler struct {
	KubeClientSet kubernetes.Interface
	PollClientSet poll.Interface
	PollLister    pollListers.PollLister
	PollRunLister pollListers.PollRunLister
	PollInformer  pollInformers.PollInformer
}

func (c *Reconciler) completePoll(ctx context.Context, p *v1alpha1.Poll) pkgreconciler.Event {
	p.Status.NextPoll = &v1.Time{time.Now().Add(time.Duration(p.Spec.PollFrequency * time.Second.Nanoseconds()))}
	c.PollClientSet.TektonV1alpha1().Polls(p.Namespace).Update(ctx, p, v1.UpdateOptions{})
	return controller.NewRequeueAfter(time.Duration(p.Spec.PollFrequency * time.Second.Nanoseconds()))
}

func (c *Reconciler) ReconcileKind(ctx context.Context, p *v1alpha1.Poll) pkgreconciler.Event {
	logger := logging.FromContext(ctx)
	logger.Infof("Running poll on %s", p.Name)

	// Create a selector that will find any pollruns owned by this poll
	selector := labels.NewSelector().Add(
		mustNewRequirement("poll", selection.Equals, []string{p.Name}),
	)

	pollList, err := c.PollRunLister.List(selector)
	if err != nil {
		return controller.NewPermanentError(fmt.Errorf("Failed to list running pollruns"))
	}

	// Check to see if any of the returned polls are running, end if so
	for _, pollrun := range pollList {
		if !pollrun.Status.Finished {
			return c.completePoll(ctx, p)
		}

	}

	pipelineList, _ := pollv1alpha1.FormRunList(p.Name, p)
	if _, ok := pipelineList[p.Spec.WatchName]; !ok {
		return controller.NewPermanentError(fmt.Errorf("The requested pipeline (%s) to watch was not defined!", p.Spec.WatchName))
	}
	main := pipelineList[p.Spec.WatchName]
	logger.Infof("The main watch repo is of type %s", main.GetType())

	// Look to see if a service account is required for this
	sa, err := c.KubeClientSet.CoreV1().ServiceAccounts(p.Namespace).Get(ctx, p.Spec.PollServiceAccountName, v1.GetOptions{})

	if err != nil {
		logger.Warnf("Failed to find any service accounts for %s with the name %s, using default", p.Name, p.Spec.PollServiceAccountName)
	} else {
		for _, secretEntry := range sa.Secrets {
			if secretEntry.Name == "" {
				continue
			}

			secret, err := c.KubeClientSet.CoreV1().Secrets(p.Namespace).Get(ctx, secretEntry.Name, v1.GetOptions{})
			if err != nil {
				return controller.NewPermanentError(fmt.Errorf("Failed to get Poling secrets"))
			}
			annotations := secret.GetAnnotations()
			for _, v := range annotations {
				if v == main.GetEndpoint() {
					if err = main.Authenticate(secret); err != nil {
						logger.Errorf("Failed to authenticate to endpoint, continuing un-authenticated: %s", err.Error())
					} else {
						logger.Infof("Service account found for endpoint, authenticating")
						break
					}
				}
			}
		}
	}

	// Poll latest version
	version, err := main.PollVersion()
	if err != nil {
		return controller.NewPermanentError(fmt.Errorf("Problem requesting version from %s: %v", main.GetEndpoint(), err))
	}

	// Look if this version has already run
	for _, pollrun := range pollList {
		if pollrun.GetLabels()["version"] == version {
			return c.completePoll(ctx, p)
		}
	}

	p.Status.LastRun = &v1.Time{time.Now()}
	c.PollClientSet.TektonV1alpha1().Polls(p.Namespace).Update(ctx, p, v1.UpdateOptions{})
	var runs []v1alpha1.Run
	for _, run := range pipelineList {
		v, err := run.PollVersion()
		if err != nil {
			return controller.NewPermanentError(fmt.Errorf("Problem requesting version from %s: %v", run.GetEndpoint(), err))
		}

		// Make any version subsitutions
		var smap = make(map[string]string)
		smap["version"] = v
		spec := run.GetPipelineRunSpec()
		for i, param := range spec.Params {
			param.Value.ApplyReplacements(smap, nil)
			spec.Params[i] = param
		}

		runs = append(runs, v1alpha1.Run{
			Name:        run.GetName(),
			PollRunType: run.GetType(),
			Group:       run.GetGroup(),
			Version:     v,
			Spec:        spec,
		})
	}

	l := make(map[string]string)
	l["poll"] = p.Name
	l["version"] = version

	pollRun := v1alpha1.PollRun{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: "pollrun-" + p.Name,
			Labels:       l,
		},
		Spec: v1alpha1.PollRunSpec{
			PollRuns: runs,
		},
	}
	logger.Infof("POLLRUN ENDING UP AS %v", pollRun)

	pr, err := c.PollClientSet.TektonV1alpha1().PollRuns(p.Namespace).Create(ctx, &pollRun, v1.CreateOptions{})
	if err != nil {
		logger.Errorf(err.Error())
	}
	pr.Status.Finished = false
	c.PollClientSet.TektonV1alpha1().PollRuns(p.Namespace).Update(ctx, pr, v1.UpdateOptions{})

	return c.completePoll(ctx, p)
}

// mustNewRequirement panics if there are any errors constructing our selectors.
func mustNewRequirement(key string, op selection.Operator, vals []string) labels.Requirement {
	r, err := labels.NewRequirement(key, op, vals)
	if err != nil {
		panic(fmt.Sprintf("mustNewRequirement(%v, %v, %v) = %v", key, op, vals, err))
	}
	return *r
}
