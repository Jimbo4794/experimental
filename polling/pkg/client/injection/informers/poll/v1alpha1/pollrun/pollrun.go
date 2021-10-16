/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by injection-gen. DO NOT EDIT.

package pollrun

import (
	context "context"

	apispollv1alpha1 "github.com/tektoncd/experimental/polling/pkg/apis/poll/v1alpha1"
	versioned "github.com/tektoncd/experimental/polling/pkg/client/clientset/versioned"
	v1alpha1 "github.com/tektoncd/experimental/polling/pkg/client/informers/externalversions/poll/v1alpha1"
	client "github.com/tektoncd/experimental/polling/pkg/client/injection/client"
	factory "github.com/tektoncd/experimental/polling/pkg/client/injection/informers/factory"
	pollv1alpha1 "github.com/tektoncd/experimental/polling/pkg/client/listers/poll/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	cache "k8s.io/client-go/tools/cache"
	controller "knative.dev/pkg/controller"
	injection "knative.dev/pkg/injection"
	logging "knative.dev/pkg/logging"
)

func init() {
	injection.Default.RegisterInformer(withInformer)
	injection.Dynamic.RegisterDynamicInformer(withDynamicInformer)
}

// Key is used for associating the Informer inside the context.Context.
type Key struct{}

func withInformer(ctx context.Context) (context.Context, controller.Informer) {
	f := factory.Get(ctx)
	inf := f.Tekton().V1alpha1().PollRuns()
	return context.WithValue(ctx, Key{}, inf), inf.Informer()
}

func withDynamicInformer(ctx context.Context) context.Context {
	inf := &wrapper{client: client.Get(ctx)}
	return context.WithValue(ctx, Key{}, inf)
}

// Get extracts the typed informer from the context.
func Get(ctx context.Context) v1alpha1.PollRunInformer {
	untyped := ctx.Value(Key{})
	if untyped == nil {
		logging.FromContext(ctx).Panic(
			"Unable to fetch github.com/tektoncd/experimental/polling/pkg/client/informers/externalversions/poll/v1alpha1.PollRunInformer from context.")
	}
	return untyped.(v1alpha1.PollRunInformer)
}

type wrapper struct {
	client versioned.Interface

	namespace string
}

var _ v1alpha1.PollRunInformer = (*wrapper)(nil)
var _ pollv1alpha1.PollRunLister = (*wrapper)(nil)

func (w *wrapper) Informer() cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(nil, &apispollv1alpha1.PollRun{}, 0, nil)
}

func (w *wrapper) Lister() pollv1alpha1.PollRunLister {
	return w
}

func (w *wrapper) PollRuns(namespace string) pollv1alpha1.PollRunNamespaceLister {
	return &wrapper{client: w.client, namespace: namespace}
}

func (w *wrapper) List(selector labels.Selector) (ret []*apispollv1alpha1.PollRun, err error) {
	lo, err := w.client.TektonV1alpha1().PollRuns(w.namespace).List(context.TODO(), v1.ListOptions{
		LabelSelector: selector.String(),
		// TODO(mattmoor): Incorporate resourceVersion bounds based on staleness criteria.
	})
	if err != nil {
		return nil, err
	}
	for idx := range lo.Items {
		ret = append(ret, &lo.Items[idx])
	}
	return ret, nil
}

func (w *wrapper) Get(name string) (*apispollv1alpha1.PollRun, error) {
	return w.client.TektonV1alpha1().PollRuns(w.namespace).Get(context.TODO(), name, v1.GetOptions{
		// TODO(mattmoor): Incorporate resourceVersion bounds based on staleness criteria.
	})
}
