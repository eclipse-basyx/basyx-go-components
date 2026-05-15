package openapi

import (
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func TestNewSubmodelRepositoryAPIAPIControllerParsesVerificationModeCaseAndWhitespace(t *testing.T) {
	ctrl := NewSubmodelRepositoryAPIAPIController(nil, "", " PerMiSsIvE ")
	if ctrl.verificationMode != model.VerificationModePermissive {
		t.Fatalf("expected permissive verification mode, got %q", ctrl.verificationMode)
	}
}
