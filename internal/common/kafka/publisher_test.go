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

package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kerr"
)

func TestStructuredRecord(t *testing.T) {
	envelope := []byte(`{"specversion":"1.0","id":"same-id","source":"https://example.com","type":"io.admin-shell.submodel.created.v1","data":{"id":"urn:test"}}`)
	routing, err := Routing("custom.events", "submodel_history:urn:test")
	require.NoError(t, err)
	record, err := structuredRecord(routing, envelope)
	require.NoError(t, err)
	require.Equal(t, "custom.events", record.Topic)
	require.Equal(t, "submodel_history:urn:test", string(record.Key))
	require.Equal(t, envelope, record.Value)
	require.Len(t, record.Headers, 1)
	require.Equal(t, "content-type", record.Headers[0].Key)
	require.Equal(t, "application/cloudevents+json", string(record.Headers[0].Value))
	for _, invalid := range []string{`null`, `"topic"`, `{}`, `{"topic":"a/b","key":"x"}`} {
		_, err = structuredRecord(json.RawMessage(invalid), envelope)
		require.Error(t, err)
	}
}

func TestBrokerErrorCauseOmitsBrokerSuppliedDetails(t *testing.T) {
	for _, cause := range []error{kerr.SaslAuthenticationFailed, kerr.TopicAuthorizationFailed, kerr.MessageTooLarge} {
		err := brokerErrorCause(fmt.Errorf("%w: private-user private-password", cause))
		require.ErrorIs(t, err, cause)
		require.NotContains(t, err.Error(), "private-")
	}
	require.ErrorIs(t, brokerErrorCause(context.DeadlineExceeded), context.DeadlineExceeded)
}
