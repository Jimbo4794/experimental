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
package scmpoll

import (
	"fmt"

	scmpollv1alpha1 "github.com/tektoncd/experimental/polling/pkg/apis/scmpoll/v1alpha1"
	"github.com/tektoncd/experimental/polling/pkg/apis/scmpoll/v1alpha1/github"
)

// GroupName is the group name used in this package
const (
	GroupName = "tekton.dev"

	SCMPollLabelKey = "/scmpoll"
)

func GetCurrentRepoPollStates(s *scmpollv1alpha1.SCMPollState) (map[string]scmpollv1alpha1.SCMPollStateInterface, error) {
	repoStates := make(map[string]scmpollv1alpha1.SCMPollStateInterface)
	// Returns a typed PollState object that can be used for versions compares
	for _, state := range s.Status.SCMPollStates {
		switch state.Type {
		case scmpollv1alpha1.SCMPollTypeGithubHead:
			gState, err := github.GetGithubHeadType(state)
			if err != nil {
				return nil, fmt.Errorf("Failed to create git state: ", err)
			}
			repoStates[state.Name] = gState
		case scmpollv1alpha1.SCMPollTypeGithubPR:
			gprState, err := github.GetGithubPRType(state)
			if err != nil {
				return nil, fmt.Errorf("Failed to create git pr state: ", err)
			}
			repoStates[state.Name] = gprState
		default:
			return nil, fmt.Errorf("%v is an invalid or unimplemented poll state", s)
		}
	}
	return repoStates, nil

}

// Creates and returns a list of the endpoints to watch and poll.
func FormRunList(name string, r *scmpollv1alpha1.SCMPoll) (map[string]scmpollv1alpha1.SCMPollRepositoryInteface, error) {
	pipelines := make(map[string]scmpollv1alpha1.SCMPollRepositoryInteface)
	// Check we support all the polls then add to the list of polls
	for _, poll := range r.Spec.Repositories {
		switch poll.SCMPollType {
		case scmpollv1alpha1.SCMPollTypeGithubHead:
			gPoll, err := github.NewHead(poll.Name, poll)
			if err != nil {
				return nil, fmt.Errorf("Failed to create git poll: ", err)
			}
			pipelines[gPoll.Name] = gPoll
		case scmpollv1alpha1.SCMPollTypeGithubPR:
			gprPoll, err := github.NewPR(poll.Name, poll)
			if err != nil {
				return nil, fmt.Errorf("Failed to create git pr poll: ", err)
			}
			pipelines[gprPoll.Name] = gprPoll
		default:
			return nil, fmt.Errorf("%v is an invalid or unimplemented poll endpoint", poll.SCMPollType)
		}
	}
	return pipelines, nil
}
