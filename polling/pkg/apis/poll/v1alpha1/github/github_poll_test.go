package github_test

import (
	"testing"

	"github.com/tektoncd/experimental/polling/pkg/apis/poll/v1alpha1"
	"github.com/tektoncd/experimental/polling/pkg/apis/poll/v1alpha1/github"
	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewPollWithIncorrectType(t *testing.T) {
	r := &v1alpha1.Poll{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pipeline",
			Namespace: "my-namespace",
		},
		Spec: v1alpha1.PollSpec{
			Description: "",
			WatchName:   "FirstPipeline",
			Pipelines: []v1alpha1.PipelinePoll{
				v1alpha1.PipelinePoll{
					Name:     "FirstPipeline",
					Group:    1,
					PollType: "wrong",
					PollParams: []v1alpha1.Param{
						v1alpha1.Param{
							Name:  "url",
							Value: "api.example.com",
						},
						v1alpha1.Param{
							Name:  "branch",
							Value: "main",
						},
					},
					Spec: pipelines.PipelineRunSpec{},
				},
			},
		},
	}

	for _, poll := range r.Spec.Pipelines {
		if _, err := github.NewPoll("test-poll", poll); err == nil {
			t.Error("Failed evaluating incorrect Poll Type")
		}
	}
}
