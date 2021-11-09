package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"

	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

type SCMPollType = string

const (
	// PipelineResourceTypeGit indicates that this source is a GitHub repo.
	SCMPollTypeGithubHead SCMPollType = "github-head"
	SCMPollTypeGithubPR   SCMPollType = "github-pr"
)

// AllResourceTypes can be used for validation to check if a provided Resource type is one of the known types. Again only github for now
var AllSCMPollTypes = []SCMPollType{SCMPollTypeGithubHead, SCMPollTypeGithubPR}

// These type can only create a single scmpollstate, as we are polling a static object, e.g branch on a git repo
var StaticSCMPollTypes = []SCMPollType{SCMPollTypeGithubHead}

// These type can create many scmpollstates relative to what is being polled, e.g pull requests on a repo
var DynamicSCMPollTypes = []SCMPollType{SCMPollTypeGithubPR}

// +genclient
// +genreconciler:krshapedlogic=false
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SCMPoll describes a endpoint to scmpoll
type SCMPoll struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the SCMPoll
	Spec SCMPollSpec `json:"spec,omitempty"`

	// +optional
	Status SCMPollStatus `json:"status,omitempty"`
}

type SCMPollStatus struct {
	duckv1beta1.Status `json:",inline"`
}

type SCMRepositoryStatus struct {
	Name         string            `json:"name"`
	StateId      string            `json:"stateId"`
	Type         SCMPollType       `json:"type"`
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

// SCMPollSpec defines an endpoint to watch.
type SCMPollSpec struct {
	// Description is a user-facing description of the resource that may be
	// used to populate a UI.
	// +optional
	Description         string                    `json:"description,omitempty"`
	SCMPollFrequency    int64                     `json:"pollFrequency"`
	Repositories        []Repository              `json:"repositories"`
	PipelineRunSpec     pipelines.PipelineRunSpec `json:"pipelineRunSpec"`
	Tidy                bool                      `json:"tidy"`
	ConcurrentPipelines bool                      `json:"concurrentPipelines"`
}

type Repository struct {
	Name                      string            `json:"name"`
	SCMPollType               string            `json:"type"`
	SCMPollServiceAccountName string            `json:"serviceAccountName,omitempty"`
	SCMPollParams             []pipelines.Param `json:"params,omitempty"`
}

// GetGroupVersionKind implements kmeta.OwnerRefable.
func (*SCMPoll) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("SCMPoll")
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SCMPollList contains a list of PipelineResources
type SCMPollList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SCMPoll `json:"items"`
}
