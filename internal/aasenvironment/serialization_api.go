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

package aasenvironment

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

// RegisterSerializationAPI registers GET /serialization on the supplied router.
//
// Parameters:
//   - r: Router receiving the serialization route.
//   - service: Business service used to build serialized environment responses.
func RegisterSerializationAPI(r chi.Router, service SerializationService) {
	api := &serializationAPI{service: service}
	r.Get("/serialization", api.GenerateSerializationByIDs)
}

// SerializationService defines serialization business logic without HTTP dependencies.
type SerializationService interface {
	GenerateSerializationByIds(ctx context.Context, aasIDs []string, submodelIDs []string, includeConceptDescriptions bool) (commonmodel.ImplResponse, error)
}

type serializationAPI struct {
	service SerializationService
}

func (a *serializationAPI) GenerateSerializationByIDs(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.service == nil {
		writeSerializationError(w, http.StatusInternalServerError, errors.New("serialization service is required"), "AASENV-SERIALIZATIONAPI-NILSERVICE")
		return
	}

	query := r.URL.Query()
	aasIDsParam := parseSerializationStringArrayQueryParam(query["aasIds"])
	submodelIDsParam := parseSerializationStringArrayQueryParam(query["submodelIds"])

	includeConceptDescriptionsParam := true
	if query.Has("includeConceptDescriptions") {
		parsedIncludeConceptDescriptions, parseErr := parseSerializationBoolQueryParam(query.Get("includeConceptDescriptions"))
		if parseErr != nil {
			writeSerializationError(w, http.StatusBadRequest, parseErr, "AASENV-SERIALIZATIONAPI-PARSEINCLUDECDS")
			return
		}
		includeConceptDescriptionsParam = parsedIncludeConceptDescriptions
	}

	requestContext := common.WithAcceptHeader(r.Context(), r.Header.Get("Accept"))
	result, err := a.service.GenerateSerializationByIds(requestContext, aasIDsParam, submodelIDsParam, includeConceptDescriptionsParam)
	if err != nil {
		writeSerializationError(w, http.StatusInternalServerError, err, "AASENV-SERIALIZATIONAPI-HANDLER")
		return
	}

	switch fileDownload := result.Body.(type) {
	case SerializationFileDownload:
		writeSerializationFileDownload(w, result.Code, fileDownload)
		return
	case *SerializationFileDownload:
		if fileDownload != nil {
			writeSerializationFileDownload(w, result.Code, *fileDownload)
			return
		}
	}

	if encodeErr := commonmodel.EncodeJSONResponse(result.Body, &result.Code, w); encodeErr != nil {
		writeSerializationError(w, http.StatusInternalServerError, encodeErr, "AASENV-SERIALIZATIONAPI-ENCODERESPONSE")
	}
}

func writeSerializationFileDownload(w http.ResponseWriter, status int, payload SerializationFileDownload) {
	if payload.Close != nil {
		defer func() {
			if err := payload.Close(); err != nil {
				log.Printf("AASENV-SERIALIZATIONAPI-CLOSE response cleanup failed: %v", err)
			}
		}()
	}
	commonmodel.SetSafeDownloadHeaders(w.Header(), payload.Filename, payload.ContentType)
	if status <= 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	if payload.WriteTo != nil {
		if err := payload.WriteTo(w); err != nil {
			log.Printf("AASENV-SERIALIZATIONAPI-STREAM response stream failed: %v", err)
		}
		return
	}
	// #nosec G705 -- writing binary attachment payload with Content-Disposition attachment and nosniff header
	_, _ = w.Write(payload.Content)
}

func writeSerializationError(w http.ResponseWriter, status int, err error, info string) {
	resp := common.NewErrorResponse(err, status, "AASENV", "SerializationAPI", info)
	if encodeErr := commonmodel.EncodeJSONResponse(resp.Body, &resp.Code, w); encodeErr != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func parseSerializationBoolQueryParam(value string) (bool, error) {
	if value == "" {
		return false, nil
	}

	return strconv.ParseBool(value)
}

func parseSerializationStringArrayQueryParam(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}
