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
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestStoredOperationFailureResponseReturnsOwnResults(t *testing.T) {
	ownResponse := newAPIErrorResponse(errors.New("delegate unreachable"), http.StatusBadGateway, "InvokeOperationAsync", "DelegateOperationCall")
	ownMessage := ownResponse.Body.(gen.Result).Messages[0]

	tests := []struct {
		name string
		body any
	}{
		{name: "typed result", body: ownResponse.Body},
		{name: "persisted result", body: jsonRoundTrip(t, ownResponse.Body)},
		{name: "persisted legacy message array", body: jsonRoundTrip(t, ownResponse.Body.(gen.Result).Messages)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := asyncjob.Record{ErrorStatus: http.StatusBadGateway, ErrorBody: test.body}

			response := storedOperationFailureResponse(record, "GetOperationAsyncResult")

			require.Equal(t, http.StatusBadGateway, response.Code)
			require.Equal(t, gen.Result{Messages: []gen.Message{ownMessage}}, response.Body)
		})
	}
}

func TestStoredOperationFailureResponseNormalizesLegacyTimestampsToUTC(t *testing.T) {
	record := asyncjob.Record{ErrorStatus: http.StatusBadGateway, ErrorBody: jsonRoundTrip(t, []map[string]string{{
		"code":          "502",
		"correlationId": "SMREPO-502-InvokeOperationAsync-BadGateway-DelegateOperationCall",
		"messageType":   "Error",
		"text":          "delegate unreachable",
		"timestamp":     "2026-10-12T19:55:16+02:00",
	}})}

	response := storedOperationFailureResponse(record, "GetOperationAsyncResult")

	require.Equal(t, http.StatusBadGateway, response.Code)
	require.Equal(t, "2026-10-12T17:55:16Z", response.Body.(gen.Result).Messages[0].Timestamp)
}

func TestStoredOperationFailureResponseReplacesDelegatedEndpointBodies(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   any
	}{
		{name: "array of foreign objects", status: http.StatusNotFound, body: []any{map[string]any{"remoteDetail": "stack trace"}}},
		{name: "foreign object", status: http.StatusInternalServerError, body: map[string]any{"message": "stack trace"}},
		{name: "empty body", status: http.StatusBadRequest, body: []any{}},
		{name: "result with foreign correlation", status: http.StatusConflict, body: map[string]any{"messages": []any{map[string]any{
			"code": "409", "correlationId": "REMOTE-1", "messageType": "Error", "text": "stack trace", "timestamp": "2026-10-12T17:55:16Z",
		}}}},
		{name: "redirect status", status: http.StatusFound, body: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := asyncjob.Record{ErrorStatus: test.status, ErrorBody: test.body}

			response := storedOperationFailureResponse(record, "GetOperationAsyncResult")

			require.Equal(t, http.StatusBadGateway, response.Code)
			messages := response.Body.(gen.Result).Messages
			require.Len(t, messages, 1)
			require.Contains(t, messages[0].Text, "SMREPO-GETOPASYRES-FAILED")
			require.NotContains(t, messages[0].Text, "stack trace")
		})
	}
}

func TestStoredOperationFailureResponseWithoutStatusReturnsInternalServerError(t *testing.T) {
	response := storedOperationFailureResponse(asyncjob.Record{}, "GetOperationAsyncResult")

	require.Equal(t, http.StatusInternalServerError, response.Code)
}

func jsonRoundTrip(t *testing.T, value any) any {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	return decoded
}
