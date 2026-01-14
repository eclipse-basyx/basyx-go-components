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

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
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

// Create persists a new root-level RelationshipElement to the database.
//
// This method creates a RelationshipElement at
// the root level of a submodel. It delegates
// base element creation to the decorated handler, then persists the relationship-specific
// data including the first and second references that define the relationship.
//
// The method performs the following operations in sequence:
//  1. Type assertion to ensure the element is a RelationshipElement
//  2. Base element creation (idShort, category, model type, semantic ID)
//  3. Reference persistence (first and second references with their keys)
//  4. Insertion into relationship_element table
//
// All operations are performed within the provided transaction for atomicity.
//
// Parameters:
//   - tx: Active transaction context for atomic operations
//   - submodelID: ID of the parent submodel
//   - submodelElement: The RelationshipElement to create (must be *gen.RelationshipElement)
//
// Returns:
//   - int: Database ID of the newly created element
//   - error: An error if type assertion fails, base creation fails, or reference insertion fails
//
// Example:
//
//	relElem := &gen.RelationshipElement{
//	    IdShort: "dependsOn",
//	    First:   &gen.Reference{...},
//	    Second:  &gen.Reference{...},
//	}
//	id, err := handler.Create(tx, "submodel123", relElem)
func (p PostgreSQLRelationshipElementHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	relElem, ok := submodelElement.(*gen.RelationshipElement)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type RelationshipElement")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// RelationshipElement-specific database insertion
	err = insertRelationshipElement(relElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested persists a nested RelationshipElement within a hierarchical structure.
//
// This method creates a RelationshipElement as a child of another element (typically within
// a SubmodelElementCollection or SubmodelElementList). It manages parent-child relationships,
// position ordering, and full path tracking in addition to relationship-specific data.
//
// The method is used when creating relationships within collections or lists where explicit
// path and position management is required for proper hierarchy reconstruction.
//
// Parameters:
//   - tx: Active transaction context for atomic operations
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent element
//   - idShortPath: Full path from root (e.g., "collection1.dependsOn" or "relationships[0]")
//   - submodelElement: The RelationshipElement to create (must be *gen.RelationshipElement)
//   - pos: Position index within parent for ordering
//
// Returns:
//   - int: Database ID of the newly created nested element
//   - error: An error if type assertion fails, creation fails, or reference insertion fails
//
// Example:
//
//	id, err := handler.CreateNested(tx, "submodel123", parentDbID, "relations.dependsOn", relElem, 0)
func (p PostgreSQLRelationshipElementHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	relElem, ok := submodelElement.(*gen.RelationshipElement)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type RelationshipElement")
	}

	// Create the nested relElem with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	// RelationshipElement-specific database insertion for nested element
	err = insertRelationshipElement(relElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update updates an existing RelationshipElement identified by its idShort or path.
//
// This method delegates to the decorated handler for update operations. It's currently
// a pass-through that will leverage base handler update logic when implemented.
//
// Parameters:
//   - idShortOrPath: The idShort or full path of the element to update
//   - submodelElement: The updated element data
//
// Returns:
//   - error: An error if the decorated update operation fails
func (p PostgreSQLRelationshipElementHandler) Update(submodelID string, idShortOrPath string, submodelElement gen.SubmodelElement, tx *sql.Tx, isPut bool) error {
	return p.decorated.Update(submodelID, idShortOrPath, submodelElement, tx, isPut)
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

	if !isEmptyReference(relElemVal.First) {
		firstRefByte, err := json.Marshal(relElemVal.First)
		if err != nil {
			return err
		}
		updateRecord["first"] = string(firstRefByte)
	}

	if !isEmptyReference(relElemVal.Second) {
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

// insertRelationshipElement persists RelationshipElement-specific data to the database.
//
// This internal helper function handles the insertion of relationship-specific data into
// the relationship_element table. It manages the first and second references that define
// the directed relationship, creating full reference records with their keys if the
// references are not empty.
//
// The function:
//   - Checks if first and second references are non-empty
//   - Inserts complete reference structures (type, keys with positions and values)
//   - Links references to the relationship element via foreign keys
//   - Handles NULL values for empty references
//
// Parameters:
//   - relElem: The RelationshipElement containing the references to persist
//   - tx: Active transaction context for atomic operations
//   - id: Database ID of the parent submodel element
//
// Returns:
//   - error: An error if reference insertion fails or the final relationship_element insert fails
func insertRelationshipElement(relElem *gen.RelationshipElement, tx *sql.Tx, id int) error {
	var firstRef, secondRef string
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	if !isEmptyReference(relElem.First) {
		ref, err := json.Marshal(relElem.First)
		if err != nil {
			return err
		}
		firstRef = string(ref)
	}

	if !isEmptyReference(relElem.Second) {
		ref, err := json.Marshal(relElem.Second)
		if err != nil {
			return err
		}
		secondRef = string(ref)
	}

	_, err := tx.Exec(`INSERT INTO relationship_element (id, first, second) VALUES ($1, $2, $3)`,
		id, firstRef, secondRef)
	return err
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
func insertReference(tx *sql.Tx, ref gen.Reference) (int, error) {
	var refID int
	err := tx.QueryRow(`INSERT INTO reference (type) VALUES ($1) RETURNING id`, ref.Type).Scan(&refID)
	if err != nil {
		return 0, err
	}
	for i, key := range ref.Keys {
		_, err = tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`,
			refID, i, key.Type, key.Value)
		if err != nil {
			return 0, err
		}
	}
	return refID, nil
}
