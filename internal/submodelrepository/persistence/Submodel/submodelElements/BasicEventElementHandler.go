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
// including basic event elements.
package submodelelements

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLBasicEventElementHandler provides PostgreSQL-based persistence operations
// for BasicEventElement submodel elements. It implements CRUD operations and handles
// the event-specific properties such as observed references, message brokers, and timing intervals.
type PostgreSQLBasicEventElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLBasicEventElementHandler creates a new handler for BasicEventElement persistence.
// It initializes the handler with a database connection and sets up the decorated CRUD handler
// for common submodel element operations.
//
// Parameters:
//   - db: PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLBasicEventElementHandler: Configured handler instance
//   - error: Error if handler initialization fails
func NewPostgreSQLBasicEventElementHandler(db *sql.DB) (*PostgreSQLBasicEventElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLBasicEventElementHandler{db: db, decorated: decoratedHandler}, nil
}

// Create inserts a new BasicEventElement into the database as a top-level submodel element.
// This method handles both the common submodel element properties and the specific event
// properties such as observed references, message brokers, and timing intervals.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the parent submodel
//   - submodelElement: The BasicEventElement to create
//
// Returns:
//   - int: Database ID of the created element
//   - error: Error if creation fails or element is not of correct type
func (p PostgreSQLBasicEventElementHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	basicEvent, ok := submodelElement.(*gen.BasicEventElement)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type BasicEventElement")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// BasicEventElement-specific database insertion
	err = insertBasicEventElement(basicEvent, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested inserts a new BasicEventElement as a nested element within a collection or list.
// This method creates the element at a specific hierarchical path and position within its parent container.
// It handles both the parent-child relationship and the specific basic event element data.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent element
//   - idShortPath: Hierarchical path where the element should be created
//   - submodelElement: The BasicEventElement to create
//   - pos: Position within the parent container
//
// Returns:
//   - int: Database ID of the created nested element
//   - error: Error if creation fails or element is not of correct type
func (p PostgreSQLBasicEventElementHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	basicEvent, ok := submodelElement.(*gen.BasicEventElement)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type BasicEventElement")
	}

	// Create the nested basic event element with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	// BasicEventElement-specific database insertion for nested element
	err = insertBasicEventElement(basicEvent, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing BasicEventElement identified by its idShort or path.
// This method delegates the update operation to the decorated CRUD handler which handles
// the common submodel element update logic.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the element to update
//   - submodelElement: Updated element data
//
// Returns:
//   - error: Error if update fails
func (p PostgreSQLBasicEventElementHandler) Update(submodelID string, idShortOrPath string, submodelElement gen.SubmodelElement) error {
	return p.decorated.Update(submodelID, idShortOrPath, submodelElement)
}

func (p PostgreSQLBasicEventElementHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	basicEventValue, ok := valueOnly.(gen.BasicEventElementValue)
	if !ok {
		return common.NewErrBadRequest("valueOnly is not of type BasicEventElementValue")
	}

	// Begin transaction
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	dialect := goqu.Dialect("postgres")

	// Get the element ID from the database
	var elementID int
	var oldObservedRefID sql.NullInt64
	query, args, err := dialect.From("submodel_element").
		InnerJoin(
			goqu.T("basic_event_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("basic_event_element.id"))),
		).
		Select("submodel_element.id", "basic_event_element.observed_ref").
		Where(
			goqu.C("idshort_path").Eq(idShortOrPath),
			goqu.C("submodel_id").Eq(submodelID),
		).
		ToSQL()
	if err != nil {
		return err
	}

	err = tx.QueryRow(query, args...).Scan(&elementID, &oldObservedRefID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("BasicEventElement not found")
		}
		return err
	}

	// Insert new observed reference
	var newObservedRefID sql.NullInt64
	if len(basicEventValue.Observed.Keys) > 0 {
		insertQuery, insertArgs, err := dialect.Insert("reference").
			Rows(goqu.Record{"type": basicEventValue.Observed.Type}).
			Returning("id").
			ToSQL()
		if err != nil {
			return err
		}

		var refID int
		err = tx.QueryRow(insertQuery, insertArgs...).Scan(&refID)
		if err != nil {
			return err
		}
		newObservedRefID = sql.NullInt64{Int64: int64(refID), Valid: true}

		// Insert reference keys
		for i, key := range basicEventValue.Observed.Keys {
			keyQuery, keyArgs, err := dialect.Insert("reference_key").
				Rows(goqu.Record{
					"reference_id": refID,
					"position":     i,
					"type":         key.Type,
					"value":        key.Value,
				}).
				ToSQL()
			if err != nil {
				return err
			}
			_, err = tx.Exec(keyQuery, keyArgs...)
			if err != nil {
				return err
			}
		}
	}

	// Update the basic_event_element table with new observed reference
	updateQuery, updateArgs, err := dialect.Update("basic_event_element").
		Set(goqu.Record{"observed_ref": newObservedRefID}).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = tx.Exec(updateQuery, updateArgs...)
	if err != nil {
		return err
	}

	// Delete old observed reference if it exists
	if oldObservedRefID.Valid {
		deleteQuery, deleteArgs, err := dialect.Delete("reference").
			Where(goqu.C("id").Eq(oldObservedRefID.Int64)).
			ToSQL()
		if err != nil {
			return err
		}
		_, err = tx.Exec(deleteQuery, deleteArgs...)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Delete removes a BasicEventElement identified by its idShort or path from the database.
// This method delegates the deletion operation to the decorated CRUD handler which handles
// the cascading deletion of all related data and child elements.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the element to delete
//
// Returns:
//   - error: Error if deletion fails
func (p PostgreSQLBasicEventElementHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

func insertBasicEventElement(basicEvent *gen.BasicEventElement, tx *sql.Tx, id int) error {
	var observedRefID sql.NullInt64
	if !isEmptyReference(basicEvent.Observed) {
		var refID int
		err := tx.QueryRow(`INSERT INTO reference (type) VALUES ($1) RETURNING id`, basicEvent.Observed.Type).Scan(&refID)
		if err != nil {
			return err
		}
		observedRefID = sql.NullInt64{Int64: int64(refID), Valid: true}

		keys := basicEvent.Observed.Keys
		for i := range keys {
			_, err = tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`,
				refID, i, keys[i].Type, keys[i].Value)
			if err != nil {
				return err
			}
		}
	}

	var messageBrokerRefID sql.NullInt64
	if !isEmptyReference(basicEvent.MessageBroker) {
		var refID int
		err := tx.QueryRow(`INSERT INTO reference (type) VALUES ($1) RETURNING id`, basicEvent.MessageBroker.Type).Scan(&refID)
		if err != nil {
			return err
		}
		messageBrokerRefID = sql.NullInt64{Int64: int64(refID), Valid: true}

		keys := basicEvent.MessageBroker.Keys
		for i := range keys {
			_, err = tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`,
				refID, i, keys[i].Type, keys[i].Value)
			if err != nil {
				return err
			}
		}
	}

	// Handle nullable fields
	var lastUpdate sql.NullString
	if basicEvent.LastUpdate != "" {
		lastUpdate = sql.NullString{String: basicEvent.LastUpdate, Valid: true}
	}

	var minInterval sql.NullString
	if basicEvent.MinInterval != "" {
		minInterval = sql.NullString{String: basicEvent.MinInterval, Valid: true}
	}

	var maxInterval sql.NullString
	if basicEvent.MaxInterval != "" {
		maxInterval = sql.NullString{String: basicEvent.MaxInterval, Valid: true}
	}

	var messageTopic sql.NullString
	if basicEvent.MessageTopic != "" {
		messageTopic = sql.NullString{String: basicEvent.MessageTopic, Valid: true}
	}

	_, err := tx.Exec(`INSERT INTO basic_event_element (id, observed_ref, direction, state, message_topic, message_broker_ref, last_update, min_interval, max_interval) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, observedRefID, basicEvent.Direction, basicEvent.State, messageTopic, messageBrokerRefID, lastUpdate, minInterval, maxInterval)
	return err
}
