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

package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tektoncd/experimental/polling/pkg/apis/scmpoll/v1alpha1"
	v1 "k8s.io/api/core/v1"

	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

var (
	gitSource = "git-source"
)

// GithubPoll is the endpoint for which repo to watch and what branch. Upon activity of the
// watched endpoint, a pollrun will be created, creating a snapshot of all current revisions
// of the watched branch/repo and any downstream pipelines.
type GithubBranchHead struct {
	Name               string               `json:"name"`
	Type               v1alpha1.SCMPollType `json:"type"`
	URL                string               `json:"url"`
	Branch             string               `json:"branch"`
	ServiceAccountName string               `json:"serviceAccountName"`
	PollClient         GitClient
}

type GitBranchJson struct {
	Name   string     `json:"name"`
	Commit CommitJson `json:"commit"`
}

type CommitJson struct {
	Sha string `json:"sha"`
}

type GitClient struct {
	Client   *http.Client
	Username string
	Token    string
}

func (s *GithubBranchHead) Update(status v1alpha1.SCMRepositoryStatus, conds duckv1beta1.Conditions) error {
	return nil
}

func NewHead(name string, r v1alpha1.Repository) (*GithubBranchHead, error) {
	if r.SCMPollType != v1alpha1.SCMPollTypeGithubHead {
		return nil, fmt.Errorf("github.poll: Cannot create a Github poll from a %s ", r.SCMPollType)
	}

	githubPoll := &GithubBranchHead{
		Name:               name,
		Type:               r.SCMPollType,
		ServiceAccountName: r.SCMPollServiceAccountName,
		PollClient: GitClient{
			Client: &http.Client{Timeout: time.Second * 10},
		},
	}
	for _, param := range r.SCMPollParams {
		switch {
		case strings.EqualFold(param.Name, "url"):
			githubPoll.URL = param.Value.StringVal
		case strings.EqualFold(param.Name, "branch"):
			githubPoll.Branch = param.Value.StringVal
		}
	}
	if githubPoll.URL == "" {
		return nil, fmt.Errorf("A url poll param must be passed. Please provide a https://api.github.com/repos/<ORG>/<REPO>")
	}
	if githubPoll.Branch == "" {
		return nil, fmt.Errorf("A branch poll param must be passed.")
	}

	return githubPoll, nil

}

// GetName returns the name of the Poll
func (s *GithubBranchHead) GetName() string {
	return s.Name
}

// GetType returns the type of the Poll, in this case "Github"
func (s *GithubBranchHead) GetType() v1alpha1.SCMPollType {
	return v1alpha1.SCMPollTypeGithubHead
}

func (s *GithubBranchHead) GetEndpoint() string {
	return s.URL
}

func (s *GithubBranchHead) GetServiceAccountName() string {
	return s.ServiceAccountName
}

func (s *GithubBranchHead) Poll() ([]v1alpha1.SCMRepositoryStatus, error) {
	var response []v1alpha1.SCMRepositoryStatus
	lastCommit, err := getLastCommit(s.PollClient, s.URL, s.Branch)
	if err != nil {
		return nil, err
	}
	response = append(response, v1alpha1.SCMRepositoryStatus{
		Name:       s.Name,
		StateId:    "head",
		Type:       v1alpha1.SCMPollTypeGithubHead,
		Authorised: true,
		StatusParams: []pipelines.Param{
			pipelines.Param{
				Name: "commitSHA",
				Value: pipelines.ArrayOrString{
					Type:      pipelines.ParamTypeString,
					StringVal: lastCommit,
				},
			},
		},
	})
	return response, nil
}

func (s *GithubBranchHead) Authenticate(secret *v1.Secret) error {
	if secret.Type != v1.SecretTypeBasicAuth {
		return fmt.Errorf("Unsupported Auth type, basic token only")
	}

	if user, ok := secret.Data["username"]; ok {
		s.PollClient.Username = string(user)
	} else {
		return fmt.Errorf("No username found in secret %v", secret)
	}

	if token, ok := secret.Data["token"]; ok {
		s.PollClient.Token = string(token)
	} else {
		return fmt.Errorf("No token found in secret %v", secret)
	}
	return nil
}

// Returns the last commitSHA for a specified branch
func getLastCommit(gitC GitClient, url, branch string) (string, error) {
	var branchStatus GitBranchJson
	req, _ := http.NewRequest("GET", url+"/branches/"+branch, nil)
	if gitC.Username != "" && gitC.Token != "" {
		req.SetBasicAuth(gitC.Username, gitC.Token)
	}

	resp, err := gitC.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("github.poll: Failed to get commit id from repo %s on branch %s: %s", url, branch, err)
	}
	if resp.StatusCode != 200 {
		if resp.Header.Get("x-ratelimit-remaining") == "0" {
			return "", fmt.Errorf("github.poll: RATE LIMIT EXCEEDED: Curent rate limit: %v, Rate reset at %v", resp.Header.Get("X-RateLimit-limit"), resp.Header.Get("X-RateLimit-Reset"))
		}
		return "", fmt.Errorf("github.poll: Bad response from repo %s on branch %s: %s", url, branch, err)
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&branchStatus)
	if err != nil {
		return "", fmt.Errorf("github.poll: Failed to decode response from repo %s on branch %s: %s", url, branch, err)
	}

	return branchStatus.Commit.Sha, nil
}
