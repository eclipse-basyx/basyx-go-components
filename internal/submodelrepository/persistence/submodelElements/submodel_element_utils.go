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
	"strconv"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	jsoniter "github.com/json-iterator/go"
	"github.com/lib/pq"
)

// ValueOnlyElementsToProcess represents a SubmodelElementValue along with its ID short path.
type ValueOnlyElementsToProcess struct {
	Element     gen.SubmodelElementValue
	IdShortPath string
}

// SubmodelElementToProcess represents a SubmodelElement along with its ID short path.
type SubmodelElementToProcess struct {
	Element     types.ISubmodelElement
	IdShortPath string
}

// BuildElementsToProcessStackValueOnly builds a stack of SubmodelElementValues to process iteratively.
//
// This function constructs a stack of SubmodelElementValues starting from a given root element.
// It processes the elements iteratively, handling collections, lists, and ambiguous types
// (like MultiLanguageProperty or SubmodelElementList) by querying the database to determine
// their actual types. The resulting stack contains all elements to be processed along with
// their corresponding ID short paths.
//
// Parameters:
//   - db: Database connection
//   - submodelID: String identifier of the submodel
//   - idShortOrPath: ID short or path of the root element
//   - valueOnly: The root SubmodelElementValue to start processing from
//
// Returns:
//   - []ValueOnlyElementsToProcess: Slice of elements to process with their ID short paths
//   - error: An error if any database query fails or if type conversion fails
func buildElementsToProcessStackValueOnly(db *sql.DB, submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) ([]ValueOnlyElementsToProcess, error) {
	stack := []ValueOnlyElementsToProcess{}
	elementsToProcess := []ValueOnlyElementsToProcess{}
	stack = append(stack, ValueOnlyElementsToProcess{
		Element:     valueOnly,
		IdShortPath: idShortOrPath,
	})
	// Build Iteratively
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch elem := current.Element.(type) {
		case gen.AmbiguousSubmodelElementValue:
			// Check if it is a MLP or SME List in the database
			sqlQuery, args, err := buildCheckMultiLanguagePropertyOrSubmodelElementListQuery(current.IdShortPath, submodelID)
			if err != nil {
				return nil, common.NewInternalServerError("SMREPO-BUILDELPROC-BUILDCHECKQUERY " + err.Error())
			}
			row := db.QueryRow(sqlQuery, args...)
			var modelType int64
			if err := row.Scan(&modelType); err != nil {
				return nil, common.NewErrNotFound(fmt.Sprintf("Submodel Element with ID Short Path %s and Submodel ID %s not found", current.IdShortPath, submodelID))
			}
			if modelType == int64(types.ModelTypeMultiLanguageProperty) {
				mlpValue, err := elem.ConvertToMultiLanguagePropertyValue()
				if err != nil {
					return nil, err
				}
				el := ValueOnlyElementsToProcess{
					Element:     mlpValue,
					IdShortPath: current.IdShortPath,
				}
				elementsToProcess = append(elementsToProcess, el)
			} else {
				value, err := elem.ConvertToSubmodelElementListValue()
				if err != nil {
					return nil, err
				}
				for i, v := range value {
					stack = append(stack, ValueOnlyElementsToProcess{
						Element:     v,
						IdShortPath: current.IdShortPath + "[" + strconv.Itoa(i) + "]",
					})
				}
			}
		case gen.SubmodelElementCollectionValue:
			for idShort, v := range elem {
				stack = append(stack, ValueOnlyElementsToProcess{
					Element:     v,
					IdShortPath: current.IdShortPath + "." + idShort,
				})
			}
		case gen.SubmodelElementListValue:
			for i, v := range elem {
				el := ValueOnlyElementsToProcess{
					Element:     v,
					IdShortPath: current.IdShortPath + "[" + strconv.Itoa(i) + "]",
				}
				stack = append(stack, el)
			}
		case gen.AnnotatedRelationshipElementValue:
			el := ValueOnlyElementsToProcess{
				Element:     elem,
				IdShortPath: current.IdShortPath,
			}
			elementsToProcess = append(elementsToProcess, el)
			for idShort, annotation := range elem.Annotations {
				stack = append(stack, ValueOnlyElementsToProcess{
					Element:     annotation,
					IdShortPath: current.IdShortPath + "." + idShort,
				})
			}
		case gen.EntityValue:
			el := ValueOnlyElementsToProcess{
				Element:     elem,
				IdShortPath: current.IdShortPath,
			}
			elementsToProcess = append(elementsToProcess, el)
			for idShort, child := range elem.Statements {
				stack = append(stack, ValueOnlyElementsToProcess{
					Element:     child,
					IdShortPath: current.IdShortPath + "." + idShort,
				})
			}
		default:
			// Process basic element
			el := ValueOnlyElementsToProcess{
				Element:     elem,
				IdShortPath: current.IdShortPath,
			}
			elementsToProcess = append(elementsToProcess, el)
		}
	}
	return elementsToProcess, nil
}

