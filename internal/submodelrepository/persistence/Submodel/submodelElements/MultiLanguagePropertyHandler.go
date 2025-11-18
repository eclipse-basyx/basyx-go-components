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

// Package submodelelements provides persistence handlers for various submodel element types
// in the Eclipse BaSyx submodel repository. It implements CRUD operations for different
// submodel element types such as Range, Property, Collection, and others, with PostgreSQL
// as the underlying database.
//
// Author: Jannik Fried ( Fraunhofer IESE )
package submodelelements

import (
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLMultiLanguagePropertyHandler handles persistence operations for MultiLanguageProperty submodel elements.
// It uses the decorator pattern to extend the base PostgreSQLSMECrudHandler with
// MultiLanguageProperty-specific functionality. MultiLanguageProperty elements represent text values
// that can be expressed in multiple languages, with each language variant stored as a separate value.
type PostgreSQLMultiLanguagePropertyHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLMultiLanguagePropertyHandler creates a new PostgreSQLMultiLanguagePropertyHandler instance.
// It initializes the decorated PostgreSQLSMECrudHandler for base submodel element operations.
//
// Parameters:
//   - db: A PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLMultiLanguagePropertyHandler: A new handler instance
//   - error: An error if the decorated handler initialization fails
func NewPostgreSQLMultiLanguagePropertyHandler(db *sql.DB) (*PostgreSQLMultiLanguagePropertyHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLMultiLanguagePropertyHandler{db: db, decorated: decoratedHandler}, nil
}

// Create persists a new MultiLanguageProperty submodel element to the database within a transaction.
// It first creates the base submodel element using the decorated handler, then inserts
// MultiLanguageProperty-specific data including all language-text pairs as separate value entries.
//
// Parameters:
//   - tx: The database transaction
//   - submodelID: The ID of the parent submodel
//   - submodelElement: The MultiLanguageProperty element to create (must be of type *gen.MultiLanguageProperty)
//
// Returns:
//   - int: The database ID of the created element
//   - error: An error if the element is not a MultiLanguageProperty type or if database operations fail
func (p PostgreSQLMultiLanguagePropertyHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	mlp, ok := submodelElement.(*gen.MultiLanguageProperty)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type MultiLanguageProperty")
	}
	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// MultiLanguageProperty-specific database insertion
	err = insertMultiLanguageProperty(mlp, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested persists a new nested MultiLanguageProperty submodel element to the database within a transaction.
// This method is used when creating MultiLanguageProperty elements within collection-like structures
// (e.g., SubmodelElementCollection). It creates the base nested element with the provided idShortPath
// and position, then inserts MultiLanguageProperty-specific data including all language values.
//
// Parameters:
//   - tx: The database transaction
//   - submodelID: The ID of the parent submodel
//   - parentID: The database ID of the parent collection element
//   - idShortPath: The path identifying the element's location within the hierarchy
//   - submodelElement: The MultiLanguageProperty element to create (must be of type *gen.MultiLanguageProperty)
//   - pos: The position of the element within the parent collection
//
// Returns:
//   - int: The database ID of the created element
//   - error: An error if the element is not a MultiLanguageProperty type or if database operations fail
func (p PostgreSQLMultiLanguagePropertyHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	mlp, ok := submodelElement.(*gen.MultiLanguageProperty)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type MultiLanguageProperty")
	}

	// Create the nested mlp with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	// MultiLanguageProperty-specific database insertion for nested element
	err = insertMultiLanguageProperty(mlp, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing MultiLanguageProperty submodel element in the database.
// Currently delegates all update operations to the decorated handler for base submodel element properties.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifying the element to update
//   - submodelElement: The updated MultiLanguageProperty element data
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLMultiLanguagePropertyHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}

// Delete removes a MultiLanguageProperty submodel element from the database.
// Currently delegates all delete operations to the decorated handler. MultiLanguageProperty-specific data
// (including all language values) is typically removed via database cascade constraints.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifying the element to delete
//
// Returns:
//   - error: An error if the delete operation fails
func (p PostgreSQLMultiLanguagePropertyHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

// insertMultiLanguageProperty is a helper function that inserts MultiLanguageProperty-specific data
// into the multilanguage_property and multilanguage_property_value tables. It creates the parent
// multilanguage_property record, then inserts each language-text pair as a separate value record.
//
// Parameters:
//   - mlp: The MultiLanguageProperty element containing the data to insert
//   - tx: The database transaction
//   - id: The database ID of the parent submodel element
//
// Returns:
//   - error: An error if the database insert operation fails
func insertMultiLanguageProperty(mlp *gen.MultiLanguageProperty, tx *sql.Tx, id int) error {
	// Insert into multilanguage_property
	_, err := tx.Exec(`INSERT INTO multilanguage_property (id) VALUES ($1)`, id)
	if err != nil {
		return err
	}

	// Insert values
	for _, val := range mlp.Value {
		_, err = tx.Exec(`INSERT INTO multilanguage_property_value (mlp_id, language, text) VALUES ($1, $2, $3)`,
			id, val.Language, val.Text)
		if err != nil {
			return err
		}
	}
	return nil
}
