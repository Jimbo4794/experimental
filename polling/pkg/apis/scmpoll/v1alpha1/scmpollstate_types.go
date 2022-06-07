package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"

	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
)

// +genclient
// +genreconciler:krshapedlogic=false
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SCMPollState struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the Poll from the client
	Spec SCMPollStateSpec `json:"spec,omitempty"`

	// +optional
	Status SCMPollStateStatus `json:"status,omitempty"`
}

type SCMPollStateSpec struct {
	Name         string                    `json:"name"`
	Repositories []Repository              `json:"repositories"`
	Tidy         bool                      `json:"tidy"`
	Pending      bool                      `json:"pending"`
	PipelineSpec pipelines.PipelineRunSpec `json:"pipelineSpec"`
}

type SCMPollStateStatus struct {
	duckv1beta1.Status `json:",inline"`

	// PipelineRunStatusFields inlines the status fields.
	SCMPollStateStatusFields `json:",inline"`
}

type StateStatus = string

const (
	StatePass    StateStatus = "pass"
	StateFailed  StateStatus = "failed"
	StateUnknown StateStatus = "unknown"
)

var StateStatusReasons = []SCMPollType{StatePass, StateFailed, StateUnknown}

type SCMPollStateStatusFields struct {
	// +optional
	PipelineRunning bool `json:"pipelineRunning"`

	// +optional
	Outdated bool `json:"outdated"`

	// +optional
	LastUpdate *metav1.Time `json:"lastUpdate"`

	// A state of the pipelines
	// +optional
	StateStatus StateStatus `json:"stateStatus"`

	// The status of repos being watched from a scmpoll
	// +optional
	SCMPollRepos map[string]*SCMRepositoryStatus `json:"scmpollRepos"`
}

type SCMPollStateInterface interface {
	GetName() string
	HasStatusChanged(s SCMRepositoryStatus) (bool, error)
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PollList contains a list of PipelineResources
type SCMPollStateList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SCMPollState `json:"items"`
}
