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
	"io"
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
	ApprovalString     string               `json:"approvalString"`
	ServiceAccountName string               `json:"serviceAccountName"`
	PollClient         GitClient
}

type GithubRepoJson struct {
	NodeId           string `json:"node_id"`
	Name             string `json:"name"`
	CollaboratorsUrl string `json:"collaborators_url"`
}

type GithubPRJson struct {
	Number     int              `json:"number"`
	State      string           `json:"state"`
	User       GithubPRUserJson `json:"user"`
	Head       GithubPRHeadJson `json:"head"`
	Base       GithubPRBaseJson `json:"base"`
	CommentURL string           `json:"comments_url"`
	CommitsURL string           `json:"commits_url"`
}

type GithubCollaboratorsJson struct {
	Login    string `json:"login"`
	RoleName string `json:"role_name"`
}

type GithubPRCommentJson struct {
	Id          int64            `json:"id"`
	CreatedTime *metav1.Time     `json:"created_at"`
	Body        string           `json:"body"`
	User        GithubPRUserJson `json:"user"`
}

type GithubStatusJson struct {
	State string `json:"state"`
}

type GithubPRCommitJson struct {
	Sha    string           `json:"sha"`
	Commit GithubCommitJson `json:"commit"`
}
type GithubCommitJson struct {
	Author GthubAuthorJson `json:"author"`
}

type GthubAuthorJson struct {
	Date *metav1.Time `json:"date"`
}

type GithubPRHeadJson struct {
	Sha string `json:"sha"`
}

