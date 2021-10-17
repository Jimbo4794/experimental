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

	"github.com/tektoncd/experimental/polling/pkg/apis/poll/v1alpha1"
	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
)

var (
	gitSource = "git-source"
)

// GithubPoll is the endpoint for which repo to watch and what branch. Upon activity of the
// watched endpoint, a pollrun will be created, creating a snapshot of all current revisions
// of the watched branch/repo and any downstream pipelines.
type Poll struct {
	Name   string                    `json:"name"`
	Type   v1alpha1.PollType         `json:"type"`
	Group  int                       `json:"group"`
	URL    string                    `json:"url"`
	Branch string                    `json:"branch"`
	Spec   pipelines.PipelineRunSpec `json:"spec"`
	// PipelineRef                string                              `json:"pipelineRef"`
	// PipelineParams             []pipelines.Param                   `json:"pipelineParams"`
	// PipelinesResources         []pipelines.PipelineResourceBinding `json:"pipelineresources"`
	// PipelineServiceAccountName string                              `json:"pipelinesServiceAccountName"`
	PollClient GitClient
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

func NewPoll(name string, r v1alpha1.PipelinePoll) (*Poll, error) {
	if r.PollType != v1alpha1.PollTypeGithub {
		return nil, fmt.Errorf("github.poll: Cannot create a Github poll from a %s ", r.PollType)
	}

	githubPoll := Poll{
		Name:  name,
		Type:  r.PollType,
		Group: r.Group,
		Spec:  r.Spec,
		PollClient: GitClient{
			Client: &http.Client{Timeout: time.Second * 10},
		},
	}
	for _, param := range r.PollParams {
		switch {
		case strings.EqualFold(param.Name, "url"):
			githubPoll.URL = param.Value
		case strings.EqualFold(param.Name, "branch"):
			githubPoll.Branch = param.Value
		}
	}

	return &githubPoll, nil

}

// GetName returns the name of the Poll
func (s *Poll) GetName() string {
	return s.Name
}

// GetType returns the type of the Poll, in this case "Github"
func (s *Poll) GetType() v1alpha1.PollType {
	return v1alpha1.PollTypeGithub
}

// Returns the runing group
func (s *Poll) GetGroup() int {
	return s.Group
}

func (s *Poll) GetEndpoint() string {
	return s.URL
}

func (s *Poll) PollVersion() (string, error) {
	lastCommit, err := getLastCommit(s.PollClient, s.URL, s.Branch)
	if err != nil {
		return "UNKNOWN", err
	}
	return lastCommit, nil
}

func (s *Poll) GetPipelineRunSpec() pipelines.PipelineRunSpec {
	return s.Spec
}

func (s *Poll) Authenticate(secret *v1.Secret) error {
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
