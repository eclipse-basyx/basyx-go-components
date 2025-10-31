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

type PostgreSQLBasicEventElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLBasicEventElementHandler(db *sql.DB) (*PostgreSQLBasicEventElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLBasicEventElementHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLBasicEventElementHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	basicEvent, ok := submodelElement.(*gen.BasicEventElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type BasicEventElement")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
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

func (p PostgreSQLBasicEventElementHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	basicEvent, ok := submodelElement.(*gen.BasicEventElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type BasicEventElement")
	}

	// Create the nested basic event element with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
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

func (p PostgreSQLBasicEventElementHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLBasicEventElementHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
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