type GithubPRUserJson struct {
	Login string `json:"login"`
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
		case strings.EqualFold(param.Name, "approvalString"):
			githubPRPoll.ApprovalString = param.Value.StringVal
		}
	}
	if githubPRPoll.URL == "" {
		return nil, fmt.Errorf("a url poll param must be passed. Please provide a https://api.github.com/repos/<ORG>/<REPO>")
	}
	if githubPRPoll.URL == "" {
		return nil, fmt.Errorf("a url poll param must be passed. Please provide a https://api.github.com/repos/<ORG>/<REPO>")
	}

	return githubPRPoll, nil
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
		lastCommitTimestamp := getLastCommitTimestamp(s.PollClient, pull.CommitsURL)
		authorised := s.checkUserHasPermissionsToTrigger(pull)
		response := v1alpha1.SCMRepositoryStatus{
			Name:       s.Name,
			StateId:    s.Name + "-pr" + strconv.Itoa(pull.Number),
			Type:       v1alpha1.SCMPollTypeGithubPR,
			Authorised: authorised,
			StatusParams: []pipelines.Param{
				{
					Name: "commitSHA",
					Value: pipelines.ArrayOrString{
						Type:      pipelines.ParamTypeString,
						StringVal: commitSha,
					},
				},
				{
					Name: "pullRequestNumber",
					Value: pipelines.ArrayOrString{
						Type:      pipelines.ParamTypeString,
						StringVal: strconv.Itoa(pull.Number),
					},
				},
				{
					Name: "refSpec",
					Value: pipelines.ArrayOrString{
						Type:      pipelines.ParamTypeString,
						StringVal: refspec,
					},
				},
				{
					Name: "rebuildCommentId",
					Value: pipelines.ArrayOrString{
						Type:      pipelines.ParamTypeString,
						StringVal: rebuildCommentId,
					},
				},
				{
					Name: "lastCommitTimestamp",
					Value: pipelines.ArrayOrString{
						Type:      pipelines.ParamTypeString,
						StringVal: lastCommitTimestamp.String(),
					},
				},
			},
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func (s *GitubPullRequest) Update(status v1alpha1.SCMRepositoryStatus, conds duckv1beta1.Conditions) error {
	var lastCommitTimestamp time.Time
	var prNum string
	for _, param := range status.StatusParams {
		if param.Name == "lastCommitTimestamp" {
			t, _ := time.Parse("2006-01-02 15:04:05 +0000 UTC", param.Value.StringVal)
			lastCommitTimestamp = t
		}
		if param.Name == "pullRequestNumber" {
			prNum = param.Value.StringVal
		}
	}
	for _, param := range status.StatusParams {
		if param.Name == "commitSHA" {
			if !status.Authorised {
				s.askForApprovalIfRequired(fmt.Sprintf("Can a repo admin please check this commit and comment '%s' to prcoeed to build.", s.ApprovalString), prNum, lastCommitTimestamp)
				return updateStatus(s.URL, "pending", param.Value.StringVal, s.PollClient)
			}
			for _, cond := range conds {
				if cond.Type == apis.ConditionSucceeded {
					switch cond.Status {
					case v1.ConditionUnknown:
						return updateStatusIfRequired(s.URL, "pending", param.Value.StringVal, s.PollClient)
					case v1.ConditionFalse:
						return updateStatusIfRequired(s.URL, "failure", param.Value.StringVal, s.PollClient)
					case v1.ConditionTrue:
						return updateStatusIfRequired(s.URL, "success", param.Value.StringVal, s.PollClient)
					}
				}
			}
		}
	}
	return fmt.Errorf("failed to update status")
}

func updateStatusIfRequired(url, status, sha string, gitC GitClient) error {
	var statusJson []GithubStatusJson
	err := getJson(gitC, url+"/statuses/"+sha, &statusJson)
	if err != nil {
		return err
	}

	if len(statusJson) == 0 {
		return updateStatus(url, status, sha, gitC)
	}

	if statusJson[0].State == status {
		return nil
	}
	return updateStatus(url, status, sha, gitC)
}

func updateStatus(url, status, sha string, gitC GitClient) error {
	var statusJson = "{\"state\":\"" + status + "\", \"description\":\"Tekton\", \"context\":\"Tekton\"}"
	return post(gitC, url+"/statuses/"+sha, bytes.NewBufferString(statusJson))
}

func (s *GitubPullRequest) askForApprovalIfRequired(commentMessage, prNum string, lastCommitTimestamp time.Time) error {
	var comments []GithubPRCommentJson
	err := getJson(s.PollClient, s.URL+"/issues/"+prNum+"/comments", &comments)
	if err != nil {
		return err
	}
	// Check to see the PR hasnt been updated already
	for _, comment := range comments {
		if comment.Body == commentMessage && lastCommitTimestamp.Before(comment.CreatedTime.Time) {
			fmt.Println("We dont need to")
			return nil
		}
	}

	req, _ := http.NewRequest("POST", s.URL+"/issues/"+prNum+"/comments", strings.NewReader("{\"body\":\""+commentMessage+"\"}"))
	if s.PollClient.Username != "" && s.PollClient.Token != "" {
		req.SetBasicAuth(s.PollClient.Username, s.PollClient.Token)
	}
	_, err = s.PollClient.Client.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to comment, asking for approval")
	}
	return nil
}

func getPRComments(gitC GitClient, pull GithubPRJson) ([]GithubPRCommentJson, error) {
	var comments []GithubPRCommentJson
	err := getJson(gitC, pull.CommentURL, &comments)
	if err != nil {
		return nil, fmt.Errorf("github.poll: Failed to decode PR comments %s: %s", pull.CommentURL, err)
	}
	return comments, nil
}

func checkForRebuildComments(gitC GitClient, rebuildString string, pull GithubPRJson) (string, error) {
	latestRebuildCommentId := ""
	rebuildCommentCreatedTime := metav1.Time{}

	comments, err := getPRComments(gitC, pull)
	if err != nil {
		return "", err
	}
	for _, comment := range comments {
		if comment.Body == rebuildString {
			if comment.CreatedTime.After(rebuildCommentCreatedTime.Time) {
				latestRebuildCommentId = strconv.FormatInt(comment.Id, 10)
			}
		}
	}

	return latestRebuildCommentId, nil
}

func getAuthorisedUsers(gitC GitClient, url string) []string {
	var repo GithubRepoJson
	var collaborators []GithubCollaboratorsJson
	var users []string
	err := getJson(gitC, url, &repo)
	if err != nil {
		return nil
	}

	err = getJson(gitC, strings.Replace(repo.CollaboratorsUrl, "{/collaborator}", "", 1), &collaborators)
	if err != nil {
		return nil
	}

	for _, u := range collaborators {
		if u.RoleName == "write" || u.RoleName == "admin" || u.RoleName == "maintain" {
			users = append(users, u.Login)
		}
	}

	return users
}

