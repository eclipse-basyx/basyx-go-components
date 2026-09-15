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

package migrationintegrationtests

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/require"
)

type inboundReferenceVersion struct {
	SourceID      int64
	OwnerID       sql.NullInt64
	TupleID       string
	TransactionID string
}

func assertInboundReferenceOwnerIndex(t *testing.T) {
	t.Helper()
	db := openMigrationDatabase(t)
	defer func() { _ = db.Close() }()
	query, args, err := goqu.From("pg_indexes").Select("indexdef").Where(goqu.Ex{
		"schemaname": "public", "tablename": "submodel_inbound_reference", "indexname": "ix_submodel_inbound_reference_owner",
	}).ToSQL()
	require.NoError(t, err)
	var definition string
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&definition))
	require.Contains(t, definition, "USING btree (owner_submodel_id)")
}

func assertUnchangedInboundReferencesAreNotRewritten(t *testing.T, db *sql.DB) {
	t.Helper()
	const target = "urn:basyx:migration:submodel:1"
	before := readInboundReferenceVersion(t, db, target)
	require.True(t, before.OwnerID.Valid)
	query, args, err := goqu.From("reference_element").Select("value").Where(goqu.C("id").Eq(before.SourceID)).ToSQL()
	require.NoError(t, err)
	var payload []byte
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&payload))

	updateInboundReferenceSource(t, db, before.SourceID, payload)
	require.Equal(t, before, readInboundReferenceVersion(t, db, target), "identical source update must preserve inventory tuples")

	var reference map[string]any
	require.NoError(t, json.Unmarshal(payload, &reference))
	reference["referredSemanticId"] = map[string]any{
		"type": "ExternalReference", "keys": []any{map[string]any{"type": "GlobalReference", "value": "urn:basyx:migration:reference-label"}},
	}
	changed, err := json.Marshal(reference)
	require.NoError(t, err)
	updateInboundReferenceSource(t, db, before.SourceID, changed)
	require.Equal(t, before, readInboundReferenceVersion(t, db, target), "changed JSON with unchanged targets must preserve inventory tuples")

	assertReferenceOwnerChangesAreNotSkipped(t, db, before, target)
	require.Equal(t, before, readInboundReferenceVersion(t, db, target))
}

func readInboundReferenceVersion(t *testing.T, db *sql.DB, target string) inboundReferenceVersion {
	t.Helper()
	query, args, err := goqu.From("submodel_inbound_reference").
		Select("source_id", "owner_submodel_id", goqu.C("ctid").Cast("text"), goqu.C("xmin").Cast("text")).
		Where(goqu.Ex{"source_table": "reference_element", "target_id": target}).ToSQL()
	require.NoError(t, err)
	var version inboundReferenceVersion
	require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&version.SourceID, &version.OwnerID, &version.TupleID, &version.TransactionID))
	return version
}

func updateInboundReferenceSource(t *testing.T, db *sql.DB, sourceID int64, payload []byte) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").Update("reference_element").
		Set(goqu.Record{"value": goqu.L("?::jsonb", string(payload))}).Where(goqu.C("id").Eq(sourceID)).Prepared(true).ToSQL()
	require.NoError(t, err)
	result, err := db.ExecContext(t.Context(), query, args...)
	require.NoError(t, err)
	count, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func assertReferenceOwnerChangesAreNotSkipped(t *testing.T, db *sql.DB, before inboundReferenceVersion, target string) {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, replacePerformanceTestReference(t.Context(), tx, before.SourceID, nil, target))
	query, args, err := goqu.From("submodel_inbound_reference").Select("owner_submodel_id").
		Where(goqu.Ex{"source_table": "reference_element", "source_id": before.SourceID}).ToSQL()
	require.NoError(t, err)
	var owner sql.NullInt64
	require.NoError(t, tx.QueryRowContext(t.Context(), query, args...).Scan(&owner))
	require.False(t, owner.Valid, "same target with a different owner must update the inventory")
}

func replacePerformanceTestReference(ctx context.Context, tx *sql.Tx, sourceID int64, ownerID any, target string) error {
	query, args, err := goqu.Dialect("postgres").Select(goqu.Func("basyx_replace_submodel_inbound_references",
		"reference_element", sourceID, ownerID, goqu.L("ARRAY[?, ?, ?, NULL]::text[]", target, "", target),
	)).Prepared(true).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, query, args...)
	return err
}
