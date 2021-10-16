package poll

import (
	"context"

	// pollclient "github.com/tektoncd/experimental/polling/pkg/client/injection/client"

	pollclient "github.com/tektoncd/experimental/polling/pkg/client/injection/client"
	pollinformer "github.com/tektoncd/experimental/polling/pkg/client/injection/informers/poll/v1alpha1/poll"
	pollruninformer "github.com/tektoncd/experimental/polling/pkg/client/injection/informers/poll/v1alpha1/pollrun"
	pollreconciler "github.com/tektoncd/experimental/polling/pkg/client/injection/reconciler/poll/v1alpha1/poll"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
)

func NewController(namespace string) func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
	return func(ctx context.Context, cmw configmap.Watcher) *controller.Impl {
		logger := logging.FromContext(ctx)
		logger.Info("Creating Polling controller...")

		kubeclientset := kubeclient.Get(ctx)
		pollclientset := pollclient.Get(ctx)
		pollInformer := pollinformer.Get(ctx)
		pollRunInformer := pollruninformer.Get(ctx)

		c := &Reconciler{
			KubeClientSet: kubeclientset,
			PollClientSet: pollclientset,
			PollLister:    pollInformer.Lister(),
			PollRunLister: pollRunInformer.Lister(),
		}

		impl := pollreconciler.NewImpl(ctx, c, func(impl *controller.Impl) controller.Options {
			return controller.Options{
				ConfigStore: nil,
			}
		})

		pollInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

		return impl
	}
}
