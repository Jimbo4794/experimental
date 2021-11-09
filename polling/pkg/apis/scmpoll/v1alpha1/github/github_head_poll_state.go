package github

import (
	"fmt"
	"strings"

	"github.com/tektoncd/experimental/polling/pkg/apis/scmpoll/v1alpha1"
)

type GithubHeadState struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	CommitSHA string `json:"commitSHA"`
}

func GetGithubHeadType(s *v1alpha1.SCMRepositoryStatus) (*GithubHeadState, error) {
	state := &GithubHeadState{
		Name: s.Name,
		Type: s.Type,
	}

	for _, param := range s.StatusParams {
		switch {
		case strings.EqualFold(param.Name, "commitSHA"):
			state.CommitSHA = param.Value.StringVal
		}
	}

	if state.CommitSHA == "" {
		return nil, fmt.Errorf("No CommitSHA status param found!")
	}

	return state, nil
}

func (g *GithubHeadState) GetName() string {
	return g.Name
}

func (g *GithubHeadState) HasStatusChanged(s v1alpha1.SCMRepositoryStatus) (bool, error) {
	if s.Type != v1alpha1.SCMPollTypeGithubHead {
		return false, fmt.Errorf("%v is the incorrect type to compare to a github-head poll type", s)
	}
	incomingSHA := ""
	for _, param := range s.StatusParams {
		switch {
		case strings.EqualFold(param.Name, "commitSHA"):
			incomingSHA = param.Value.StringVal
		}
	}
	if incomingSHA == "" {
		return false, fmt.Errorf("No CommitSHA was passed to compare with")
	}

	if incomingSHA != g.CommitSHA {
		return true, nil
	}

	return false, nil
}
