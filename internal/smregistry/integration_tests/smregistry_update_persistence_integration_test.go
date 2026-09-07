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
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

type submodelDescriptorPersistenceState struct {
	descriptorID      int64
	rootUpdatedAt     time.Time
	payloadUpdatedAt  time.Time
	endpointID        int64
	endpointUpdatedAt time.Time
}

func TestSubmodelDescriptorPutPreservesUnchangedRows(t *testing.T) {
	submodelID := fmt.Sprintf("urn:example:submodel:update-persistence:%d", time.Now().UnixNano())
	t.Cleanup(func() { cleanupSubmodelDescriptorHTTP(t, submodelID) })
	identifier := base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	endpoint := fmt.Sprintf("%s/submodel-descriptors/%s", smRegistryBaseURL, identifier)
	payload := func(description string) map[string]any {
		return map[string]any{
			"id": submodelID,
			"description": []any{
				map[string]any{"language": "en", "text": description},
			},
			"endpoints": []any{
				map[string]any{
					"interface": "SUBMODEL-3.0",
					"protocolInformation": map[string]any{
						"href":             "https://example.com/submodels/" + identifier,
						"endpointProtocol": "https",
					},
				},
			},
		}
	}

	status, body, _ := doRequest(t, smNoRedirectClient, http.MethodPost, smRegistryBaseURL+"/submodel-descriptors", payload("before"))
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))

	dsn := testenv.PostgresURLFromEnv("BASYX_IT_DB_PORT", 6432, "basyxTestDB")
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	before := readSubmodelDescriptorPersistenceState(t, db, submodelID)

	invalidPayload := payload("invalid")
	invalidPayload["endpoints"] = []any{}
	status, body, _ = doRequest(t, smNoRedirectClient, http.MethodPut, endpoint, invalidPayload)
	require.Equal(t, http.StatusBadRequest, status, "response=%s", string(body))
	require.Equal(t, before, readSubmodelDescriptorPersistenceState(t, db, submodelID))

	time.Sleep(20 * time.Millisecond)
	status, body, _ = doRequest(t, smNoRedirectClient, http.MethodPut, endpoint, payload("after"))
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))
	afterChange := readSubmodelDescriptorPersistenceState(t, db, submodelID)
	require.Equal(t, before.descriptorID, afterChange.descriptorID)
	require.Equal(t, before.endpointID, afterChange.endpointID)
	require.Equal(t, before.endpointUpdatedAt, afterChange.endpointUpdatedAt)
	require.True(t, afterChange.rootUpdatedAt.After(before.rootUpdatedAt))
	require.True(t, afterChange.payloadUpdatedAt.After(before.payloadUpdatedAt))

	time.Sleep(20 * time.Millisecond)
	status, body, _ = doRequest(t, smNoRedirectClient, http.MethodPut, endpoint, payload("after"))
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))
	afterNoOp := readSubmodelDescriptorPersistenceState(t, db, submodelID)
	require.Equal(t, afterChange, afterNoOp)
}

func readSubmodelDescriptorPersistenceState(t *testing.T, db *sql.DB, submodelID string) submodelDescriptorPersistenceState {
	t.Helper()
	submodel := goqu.T(common.TblSubmodelDescriptor).As("submodel")
	payload := goqu.T(common.TblDescriptorPayload).As("payload")
	endpoint := goqu.T(common.TblAASDescriptorEndpoint).As("endpoint")
	query, args, err := goqu.Dialect(common.Dialect).
		From(submodel).
		InnerJoin(payload, goqu.On(payload.Col(common.ColDescriptorID).Eq(submodel.Col(common.ColDescriptorID)))).
		InnerJoin(endpoint, goqu.On(endpoint.Col(common.ColDescriptorID).Eq(submodel.Col(common.ColDescriptorID)))).
		Select(
			submodel.Col(common.ColDescriptorID),
			submodel.Col("db_updated_at"),
			payload.Col("db_updated_at"),
			endpoint.Col(common.ColID),
			endpoint.Col("db_updated_at"),
		).
		Where(
			submodel.Col(common.ColAASID).Eq(submodelID),
			submodel.Col(common.ColAASDescriptorID).IsNull(),
			endpoint.Col(common.ColPosition).Eq(0),
		).
		Prepared(true).
		ToSQL()
	require.NoError(t, err)

	var state submodelDescriptorPersistenceState
	err = db.QueryRowContext(t.Context(), query, args...).Scan(
		&state.descriptorID,
		&state.rootUpdatedAt,
		&state.payloadUpdatedAt,
		&state.endpointID,
		&state.endpointUpdatedAt,
	)
	require.NoError(t, err)
	return state
}
