package scmpoll

import (
	"context"

	// pollclient "github.com/tektoncd/experimental/polling/pkg/client/injection/client"

	scmpollclient "github.com/tektoncd/experimental/polling/pkg/client/injection/client"
	scmpollinformer "github.com/tektoncd/experimental/polling/pkg/client/injection/informers/scmpoll/v1alpha1/scmpoll"
	scmpollstateinformer "github.com/tektoncd/experimental/polling/pkg/client/injection/informers/scmpoll/v1alpha1/scmpollstate"
	scmpollreconciler "github.com/tektoncd/experimental/polling/pkg/client/injection/reconciler/scmpoll/v1alpha1/scmpoll"
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
		pollclientset := scmpollclient.Get(ctx)
		pollInformer := scmpollinformer.Get(ctx)
		pollRunInformer := scmpollstateinformer.Get(ctx)

		c := &Reconciler{
			KubeClientSet:      kubeclientset,
			SCMPollClientSet:   pollclientset,
			SCMPollLister:      pollInformer.Lister(),
			SCMPollStateLister: pollRunInformer.Lister(),
		}

		impl := scmpollreconciler.NewImpl(ctx, c, func(impl *controller.Impl) controller.Options {
			return controller.Options{
				ConfigStore: nil,
			}
		})

		pollInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

		return impl
	}
}
