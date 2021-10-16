package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"

	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

type PollType = string

// Only github polling for now, but a list of poll endpoints
const (
	// PipelineResourceTypeGit indicates that this source is a GitHub repo.
	PollTypeGithub PollType = "github"
)

// AllResourceTypes can be used for validation to check if a provided Resource type is one of the known types. Again only github for now
var AllPollTypes = []PollType{PollTypeGithub}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Poll describes a endpoint to poll
type Poll struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the Poll
	Spec PollSpec `json:"spec,omitempty"`

	// +optional
	Status PollStatus `json:"status,omitempty"`
}

type PollStatus struct {
	duckv1beta1.Status `json:",inline"`

	// PollStatusFields inlines the status fields.
	PollStatusFields `json:",inline"`
}

type PollStatusFields struct {
	// An indication of when the next poll is due
	// +optional
	NextPoll *metav1.Time `json:"nextPoll,omitempty"`

	// Last creation of a pollrun
	// +optional
	LastRun *metav1.Time `json:"lastRun,omitempty"`
}

type PollInterface interface {
	// Get the name of the poll in question
	GetName() string
	// Get the type of the Poll, currently only github
	GetType() PollType
	// Get the Group number. Allows polls to be ordered and congruent
	GetGroup() int
	// Provides a method for any client to be authenticated ready for polling
	Authenticate(*v1.Secret) error
	// Return the pollingEndpoint
	GetEndpoint() string
	// A general versioning value retireved from the poll endpoint not annotation. E.g commitSHA for github
	PollVersion() (string, error)
	// Get the PipelineRunSpec
	GetPipelineRunSpec() pipelines.PipelineRunSpec
}

// PollSpec defines an endpoint to watch.
type PollSpec struct {
	// Description is a user-facing description of the resource that may be
	// used to populate a UI.
	// +optional
	Description string `json:"description,omitempty"`
	// Name to reference in the pipelines which repo is being watched
	WatchName string         `json:"watchName"`
	Pipelines []PipelinePoll `json:"pipelineRuns"`

	PollFrequency          int64  `json:"pollFrequency"`
	PollServiceAccountName string `json:"pollServiceAccountName"`
}

type PipelinePoll struct {
	Name string `json:"name"`
	// +optional
	Group      int                       `json:"group,omitempty"`
	PollType   string                    `json:"pollType"`
	PollParams []Param                   `json:"pollParams,omitempty"`
	Spec       pipelines.PipelineRunSpec `json:"spec"`
}

// PollParam declares a string value to use for the parameter called Name, and is used in
// the specific context of Poll.
type Param struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// GetGroupVersionKind implements kmeta.OwnerRefable.
func (*Poll) GetGroupVersionKind() schema.GroupVersionKind {
	return SchemeGroupVersion.WithKind("Poll")
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PollList contains a list of PipelineResources
type PollList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Poll `json:"items"`
}
