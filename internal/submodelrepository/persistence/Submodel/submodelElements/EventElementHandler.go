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

package submodelelements

import (
	"database/sql"
	"errors"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLEventElementHandler handles the persistence operations for EventElement submodel elements.
// It implements the SubmodelElementHandler interface and uses the decorator pattern
// to extend the base CRUD operations with EventElement-specific functionality.
type PostgreSQLEventElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLEventElementHandler creates a new PostgreSQLEventElementHandler instance.
// It initializes the handler with a database connection and creates the decorated
// base CRUD handler for common SubmodelElement operations.
//
// Parameters:
//   - db: Database connection to PostgreSQL
//
// Returns:
//   - *PostgreSQLEventElementHandler: Configured EventElement handler instance
//   - error: Error if the decorated handler creation fails
func NewPostgreSQLEventElementHandler(db *sql.DB) (*PostgreSQLEventElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLEventElementHandler{db: db, decorated: decoratedHandler}, nil
}

// Create persists a new EventElement submodel element to the database.
// Currently creates the base SubmodelElement data using the decorated handler.
// EventElement-specific operations are not yet implemented but the structure is prepared.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - submodelElement: The EventElement to create (must be of type *gen.EventElement)
//
// Returns:
//   - int: The database ID of the created event element
//   - error: Error if the element is not an EventElement or if database operations fail
func (p PostgreSQLEventElementHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	_, ok := submodelElement.(*gen.EventElement)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type EventElement")
	}
	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// EventElement-specific database insertion
	// Determine which column to use based on valueType

	// Then, perform EventElement-specific operations within the same transaction

	// Commit the transaction only if everything succeeded
	return id, nil
}

// CreateNested creates a nested EventElement submodel element.
// This operation is currently not implemented for EventElement types.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent SubmodelElement
//   - idShortPath: Path identifier for the nested element
//   - submodelElement: The EventElement to create
//   - pos: Position of the element within its parent
//
// Returns:
//   - int: Always returns 0 (not implemented)
//   - error: Always returns "not implemented" error
//
//nolint:revive
func (p PostgreSQLEventElementHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	return 0, errors.New("not implemented")
}

// Update modifies an existing EventElement submodel element in the database.
// Currently delegates to the decorated handler for base SubmodelElement updates.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to update
//   - submodelElement: The updated EventElement data
//
// Returns:
//   - error: Error if the update operation fails
func (p PostgreSQLEventElementHandler) Update(submodelID string, idShortOrPath string, submodelElement gen.SubmodelElement) error {
	return p.decorated.Update(submodelID, idShortOrPath, submodelElement)
}

func (p PostgreSQLEventElementHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	return nil
}

// Delete removes an EventElement submodel element from the database.
// Currently delegates to the decorated handler for base SubmodelElement deletion.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to delete
//
// Returns:
//   - error: Error if the delete operation fails
func (p PostgreSQLEventElementHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}
