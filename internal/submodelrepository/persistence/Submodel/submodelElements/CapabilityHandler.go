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
// including capability elements for representing functional capabilities.
package submodelelements

import (
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLCapabilityHandler provides PostgreSQL-based persistence operations for Capability submodel elements.
// It implements CRUD operations for capability elements which represent functional capabilities or services
// that can be provided by an asset or component within the Industrial Digital Twin context.
type PostgreSQLCapabilityHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLCapabilityHandler creates a new handler for Capability element persistence.
// It initializes the handler with a database connection and sets up the decorated CRUD handler
// for common submodel element operations.
//
// Parameters:
//   - db: PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLCapabilityHandler: Configured handler instance
//   - error: Error if handler initialization fails
func NewPostgreSQLCapabilityHandler(db *sql.DB) (*PostgreSQLCapabilityHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLCapabilityHandler{db: db, decorated: decoratedHandler}, nil
}

// Create inserts a new Capability element into the database as a top-level submodel element.
// This method handles both the common submodel element properties and the specific capability
// element data. Capabilities represent functional services or abilities of an asset.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the parent submodel
//   - submodelElement: The Capability element to create
//
// Returns:
//   - int: Database ID of the created element
//   - error: Error if creation fails or element is not of correct type
func (p PostgreSQLCapabilityHandler) Create(tx *sql.Tx, submodelID string, submodelElement types.ISubmodelElement) (int, error) {
	_, ok := submodelElement.(*types.Capability)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type Capability")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested inserts a new Capability element as a nested element within a collection or list.
// This method creates the element at a specific hierarchical path and position within its parent container.
// It handles both the parent-child relationship and the specific capability element data.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent element
//   - idShortPath: Hierarchical path where the element should be created
//   - submodelElement: The Capability element to create
//   - pos: Position within the parent container
//
// Returns:
//   - int: Database ID of the created nested element
//   - error: Error if creation fails or element is not of correct type
func (p PostgreSQLCapabilityHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement types.ISubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	_, ok := submodelElement.(*types.Capability)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type Capability")
	}

	// Create the nested capability with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing Capability element identified by its idShort or path.
// This method delegates the update operation to the decorated CRUD handler which handles
// the common submodel element update logic.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the element to update
//   - submodelElement: Updated element data
//
// Returns:
//   - error: Error if update fails
func (p PostgreSQLCapabilityHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	return p.decorated.Update(submodelID, idShortOrPath, submodelElement, tx, isPut)
}

// UpdateValueOnly updates only the value of an existing Capability submodel element identified by its idShort or path.
// Capability has no value only representation, so this method currently performs no operation.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLCapabilityHandler) UpdateValueOnly(_ string, _ string, _ gen.SubmodelElementValue) error {
	return nil
}

// Delete removes a Capability element identified by its idShort or path from the database.
// This method delegates the deletion operation to the decorated CRUD handler which handles
// the cascading deletion of all related data and child elements.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the element to delete
//
// Returns:
//   - error: Error if deletion fails
func (p PostgreSQLCapabilityHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}
