/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// Create inserts a new SubmodelElementCollection into the database as a top-level submodel element.
// This method handles both the common submodel element properties and creates the collection
// container that can hold other submodel elements in a hierarchical structure.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the parent submodel
//   - submodelElement: The SubmodelElementCollection to create
//
// Returns:
//   - int: Database ID of the created collection element
//   - error: Error if creation fails or element is not of correct type
func (p PostgreSQLSubmodelElementCollectionHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	_, ok := submodelElement.(*gen.SubmodelElementCollection)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type SubmodelElementCollection")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// SubmodelElementCollection-specific database insertion
	_, err = tx.Exec(`INSERT INTO submodel_element_collection (id) VALUES ($1)`, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested inserts a new SubmodelElementCollection as a nested element within another collection or list.
// This method creates the collection at a specific hierarchical path and position within its parent container.
// It allows for deep nesting of collections to create complex hierarchical data structures.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent element
//   - idShortPath: Hierarchical path where the collection should be created
//   - submodelElement: The SubmodelElementCollection to create
//   - pos: Position within the parent container
//
// Returns:
//   - int: Database ID of the created nested collection
//   - error: Error if creation fails or element is not of correct type
func (p PostgreSQLSubmodelElementCollectionHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	_, ok := submodelElement.(*gen.SubmodelElementCollection)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type SubmodelElementCollection")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	// SubmodelElementCollection-specific database insertion
	_, err = tx.Exec(`INSERT INTO submodel_element_collection (id) VALUES ($1)`, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing SubmodelElementCollection identified by its idShort or path.
// This method delegates the update operation to the decorated CRUD handler which handles
// the common submodel element update logic.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the collection to update
//   - submodelElement: Updated collection data
//
// Returns:
//   - error: Error if update fails
func (p PostgreSQLSubmodelElementCollectionHandler) Update(submodelID string, idShortOrPath string, submodelElement gen.SubmodelElement, tx *sql.Tx, isPut bool) error {
	return p.decorated.Update(submodelID, idShortOrPath, submodelElement, tx, isPut)
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
