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

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLFileHandler handles the persistence operations for File submodel elements.
// It implements the SubmodelElementHandler interface and uses the decorator pattern
// to extend the base CRUD operations with File-specific functionality.
type PostgreSQLFileHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLFileHandler creates a new PostgreSQLFileHandler instance.
// It initializes the handler with a database connection and creates the decorated
// base CRUD handler for common SubmodelElement operations.
//
// Parameters:
//   - db: Database connection to PostgreSQL
//
// Returns:
//   - *PostgreSQLFileHandler: Configured File handler instance
//   - error: Error if the decorated handler creation fails
func NewPostgreSQLFileHandler(db *sql.DB) (*PostgreSQLFileHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLFileHandler{db: db, decorated: decoratedHandler}, nil
}

// Create persists a new File submodel element to the database.
// It first creates the base SubmodelElement data using the decorated handler,
// then adds File-specific data including content type and value.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - submodelElement: The File element to create (must be of type *gen.File)
//
// Returns:
//   - int: The database ID of the created file element
//   - error: Error if the element is not a File or if database operations fail
func (p PostgreSQLFileHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	file, ok := submodelElement.(*gen.File)
	if !ok {
		return 0, errors.New("submodelElement is not of type File")
	}
	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// File-specific database insertion
	_, err = tx.Exec(`INSERT INTO file_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, file.ContentType, file.Value)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested persists a new nested File submodel element to the database.
// It creates the File as a child element of another SubmodelElement with a specific position
// and idShortPath for hierarchical organization.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent SubmodelElement
//   - idShortPath: Path identifier for the nested element
//   - submodelElement: The File element to create (must be of type *gen.File)
//   - pos: Position of the element within its parent
//
// Returns:
//   - int: The database ID of the created nested file element
//   - error: Error if the element is not a File or if database operations fail
func (p PostgreSQLFileHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	file, ok := submodelElement.(*gen.File)
	if !ok {
		return 0, errors.New("submodelElement is not of type File")
	}

	// Create the nested file with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// File-specific database insertion for nested element
	_, err = tx.Exec(`INSERT INTO file_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, file.ContentType, file.Value)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing File submodel element in the database.
// Currently delegates to the decorated handler for base SubmodelElement updates.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to update
//   - submodelElement: The updated File element data
//
// Returns:
//   - error: Error if the update operation fails
func (p PostgreSQLFileHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}

// Delete removes a File submodel element from the database.
// Currently delegates to the decorated handler for base SubmodelElement deletion.
// File-specific data is automatically deleted due to foreign key constraints.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to delete
//
// Returns:
//   - error: Error if the delete operation fails
func (p PostgreSQLFileHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
