package v1alpha1

import (
	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	v1 "k8s.io/api/core/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

type SCMRepositoryStatus struct {
	Name         string            `json:"name"`
	StateId      string            `json:"stateId"`
	Type         SCMPollType       `json:"type"`
	Authorised   bool              `json:"authorised"`
	StatusParams []pipelines.Param `json:"statusParams"`
}

type SCMPollRepositoryInteface interface {
	// Get the name of the scmpoll in question
	GetName() string
	// Get the service account to use
	GetServiceAccountName() string
	// Return the url endpoint we are polling
	GetEndpoint() string
	// Get what type of poll this is
	GetType() string
	// Provides a method for any client to be authenticated ready for scmpolling
	Authenticate(*v1.Secret) error
	// A general versioning value retireved from the scmpoll endpoint not annotation. E.g commitSHA for github
	Poll() ([]SCMRepositoryStatus, error)
	// Provides an extension to provide an update back to a repo, e.g git statuses
	Update(SCMRepositoryStatus, duckv1beta1.Conditions) error
}
