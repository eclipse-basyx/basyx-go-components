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
// Author: Jannik Fried ( Fraunhofer IESE )

// Package submodelelements provides handlers for different types of submodel elements in the BaSyx framework.
//
// This package contains PostgreSQL-based persistence implementations for various submodel element types
// including relationship elements that define directed relationships between other elements.
package submodelelements

import (
	"database/sql"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	jsoniter "github.com/json-iterator/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLRelationshipElementHandler provides persistence operations for RelationshipElement types.
//
// This handler implements the decorator pattern, wrapping the base PostgreSQLSMECrudHandler
// to add RelationshipElement-specific functionality. A RelationshipElement represents a
// directed relationship between two elements in the AAS model, identified by "first" and
// "second" references.
//
// The handler manages:
//   - Base submodel element properties (via decorated handler)
//   - First and second reference persistence
//   - Reference keys and their positions
//   - Both root-level and nested relationship elements
type PostgreSQLRelationshipElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLRelationshipElementHandler creates a new handler for RelationshipElement persistence.
//
// This constructor initializes a RelationshipElement handler with a decorated base handler
// for common submodel element operations. The decorator pattern allows for separation of
// concerns between generic element handling and type-specific logic.
//
// Parameters:
//   - db: PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLRelationshipElementHandler: Initialized handler ready for CRUD operations
//   - error: An error if the decorated handler creation fails
func NewPostgreSQLRelationshipElementHandler(db *sql.DB) (*PostgreSQLRelationshipElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLRelationshipElementHandler{db: db, decorated: decoratedHandler}, nil
}

// Update updates an existing RelationshipElement identified by its idShort or path.
// This method handles both the common submodel element properties and the specific
// relationship element data including first and second references.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: The idShort or full path of the element to update
//   - submodelElement: The updated element data
//   - tx: Active database transaction (can be nil, will create one if needed)
//   - isPut: true: Replaces the element with the body data; false: Updates only passed data
//
// Returns:
//   - error: An error if the decorated update operation fails
func (p PostgreSQLRelationshipElementHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	relElem, ok := submodelElement.(*types.RelationshipElement)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type RelationshipElement")
	}

	var err error
	cu, localTx, err := persistenceutils.StartTXIfNeeded(tx, err, p.db)
	if err != nil {
		return err
	}
	defer cu(&err)
	err = p.decorated.Update(submodelID, idShortOrPath, submodelElement, localTx, isPut)
	if err != nil {
		return err
	}

	elementID, err := p.decorated.GetDatabaseID(submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	dialect := goqu.Dialect("postgres")
	json := jsoniter.ConfigCompatibleWithStandardLibrary

	// Build update record based on isPut flag
	updateRecord, err := buildUpdateRelationshipElementRecordObject(isPut, relElem, json)
	if err != nil {
		return err
	}

	// Only execute update if there are fields to update
	if persistenceutils.AnyFieldsToUpdate(updateRecord) {
		updateQuery, updateArgs, err := dialect.Update("relationship_element").
			Set(updateRecord).
			Where(goqu.C("id").Eq(elementID)).
			ToSQL()
		if err != nil {
			return err
		}

		_, err = localTx.Exec(updateQuery, updateArgs...)
		if err != nil {
			return err
		}
	}

	return persistenceutils.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly updates only the value fields of an existing RelationshipElement.
//
// This method allows for partial updates of a RelationshipElement, specifically targeting
// the "first" and "second" references without modifying other base element properties.
// It constructs an update record dynamically based on which fields are provided in
// the valueOnly parameter.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: idShort or hierarchical path to the element to update
//   - valueOnly: The RelationshipElementValue containing fields to update
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLRelationshipElementHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	relElemVal, ok := valueOnly.(gen.RelationshipElementValue)
	if !ok {
		return common.NewErrBadRequest("valueOnly is not of type RelationshipElementValue")
	}

	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	dialect := goqu.Dialect("postgres")
	json := jsoniter.ConfigCompatibleWithStandardLibrary

	// Build update record with only the fields that are set
	updateRecord := goqu.Record{}

	// check if first is set (it is map[string]interface{} so we check for nil)
	if relElemVal.First != nil {
		firstRefByte, err := json.Marshal(relElemVal.First)
		if err != nil {
			return err
		}
		updateRecord["first"] = string(firstRefByte)
	}

	if relElemVal.Second != nil {
		secondRefByte, err := json.Marshal(relElemVal.Second)
		if err != nil {
			return err
		}
		updateRecord["second"] = string(secondRefByte)
	}

	// If nothing to update, return early
	if len(updateRecord) == 0 {
		return nil
	}

	query, args, err := dialect.Update("relationship_element").
		Set(updateRecord).
		Where(goqu.I("id").Eq(
			dialect.From("submodel_element").
				Select("id").
				Where(goqu.Ex{
					"submodel_id":  submodelID,
					"idshort_path": idShortOrPath,
				}),
		)).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = tx.Exec(query, args...)
	if err != nil {
		return common.NewInternalServerError(fmt.Sprintf("failed to execute update for RelationshipElement: %s", err.Error()))
	}
	err = tx.Commit()
	return err
}

// Delete removes a RelationshipElement identified by its idShort or path.
//
// This method delegates to the decorated handler for delete operations. When implemented,
// it will handle cascading deletion of relationship-specific data along with base element data.
//
// Parameters:
//   - idShortOrPath: The idShort or full path of the element to delete
//
// Returns:
//   - error: An error if the decorated delete operation fails
func (p PostgreSQLRelationshipElementHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

// GetInsertQueryPart returns the type-specific insert query part for batch insertion of RelationshipElement elements.
// It returns the table name and record for inserting into the relationship_element table.
//
// Parameters:
//   - tx: Active database transaction (not used for RelationshipElement)
//   - id: The database ID of the base submodel_element record
//   - element: The RelationshipElement element to insert
//
// Returns:
//   - *InsertQueryPart: The table name and record for relationship_element insert
//   - error: An error if the element is not of type RelationshipElement
func (p PostgreSQLRelationshipElementHandler) GetInsertQueryPart(_ *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error) {
	relElem, ok := element.(*types.RelationshipElement)
	if !ok {
		return nil, common.NewErrBadRequest("submodelElement is not of type RelationshipElement")
	}

	var firstRef, secondRef string
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	if !isEmptyReference(relElem.First()) {
		jsonable, err := jsonization.ToJsonable(relElem.First())
		if err != nil {
			return nil, common.NewErrBadRequest("SMREPO-GIQP-RELEL-FIRSTJSON Failed to convert first reference to jsonable: " + err.Error())
		}
		ref, err := json.Marshal(jsonable)
		if err != nil {
			return nil, err
		}
		firstRef = string(ref)
	}

	if !isEmptyReference(relElem.Second()) {
		jsonable, err := jsonization.ToJsonable(relElem.Second())
		if err != nil {
			return nil, common.NewErrBadRequest("SMREPO-GIQP-RELEL-SECONDJSON Failed to convert second reference to jsonable: " + err.Error())
		}
		ref, err := json.Marshal(jsonable)
		if err != nil {
			return nil, err
		}
		secondRef = string(ref)
	}

	return &InsertQueryPart{
		TableName: "relationship_element",
		Record: goqu.Record{
			"id":     id,
			"first":  firstRef,
			"second": secondRef,
		},
	}, nil
}

// insertReference creates a complete reference record with its keys in the database.
//
// This utility function persists a reference structure to the database, including the
// reference type and all associated keys with their positions, types, and values. The
// function ensures proper ordering of keys through position tracking.
//
// The function performs:
//   - Insertion of the reference record with its type
//   - Iteration through all keys in the reference
//   - Insertion of each key with its position (index), type, and value
//   - Proper ordering preservation via position field
//
// Parameters:
//   - tx: Active transaction context for atomic operations
//   - ref: The Reference object containing type and keys to persist
//
// Returns:
//   - int: Database ID of the newly created reference
//   - error: An error if reference or key insertion fails
//
// Note: This function is used for both first and second references in relationship elements,
// as well as any other reference structures that need full persistence with keys.
func insertReference(tx *sql.Tx, ref types.IReference) (int, error) {
	dialect := goqu.Dialect("postgres")

	// Insert reference and get ID
	insertQuery, insertArgs, err := dialect.Insert("reference").
		Rows(goqu.Record{"type": ref.Type()}).
		Returning("id").
		ToSQL()
	if err != nil {
		return 0, err
	}

	var refID int
	err = tx.QueryRow(insertQuery, insertArgs...).Scan(&refID)
	if err != nil {
		return 0, err
	}

	// Insert keys
	for i, key := range ref.Keys() {
		insertKeyQuery, insertKeyArgs, err := dialect.Insert("reference_key").
			Rows(goqu.Record{
				"reference_id": refID,
				"position":     i,
				"type":         key.Type(),
				"value":        key.Value(),
			}).
			ToSQL()
		if err != nil {
			return 0, err
		}
		_, err = tx.Exec(insertKeyQuery, insertKeyArgs...)
		if err != nil {
			return 0, err
		}
	}
	return refID, nil
}

func buildUpdateRelationshipElementRecordObject(isPut bool, relElem types.IRelationshipElement, json jsoniter.API) (goqu.Record, error) {
	updateRecord := goqu.Record{}

	// Handle First reference - optional field
	// For PUT: always update (even if nil, which clears the field)
	// For PATCH: only update if provided (not nil)
	if isPut || relElem.First() != nil {
		var firstRef string
		if relElem.First() != nil && !isEmptyReference(relElem.First()) {
			jsonable, err := jsonization.ToJsonable(relElem.First())
			if err != nil {
				return nil, common.NewErrBadRequest("SMREPO-BURERO-FIRSTJSONABLE Failed to convert first reference to jsonable: " + err.Error())
			}
			ref, err := json.Marshal(jsonable)
			if err != nil {
				return nil, err
			}
			firstRef = string(ref)
		}
		updateRecord["first"] = firstRef
	}

	// Handle Second reference - optional field
	// For PUT: always update (even if nil, which clears the field)
	// For PATCH: only update if provided (not nil)
	if isPut || relElem.Second() != nil {
		var secondRef string
		if relElem.Second() != nil && !isEmptyReference(relElem.Second()) {
			jsonable, err := jsonization.ToJsonable(relElem.Second())
			if err != nil {
				return nil, common.NewErrBadRequest("SMREPO-BURERO-SECONDJSONABLE Failed to convert second reference to jsonable: " + err.Error())
			}
			ref, err := json.Marshal(jsonable)
			if err != nil {
				return nil, err
			}
			secondRef = string(ref)
		}
		updateRecord["second"] = secondRef
	}
	return updateRecord, nil
}
