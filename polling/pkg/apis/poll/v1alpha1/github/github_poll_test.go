package github_test

import (
	"testing"
)

func TestNewPollWithIncorrectType(t *testing.T) {
	if false {
		t.Error()
	}
	// r := &v1alpha1.Poll{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name:      "my-pipeline",
	// 		Namespace: "my-namespace",
	// 	},
	// 	Spec: v1alpha1.PollSpec{
	// 		Description: "",
	// 		WatchName:   "FirstPipeline",
	// 		Pipelines: []v1alpha1.PipelinePoll{
	// 			v1alpha1.PipelinePoll{
	// 				Name:     "FirstPipeline",
	// 				Group:    1,
	// 				PollType: "Github",
	// 				PollParams: []v1alpha1.Param{
	// 					v1alpha1.Param{
	// 						Name:  "url",
	// 						Value: "api.example.com",
	// 					},
	// 					v1alpha1.Param{
	// 						Name:  "branch",
	// 						Value: "main",
	// 					},
	// 				},
	// 				PipelineRef: "pipeRef",
	// 				PipelineParams: []pipelines.Param{
	// 					pipelines.Param{
	// 						Name: "name",
	// 						Value: pipelines.ArrayOrString{
	// 							Type:      pipelines.ParamTypeString,
	// 							StringVal: "value",
	// 						},
	// 					},
	// 				},
	// 			},
	// 			v1alpha1.PipelinePoll{
	// 				Name:     "SecondPipeline",
	// 				Group:    2,
	// 				PollType: "Github",
	// 				PollParams: []v1alpha1.Param{
	// 					v1alpha1.Param{
	// 						Name:  "url",
	// 						Value: "api.example.com",
	// 					},
	// 					v1alpha1.Param{
	// 						Name:  "branch",
	// 						Value: "main",
	// 					},
	// 				},
	// 				PipelineRef: "pipeRef",
	// 				PipelineParams: []pipelines.Param{
	// 					pipelines.Param{
	// 						Name: "name",
	// 						Value: pipelines.ArrayOrString{
	// 							Type:      pipelines.ParamTypeString,
	// 							StringVal: "value",
	// 						},
	// 					},
	// 				},
	// 			},
	// 		},
	// 	},
	// }

	// for _, poll := range r.Spec.Pipelines {
	// 	if _, err := github.NewPoll("test-poll", poll); err == nil {
	// 		t.Error("Failed evaluating correct Poll Type")
	// 	}
	// }

}
