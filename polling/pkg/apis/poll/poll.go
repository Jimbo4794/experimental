/*
Copyright 2021 The Tekton Authors
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
package poll

import (
	"fmt"

	pollv1alpha1 "github.com/tektoncd/experimental/polling/pkg/apis/poll/v1alpha1"
	"github.com/tektoncd/experimental/polling/pkg/apis/poll/v1alpha1/github"
)

// GroupName is the group name used in this package
const (
	GroupName = "tekton.dev"

	PollLabelKey = "/poll"
)

// Creates and returns a list of the endpoints to watch and poll.
func FormRunList(name string, r *pollv1alpha1.Poll) (map[string]pollv1alpha1.PollInterface, error) {
	pipelines := make(map[string]pollv1alpha1.PollInterface)
	// Check we support all the polls then add to the list of polls
	for _, poll := range r.Spec.Pipelines {
		switch poll.PollType {
		case pollv1alpha1.PollTypeGithub:
			gPoll, err := github.NewPoll(poll.Name, poll)
			if err != nil {
				return nil, fmt.Errorf("Failed to create git poll", err)
			}
			pipelines[gPoll.Name] = gPoll
		default:
			return nil, fmt.Errorf("%s is an invalid or unimplemented poll endpoint", poll.PollType)
		}
	}
	return pipelines, nil
}
