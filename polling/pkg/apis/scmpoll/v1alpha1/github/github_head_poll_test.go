package github_test

// func TestNewPoll(t *testing.T) {
// 	r := &v1alpha1.Poll{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      "my-pipeline",
// 			Namespace: "my-namespace",
// 		},
// 		Spec: v1alpha1.PollSpec{
// 			Description: "",
// 			WatchName:   "FirstPipeline",
// 			Pipelines: []v1alpha1.PipelinePoll{
// 				v1alpha1.PipelinePoll{
// 					Name:     "FirstPipeline",
// 					Group:    1,
// 					PollType: "github",
// 					PollParams: []v1alpha1.Param{
// 						v1alpha1.Param{
// 							Name:  "url",
// 							Value: "api.example.com",
// 						},
// 						v1alpha1.Param{
// 							Name:  "branch",
// 							Value: "main",
// 						},
// 					},
// 					Spec: pipelines.PipelineRunSpec{},
// 				},
// 			},
// 		},
// 	}

// 	for _, poll := range r.Spec.Pipelines {
// 		if _, err := github.NewPoll("test-poll", poll); err != nil {
// 			t.Error("Failed to create new Github Poll Type")
// 		}
// 	}
// }

// func TestNewPollWithIncorrectType(t *testing.T) {
// 	r := &v1alpha1.Poll{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      "my-pipeline",
// 			Namespace: "my-namespace",
// 		},
// 		Spec: v1alpha1.PollSpec{
// 			Description: "",
// 			WatchName:   "FirstPipeline",
// 			Pipelines: []v1alpha1.PipelinePoll{
// 				v1alpha1.PipelinePoll{
// 					Name:     "FirstPipeline",
// 					Group:    1,
// 					PollType: "wrong",
// 					PollParams: []v1alpha1.Param{
// 						v1alpha1.Param{
// 							Name:  "url",
// 							Value: "api.example.com",
// 						},
// 						v1alpha1.Param{
// 							Name:  "branch",
// 							Value: "main",
// 						},
// 					},
// 					Spec: pipelines.PipelineRunSpec{},
// 				},
// 			},
// 		},
// 	}

// 	for _, poll := range r.Spec.Pipelines {
// 		if _, err := github.NewPoll("test-poll", poll); err == nil {
// 			t.Error("Failed evaluating incorrect Poll Type")
// 		}
// 	}
// }

// func TestMissingURLError(t *testing.T) {
// 	r := &v1alpha1.Poll{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      "my-pipeline",
// 			Namespace: "my-namespace",
// 		},
// 		Spec: v1alpha1.PollSpec{
// 			Description: "",
// 			WatchName:   "FirstPipeline",
// 			Pipelines: []v1alpha1.PipelinePoll{
// 				v1alpha1.PipelinePoll{
// 					Name:     "FirstPipeline",
// 					Group:    1,
// 					PollType: "github",
// 					PollParams: []v1alpha1.Param{
// 						v1alpha1.Param{
// 							Name:  "branch",
// 							Value: "main",
// 						},
// 					},
// 					Spec: pipelines.PipelineRunSpec{},
// 				},
// 			},
// 		},
// 	}

// 	for _, poll := range r.Spec.Pipelines {
// 		if _, err := github.NewPoll("test-poll", poll); err == nil {
// 			t.Error("Failed to error if url missing")
// 		}
// 	}
// }

// func TestMissingBranchError(t *testing.T) {
// 	r := &v1alpha1.Poll{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      "my-pipeline",
// 			Namespace: "my-namespace",
// 		},
// 		Spec: v1alpha1.PollSpec{
// 			Description: "",
// 			WatchName:   "FirstPipeline",
// 			Pipelines: []v1alpha1.PipelinePoll{
// 				v1alpha1.PipelinePoll{
// 					Name:     "FirstPipeline",
// 					Group:    1,
// 					PollType: "github",
// 					PollParams: []v1alpha1.Param{
// 						v1alpha1.Param{
// 							Name:  "url",
// 							Value: "api.example.com",
// 						},
// 					},
// 					Spec: pipelines.PipelineRunSpec{},
// 				},
// 			},
// 		},
// 	}

