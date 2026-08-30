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

//nolint:all
package main

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

type aasDescriptorPersistenceState struct {
	descriptorID      int64
	aasUpdatedAt      time.Time
	payloadUpdatedAt  time.Time
	endpointID        int64
	endpointUpdatedAt time.Time
}

func TestAASDescriptorPutPreservesUnchangedRows(t *testing.T) {
	aasID := fmt.Sprintf("urn:example:aas:update-persistence:%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupAASDescriptor(t, aasRegistryBaseURL, aasID) })
	identifier := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	endpoint := fmt.Sprintf("%s/shell-descriptors/%s", aasRegistryBaseURL, identifier)
	payload := func(description string) string {
		return fmt.Sprintf(
			`{"id":"%s","description":[{"language":"en","text":"%s"}],"endpoints":[{"interface":"AAS-3.0","protocolInformation":{"href":"https://example.com/aas/%s","endpointProtocol":"https"}}]}`,
			aasID,
			description,
			identifier,
		)
	}

	_, status, _, err := postJSONResponse(aasRegistryBaseURL+"/shell-descriptors", payload("before"))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	db, err := sql.Open("pgx", aasRegistryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	before := readAASDescriptorPersistenceState(t, db, aasID)

	time.Sleep(20 * time.Millisecond)
	_, status, _, err = putJSONResponse(endpoint, payload("after"))
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)
	afterChange := readAASDescriptorPersistenceState(t, db, aasID)
	require.Equal(t, before.descriptorID, afterChange.descriptorID)
	require.Equal(t, before.endpointID, afterChange.endpointID)
	require.Equal(t, before.endpointUpdatedAt, afterChange.endpointUpdatedAt)
	require.True(t, afterChange.aasUpdatedAt.After(before.aasUpdatedAt))
	require.True(t, afterChange.payloadUpdatedAt.After(before.payloadUpdatedAt))

	time.Sleep(20 * time.Millisecond)
	_, status, _, err = putJSONResponse(endpoint, payload("after"))
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)
	afterNoOp := readAASDescriptorPersistenceState(t, db, aasID)
	require.Equal(t, afterChange, afterNoOp)
}

func readAASDescriptorPersistenceState(t *testing.T, db *sql.DB, aasID string) aasDescriptorPersistenceState {
	t.Helper()
	aas := goqu.T(common.TblAASDescriptor).As("aas")
	payload := goqu.T(common.TblDescriptorPayload).As("payload")
	endpoint := goqu.T(common.TblAASDescriptorEndpoint).As("endpoint")
	query, args, err := goqu.Dialect(common.Dialect).
		From(aas).
		InnerJoin(payload, goqu.On(payload.Col(common.ColDescriptorID).Eq(aas.Col(common.ColDescriptorID)))).
		InnerJoin(endpoint, goqu.On(endpoint.Col(common.ColDescriptorID).Eq(aas.Col(common.ColDescriptorID)))).
		Select(
			aas.Col(common.ColDescriptorID),
			aas.Col("db_updated_at"),
			payload.Col("db_updated_at"),
			endpoint.Col(common.ColID),
			endpoint.Col("db_updated_at"),
		).
		Where(
			aas.Col(common.ColAASID).Eq(aasID),
			endpoint.Col(common.ColPosition).Eq(0),
		).
		Prepared(true).
		ToSQL()
	require.NoError(t, err)
	var state aasDescriptorPersistenceState
	err = db.QueryRowContext(t.Context(), query, args...).Scan(
		&state.descriptorID,
		&state.aasUpdatedAt,
		&state.payloadUpdatedAt,
		&state.endpointID,
		&state.endpointUpdatedAt,
	)
	require.NoError(t, err)
	return state
}
