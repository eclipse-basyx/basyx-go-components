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
// Author: Martin Stemmer ( Fraunhofer IESE )

package aasenvironment

import (
	"mime"
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

const (
	serializationUploadComponent = "AASENV"

	mediaTypeJSON = "application/json"
	mediaTypeXML  = "application/xml"

	mediaTypeAASXJSON            = "application/aasx+json"
	mediaTypeAASXXML             = "application/aasx+xml"
	mediaTypeAASJSONAlias        = "application/asset-administration-shell+json"
	mediaTypeAASXMLAlias         = "application/asset-administration-shell+xml"
	mediaTypeAASXLegacyXMLBundle = "application/asset-administration-shell-package+xml"
)

type serializationKind string

const (
	serializationKindJSON serializationKind = "json"
	serializationKindXML  serializationKind = "xml"
)

// SerializationUploadService hosts custom AAS Environment upload/serialization endpoints.
type SerializationUploadService struct {
	aasRepository                *CustomAASRepositoryService
	submodelRepository           *CustomSubmodelRepositoryService
	conceptDescriptionRepository *CustomConceptDescriptionRepositoryService
}

// NewSerializationUploadService constructs upload/serialization endpoint handlers.
func NewSerializationUploadService(
	aasRepository *CustomAASRepositoryService,
	submodelRepository *CustomSubmodelRepositoryService,
	conceptDescriptionRepository *CustomConceptDescriptionRepositoryService,
) *SerializationUploadService {
	return &SerializationUploadService{
		aasRepository:                aasRepository,
		submodelRepository:           submodelRepository,
		conceptDescriptionRepository: conceptDescriptionRepository,
	}
}

// RegisterRoutes attaches /serialization and /upload endpoints.
func (s *SerializationUploadService) RegisterRoutes(router chi.Router) {
	router.Get("/serialization", s.HandleSerialization)
	router.Post("/upload", s.HandleUpload)
}

func normalizeMediaType(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(trimmed)
	if err != nil {
		mediaType = strings.Split(trimmed, ";")[0]
	}

	return strings.ToLower(strings.TrimSpace(mediaType))
}

func statusFromError(err error) int {
	switch {
	case err == nil:
		return http.StatusInternalServerError
	case common.IsErrBadRequest(err):
		return http.StatusBadRequest
	case common.IsErrDenied(err):
		return http.StatusForbidden
	case common.IsErrNotFound(err):
		return http.StatusNotFound
	case common.IsErrConflict(err):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func (s *SerializationUploadService) writeErrorResponse(
	w http.ResponseWriter,
	statusCode int,
	operation string,
	info string,
	err error,
) {
	response := common.NewErrorResponse(err, statusCode, serializationUploadComponent, operation, info)
	_ = model.EncodeJSONResponse(response.Body, &response.Code, w)
}
