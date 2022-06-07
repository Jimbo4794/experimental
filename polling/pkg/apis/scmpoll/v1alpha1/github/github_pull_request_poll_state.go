package github

import (
	"fmt"
	"strings"

	"github.com/tektoncd/experimental/polling/pkg/apis/scmpoll/v1alpha1"
)

type GithubPRState struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	PullNumber       string `json:"pullNumber"`
	RefSpec          string `json:"refSpec"`
	CommitSHA        string `json:"commitSHA"`
	RebuildCommentId string `json:"rebuildCommentId"`
	Authorised       bool   `json:"authorised"`
	PollClient       GitClient
}

func GetGithubPRType(s *v1alpha1.SCMRepositoryStatus) (*GithubPRState, error) {
	state := &GithubPRState{
		Name:             s.Name,
		Type:             s.Type,
		Authorised:       s.Authorised,
		RebuildCommentId: "",
	}

	for _, param := range s.StatusParams {
		switch {
		case strings.EqualFold(param.Name, "refSpec"):
			state.RefSpec = param.Value.StringVal
		case strings.EqualFold(param.Name, "commitSHA"):
			state.CommitSHA = param.Value.StringVal
		case strings.EqualFold(param.Name, "pullNumber"):
			state.PullNumber = param.Value.StringVal
		case strings.EqualFold(param.Name, "rebuildCommentId"):
			state.RebuildCommentId = param.Value.StringVal
		}

	}

	if state.CommitSHA == "" {
		return nil, fmt.Errorf("No CommitSHA status param found!")
	}
	if state.RefSpec == "" {
		return nil, fmt.Errorf("No RefSpec status param found!")
	}

	return state, nil
}

func (g *GithubPRState) GetName() string {
	return g.Name
}

func (g *GithubPRState) HasStatusChanged(s v1alpha1.SCMRepositoryStatus) (bool, error) {
	if s.Type != v1alpha1.SCMPollTypeGithubPR {
		return false, fmt.Errorf("%v is the incorrect type to compare to a github-pr poll type", s)
	}
	incomingSHA := ""
	incomingRefSpec := ""
	incomingCommentID := ""
	for _, param := range s.StatusParams {
		switch {
		case strings.EqualFold(param.Name, "commitSHA"):
			incomingSHA = param.Value.StringVal
		case strings.EqualFold(param.Name, "refSpec"):
			incomingRefSpec = param.Value.StringVal
		case strings.EqualFold(param.Name, "rebuildCommentId"):
			incomingCommentID = param.Value.StringVal
		}
	}
	if incomingSHA == "" {
		return false, fmt.Errorf("No CommitSHA was passed to compare with")
	}
	if incomingRefSpec == "" {
		return false, fmt.Errorf("No RefSpec was passed to compare with")
	}

	if g.Authorised != s.Authorised {
		return true, nil
	}

	if incomingSHA != g.CommitSHA {
		return true, nil
	}
	if incomingRefSpec != g.RefSpec {
		return true, nil
	}
	if incomingCommentID != g.RebuildCommentId {
		return true, nil
	}

	return false, nil
}
