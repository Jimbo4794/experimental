package github

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

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tektoncd/experimental/polling/pkg/apis/scmpoll/v1alpha1"
	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

// var (
// 	gitSource = "git-source"
// )

type GitubPullRequest struct {
	Name               string               `json:"name"`
	Type               v1alpha1.SCMPollType `json:"type"`
	URL                string               `json:"url"`
	RebuildString      string               `json:"rebuildString"`
	ServiceAccountName string               `json:"serviceAccountName"`
	PollClient         GitClient
}

type GithubPRJson struct {
	Number     int              `json:"number"`
	State      string           `json:"state"`
	Head       GithubPRHeadJson `json:"head"`
	Base       GithubPRBaseJson `json:"base"`
	CommentURL string           `json:"comments_url"`
}

type GithubPRCommentJson struct {
	Id          int64        `json:"id"`
	CreatedTime *metav1.Time `json:"created_at"`
	Body        string       `json:"body"`
}

type GithubPRHeadJson struct {
	Sha string `json:"sha"`
}

type GithubPRBaseJson struct {
	Ref string `json:"ref"`
}

func NewPR(name string, r v1alpha1.Repository) (*GitubPullRequest, error) {
	if r.SCMPollType != v1alpha1.SCMPollTypeGithubPR {
		return nil, fmt.Errorf("github.poll: Cannot create a Github poll from a %s ", r.SCMPollType)
	}

	githubPRPoll := &GitubPullRequest{
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
			githubPRPoll.URL = param.Value.StringVal
		case strings.EqualFold(param.Name, "rebuildString"):
			githubPRPoll.RebuildString = param.Value.StringVal
		}
	}
	if githubPRPoll.URL == "" {
		return nil, fmt.Errorf("A url poll param must be passed. Please provide a https://api.github.com/repos/<ORG>/<REPO>")
	}
	if githubPRPoll.URL == "" {
		return nil, fmt.Errorf("A url poll param must be passed. Please provide a https://api.github.com/repos/<ORG>/<REPO>")
	}

	return githubPRPoll, nil
}

func updateStatus(url, status, sha string, gitC GitClient) error {
	var statusJson = []byte("{\"state\":\"" + status + "\", \"description\":\"Tekton\", \"context\":\"Tekton\"}")
	req, _ := http.NewRequest("POST", url+"/statuses/"+sha, bytes.NewBuffer(statusJson))
	if gitC.Username != "" && gitC.Token != "" {
		req.SetBasicAuth(gitC.Username, gitC.Token)
	}

	resp, err := gitC.Client.Do(req)
	if err != nil {
		return fmt.Errorf("github.poll: Update status onrepo %s: %s", url, err)
	}
	if resp.StatusCode != 200 {
		if resp.Header.Get("x-ratelimit-remaining") == "0" {
			return fmt.Errorf("github.poll: RATE LIMIT EXCEEDED: Curent rate limit: %v, Rate reset at %v", resp.Header.Get("X-RateLimit-limit"), resp.Header.Get("X-RateLimit-Reset"))
		}
		return fmt.Errorf("github.poll: Bad response from repo %s: %s", url, err)
	}
	defer resp.Body.Close()
	return nil
}

func (s *GitubPullRequest) Update(status v1alpha1.SCMRepositoryStatus, conds duckv1beta1.Conditions) error {
	for _, param := range status.StatusParams {
		if param.Name == "commitSHA" {
			for _, cond := range conds {
				if cond.Type == apis.ConditionSucceeded {
					switch cond.Status {
					case v1.ConditionUnknown:
						return updateStatus(s.URL, "pending", param.Value.StringVal, s.PollClient)
					case v1.ConditionFalse:
						return updateStatus(s.URL, "failure", param.Value.StringVal, s.PollClient)
					case v1.ConditionTrue:
						return updateStatus(s.URL, "success", param.Value.StringVal, s.PollClient)
					}
				}
			}
		}
	}
	return fmt.Errorf("Failed to update status")
}

