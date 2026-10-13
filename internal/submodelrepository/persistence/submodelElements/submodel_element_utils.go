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

package submodelelements

import (
	"database/sql"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	jsoniter "github.com/json-iterator/go"
)

// SubmodelElementToProcess represents a SubmodelElement along with its ID short path.
type SubmodelElementToProcess struct {
	Element     types.ISubmodelElement
	IdShortPath string
}

// anyFieldsToUpdate checks if there are any fields to update in a goqu.Record
func anyFieldsToUpdate(updateRecord goqu.Record) bool {
	return len(updateRecord) > 0
}

// CreateContextReferenceByOwnerID upserts a context reference for a given owner ID.
//
// Description:
// This function writes a complete reference triplet (reference base row, payload row,
// and key rows) for a specific owner into the dynamic reference tables derived from
// tableBaseName. Existing payload and keys for the owner are replaced atomically inside
// the provided transaction.
//
// Parameters:
//   - tx: Active SQL transaction used for all operations. Must not be nil.
//   - ownerID: The owner identifier used as primary key in <tableBaseName>_reference.
//   - tableBaseName: Base name used to resolve target tables:
//   - <tableBaseName>_reference
//   - <tableBaseName>_reference_payload
//   - <tableBaseName>_reference_key
//   - reference: Reference object to persist. If nil, no row is written and an invalid
//     sql.NullInt64 is returned without error.
//
// Returns:
//   - sql.NullInt64: Reference ID (equal to ownerID) when a reference is persisted.
//   - error: Internal server error if query build or execution fails.
//
// Usage:
//
//	refID, err := CreateContextReferenceByOwnerID(tx, int64(submodelElementID), "submodel_element_semantic_id", element.SemanticID())
//	if err != nil {
//		return err
//	}
//	if !refID.Valid {
//		// semantic reference was nil
//	}
func CreateContextReferenceByOwnerID(tx *sql.Tx, ownerID int64, tableBaseName string, reference types.IReference) (sql.NullInt64, error) {
	if tx == nil {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-NILTX Transaction is nil")
	}
	if reference == nil {
		return sql.NullInt64{Valid: false}, nil
	}

	parentReferencePayload, err := getReferenceAsJSON(reference)
	if err != nil {
		return sql.NullInt64{Valid: false}, err
	}
	if !parentReferencePayload.Valid {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-BUILDJSON Invalid reference payload")
	}

	keys := reference.Keys()
	positions := make([]int, 0, len(keys))
	keyTypes := make([]int, 0, len(keys))
	keyValues := make([]string, 0, len(keys))
	for i, key := range keys {
		positions = append(positions, i)
		keyTypes = append(keyTypes, int(key.Type()))
		keyValues = append(keyValues, key.Value())
	}

	referenceTable := fmt.Sprintf("%s_reference", tableBaseName)
	referenceKeyTable := fmt.Sprintf("%s_reference_key", tableBaseName)
	referencePayloadTable := fmt.Sprintf("%s_reference_payload", tableBaseName)

	dialect := goqu.Dialect("postgres")

	upsertReferenceQuery, upsertReferenceArgs, err := dialect.
		Insert(referenceTable).
		Rows(goqu.Record{
			"id":   ownerID,
			"type": int(reference.Type()),
		}).
		OnConflict(
			goqu.DoUpdate("id", goqu.Record{
				"type": goqu.L("EXCLUDED.type"),
			}),
		).
		ToSQL()
	if err != nil {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-BUILDUPSERTREF " + err.Error())
	}
	if _, execErr := tx.Exec(upsertReferenceQuery, upsertReferenceArgs...); execErr != nil {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-EXECUPSERTREF " + execErr.Error())
	}

	deletePayloadQuery, deletePayloadArgs, err := dialect.
		Delete(referencePayloadTable).
		Where(goqu.C("reference_id").Eq(ownerID)).
		ToSQL()
	if err != nil {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-BUILDDELPAYLOAD " + err.Error())
	}
	if _, execErr := tx.Exec(deletePayloadQuery, deletePayloadArgs...); execErr != nil {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-EXECDELPAYLOAD " + execErr.Error())
	}

	deleteKeysQuery, deleteKeysArgs, err := dialect.
		Delete(referenceKeyTable).
		Where(goqu.C("reference_id").Eq(ownerID)).
		ToSQL()
	if err != nil {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-BUILDDELKEYS " + err.Error())
	}
	if _, execErr := tx.Exec(deleteKeysQuery, deleteKeysArgs...); execErr != nil {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-EXECDELKEYS " + execErr.Error())
	}

	insertPayloadQuery, insertPayloadArgs, err := dialect.
		Insert(referencePayloadTable).
		Rows(goqu.Record{
			"reference_id":             ownerID,
			"parent_reference_payload": goqu.L("?::jsonb", parentReferencePayload.String),
		}).
		ToSQL()
	if err != nil {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-BUILDINSPAYLOAD " + err.Error())
	}
	if _, execErr := tx.Exec(insertPayloadQuery, insertPayloadArgs...); execErr != nil {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-EXECINSPAYLOAD " + execErr.Error())
	}

	if len(keys) > 0 {
		keyRows := make([]goqu.Record, 0, len(keys))
		for i := range keys {
			keyRows = append(keyRows, goqu.Record{
				"reference_id": ownerID,
				"position":     positions[i],
				"type":         keyTypes[i],
				"value":        keyValues[i],
			})
		}

		insertKeysQuery, insertKeysArgs, buildKeysErr := dialect.
			Insert(referenceKeyTable).
			Rows(keyRows).
			ToSQL()
		if buildKeysErr != nil {
			return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-BUILDINSKEYS " + buildKeysErr.Error())
		}
		if _, execKeysErr := tx.Exec(insertKeysQuery, insertKeysArgs...); execKeysErr != nil {
			return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-EXECINSKEYS " + execKeysErr.Error())
		}
	}

	return sql.NullInt64{Int64: ownerID, Valid: true}, nil
}

func getReferenceAsJSON(ref types.IReference) (sql.NullString, error) {
	if ref == nil {
		return sql.NullString{Valid: false}, nil
	}
	jsonable, err := jsonization.ToJsonable(ref)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to convert reference to jsonable: %w", err)
	}
	jsonParser := jsoniter.ConfigCompatibleWithStandardLibrary
	jsonBytes, err := jsonParser.Marshal(jsonable)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to marshal reference jsonable: %w", err)
	}
	return sql.NullString{String: string(jsonBytes), Valid: true}, nil
}