// 	for _, poll := range r.Spec.Pipelines {
// 		if _, err := github.NewPoll("test-poll", poll); err == nil {
// 			t.Error("Failed to error if branch missing")
// 		}
// 	}
// }

// func TestNewPollGetters(t *testing.T) {
// 	r := &v1alpha1.Poll{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      "my-pipeline",
// 			Namespace: "my-namespace",
// 		},
// 		Spec: v1alpha1.PollSpec{
// 			Description: "",
// 			WatchName:   "FirstPipeline",
// 			Pipelines: []v1alpha1.PipelinePoll{
// 				v1alpha1.PipelinePoll{
// 					Name:     "Tekton-pipelines",
// 					Group:    0,
// 					PollType: "github",
// 					PollParams: []v1alpha1.Param{
// 						v1alpha1.Param{
// 							Name:  "url",
// 							Value: "https://api.github.com/repos/tektoncd/pipeline",
// 						},
// 						v1alpha1.Param{
// 							Name:  "branch",
// 							Value: "main",
// 						},
// 					},
// 					Spec: pipelines.PipelineRunSpec{},
// 				},
// 			},
// 		},
// 	}

// 	for _, polls := range r.Spec.Pipelines {
// 		poll, err := github.NewPoll("test-poll", polls)
// 		if err != nil {
// 			t.Error("Failed to create new Github Poll Type")
// 		}

// 		if poll.GetName() != "test-poll" {
// 			t.Errorf("Get Name Getter failed: %s", poll.GetName())
// 		}

// 		if poll.GetType() != "github" {
// 			t.Errorf("Get type Getter failed: %s", poll.GetType())
// 		}

// 		if poll.GetEndpoint() != "https://api.github.com/repos/tektoncd/pipeline" {
// 			t.Errorf("Get endpoint Getter failed: %s", poll.GetEndpoint())
// 		}

// 		if poll.GetGroup() != 0 {
// 			t.Errorf("Get Group Getter failed: %v", poll.GetGroup())
// 		}
// 	}
// }

// func TestAuthenticate(t *testing.T) {
// 	smap := make(map[string][]byte)
// 	smap["username"] = []byte("test")
// 	smap["token"] = []byte("token")

// 	s := &v1.Secret{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name: "test-secret",
// 		},
// 		Type: v1.SecretTypeBasicAuth,
// 		Data: smap,
// 	}
// 	r := &v1alpha1.Poll{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      "my-pipeline",
// 			Namespace: "my-namespace",
// 		},
// 		Spec: v1alpha1.PollSpec{
// 			Description: "",
// 			WatchName:   "FirstPipeline",
// 			Pipelines: []v1alpha1.PipelinePoll{
// 				v1alpha1.PipelinePoll{
// 					Name:     "Tekton-pipelines",
// 					Group:    0,
// 					PollType: "github",
// 					PollParams: []v1alpha1.Param{
// 						v1alpha1.Param{
// 							Name:  "url",
// 							Value: "https://api.github.com/repos/tektoncd/pipeline",
// 						},
// 						v1alpha1.Param{
// 							Name:  "branch",
// 							Value: "main",
// 						},
// 					},
// 					Spec: pipelines.PipelineRunSpec{},
// 				},
// 			},
// 		},
// 	}
// 	for _, polls := range r.Spec.Pipelines {
// 		poll, err := github.NewPoll("test-poll", polls)
// 		if err != nil {
// 			t.Error("Failed to create new Github Poll Type")
// 		}
// 		err = poll.Authenticate(s)
// 		if err != nil {
// 			t.Errorf("Failed to authenticate: %s", err.Error())
// 		}
// 	}
// }