func (s *GitubPullRequest) checkUserHasPermissionsToTrigger(pr GithubPRJson) bool {
	fmt.Println("CHECKING USER HAS PERMISSIONS TO TRIGGER")
	allowedUsers := getAuthorisedUsers(s.PollClient, s.URL)
	for _, u := range allowedUsers {
		if pr.User.Login == u {
			return true
		}
	}
	return s.checkForApproval(pr, allowedUsers)
}

func getLastCommitTimestamp(gitC GitClient, commitsURL string) *metav1.Time {
	var commits []GithubPRCommitJson
	err := getJson(gitC, commitsURL, &commits)
	if err != nil {
		return nil
	}
	if len(commits) == 0 {
		return nil
	}
	return commits[len(commits)-1].Commit.Author.Date
}

func (s *GitubPullRequest) checkForApproval(pr GithubPRJson, allowedUsers []string) bool {
	lastCommitTimestamp := getLastCommitTimestamp(s.PollClient, pr.CommitsURL)
	comments, _ := getPRComments(s.PollClient, pr)
	for _, comment := range comments {
		// check comments after latest commit as dont want to approve a newer commit with old comment
		if comment.CreatedTime.Time.Before(lastCommitTimestamp.Time) {
			continue
		}
		// Look for approval string
		if !strings.EqualFold(comment.Body, s.ApprovalString) {
			continue
		}
		// Make sure author is authorised to approve
		for _, u := range allowedUsers {
			if comment.User.Login == u {
				return true
			}
		}
	}

	return false
}

func getPullRequests(gitC GitClient, url string) ([]GithubPRJson, error) {
	var pulls []GithubPRJson
	err := getJson(gitC, url+"/pulls", &pulls)
	if err != nil {
		return nil, fmt.Errorf("github.poll: failed to get PR's: %v", err)
	}

	return pulls, nil
}

func getJson(gitC GitClient, url string, v interface{}) error {
	req, _ := http.NewRequest("GET", url, nil)
	if gitC.Username != "" && gitC.Token != "" {
		req.SetBasicAuth(gitC.Username, gitC.Token)
	}
	resp, err := gitC.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get request on %s", url)
	}
	if resp.StatusCode != 200 {
		if resp.Header.Get("x-ratelimit-remaining") == "0" {
			return fmt.Errorf("github.poll: RATE LIMIT EXCEEDED: Curent rate limit: %v, Rate reset at %v", resp.Header.Get("X-RateLimit-limit"), resp.Header.Get("X-RateLimit-Reset"))
		}
		return fmt.Errorf("github.poll: Bad response from repo %s: %s", url, err)
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&v)
	if err != nil {
		return fmt.Errorf("github.poll: Failed to decode response from repo %s: %s", url, err)
	}

	return nil
}

func post(gitC GitClient, url string, body io.Reader) error {
	req, _ := http.NewRequest("POST", url, body)
	if gitC.Username != "" && gitC.Token != "" {
		req.SetBasicAuth(gitC.Username, gitC.Token)
	}

	resp, err := gitC.Client.Do(req)
	if err != nil {
		return fmt.Errorf("github.poll: update status onrepo %s: %s", url, err)
	}
	if resp.StatusCode != 200 {
		if resp.Header.Get("x-ratelimit-remaining") == "0" {
			return fmt.Errorf("github.poll: rate limit exceeded: curent rate limit: %v, rate reset at %v", resp.Header.Get("X-RateLimit-limit"), resp.Header.Get("X-RateLimit-Reset"))
		}
		return fmt.Errorf("github.poll: bad response %v from repo %s: %s", resp, url, err)
	}
	defer resp.Body.Close()
	return nil
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
		return fmt.Errorf("unsupported Auth type, basic token only")
	}

	if user, ok := secret.Data["username"]; ok {
		s.PollClient.Username = string(user)
	} else {
		return fmt.Errorf("no username found in secret %v", secret)
	}

	if token, ok := secret.Data["token"]; ok {
		s.PollClient.Token = string(token)
	} else {
		return fmt.Errorf("no token found in secret %v", secret)
	}
	return nil
}
