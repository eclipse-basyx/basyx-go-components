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

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// storedOperationFailureResponse returns the response for a failed asynchronous
// operation. Only an error Result created by this service is returned as
// stored; earlier versions also stored delegated endpoint bodies unchanged,
// which are replaced by a generic 502 so that they never reach clients.
func storedOperationFailureResponse(record asyncjob.Record, operation string) gen.ImplResponse {
	failedErr := errors.New("SMREPO-GETOPASYRES-FAILED delegated operation failed")
	if record.ErrorStatus <= 0 {
		return newAPIErrorResponse(failedErr, http.StatusInternalServerError, operation, "DelegatedOperationFailed")
	}
	if result, ok := ownErrorResult(record.ErrorBody); ok && record.ErrorStatus >= http.StatusBadRequest {
		return gen.Response(record.ErrorStatus, result)
	}
	return newAPIErrorResponse(failedErr, http.StatusBadGateway, operation, "DelegatedOperationFailed")
}

// ownErrorResult accepts a stored error body as a Result object or, as written
// by earlier versions, a bare message array, and normalizes its timestamps to UTC.
func ownErrorResult(body any) (gen.Result, bool) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return gen.Result{}, false
	}

	var result gen.Result
	if decodeStrictJSON(encoded, &result) != nil && decodeStrictJSON(encoded, &result.Messages) != nil {
		return gen.Result{}, false
	}
	if len(result.Messages) == 0 {
		return gen.Result{}, false
	}
	for index := range result.Messages {
		if !normalizeOwnErrorMessage(&result.Messages[index]) {
			return gen.Result{}, false
		}
	}
	return result, true
}

func normalizeOwnErrorMessage(message *gen.Message) bool {
	if message.MessageType != "Error" || !strings.HasPrefix(message.CorrelationID, componentName+"-") {
		return false
	}
	timestamp, err := time.Parse(time.RFC3339, message.Timestamp)
	if err != nil {
		return false
	}
	message.Timestamp = timestamp.UTC().Format(time.RFC3339)
	return true
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
