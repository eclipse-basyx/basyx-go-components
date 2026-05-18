package aasenvironment

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	aasx "github.com/aas-core-works/aas-package3-golang"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func TestUploadProcessingStatus_PropagatesWrappedCommonErrors(t *testing.T) {
	testCases := []struct {
		name   string
		err    error
		status int
	}{
		{
			name:   "bad request",
			err:    fmt.Errorf("wrap: %w", common.NewErrBadRequest("x")),
			status: http.StatusBadRequest,
		},
		{
			name:   "method not allowed",
			err:    fmt.Errorf("wrap: %w", common.NewErrMethodNotAllowed("x")),
			status: http.StatusMethodNotAllowed,
		},
		{
			name:   "not found",
			err:    fmt.Errorf("wrap: %w", common.NewErrNotFound("x")),
			status: http.StatusNotFound,
		},
		{
			name:   "denied",
			err:    fmt.Errorf("wrap: %w", common.NewErrDenied("x")),
			status: http.StatusForbidden,
		},
		{
			name:   "conflict",
			err:    fmt.Errorf("wrap: %w", common.NewErrConflict("x")),
			status: http.StatusConflict,
		},
		{
			name:   "internal",
			err:    fmt.Errorf("wrap: %w", common.NewInternalServerError("x")),
			status: http.StatusInternalServerError,
		},
		{
			name:   "unknown defaults to internal",
			err:    fmt.Errorf("random"),
			status: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := uploadProcessingStatus(tc.err)
			if got != tc.status {
				t.Fatalf("expected status %d, got %d for %v", tc.status, got, tc.err)
			}
		})
	}
}

func TestReadEnvironmentFromAASXSpec_AdaptsLegacyNamespace(t *testing.T) {
	hartingPath := filepath.Join("integration_tests", "testdata", "HARTING_AAS_09140009950.aasx")
	if _, err := os.Stat(hartingPath); err != nil {
		t.Fatalf("failed to access HARTING fixture: %v", err)
	}

	hartingFile, err := os.Open(hartingPath) // #nosec G304 -- test fixture path is static and controlled by repository sources.
	if err != nil {
		t.Fatalf("failed to open HARTING fixture: %v", err)
	}
	defer func() {
		_ = hartingFile.Close()
	}()

	packaging := aasx.NewPackaging()
	packageReader, err := packaging.OpenReadFromStream(hartingFile)
	if err != nil {
		t.Fatalf("failed to open AASX package: %v", err)
	}
	defer func() {
		_ = packageReader.Close()
	}()

	_, environment, parseErr := readEnvironmentFromAASXSpec(packageReader)
	if parseErr != nil {
		t.Fatalf("expected legacy namespace to be adapted successfully, got error: %v", parseErr)
	}
	if environment == nil {
		t.Fatal("expected parsed environment, got nil")
	}
	if len(environment.AssetAdministrationShells()) == 0 {
		t.Fatal("expected parsed environment to contain at least one AAS")
	}
}
