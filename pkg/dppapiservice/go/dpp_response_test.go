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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func TestMapPersistenceErrorPreservesWrappedDPPHTTPError(t *testing.T) {
	err := fmt.Errorf(
		"DPP-CONTENT-FULL convert submodel: %w",
		newDPPHTTPError(
			http.StatusInternalServerError,
			"DPP-FILEURL-MISSINGEXTERNALURL",
			"managed attachment requires a valid general.externalUrl",
		),
	)

	response := mapPersistenceError(err, http.StatusNotFound)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("mapPersistenceError() status = %d, want %d", response.Code, http.StatusInternalServerError)
	}

	result, ok := response.Body.(Result)
	if !ok {
		t.Fatalf("mapPersistenceError() body type = %T, want Result", response.Body)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("mapPersistenceError() messages = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Code != "DPP-FILEURL-MISSINGEXTERNALURL" {
		t.Fatalf(
			"mapPersistenceError() code = %q, want %q",
			result.Messages[0].Code,
			"DPP-FILEURL-MISSINGEXTERNALURL",
		)
	}
	wantCorrelationID := "DPP-500-Service-InternalServerError-DPP-FILEURL-MISSINGEXTERNALURL"
	if result.Messages[0].CorrelationId != wantCorrelationID {
		t.Fatalf("mapPersistenceError() correlationId = %q, want %q", result.Messages[0].CorrelationId, wantCorrelationID)
	}
}

func TestMapPersistenceErrorUsesTypedRepositoryErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "bad request", err: common.NewErrBadRequest("SMREPO-X-INVALIDIDSHORT idShort is invalid"), want: http.StatusBadRequest},
		{name: "bad request mentioning not found", err: common.NewErrBadRequest("SMREPO-X-REF referenced element not found"), want: http.StatusBadRequest},
		{name: "denied", err: common.NewErrDenied("AASREPO-NEWAAS-CHKDUP-ABACDENIED existing AAS is not accessible"), want: http.StatusForbidden},
		{name: "not found", err: common.NewErrNotFound("AASREPO-LOCKAAS-AASNOTFOUND AAS not found"), want: http.StatusNotFound},
		{name: "conflict", err: common.NewErrConflict("AASREPO-NEWAAS-CONFLICT AAS with given id already exists"), want: http.StatusConflict},
		{name: "internal", err: common.NewInternalServerError("AASREPO-GETDPPASSETIDS-EXECSQL connection reset"), want: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := fmt.Errorf("DPP-CREATEDPP-CREATEAAS create AAS: %w", test.err)
			response := mapPersistenceError(err, http.StatusConflict)
			if response.Code != test.want {
				t.Fatalf("mapPersistenceError() status = %d, want %d", response.Code, test.want)
			}
			requireSingleMessage(t, response, err.Error(), "DPP-CREATEDPP-CREATEAAS")
		})
	}
}

func TestMapPersistenceErrorClassifiesUntypedErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		fallback int
		want     int
	}{
		{name: "no rows", err: errors.New("DPP-X-GET sql: no rows in result set"), fallback: http.StatusInternalServerError, want: http.StatusNotFound},
		{name: "not found", err: errors.New("DPP-X-GET element not found"), fallback: http.StatusBadRequest, want: http.StatusNotFound},
		{name: "duplicate", err: errors.New("DPP-X-PUT duplicate key value violates unique constraint"), fallback: http.StatusInternalServerError, want: http.StatusConflict},
		{name: "already wins over not found", err: errors.New("DPP-X-PUT not found or already exists"), fallback: http.StatusInternalServerError, want: http.StatusConflict},
		{name: "fallback", err: errors.New("DPP-COMPOSE-FULLTYPE full content is not a DataElement object"), fallback: http.StatusInternalServerError, want: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := mapPersistenceError(test.err, test.fallback)
			if response.Code != test.want {
				t.Fatalf("mapPersistenceError() status = %d, want %d", response.Code, test.want)
			}
			requireSingleMessage(t, response, test.err.Error(), firstErrorCode(test.err.Error()))
		})
	}
}

func requireSingleMessage(t *testing.T, response ImplResponse, wantText string, wantCode string) {
	t.Helper()
	result, ok := response.Body.(Result)
	if !ok {
		t.Fatalf("mapPersistenceError() body type = %T, want Result", response.Body)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("mapPersistenceError() messages = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Text != wantText {
		t.Fatalf("mapPersistenceError() text = %q, want %q", result.Messages[0].Text, wantText)
	}
	if result.Messages[0].Code != wantCode {
		t.Fatalf("mapPersistenceError() code = %q, want %q", result.Messages[0].Code, wantCode)
	}
}
