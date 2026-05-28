//nolint:all
package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
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

func TestSearchAllAssetAdministrationShellIdsByAssetLinkDoesNotShortCircuitWhenConstrainedAndEmpty(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	backend, err := persistencepostgresql.NewPostgreSQLDiscoveryBackendFromDB(db)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	service := NewAssetAdministrationShellBasicDiscoveryAPIAPIService(*backend)

	rows := sqlmock.NewRows([]string{"aasid"}).AddRow("urn:aas:test:constrained")
	mock.ExpectQuery(`SELECT "aas_identifier"\."aasid"`).WillReturnRows(rows)

	ctx := WithAssetLinksAlreadyConstrained(context.Background())
	response, searchErr := service.SearchAllAssetAdministrationShellIdsByAssetLink(ctx, 100, "", []model.AssetLink{})
	if searchErr != nil {
		t.Fatalf("expected no error, got %v", searchErr)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	body, ok := response.Body.(model.GetAllAssetAdministrationShellIdsByAssetLink200Response)
	if !ok {
		t.Fatalf("expected response body type GetAllAssetAdministrationShellIdsByAssetLink200Response, got %T", response.Body)
	}
	if len(body.Result) != 1 || body.Result[0] != "urn:aas:test:constrained" {
		t.Fatalf("expected one backend result, got %#v", body.Result)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected backend query to be executed, but expectations were not met: %v", err)
	}
}
