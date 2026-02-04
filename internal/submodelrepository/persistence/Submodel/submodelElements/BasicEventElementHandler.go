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
// including basic event elements.
package submodelelements

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
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
func (p PostgreSQLBasicEventElementHandler) Create(tx *sql.Tx, submodelID string, submodelElement types.ISubmodelElement) (int, error) {
	basicEvent, ok := submodelElement.(*types.BasicEventElement)
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
func (p PostgreSQLBasicEventElementHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement types.ISubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	basicEvent, ok := submodelElement.(*types.BasicEventElement)
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
// This method handles both the common submodel element properties and the specific event
// properties such as observed references, message brokers, and timing intervals.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: idShort or hierarchical path to the element to update
//   - submodelElement: Updated element data
//   - tx: Active database transaction (can be nil, will create one if needed)
//   - isPut: true: Replaces the Submodel Element with the Body Data (Deletes non-specified fields); false: Updates only passed request body data, unspecified is ignored
//
// Returns:
//   - error: Error if update fails
func (p PostgreSQLBasicEventElementHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	basicEvent, ok := submodelElement.(*types.BasicEventElement)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type BasicEventElement")
	}

	var err error
	cu, localTx, err := persistenceutils.StartTXIfNeeded(tx, err, p.db)
	if err != nil {
		return err
	}
	defer cu(&err)
	err = p.decorated.Update(submodelID, idShortOrPath, submodelElement, localTx, isPut)
	if err != nil {
		return err
	}

	elementID, err := p.decorated.GetDatabaseID(submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	// Validate required fields
	if basicEvent.Observed() == nil {
		return common.NewErrBadRequest(fmt.Sprintf("Missing Field 'Observed' for BasicEventElement with idShortPath '%s'", idShortOrPath))
	}
	if basicEvent.Direction() == 0 {
		return common.NewErrBadRequest(fmt.Sprintf("Missing Field 'Direction' for BasicEventElement with idShortPath '%s'", idShortOrPath))
	}
	if basicEvent.State() == 0 {
		return common.NewErrBadRequest(fmt.Sprintf("Missing Field 'State' for BasicEventElement with idShortPath '%s'", idShortOrPath))
	}

	// Update with goqu
	dialect := goqu.Dialect("postgres")

	// Build the update record
	updateRecord, err := buildUpdateBasicEventElementRecordObject(basicEvent, isPut)
	if err != nil {
		return err
	}

	// Update the BasicEventElement-specific table
	updateQuery, updateArgs, err := dialect.Update("basic_event_element").
		Set(updateRecord).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return err
	}
	_, err = localTx.Exec(updateQuery, updateArgs...)
	if err != nil {
		return err
	}

	return persistenceutils.CommitTransactionIfNeeded(tx, localTx)
}

func buildUpdateBasicEventElementRecordObject(basicEvent *types.BasicEventElement, isPut bool) (goqu.Record, error) {
	updateRecord := goqu.Record{}

	// Required fields - always update
	var observedRefJson sql.NullString
	if !isEmptyReference(basicEvent.Observed()) {
		jsonable, err := jsonization.ToJsonable(basicEvent.Observed())
		if err != nil {
			return nil, common.NewErrBadRequest("SMREPO-BUBEERO-OBSJSONABLE Failed to convert observed to jsonable: " + err.Error())
		}
		observedBytes, err := json.Marshal(jsonable)
		if err != nil {
			return nil, err
		}
		observedRefJson = sql.NullString{String: string(observedBytes), Valid: true}
	}
	updateRecord["observed"] = observedRefJson
	updateRecord["direction"] = basicEvent.Direction()
	updateRecord["state"] = basicEvent.State()

	// Optional fields - update based on isPut flag
	// For PUT: always update (even if empty, which clears the field)
	// For PATCH: only update if provided (not empty)
	if isPut || !isEmptyReference(basicEvent.MessageBroker()) {
		var messageBrokerRefJson sql.NullString
		if !isEmptyReference(basicEvent.MessageBroker()) {
			jsonable, err := jsonization.ToJsonable(basicEvent.MessageBroker())
			if err != nil {
				return nil, common.NewErrBadRequest("SMREPO-BUBEERO-MBJSONABLE Failed to convert message broker to jsonable: " + err.Error())
			}
			messageBrokerBytes, err := json.Marshal(jsonable)
			if err != nil {
				return nil, err
			}
			messageBrokerRefJson = sql.NullString{String: string(messageBrokerBytes), Valid: true}
		}
		updateRecord["message_broker"] = messageBrokerRefJson
	}

	if isPut || (basicEvent.LastUpdate() != nil && *basicEvent.LastUpdate() != "") {
		var lastUpdate sql.NullString
		if basicEvent.LastUpdate() != nil && *basicEvent.LastUpdate() != "" {
			lastUpdate = sql.NullString{String: *basicEvent.LastUpdate(), Valid: true}
		}
		updateRecord["last_update"] = lastUpdate
	}

	if isPut || (basicEvent.MinInterval() != nil && *basicEvent.MinInterval() != "") {
		var minInterval sql.NullString
		if basicEvent.MinInterval() != nil && *basicEvent.MinInterval() != "" {
			minInterval = sql.NullString{String: *basicEvent.MinInterval(), Valid: true}
		}
		updateRecord["min_interval"] = minInterval
	}

	if isPut || (basicEvent.MaxInterval() != nil && *basicEvent.MaxInterval() != "") {
		var maxInterval sql.NullString
		if basicEvent.MaxInterval() != nil && *basicEvent.MaxInterval() != "" {
			maxInterval = sql.NullString{String: *basicEvent.MaxInterval(), Valid: true}
		}
		updateRecord["max_interval"] = maxInterval
	}

	if isPut || (basicEvent.MessageTopic() != nil && *basicEvent.MessageTopic() != "") {
		var messageTopic sql.NullString
		if basicEvent.MessageTopic() != nil && *basicEvent.MessageTopic() != "" {
			messageTopic = sql.NullString{String: *basicEvent.MessageTopic(), Valid: true}
		}
		updateRecord["message_topic"] = messageTopic
	}
	return updateRecord, nil
}

// UpdateValueOnly updates only the value of an existing BasicEventElement submodel element identified by its idShort or path.
// It updates the observed reference in the database.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set (must be of type gen.BasicEventElementValue)
//
// Returns:
//   - error: An error if the update operation fails or if the valueOnly type is incorrect
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

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	dialect := goqu.Dialect("postgres")

	var newObservedJson sql.NullString
	observedBytes, err := json.Marshal(basicEventValue.Observed)
	if err != nil {
		return common.NewErrBadRequest(fmt.Sprintf("failed to marshal observed value: %s", err))
	}
	newObservedJson = sql.NullString{String: string(observedBytes), Valid: true}

	// Get the element ID from the database
	var elementID int
	query, args, err := dialect.From("submodel_element").
		Select("id").
		Where(
			goqu.C("idshort_path").Eq(idShortOrPath),
			goqu.C("submodel_id").Eq(submodelID),
		).
		ToSQL()
	if err != nil {
		return err
	}

	err = tx.QueryRow(query, args...).Scan(&elementID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("BasicEventElement not found")
		}
		return err
	}

	// Update the basic_event_element table with new observed reference
	updateQuery, updateArgs, err := dialect.Update("basic_event_element").
		Set(goqu.Record{"observed": newObservedJson}).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = tx.Exec(updateQuery, updateArgs...)
	if err != nil {
		return err
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

func insertBasicEventElement(basicEvent *types.BasicEventElement, tx *sql.Tx, id int) error {
	var observedRefJson sql.NullString
	if !isEmptyReference(basicEvent.Observed()) {
		jsonable, err := jsonization.ToJsonable(basicEvent.Observed())
		if err != nil {
			return common.NewErrBadRequest("SMREPO-IBEE-OBSJSONABLE")
		}
		observedBytes, err := json.Marshal(jsonable)
		if err != nil {
			return err
		}
		observedRefJson = sql.NullString{String: string(observedBytes), Valid: true}
	}

	var messageBrokerRefJson sql.NullString
	if !isEmptyReference(basicEvent.MessageBroker()) {
		jsonable, err := jsonization.ToJsonable(basicEvent.MessageBroker())
		if err != nil {
			return common.NewErrBadRequest("SMREPO-IBEE-MBJSONABLE")
		}
		messageBrokerBytes, err := json.Marshal(jsonable)
		if err != nil {
			return err
		}
		messageBrokerRefJson = sql.NullString{String: string(messageBrokerBytes), Valid: true}
	}

	// Handle nullable fields
	var lastUpdate sql.NullString
	if basicEvent.LastUpdate() != nil && *basicEvent.LastUpdate() != "" {
		lastUpdate = sql.NullString{String: *basicEvent.LastUpdate(), Valid: true}
	}

	var minInterval sql.NullString
	if basicEvent.MinInterval() != nil && *basicEvent.MinInterval() != "" {
		minInterval = sql.NullString{String: *basicEvent.MinInterval(), Valid: true}
	}

	var maxInterval sql.NullString
	if basicEvent.MaxInterval() != nil && *basicEvent.MaxInterval() != "" {
		maxInterval = sql.NullString{String: *basicEvent.MaxInterval(), Valid: true}
	}

	var messageTopic sql.NullString
	if basicEvent.MessageTopic() != nil && *basicEvent.MessageTopic() != "" {
		messageTopic = sql.NullString{String: *basicEvent.MessageTopic(), Valid: true}
	}

	dialect := goqu.Dialect("postgres")
	insertQuery, insertArgs, err := dialect.Insert("basic_event_element").
		Rows(goqu.Record{
			"id":             id,
			"observed":       observedRefJson,
			"direction":      basicEvent.Direction(),
			"state":          basicEvent.State(),
			"message_topic":  messageTopic,
			"message_broker": messageBrokerRefJson,
			"last_update":    lastUpdate,
			"min_interval":   minInterval,
			"max_interval":   maxInterval,
		}).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = tx.Exec(insertQuery, insertArgs...)
	return err
}
