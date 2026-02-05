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
// This package contains PostgreSQL-based persistence implementations for various submodel element types
// including submodel element collections for organizing related elements.
package submodelelements

import (
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLSubmodelElementCollectionHandler provides PostgreSQL-based persistence operations
// for SubmodelElementCollection elements. It implements CRUD operations for collections that
// group related submodel elements together in a hierarchical structure with dot-notation addressing.
type PostgreSQLSubmodelElementCollectionHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLSubmodelElementCollectionHandler creates a new handler for SubmodelElementCollection persistence.
// It initializes the handler with a database connection and sets up the decorated CRUD handler
// for common submodel element operations.
//
// Parameters:
//   - db: PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLSubmodelElementCollectionHandler: Configured handler instance
//   - error: Error if handler initialization fails
func NewPostgreSQLSubmodelElementCollectionHandler(db *sql.DB) (*PostgreSQLSubmodelElementCollectionHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLSubmodelElementCollectionHandler{db: db, decorated: decoratedHandler}, nil
}

// Update modifies an existing SubmodelElementCollection element identified by its idShort or path.
// This method delegates the update operation to the decorated CRUD handler which handles
// the common submodel element update logic.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: idShort or hierarchical path to the collection to update
//   - submodelElement: Updated collection data (must be of type *gen.SubmodelElementCollection)
//   - tx: Optional database transaction (created if nil)
//   - isPut: true for PUT (replace all), false for PATCH (update only provided fields)
//
// Returns:
//   - error: Error if update fails or element is not of correct type
func (p PostgreSQLSubmodelElementCollectionHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	_, ok := submodelElement.(*types.SubmodelElementCollection)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type SubmodelElementCollection")
	}

	var err error
	cu, localTx, err := persistenceutils.StartTXIfNeeded(tx, err, p.db)
	if err != nil {
		return err
	}
	defer cu(&err)
	// For PUT operations, delete all children first (complete replacement)
	if isPut {
		err = DeleteAllChildren(p.db, submodelID, idShortOrPath, localTx)
		if err != nil {
			return err
		}
	}

	// PATCH operations preserve existing children, so no deletion needed - TODO

	// Update base submodel element properties
	err = p.decorated.Update(submodelID, idShortOrPath, submodelElement, tx, isPut)
	if err != nil {
		return err
	}

	return persistenceutils.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly updates only the value of an existing SubmodelElementCollection submodel element identified by its idShort or path.
// It processes the new value and updates nested elements accordingly.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set (must be of type gen.SubmodelElementValue)
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLSubmodelElementCollectionHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	elems, err := persistenceutils.BuildElementsToProcessStackValueOnly(p.db, submodelID, idShortOrPath, valueOnly)
	if err != nil {
		return err
	}
	err = UpdateNestedElementsValueOnly(p.db, elems, idShortOrPath, submodelID)
	if err != nil {
		return err
	}
	return nil
}

// Delete removes a SubmodelElementCollection identified by its idShort or path from the database.
// This method delegates the deletion operation to the decorated CRUD handler which handles
// the cascading deletion of all related data and child elements within the collection.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the collection to delete
//
// Returns:
//   - error: Error if deletion fails
func (p PostgreSQLSubmodelElementCollectionHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

// GetInsertQueryPart returns the type-specific insert query part for batch insertion of SubmodelElementCollection elements.
// It returns the table name and record for inserting into the submodel_element_collection table.
//
// Parameters:
//   - tx: Active database transaction (not used for Collection)
//   - id: The database ID of the base submodel_element record
//   - element: The SubmodelElementCollection element to insert
//
// Returns:
//   - *InsertQueryPart: The table name and record for submodel_element_collection insert
//   - error: An error if the element is not of type SubmodelElementCollection
func (p PostgreSQLSubmodelElementCollectionHandler) GetInsertQueryPart(_ *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error) {
	_, ok := element.(*types.SubmodelElementCollection)
	if !ok {
		return nil, common.NewErrBadRequest("submodelElement is not of type SubmodelElementCollection")
	}

	return &InsertQueryPart{
		TableName: "submodel_element_collection",
		Record: goqu.Record{
			"id": id,
		},
	}, nil
}
