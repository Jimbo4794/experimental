package v1alpha1

import (
	pipelines "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

type PollRunType = string

// Only github polling for now, but a list of poll endpoints
const (
	// PollRunTypeGithub indicates that this source is a GitHub repo.
	PollRunTypeGithub PollRunType = "github"
)

// AllPollRunTypes can be used for validation to check if a provided PollRun type is one of the known types. Again only github for now
var AllPollRunTypes = []PollRunType{PollRunTypeGithub}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Poll describes a endpoint to poll
type PollRun struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the Poll from the client
	// +optional
	Spec PollRunSpec `json:"spec,omitempty"`

	// +optional
	Status PollRunStatus `json:"status,omitempty"`
}

type PollRunSpec struct {
	// Any information required to take a snapshot of current endpoint state. This avoids any clashes between
	// multiple triggers.
	PollRuns []Run `json:"pollRuns"`
}

type Run struct {
	Name        string                    `json:"name"`
	PollRunType string                    `json:"pollRunType"`
	Group       int                       `json:"group"`
	Version     string                    `json:"version"`
	Spec        pipelines.PipelineRunSpec `json:"spec"`
}

type PollRunStatus struct {
	duckv1beta1.Status `json:",inline"`

	// PipelineRunStatusFields inlines the status fields.
	PollRunStatusFields `json:",inline"`
}

type PollRunStatusFields struct {
	// map of PipelineRunRunStatus with the run name as the key
	// +optional
	Runs []*pipelines.PipelineRunStatus `json:"runs,omitempty"`

	// Flag to show if all the pipelines are complete
	Finished bool `json:"finished"`

	// The Active group runningq
	// +optional
	ActiveGroup string `json:"activeGroup,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PollList contains a list of PipelineResources
type PollRunList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PollRun `json:"items"`
}
