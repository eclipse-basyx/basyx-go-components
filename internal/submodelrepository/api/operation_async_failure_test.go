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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestStoredOperationFailureResultReturnsOwnFailedOperationResult(t *testing.T) {
	ownMessages := errorResponseMessages(newAPIErrorResponse(errors.New("deadline exceeded"), http.StatusBadGateway, "InvokeOperationAsync", "DelegateOperationCall"))
	stored := newFailedOperationResult(gen.EXECUTIONSTATE_TIMEOUT, ownMessages)

	for name, body := range map[string]any{"typed result": stored, "persisted result": jsonRoundTrip(t, stored)} {
		t.Run(name, func(t *testing.T) {
			result := storedOperationFailureResult(asyncjob.Record{ErrorStatus: http.StatusBadGateway, ErrorBody: body}, "GetOperationAsyncResult")

			require.Equal(t, stored, result)
		})
	}
}

func TestStoredOperationFailureResultReturnsAbandonedWorkerResult(t *testing.T) {
	abandonedMessage := gen.NewErrorMessage(
		"The worker processing the asynchronous request stopped before completion.",
		"500",
		gen.NewErrorCorrelationID("ASYNCJOB", http.StatusInternalServerError, "Worker", "Abandoned"),
	)
	body := jsonRoundTrip(t, map[string]any{"executionState": "Failed", "success": false, "messages": []gen.Message{abandonedMessage}})

	result := storedOperationFailureResult(asyncjob.Record{ErrorStatus: http.StatusInternalServerError, ErrorBody: body}, "GetOperationAsyncResult")

	require.Equal(t, newFailedOperationResult(gen.EXECUTIONSTATE_FAILED, []gen.Message{abandonedMessage}), result)
}

func TestStoredOperationFailureResultMapsLegacyErrorResultsToFailed(t *testing.T) {
	ownResponse := newAPIErrorResponse(errors.New("delegate unreachable"), http.StatusBadGateway, "InvokeOperationAsync", "DelegateOperationCall")
	ownMessages := errorResponseMessages(ownResponse)

	tests := []struct {
		name string
		body any
	}{
		{name: "typed result", body: ownResponse.Body},
		{name: "persisted result", body: jsonRoundTrip(t, ownResponse.Body)},
		{name: "persisted legacy message array", body: jsonRoundTrip(t, ownMessages)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := asyncjob.Record{ErrorStatus: http.StatusBadGateway, ErrorBody: test.body}

			result := storedOperationFailureResult(record, "GetOperationAsyncResult")

			require.Equal(t, newFailedOperationResult(gen.EXECUTIONSTATE_FAILED, ownMessages), result)
		})
	}
}

func TestStoredOperationFailureResultNormalizesLegacyTimestampsToUTC(t *testing.T) {
	record := asyncjob.Record{ErrorStatus: http.StatusBadGateway, ErrorBody: jsonRoundTrip(t, []map[string]string{{
		"code":          "502",
		"correlationId": "SMREPO-502-InvokeOperationAsync-BadGateway-DelegateOperationCall",
		"messageType":   "Error",
		"text":          "delegate unreachable",
		"timestamp":     "2026-10-12T19:55:16+02:00",
	}})}

	result := storedOperationFailureResult(record, "GetOperationAsyncResult")

	require.Equal(t, gen.EXECUTIONSTATE_FAILED, result.ExecutionState)
	require.Equal(t, "2026-10-12T17:55:16Z", result.Messages[0].Timestamp)
}

func TestStoredOperationFailureResultReplacesForeignBodies(t *testing.T) {
	foreignMessage := map[string]any{
		"code": "409", "correlationId": "REMOTE-1", "messageType": "Error", "text": "stack trace", "timestamp": "2026-10-12T17:55:16Z",
	}
	tests := []struct {
		name   string
		status int
		body   any
	}{
		{name: "array of foreign objects", status: http.StatusNotFound, body: []any{map[string]any{"remoteDetail": "stack trace"}}},
		{name: "foreign object", status: http.StatusInternalServerError, body: map[string]any{"message": "stack trace"}},
		{name: "empty body", status: http.StatusBadRequest, body: []any{}},
		{name: "result with foreign correlation", status: http.StatusConflict, body: map[string]any{"messages": []any{foreignMessage}}},
		{name: "operation result with foreign correlation", status: http.StatusConflict, body: map[string]any{
			"executionState": "Failed", "success": false, "messages": []any{foreignMessage},
		}},
		{name: "redirect status", status: http.StatusFound, body: nil},
		{name: "missing status", status: 0, body: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := asyncjob.Record{ErrorStatus: test.status, ErrorBody: test.body}

			result := storedOperationFailureResult(record, "GetOperationAsyncResult")

			requireGenericFailedOperationResult(t, result)
		})
	}
}

func TestStoredOperationFailureResultRejectsNonFailureStates(t *testing.T) {
	ownMessages := errorResponseMessages(newAPIErrorResponse(errors.New("delegate unreachable"), http.StatusBadGateway, "InvokeOperationAsync", "DelegateOperationCall"))
	for _, body := range []failedOperationResult{
		newFailedOperationResult(gen.EXECUTIONSTATE_COMPLETED, ownMessages),
		{ExecutionState: gen.EXECUTIONSTATE_FAILED, Success: true, Messages: ownMessages},
	} {
		result := storedOperationFailureResult(asyncjob.Record{ErrorStatus: http.StatusBadGateway, ErrorBody: jsonRoundTrip(t, body)}, "GetOperationAsyncResult")

		requireGenericFailedOperationResult(t, result)
	}
}

func TestDelegationFailureExecutionState(t *testing.T) {
	expiredCtx, cancelExpired := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancelExpired()
	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name     string
		ctx      context.Context
		err      error
		expected gen.ExecutionState
	}{
		{name: "expired client timeout", ctx: expiredCtx, err: context.DeadlineExceeded, expected: gen.EXECUTIONSTATE_TIMEOUT},
		{name: "http client timeout", ctx: t.Context(), err: timeoutError{}, expected: gen.EXECUTIONSTATE_TIMEOUT},
		{name: "server shutdown", ctx: canceledCtx, err: context.Canceled, expected: gen.EXECUTIONSTATE_FAILED},
		{name: "connection refused", ctx: t.Context(), err: errors.New("connection refused"), expected: gen.EXECUTIONSTATE_FAILED},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, delegationFailureExecutionState(test.ctx, test.err))
		})
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return false }

func requireGenericFailedOperationResult(t *testing.T, result failedOperationResult) {
	t.Helper()
	require.Equal(t, gen.EXECUTIONSTATE_FAILED, result.ExecutionState)
	require.False(t, result.Success)
	require.Len(t, result.Messages, 1)
	require.Contains(t, result.Messages[0].Text, "SMREPO-GETOPASYRES-FAILED")
	require.NotContains(t, result.Messages[0].Text, "stack trace")
}

func jsonRoundTrip(t *testing.T, value any) any {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	return decoded
}
