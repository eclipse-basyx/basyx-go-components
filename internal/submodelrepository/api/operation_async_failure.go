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
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const asyncJobCorrelationPrefix = "ASYNCJOB-"

// failedOperationResult is the OperationResult of an asynchronous invocation
// that failed or timed out during processing.
type failedOperationResult struct {
	ExecutionState gen.ExecutionState `json:"executionState"`
	Success        bool               `json:"success"`
	Messages       []gen.Message      `json:"messages"`
}

func newFailedOperationResult(executionState gen.ExecutionState, messages []gen.Message) failedOperationResult {
	return failedOperationResult{ExecutionState: executionState, Success: false, Messages: messages}
}

// delegationFailureExecutionState reports Timeout when the delegated call
// exceeded the effective client timeout and Failed for every other error.
func delegationFailureExecutionState(delegationCtx context.Context, delegateErr error) gen.ExecutionState {
	if errors.Is(delegationCtx.Err(), context.DeadlineExceeded) || errors.Is(delegateErr, context.DeadlineExceeded) {
		return gen.EXECUTIONSTATE_TIMEOUT
	}
	var netErr net.Error
	if errors.As(delegateErr, &netErr) && netErr.Timeout() {
		return gen.EXECUTIONSTATE_TIMEOUT
	}
	return gen.EXECUTIONSTATE_FAILED
}

func errorResponseMessages(response gen.ImplResponse) []gen.Message {
	if result, ok := response.Body.(gen.Result); ok {
		return result.Messages
	}
	return nil
}

// storedOperationFailureResult returns the OperationResult of a failed
// asynchronous operation. Only messages created by this service are returned;
// earlier versions stored error Results or delegated endpoint bodies, which are
// mapped to Failed, with foreign content replaced by a generic message.
func storedOperationFailureResult(record asyncjob.Record, operation string) failedOperationResult {
	if result, ok := ownFailedOperationResult(record.ErrorBody); ok {
		return result
	}
	if result, ok := ownErrorResult(record.ErrorBody); ok {
		return newFailedOperationResult(gen.EXECUTIONSTATE_FAILED, result.Messages)
	}
	failedErr := errors.New("SMREPO-GETOPASYRES-FAILED delegated operation failed")
	failedResponse := newAPIErrorResponse(failedErr, http.StatusBadGateway, operation, "DelegatedOperationFailed")
	return newFailedOperationResult(gen.EXECUTIONSTATE_FAILED, errorResponseMessages(failedResponse))
}

func ownFailedOperationResult(body any) (failedOperationResult, bool) {
	var result failedOperationResult
	if !decodeStoredBody(body, &result) || result.Success || !isFailureExecutionState(result.ExecutionState) {
		return failedOperationResult{}, false
	}
	if !normalizeOwnErrorMessages(result.Messages) {
		return failedOperationResult{}, false
	}
	return result, true
}

func isFailureExecutionState(executionState gen.ExecutionState) bool {
	return executionState == gen.EXECUTIONSTATE_FAILED || executionState == gen.EXECUTIONSTATE_TIMEOUT
}

// ownErrorResult accepts a stored error body as a Result object or, as written
// by earlier versions, a bare message array, and normalizes its timestamps to UTC.
func ownErrorResult(body any) (gen.Result, bool) {
	var result gen.Result
	if !decodeStoredBody(body, &result) && !decodeStoredBody(body, &result.Messages) {
		return gen.Result{}, false
	}
	if !normalizeOwnErrorMessages(result.Messages) {
		return gen.Result{}, false
	}
	return result, true
}

func decodeStoredBody(body any, target any) bool {
	encoded, err := json.Marshal(body)
	if err != nil {
		return false
	}
	return decodeStrictJSON(encoded, target) == nil
}

func normalizeOwnErrorMessages(messages []gen.Message) bool {
	if len(messages) == 0 {
		return false
	}
	for index := range messages {
		if !normalizeOwnErrorMessage(&messages[index]) {
			return false
		}
	}
	return true
}

func normalizeOwnErrorMessage(message *gen.Message) bool {
	if message.MessageType != "Error" || !isOwnCorrelationID(message.CorrelationID) {
		return false
	}
	timestamp, err := time.Parse(time.RFC3339, message.Timestamp)
	if err != nil {
		return false
	}
	message.Timestamp = timestamp.UTC().Format(time.RFC3339)
	return true
}

func isOwnCorrelationID(correlationID string) bool {
	return strings.HasPrefix(correlationID, componentName+"-") || strings.HasPrefix(correlationID, asyncJobCorrelationPrefix)
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
