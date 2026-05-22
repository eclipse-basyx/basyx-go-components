//nolint:all
package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/persistence"
)

func TestPostAllAssetLinksByIDRejectsEmptySpecificAssetIDFields(t *testing.T) {
	service := NewAssetAdministrationShellBasicDiscoveryAPIAPIService(persistencepostgresql.PostgreSQLDiscoveryDatabase{})
	aasIdentifier := common.EncodeString("urn:aas:test:empty-fields")

	tests := []struct {
		name          string
		links         []types.ISpecificAssetID
		expectedError string
	}{
		{
			name:          "empty name",
			links:         []types.ISpecificAssetID{types.NewSpecificAssetID("", "some-value")},
			expectedError: "DISC-POSTASSETLINKS-EMPTYNAME",
		},
		{
			name:          "empty value",
			links:         []types.ISpecificAssetID{types.NewSpecificAssetID("serialNumber", "")},
			expectedError: "DISC-POSTASSETLINKS-EMPTYVALUE",
		},
		{
			name:          "nil specific asset id",
			links:         []types.ISpecificAssetID{nil},
			expectedError: "DISC-POSTASSETLINKS-NILSPECIFICASSETID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := service.PostAllAssetLinksByID(context.Background(), aasIdentifier, tt.links)
			if err != nil {
				t.Fatalf("expected response error body without returned error, got %v", err)
			}
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
			}
			errorBody := response.Body.([]common.ErrorHandler)
			if len(errorBody) != 1 {
				t.Fatalf("expected one error response, got %#v", response.Body)
			}
			if !strings.Contains(errorBody[0].Text, tt.expectedError) {
				t.Fatalf("expected error message to contain %q, got %#v", tt.expectedError, response.Body)
			}
		})
	}
}

func TestAddAllAssetLinksByIDRejectsEmptySpecificAssetIDFields(t *testing.T) {
	service := NewAssetAdministrationShellBasicDiscoveryAPIAPIService(persistencepostgresql.PostgreSQLDiscoveryDatabase{})
	aasIdentifier := common.EncodeString("urn:aas:test:empty-fields")

	response, err := service.AddAllAssetLinksByID(
		context.Background(),
		aasIdentifier,
		[]types.ISpecificAssetID{types.NewSpecificAssetID("serialNumber", " ")},
	)
	if err != nil {
		t.Fatalf("expected response error body without returned error, got %v", err)
	}
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, response.Code)
	}
	errorBody := response.Body.([]common.ErrorHandler)
	if len(errorBody) != 1 {
		t.Fatalf("expected one error response, got %#v", response.Body)
	}
	if !strings.Contains(errorBody[0].Text, "DISC-ADDASSETLINKS-EMPTYVALUE") {
		t.Fatalf("expected empty value error, got %#v", response.Body)
	}
}
