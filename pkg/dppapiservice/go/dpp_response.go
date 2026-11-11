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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type dppHTTPError struct {
	status  int
	code    string
	message string
}

func newDPPHTTPError(status int, code string, message string) error {
	return &dppHTTPError{status: status, code: code, message: message}
}

func (e *dppHTTPError) Error() string {
	return e.code + " " + e.message
}

func (e *dppHTTPError) HTTPStatus() int {
	return e.status
}

func (e *dppHTTPError) ErrorCode() string {
	return e.code
}

func errorResponse(status int, err error) ImplResponse {
	status = errorStatus(err, status)
	code := errorCode(err)
	return Response(status, Result{Messages: []Message{{
		MessageType:   "Error",
		Text:          err.Error(),
		Code:          code,
		CorrelationId: commonmodel.NewErrorCorrelationID("DPP", status, "Service", code),
		Timestamp:     time.Now().UTC(),
	}}})
}

func errorStatus(err error, fallback int) int {
	var httpError interface{ HTTPStatus() int }
	if errors.As(err, &httpError) && httpError.HTTPStatus() >= 400 && httpError.HTTPStatus() <= 599 {
		return httpError.HTTPStatus()
	}
	return fallback
}

func errorCode(err error) string {
	var codedError interface{ ErrorCode() string }
	if errors.As(err, &codedError) && strings.TrimSpace(codedError.ErrorCode()) != "" {
		return codedError.ErrorCode()
	}
	return firstErrorCode(err.Error())
}

type typedPersistenceErrorStatus struct {
	matches func(error) bool
	status  int
}

var typedPersistenceErrorStatuses = []typedPersistenceErrorStatus{
	{matches: common.IsErrBadRequest, status: http.StatusBadRequest},
	{matches: common.IsErrDenied, status: http.StatusForbidden},
	{matches: common.IsErrNotFound, status: http.StatusNotFound},
	{matches: common.IsErrConflict, status: http.StatusConflict},
	{matches: common.IsErrPayloadTooLarge, status: http.StatusRequestEntityTooLarge},
	{matches: common.IsInternalServerError, status: http.StatusInternalServerError},
	{matches: common.IsErrServiceUnavailable, status: http.StatusServiceUnavailable},
}

func mapPersistenceError(err error, fallbackStatus int) ImplResponse {
	return errorResponse(persistenceErrorStatus(err, fallbackStatus), err)
}

func persistenceErrorStatus(err error, fallbackStatus int) int {
	for _, typed := range typedPersistenceErrorStatuses {
		if typed.matches(err) {
			return typed.status
		}
	}
	return untypedPersistenceErrorStatus(err, fallbackStatus)
}

func untypedPersistenceErrorStatus(err error, fallbackStatus int) int {
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "duplicate") || strings.Contains(text, "already") {
		return http.StatusConflict
	}
	if strings.Contains(text, "not found") || strings.Contains(text, "no rows") {
		return http.StatusNotFound
	}
	return fallbackStatus
}

func firstErrorCode(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "DPP-ERROR-UNKNOWN"
	}
	if strings.HasPrefix(fields[0], "DPP-") {
		return fields[0]
	}
	return "DPP-ERROR-UNKNOWN"
}
