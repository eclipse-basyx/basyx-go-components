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

type PostgreSQLRelationshipElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLRelationshipElementHandler(db *sql.DB) (*PostgreSQLRelationshipElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLRelationshipElementHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLRelationshipElementHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	relElem, ok := submodelElement.(*gen.RelationshipElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type RelationshipElement")
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

func (p PostgreSQLRelationshipElementHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	relElem, ok := submodelElement.(*gen.RelationshipElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type RelationshipElement")
	}

	// Create the nested relElem with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelID, parentID, idShortPath, submodelElement, pos)
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

func (p PostgreSQLRelationshipElementHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLRelationshipElementHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertRelationshipElement(relElem *gen.RelationshipElement, tx *sql.Tx, id int) error {
	var firstRefID, secondRefID sql.NullInt64

	if !isEmptyReference(relElem.First) {
		refID, err := insertReference(tx, *relElem.First)
		if err != nil {
			return err
		}
		firstRefID = sql.NullInt64{Int64: int64(refID), Valid: true}
	}

	if !isEmptyReference(relElem.Second) {
		refID, err := insertReference(tx, *relElem.Second)
		if err != nil {
			return err
		}
		secondRefID = sql.NullInt64{Int64: int64(refID), Valid: true}
	}

	_, err := tx.Exec(`INSERT INTO relationship_element (id, first_ref, second_ref) VALUES ($1, $2, $3)`,
		id, firstRefID, secondRefID)
	return err
}

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
