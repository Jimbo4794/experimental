package v1alpha1

import (
	"context"

	"github.com/tektoncd/pipeline/pkg/apis/validate"
	"knative.dev/pkg/apis"
)

var _ apis.Validatable = (*SCMPoll)(nil)

// Validate implements apis.Validatable
func (s *SCMPoll) Validate(ctx context.Context) *apis.FieldError {
	errs := validate.ObjectMetadata(s.GetObjectMeta()).ViaField("metadata")
	if apis.IsInDelete(ctx) {
		return nil
	}
	return errs.Also(s.Spec.Validate(apis.WithinSpec(ctx)).ViaField("spec"))
}

func (ss *SCMPollSpec) Validate(ctx context.Context) (errs *apis.FieldError) {
	numRepos := len(ss.Repositories)
	for _, repo := range ss.Repositories {
		if isDynamicState(repo.SCMPollType) && numRepos > 1 {
			return errs.Also(apis.ErrGeneric("Dynamic states can only be defined individually! Please remove other reposotries or dynamic repos"))
		}
	}
	return nil
}

func isDynamicState(state SCMPollType) bool {
	for _, s := range DynamicSCMPollTypes {
		if s == state {
			return true
		}
	}
	return false
}
