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

type PostgreSQLReferenceElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLReferenceElementHandler(db *sql.DB) (*PostgreSQLReferenceElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLReferenceElementHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLReferenceElementHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	refElem, ok := submodelElement.(*gen.ReferenceElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type ReferenceElement")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// ReferenceElement-specific database insertion
	err = insertReferenceElement(refElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLReferenceElementHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	refElem, ok := submodelElement.(*gen.ReferenceElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type ReferenceElement")
	}

	// Create the nested refElem with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelID, parentID, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// ReferenceElement-specific database insertion for nested element
	err = insertReferenceElement(refElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLReferenceElementHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLReferenceElementHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertReferenceElement(refElem *gen.ReferenceElement, tx *sql.Tx, id int) error {
	if isEmptyReference(refElem.Value) {
		// Insert with NULL
		_, err := tx.Exec(`INSERT INTO reference_element (id, value_ref) VALUES ($1, $2)`, id, nil)
		return err
	}

	// Insert the reference
	var refID int
	err := tx.QueryRow(`INSERT INTO reference (type) VALUES ($1) RETURNING id`, refElem.Value.Type).Scan(&refID)
	if err != nil {
		return err
	}

	// Insert reference keys
	for i, key := range refElem.Value.Keys {
		_, err = tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`,
			refID, i, key.Type, key.Value)
		if err != nil {
			return err
		}
	}

	// Insert reference_element
	_, err = tx.Exec(`INSERT INTO reference_element (id, value_ref) VALUES ($1, $2)`, id, refID)
	return err
}