func checkForRebuildComments(gitC GitClient, rebuildString string, pull GithubPRJson) (string, error) {
	var comments []GithubPRCommentJson
	req, _ := http.NewRequest("GET", pull.CommentURL, nil)
	if gitC.Username != "" && gitC.Token != "" {
		req.SetBasicAuth(gitC.Username, gitC.Token)
	}

	resp, err := gitC.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("github.poll: Failed to get comments from repo %s: %s", pull.CommentURL, err)
	}
	if resp.StatusCode != 200 {
		if resp.Header.Get("x-ratelimit-remaining") == "0" {
			return "", fmt.Errorf("github.poll: RATE LIMIT EXCEEDED: Curent rate limit: %v, Rate reset at %v", resp.Header.Get("X-RateLimit-limit"), resp.Header.Get("X-RateLimit-Reset"))
		}
		return "", fmt.Errorf("github.poll: Bad response from repo %s: %s", pull.CommentURL, err)
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&comments)
	if err != nil {
		return "", fmt.Errorf("github.poll: Failed to decode response from repo %s: %s", pull.CommentURL, err)
	}
	latestRebuildCommentId := ""
	rebuildCommentCreatedTime := metav1.Time{}
	for _, comment := range comments {
		if comment.Body == rebuildString {
			if comment.CreatedTime.After(rebuildCommentCreatedTime.Time) {
				latestRebuildCommentId = strconv.FormatInt(comment.Id, 10)
			}
		}
	}

	return latestRebuildCommentId, nil
}

func (s *GitubPullRequest) Poll() ([]v1alpha1.SCMRepositoryStatus, error) {
	var responses []v1alpha1.SCMRepositoryStatus
	pulls, err := getPullRequests(s.PollClient, s.URL)
	if err != nil {
		return nil, err
	}

	for _, pull := range pulls {
		refspec := "refs/pull/" + strconv.Itoa(pull.Number) + "/head:refs/heads/" + pull.Base.Ref
		commitSha := pull.Head.Sha
		rebuildCommentId, err := checkForRebuildComments(s.PollClient, s.RebuildString, pull)
		if err != nil {
			return nil, err
		}
		response := v1alpha1.SCMRepositoryStatus{
			Name:    s.Name,
			StateId: s.Name + "-pr" + strconv.Itoa(pull.Number),
			Type:    v1alpha1.SCMPollTypeGithubPR,
			StatusParams: []pipelines.Param{
				pipelines.Param{
					Name: "commitSHA",
					Value: pipelines.ArrayOrString{
						Type:      pipelines.ParamTypeString,
						StringVal: commitSha,
					},
				},
				pipelines.Param{
					Name: "refSpec",
					Value: pipelines.ArrayOrString{
						Type:      pipelines.ParamTypeString,
						StringVal: refspec,
					},
				},
				pipelines.Param{
					Name: "rebuildCommentId",
					Value: pipelines.ArrayOrString{
						Type:      pipelines.ParamTypeString,
						StringVal: rebuildCommentId,
					},
				},
			},
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func getPullRequests(gitC GitClient, url string) ([]GithubPRJson, error) {
	var pulls []GithubPRJson
	req, _ := http.NewRequest("GET", url+"/pulls", nil)
	if gitC.Username != "" && gitC.Token != "" {
		req.SetBasicAuth(gitC.Username, gitC.Token)
	}

	resp, err := gitC.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github.poll: Failed to get commit id from repo %s: %s", url, err)
	}
	if resp.StatusCode != 200 {
		if resp.Header.Get("x-ratelimit-remaining") == "0" {
			return nil, fmt.Errorf("github.poll: RATE LIMIT EXCEEDED: Curent rate limit: %v, Rate reset at %v", resp.Header.Get("X-RateLimit-limit"), resp.Header.Get("X-RateLimit-Reset"))
		}
		return nil, fmt.Errorf("github.poll: Bad response from repo %s: %s", url, err)
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&pulls)
	if err != nil {
		return nil, fmt.Errorf("github.poll: Failed to decode response from repo %s: %s", url, err)
	}

	return pulls, nil
}

// GetName returns the name of the Poll
func (s *GitubPullRequest) GetName() string {
	return s.Name
}

// GetType returns the type of the Poll, in this case "Github"
func (s *GitubPullRequest) GetType() v1alpha1.SCMPollType {
	return v1alpha1.SCMPollTypeGithubHead
}

func (s *GitubPullRequest) GetEndpoint() string {
	return s.URL
}

func (s *GitubPullRequest) GetServiceAccountName() string {
	return s.ServiceAccountName
}

func (s *GitubPullRequest) Authenticate(secret *v1.Secret) error {
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
