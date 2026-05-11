// Package api implements HTTP service behavior for the AASX file server.
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/aasxfileserver/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasxfileserverapi/go"
)

const componentName = "AASXFS"

// AASXFileServerAPIAPIService implements the generated AASX file server API service interface.
type AASXFileServerAPIAPIService struct {
	backend *persistence.AASXFileServerDatabase
}

// NewAASXFileServerAPIAPIService constructs an API service backed by a Postgres persistence layer.
func NewAASXFileServerAPIAPIService(backend *persistence.AASXFileServerDatabase) *AASXFileServerAPIAPIService {
	return &AASXFileServerAPIAPIService{backend: backend}
}

// GetAllAASXPackageIds lists available package descriptors with optional AAS filter and paging.
func (s *AASXFileServerAPIAPIService) GetAllAASXPackageIds(ctx context.Context, aasID string, limit int32, cursor string) (openapi.ImplResponse, error) {
	const operation = "GetAllAASXPackageIds"

	decodedCursor := ""
	if strings.TrimSpace(cursor) != "" {
		var decodeErr error
		decodedCursor, decodeErr = common.DecodeString(cursor)
		if decodeErr != nil {
			return newAPIErrorResponse(decodeErr, http.StatusBadRequest, operation, "BadCursor"), nil
		}
	}

	cursorID, parseErr := persistence.ParseCursorID(decodedCursor)
	if parseErr != nil {
		return newAPIErrorResponse(parseErr, http.StatusBadRequest, operation, "BadCursorValue"), nil
	}

	decodedAASID := ""
	if strings.TrimSpace(aasID) != "" {
		var decodeErr error
		decodedAASID, decodeErr = common.DecodeString(aasID)
		if decodeErr != nil {
			return newAPIErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedAasId"), nil
		}
	}

	records, nextCursorID, err := s.backend.ListPackages(ctx, limit, cursorID, decodedAASID)
	if err != nil {
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "ListPackages"), err
	}

	result := make([]openapi.PackageDescription, 0, len(records))
	for _, record := range records {
		result = append(result, toPackageDescription(record))
	}

	nextCursor := ""
	if nextCursorID > 0 {
		nextCursor = common.EncodeString(strconv.FormatInt(nextCursorID, 10))
	}

	return openapi.Response(http.StatusOK, openapi.GetPackageDescriptionsResult{
		PagingMetadata: openapi.PagedResultPagingMetadata{Cursor: nextCursor},
		Result:         result,
	}), nil
}

// PostAASXPackage stores a new package with a server-generated package identifier.
func (s *AASXFileServerAPIAPIService) PostAASXPackage(ctx context.Context, file *os.File, aasIDs []string, fileName string) (openapi.ImplResponse, error) {
	const operation = "PostAASXPackage"

	if file == nil {
		return newAPIErrorResponse(errors.New("multipart form field 'file' is required"), http.StatusBadRequest, operation, "MissingFile"), nil
	}

	rawPackageID := generatePackageID()
	record, err := s.backend.CreatePackage(ctx, rawPackageID, file, aasIDs, fileName)
	if err != nil {
		if common.IsErrConflict(err) {
			return newAPIErrorResponse(err, http.StatusConflict, operation, "PackageIdConflict"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "CreatePackage"), err
	}

	return openapi.Response(http.StatusCreated, toPackageDescription(*record)), nil
}

// GetAASXByPackageId returns the binary payload and metadata headers for one package.
func (s *AASXFileServerAPIAPIService) GetAASXByPackageId(ctx context.Context, packageID string) (openapi.ImplResponse, error) {
	const operation = "GetAASXByPackageId"

	decodedPackageID, decodeErr := common.DecodeString(packageID)
	if decodeErr != nil {
		return newAPIErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedPackageId"), nil
	}

	pkg, err := s.backend.GetPackageByID(ctx, decodedPackageID)
	if err != nil {
		if common.IsErrNotFound(err) {
			return newAPIErrorResponse(err, http.StatusNotFound, operation, "PackageNotFound"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "GetPackageByID"), err
	}

	return openapi.Response(http.StatusOK, openapi.FileDownload{
		Content:     pkg.Content,
		ContentType: pkg.ContentType,
		Filename:    pkg.FileName,
		Headers: map[string]string{
			"X-FileName": pkg.FileName,
		},
	}), nil
}

// PutAASXByPackageId creates or updates a package addressed by the path package identifier.
func (s *AASXFileServerAPIAPIService) PutAASXByPackageId(ctx context.Context, packageID string, file *os.File, aasIDs []string, fileName string) (openapi.ImplResponse, error) {
	const operation = "PutAASXByPackageId"

	if file == nil {
		return newAPIErrorResponse(errors.New("multipart form field 'file' is required"), http.StatusBadRequest, operation, "MissingFile"), nil
	}

	decodedPackageID, decodeErr := common.DecodeString(packageID)
	if decodeErr != nil {
		return newAPIErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedPackageId"), nil
	}

	updated, record, err := s.backend.PutPackage(ctx, decodedPackageID, file, aasIDs, fileName)
	if err != nil {
		if common.IsErrConflict(err) {
			return newAPIErrorResponse(err, http.StatusConflict, operation, "PackageIdConflict"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "PutPackage"), err
	}

	if updated {
		return openapi.Response(http.StatusNoContent, nil), nil
	}

	return openapi.Response(http.StatusCreated, toPackageDescription(*record)), nil
}

// DeleteAASXByPackageId removes a package and its associated large object content.
func (s *AASXFileServerAPIAPIService) DeleteAASXByPackageId(ctx context.Context, packageID string) (openapi.ImplResponse, error) {
	const operation = "DeleteAASXByPackageId"

	decodedPackageID, decodeErr := common.DecodeString(packageID)
	if decodeErr != nil {
		return newAPIErrorResponse(decodeErr, http.StatusBadRequest, operation, "MalformedPackageId"), nil
	}

	err := s.backend.DeletePackageByID(ctx, decodedPackageID)
	if err != nil {
		if common.IsErrNotFound(err) {
			return newAPIErrorResponse(err, http.StatusNotFound, operation, "PackageNotFound"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "DeletePackage"), err
	}

	return openapi.Response(http.StatusNoContent, nil), nil
}

func toPackageDescription(record persistence.PackageRecord) openapi.PackageDescription {
	aasIDs := make([]string, 0, len(record.AASIDs))
	for _, aasID := range record.AASIDs {
		aasIDs = append(aasIDs, common.EncodeString(aasID))
	}

	return openapi.PackageDescription{
		PackageId:   common.EncodeString(record.PackageID),
		AasIds:      aasIDs,
		FileName:    record.FileName,
		ContentType: record.ContentType,
	}
}

func generatePackageID() string {
	return fmt.Sprintf("pkg-%d", time.Now().UnixNano())
}

func newAPIErrorResponse(err error, status int, operation string, info string) openapi.ImplResponse {
	if err == nil {
		err = errors.New(http.StatusText(status))
	}

	response := common.NewErrorResponse(err, status, componentName, operation, info)
	return openapi.ImplResponse{Code: response.Code, Body: response.Body}
}
