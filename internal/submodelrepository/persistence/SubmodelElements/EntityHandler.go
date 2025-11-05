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

// PostgreSQLEntityHandler handles the persistence operations for Entity submodel elements.
// It implements the SubmodelElementHandler interface and uses the decorator pattern
// to extend the base CRUD operations with Entity-specific functionality.
type PostgreSQLEntityHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLEntityHandler creates a new PostgreSQLEntityHandler instance.
// It initializes the handler with a database connection and creates the decorated
// base CRUD handler for common SubmodelElement operations.
//
// Parameters:
//   - db: Database connection to PostgreSQL
//
// Returns:
//   - *PostgreSQLEntityHandler: Configured Entity handler instance
//   - error: Error if the decorated handler creation fails
func NewPostgreSQLEntityHandler(db *sql.DB) (*PostgreSQLEntityHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLEntityHandler{db: db, decorated: decoratedHandler}, nil
}

// Create persists a new Entity submodel element to the database.
// It first creates the base SubmodelElement data using the decorated handler,
// then adds Entity-specific data including entity type, global asset ID, and specific asset IDs.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - submodelElement: The Entity element to create (must be of type *gen.Entity)
//
// Returns:
//   - int: The database ID of the created entity
//   - error: Error if the element is not an Entity or if database operations fail
func (p PostgreSQLEntityHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	entity, ok := submodelElement.(*gen.Entity)
	if !ok {
		return 0, errors.New("submodelElement is not of type Entity")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// Entity-specific database insertion
	err = insertEntity(entity, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested persists a new nested Entity submodel element to the database.
// It creates the Entity as a child element of another SubmodelElement with a specific position
// and idShortPath for hierarchical organization.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent SubmodelElement
//   - idShortPath: Path identifier for the nested element
//   - submodelElement: The Entity element to create (must be of type *gen.Entity)
//   - pos: Position of the element within its parent
//
// Returns:
//   - int: The database ID of the created nested entity
//   - error: Error if the element is not an Entity or if database operations fail
func (p PostgreSQLEntityHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	entity, ok := submodelElement.(*gen.Entity)
	if !ok {
		return 0, errors.New("submodelElement is not of type Entity")
	}

	// Create the nested entity with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelID, parentID, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// Entity-specific database insertion for nested element
	err = insertEntity(entity, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing Entity submodel element in the database.
// Currently delegates to the decorated handler for base SubmodelElement updates.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to update
//   - submodelElement: The updated Entity element data
//
// Returns:
//   - error: Error if the update operation fails
func (p PostgreSQLEntityHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}

// Delete removes an Entity submodel element from the database.
// Currently delegates to the decorated handler for base SubmodelElement deletion.
// Entity-specific data is automatically deleted due to foreign key constraints.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to delete
//
// Returns:
//   - error: Error if the delete operation fails
func (p PostgreSQLEntityHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertEntity(entity *gen.Entity, tx *sql.Tx, id int) error {
	_, err := tx.Exec(`INSERT INTO entity_element (id, entity_type, global_asset_id) VALUES ($1, $2, $3)`,
		id, entity.EntityType, entity.GlobalAssetID)
	if err != nil {
		return err
	}

	// Insert specific asset ids
	for _, sai := range entity.SpecificAssetIds {
		var extRef sql.NullInt64
		if !isEmptyReference(sai.ExternalSubjectID) {
			refID, err := insertReference(tx, *sai.ExternalSubjectID)
			if err != nil {
				return err
			}
			extRef = sql.NullInt64{Int64: int64(refID), Valid: true}
		}
		_, err = tx.Exec(`INSERT INTO entity_specific_asset_id (entity_id, name, value, external_subject_ref) VALUES ($1, $2, $3, $4)`,
			id, sai.Name, sai.Value, extRef)
		if err != nil {
			return err
		}
	}
	return nil
}
