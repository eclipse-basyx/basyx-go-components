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
	"log/slog"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	jsoniter "github.com/json-iterator/go"
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
	cu, localTx, err := common.StartTXIfNeeded(tx, err, p.db)
	if err != nil {
		return err
	}
	defer cu(&err)
	err = p.decorated.Update(submodelID, idShortOrPath, submodelElement, localTx, isPut)
	if err != nil {
		return err
	}
	effectivePath := resolveUpdatedPath(idShortOrPath, submodelElement, isPut)
	smDbID, err := persistenceutils.GetSubmodelDatabaseID(localTx, submodelID)
	if err != nil {
		slog.Error("submodel database ID lookup failed", "error.code", "SMREPO-SMEUPDATE-GETSMDBID", "error", err)
		return common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}
	elementID, err := p.decorated.GetDatabaseIDWithTx(localTx, smDbID, effectivePath)
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
	if anyFieldsToUpdate(updateRecord) {
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

	return common.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly stores the decoded ValueOnly references of a RelationshipElement; absent references stay untouched.
func (p PostgreSQLRelationshipElementHandler) UpdateValueOnly(tx *sql.Tx, update gen.ValueOnlyUpdate) error {
	return updateValueOnlyRelationship(tx, "relationship_element", update, "SMREPO-UPDRELVALONLY")
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

	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	firstRef, err := serializeReference(relElem.First(), json)
	if err != nil {
		return nil, err
	}

	secondRef, err := serializeReference(relElem.Second(), json)
	if err != nil {
		return nil, err
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

func buildUpdateRelationshipElementRecordObject(isPut bool, relElem types.IRelationshipElement, json jsoniter.API) (goqu.Record, error) {
	updateRecord := goqu.Record{}

	// Handle First reference - optional field
	// For PUT: always update (even if nil, which clears the field)
	// For PATCH: only update if provided (not nil)
	if isPut || relElem.First() != nil {
		firstRef, err := serializeReference(relElem.First(), json)
		if err != nil {
			return nil, err
		}
		updateRecord["first"] = firstRef
	}

	// Handle Second reference - optional field
	// For PUT: always update (even if nil, which clears the field)
	// For PATCH: only update if provided (not nil)
	if isPut || relElem.Second() != nil {
		secondRef, err := serializeReference(relElem.Second(), json)
		if err != nil {
			return nil, err
		}
		updateRecord["second"] = secondRef
	}
	return updateRecord, nil
}
