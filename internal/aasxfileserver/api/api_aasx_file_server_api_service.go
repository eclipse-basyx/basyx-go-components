/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

// Package api implements HTTP service behavior for the AASX file server.
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

// NewAASXFileServerAPIAPIService constructs an AASX file server service.
//
// Parameters:
//   - backend: PostgreSQL persistence backend used by all service operations.
//
// Returns:
//   - *AASXFileServerAPIAPIService: Configured service instance.
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
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "ListPackages"), nil
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

// PostAASXPackage stores a staged package under a server-generated identifier.
//
// Parameters:
//   - ctx: Request context containing cancellation and configured AASX limits.
//   - file: Seekable staged package owned by the HTTP request.
//   - aasIDs: AAS identifiers associated with the package.
//   - fileName: Preferred download filename.
//
// Returns:
//   - openapi.ImplResponse: Created package description or mapped API error.
//   - error: Reserved for failures not represented by an API response.
func (s *AASXFileServerAPIAPIService) PostAASXPackage(ctx context.Context, file common.StagedUpload, aasIDs []string, fileName string) (openapi.ImplResponse, error) {
	const operation = "PostAASXPackage"

	if file == nil {
		return newAPIErrorResponse(errors.New("multipart form field 'file' is required"), http.StatusBadRequest, operation, "MissingFile"), nil
	}

	rawPackageID := generatePackageID()
	record, err := s.backend.CreatePackage(ctx, rawPackageID, file, aasIDs, fileName)
	if err != nil {
		if common.IsErrPayloadTooLarge(err) {
			return newAPIErrorResponse(err, http.StatusRequestEntityTooLarge, operation, "PayloadTooLarge"), nil
		}
		if common.IsErrConflict(err) {
			return newAPIErrorResponse(err, http.StatusConflict, operation, "PackageIdConflict"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "CreatePackage"), nil
	}

	return openapi.Response(http.StatusCreated, toPackageDescription(*record)), nil
}

// GetAASXByPackageId returns a streamed package and its download metadata.
//
// Parameters:
//   - ctx: Request context used for lookup, streaming, and cancellation.
//   - packageID: Base64URL-encoded package identifier from the request path.
//
// Returns:
//   - openapi.ImplResponse: FileDownload owning the response stream, or a mapped API error.
//   - error: Reserved for failures not represented by an API response.
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
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "GetPackageByID"), nil
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

// PutAASXByPackageId creates or replaces a staged package.
//
// Parameters:
//   - ctx: Request context containing cancellation and configured AASX limits.
//   - packageID: Base64URL-encoded package identifier from the request path.
//   - file: Seekable staged replacement package owned by the HTTP request.
//   - aasIDs: Replacement AAS identifiers associated with the package.
//   - fileName: Preferred replacement download filename.
//
// Returns:
//   - openapi.ImplResponse: HTTP 204 for replacement, HTTP 201 for creation, or a mapped API error.
//   - error: Reserved for failures not represented by an API response.
func (s *AASXFileServerAPIAPIService) PutAASXByPackageId(ctx context.Context, packageID string, file common.StagedUpload, aasIDs []string, fileName string) (openapi.ImplResponse, error) {
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
		if common.IsErrPayloadTooLarge(err) {
			return newAPIErrorResponse(err, http.StatusRequestEntityTooLarge, operation, "PayloadTooLarge"), nil
		}
		if common.IsErrConflict(err) {
			return newAPIErrorResponse(err, http.StatusConflict, operation, "PackageIdConflict"), nil
		}
		if common.IsErrBadRequest(err) {
			return newAPIErrorResponse(err, http.StatusBadRequest, operation, "BadRequest"), nil
		}
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "PutPackage"), nil
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
		return newAPIErrorResponse(err, http.StatusInternalServerError, operation, "DeletePackage"), nil
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
