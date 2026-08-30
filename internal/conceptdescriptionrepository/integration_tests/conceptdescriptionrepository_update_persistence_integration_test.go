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
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

type conceptDescriptionPersistenceState struct {
	dbCreatedAt             time.Time
	dbUpdatedAt             time.Time
	administrationCreatedAt sql.NullTime
	administrationUpdatedAt sql.NullTime
	idShort                 sql.NullString
	data                    map[string]any
}

func TestConceptDescriptionPutUpdatesExistingRowInPlace(t *testing.T) {
	conceptDescriptionID := fmt.Sprintf("urn:example:cd:update-persistence:%d", time.Now().UnixNano())
	endpoint := conceptDescriptionRepositoryBaseURL + "/concept-descriptions"
	t.Cleanup(func() { cleanupConceptDescription(t, endpoint, conceptDescriptionID) })
	encodedIdentifier := base64.RawURLEncoding.EncodeToString([]byte(conceptDescriptionID))

	status, body, err := requestJSON(
		http.MethodPost,
		endpoint,
		conceptDescriptionUpdatePayload(conceptDescriptionID, "BeforeUpdate", "2030-01-02T03:04:06Z", true),
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))

	db, err := sql.Open("pgx", conceptDescriptionRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	before := readConceptDescriptionPersistenceState(t, db, conceptDescriptionID)

	time.Sleep(20 * time.Millisecond)
	status, body, err = requestJSON(
		http.MethodPut,
		endpoint+"/"+encodedIdentifier,
		conceptDescriptionUpdatePayload(conceptDescriptionID, "AfterUpdate", "2030-01-02T03:04:07Z", false),
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))

	after := readConceptDescriptionPersistenceState(t, db, conceptDescriptionID)
	require.Equal(t, before.dbCreatedAt, after.dbCreatedAt)
	require.True(t, after.dbUpdatedAt.After(before.dbUpdatedAt))
	require.Equal(t, "AfterUpdate", after.idShort.String)
	require.True(t, after.administrationCreatedAt.Valid)
	require.Equal(t, before.administrationCreatedAt, after.administrationCreatedAt)
	require.True(t, after.administrationUpdatedAt.Valid)
	require.True(t, after.administrationUpdatedAt.Time.After(before.administrationUpdatedAt.Time))
	require.NotContains(t, after.data, "description")
	require.Equal(t, "AfterUpdate", after.data["idShort"])
}

func conceptDescriptionUpdatePayload(
	id string,
	idShort string,
	updatedAt string,
	includeDescription bool,
) map[string]any {
	payload := map[string]any{
		"id":        id,
		"idShort":   idShort,
		"modelType": "ConceptDescription",
		"administration": map[string]any{
			"createdAt": "2030-01-02T03:04:05Z",
			"updatedAt": updatedAt,
		},
	}
	if includeDescription {
		payload["description"] = []any{
			map[string]any{"language": "en", "text": "removed by full PUT"},
		}
	}
	return payload
}

func readConceptDescriptionPersistenceState(
	t *testing.T,
	db *sql.DB,
	id string,
) conceptDescriptionPersistenceState {
	t.Helper()

	query, args, err := goqu.Dialect("postgres").
		From("concept_description").
		Select(
			"db_created_at",
			"db_updated_at",
			"administration_created_at",
			"administration_updated_at",
			"id_short",
			"data",
		).
		Where(goqu.C("id").Eq(id)).
		Prepared(true).
		ToSQL()
	require.NoError(t, err)

	var state conceptDescriptionPersistenceState
	var data []byte
	err = db.QueryRowContext(t.Context(), query, args...).Scan(
		&state.dbCreatedAt,
		&state.dbUpdatedAt,
		&state.administrationCreatedAt,
		&state.administrationUpdatedAt,
		&state.idShort,
		&data,
	)
	require.NoError(t, err, "query=%s args=%v", query, args)
	require.NoError(t, json.Unmarshal(data, &state.data))
	return state
}