func buildCheckMultiLanguagePropertyOrSubmodelElementListQuery(idShortOrPath string, submodelID string) (string, []interface{}, error) {
	dialect := goqu.Dialect("postgres")

	query, args, err := dialect.From(goqu.T("submodel_element").As("sme")).
		Join(goqu.T("submodel").As("s"), goqu.On(goqu.Ex{"s.id": goqu.I("sme.submodel_id")})).
		Select(goqu.I("sme.model_type")).
		Where(goqu.Ex{
			"sme.idshort_path":      idShortOrPath,
			"s.submodel_identifier": submodelID,
		}).
		Limit(1).
		ToSQL()
	if err != nil {
		return "", nil, err
	}

	return query, args, nil
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

	ownerRow := dialect.
		Select(goqu.L("?::bigint", ownerID).As("id"))

	insRef := dialect.
		Insert(referenceTable).
		Cols("id", "type").
		FromQuery(
			dialect.
				From(goqu.T("owner_row")).
				Select(
					goqu.I("id"),
					goqu.L("?::int", int(reference.Type())),
				),
		).
		OnConflict(
			goqu.DoUpdate("id", goqu.Record{
				"type": goqu.L("EXCLUDED.type"),
			}),
		).
		Returning(goqu.I("id"))

	delPayload := dialect.
		Delete(referencePayloadTable).
		Where(
			goqu.I("reference_id").In(
				dialect.From(goqu.T("ins_ref")).Select(goqu.I("id")),
			),
		)

	delKeys := dialect.
		Delete(referenceKeyTable).
		Where(
			goqu.I("reference_id").In(
				dialect.From(goqu.T("ins_ref")).Select(goqu.I("id")),
			),
		)

	insPayload := dialect.
		Insert(referencePayloadTable).
		Cols("reference_id", "parent_reference_payload").
		FromQuery(
			dialect.
				From(goqu.T("ins_ref")).
				Select(
					goqu.I("id"),
					goqu.L("?::jsonb", parentReferencePayload.String),
				),
		)

	insKeys := dialect.
		Insert(referenceKeyTable).
		Cols("reference_id", "position", "type", "value").
		FromQuery(
			dialect.
				From(
					goqu.T("ins_ref").As("r"),
					goqu.L(
						"unnest(?::int[], ?::int[], ?::text[]) AS k(position, type, value)",
						pq.Array(positions),
						pq.Array(keyTypes),
						pq.Array(keyValues),
					),
				).
				Select(
					goqu.I("r.id"),
					goqu.I("k.position"),
					goqu.I("k.type"),
					goqu.I("k.value"),
				),
		).
		OnConflict(
			goqu.DoUpdate("reference_id, position", goqu.Record{
				"type":  goqu.L("EXCLUDED.type"),
				"value": goqu.L("EXCLUDED.value"),
			}),
		)

	query, args, err := dialect.
		From(goqu.T("ins_ref")).
		With("owner_row", ownerRow).
		With("ins_ref", insRef).
		With("del_payload", delPayload).
		With("del_keys", delKeys).
		With("ins_payload", insPayload).
		With("ins_keys", insKeys).
		Select(goqu.I("id")).
		ToSQL()
	if err != nil {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-BUILDQUERY " + err.Error())
	}

	var referenceID int64
	err = tx.QueryRow(query, args...).Scan(&referenceID)
	if err != nil {
		return sql.NullInt64{Valid: false}, common.NewInternalServerError("SMREPO-CRCTXREF-EXECQUERY " + err.Error())
	}

	return sql.NullInt64{Int64: referenceID, Valid: true}, nil
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
