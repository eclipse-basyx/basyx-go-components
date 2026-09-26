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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebac

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
)

const derivationTable = "rebac_derivation"

// recordDerivation stores that object is derived from source, so it inherits
// every permission on source.
func recordDerivation(ctx context.Context, tx *sql.Tx, kind ResourceKind, objectUUID string, source ResourceKind, sourceUUID string) error {
	ds := dialect.Insert(derivationTable).Rows(goqu.Record{
		"object_uuid": goqu.L("?::uuid", objectUUID),
		"object_type": kind.ObjectType,
		"source_uuid": goqu.L("?::uuid", sourceUUID),
		"source_type": source.ObjectType,
	}).OnConflict(goqu.DoNothing()).Prepared(true)
	_, err := execDataset(ctx, tx, "REBAC-RECORDDERIVATION", ds)
	return err
}

// deleteDerivation removes the derivation of a deleted object. Derivations
// of objects derived from it stay until the derived objects are deleted,
// because the same transaction usually deletes them next; without their
// source they grant nothing but repository administration.
func deleteDerivation(ctx context.Context, tx *sql.Tx, objectUUID string) error {
	ds := dialect.Delete(derivationTable).
		Where(goqu.C("object_uuid").Eq(goqu.L("?::uuid", objectUUID))).Prepared(true)
	_, err := execDataset(ctx, tx, "REBAC-DELETEDERIVATION", ds)
	return err
}

// derivationSourceOf returns the source of a derived object.
func derivationSourceOf(ctx context.Context, q Queryer, objectUUID string) (string, string, bool, error) {
	ds := dialect.From(derivationTable).
		Select(goqu.C("source_type"), goqu.L("source_uuid::text")).
		Where(goqu.C("object_uuid").Eq(goqu.L("?::uuid", objectUUID))).Prepared(true)
	var sourceType, sourceUUID string
	found, err := queryRowDataset(ctx, q, "REBAC-DERIVATIONSOURCE", ds, &sourceType, &sourceUUID)
	return sourceType, sourceUUID, found, err
}
