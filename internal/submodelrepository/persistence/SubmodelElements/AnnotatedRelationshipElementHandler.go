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
// including annotated relationship elements.
package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLAnnotatedRelationshipElementHandler provides PostgreSQL-based persistence operations
// for AnnotatedRelationshipElement submodel elements. It implements CRUD operations and handles
// the complex relationships and annotations associated with annotated relationship elements.
type PostgreSQLAnnotatedRelationshipElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLAnnotatedRelationshipElementHandler creates a new handler for AnnotatedRelationshipElement persistence.
// It initializes the handler with a database connection and sets up the decorated CRUD handler
// for common submodel element operations.
//
// Parameters:
//   - db: PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLAnnotatedRelationshipElementHandler: Configured handler instance
//   - error: Error if handler initialization fails
func NewPostgreSQLAnnotatedRelationshipElementHandler(db *sql.DB) (*PostgreSQLAnnotatedRelationshipElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLAnnotatedRelationshipElementHandler{db: db, decorated: decoratedHandler}, nil
}

// Create inserts a new AnnotatedRelationshipElement into the database as a top-level submodel element.
// This method handles both the common submodel element properties and the specific relationship
// and annotation data associated with annotated relationship elements.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the parent submodel
//   - submodelElement: The AnnotatedRelationshipElement to create
//
// Returns:
//   - int: Database ID of the created element
//   - error: Error if creation fails or element is not of correct type
func (p PostgreSQLAnnotatedRelationshipElementHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	areElem, ok := submodelElement.(*gen.AnnotatedRelationshipElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type AnnotatedRelationshipElement")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// AnnotatedRelationshipElement-specific database insertion
	err = insertAnnotatedRelationshipElement(areElem, tx, id, submodelID, p.db)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested inserts a new AnnotatedRelationshipElement as a nested element within a collection or list.
// This method creates the element at a specific hierarchical path and position within its parent container.
// It handles both the parent-child relationship and the specific annotated relationship element data.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent element
//   - idShortPath: Hierarchical path where the element should be created
//   - submodelElement: The AnnotatedRelationshipElement to create
//   - pos: Position within the parent container
//
// Returns:
//   - int: Database ID of the created nested element
//   - error: Error if creation fails or element is not of correct type
func (p PostgreSQLAnnotatedRelationshipElementHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	areElem, ok := submodelElement.(*gen.AnnotatedRelationshipElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type AnnotatedRelationshipElement")
	}

	// Create the nested areElem with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	// AnnotatedRelationshipElement-specific database insertion for nested element
	err = insertAnnotatedRelationshipElement(areElem, tx, id, submodelID, p.db)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing AnnotatedRelationshipElement identified by its idShort or path.
// This method delegates the update operation to the decorated CRUD handler which handles
// the common submodel element update logic.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the element to update
//   - submodelElement: Updated element data
//
// Returns:
//   - error: Error if update fails
func (p PostgreSQLAnnotatedRelationshipElementHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}

// Delete removes an AnnotatedRelationshipElement identified by its idShort or path from the database.
// This method delegates the deletion operation to the decorated CRUD handler which handles
// the cascading deletion of all related data and child elements.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the element to delete
//
// Returns:
//   - error: Error if deletion fails
func (p PostgreSQLAnnotatedRelationshipElementHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertAnnotatedRelationshipElement(areElem *gen.AnnotatedRelationshipElement, tx *sql.Tx, id int, submodelID string, db *sql.DB) error {
	// Insert into relationship_element
	var firstRefID, secondRefID sql.NullInt64

	if !isEmptyReference(areElem.First) {
		refID, err := insertReference(tx, *areElem.First)
		if err != nil {
			return err
		}
		firstRefID = sql.NullInt64{Int64: int64(refID), Valid: true}
	}

	if !isEmptyReference(areElem.Second) {
		refID, err := insertReference(tx, *areElem.Second)
		if err != nil {
			return err
		}
		secondRefID = sql.NullInt64{Int64: int64(refID), Valid: true}
	}

	_, err := tx.Exec(`INSERT INTO relationship_element (id, first_ref, second_ref) VALUES ($1, $2, $3)`,
		id, firstRefID, secondRefID)
	if err != nil {
		return err
	}

	// Create annotations as separate submodel elements
	for _, annotation := range areElem.Annotations {
		annHandler, err := GetSMEHandler(annotation, db)
		if err != nil {
			return err
		}

		annID, err := annHandler.Create(tx, submodelID, annotation)
		if err != nil {
			return err
		}

		// Insert link
		_, err = tx.Exec(`INSERT INTO annotated_rel_annotation (rel_id, annotation_sme) VALUES ($1, $2)`, id, annID)
		if err != nil {
			return err
		}
	}

	return nil
}
