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

func (p PostgreSQLRelationshipElementHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	relElem, ok := submodelElement.(*gen.RelationshipElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type RelationshipElement")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
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

func (p PostgreSQLRelationshipElementHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	relElem, ok := submodelElement.(*gen.RelationshipElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type RelationshipElement")
	}

	// Create the nested relElem with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
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

func (p PostgreSQLRelationshipElementHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	var sme gen.SubmodelElement = &gen.RelationshipElement{}
	var firstRef, secondRef sql.NullInt64
	id, err := p.decorated.Read(tx, submodelId, idShortOrPath, &sme)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRow(`SELECT first_ref, second_ref FROM relationship_element WHERE id = $1`, id).Scan(&firstRef, &secondRef)
	if err != nil {
		return sme, nil
	}
	relElem := sme.(*gen.RelationshipElement)
	if firstRef.Valid {
		ref, err := readReference(tx, firstRef.Int64)
		if err != nil {
			return nil, err
		}
		relElem.First = ref
	}
	if secondRef.Valid {
		ref, err := readReference(tx, secondRef.Int64)
		if err != nil {
			return nil, err
		}
		relElem.Second = ref
	}
	return sme, nil
}

func readReference(tx *sql.Tx, refId int64) (*gen.Reference, error) {
	var refType string
	err := tx.QueryRow(`SELECT type FROM reference WHERE id = $1`, refId).Scan(&refType)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(`SELECT type, value FROM reference_key WHERE reference_id = $1 ORDER BY position`, refId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []gen.Key
	for rows.Next() {
		var kType, kValue string
		if err := rows.Scan(&kType, &kValue); err != nil {
			return nil, err
		}
		keys = append(keys, gen.Key{Type: gen.KeyTypes(kType), Value: kValue})
	}
	return &gen.Reference{Type: gen.ReferenceTypes(refType), Keys: keys}, nil
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
	var firstRefId, secondRefId sql.NullInt64

	if !isEmptyReference(relElem.First) {
		refId, err := insertReference(tx, *relElem.First)
		if err != nil {
			return err
		}
		firstRefId = sql.NullInt64{Int64: int64(refId), Valid: true}
	}

	if !isEmptyReference(relElem.Second) {
		refId, err := insertReference(tx, *relElem.Second)
		if err != nil {
			return err
		}
		secondRefId = sql.NullInt64{Int64: int64(refId), Valid: true}
	}

	_, err := tx.Exec(`INSERT INTO relationship_element (id, first_ref, second_ref) VALUES ($1, $2, $3)`,
		id, firstRefId, secondRefId)
	return err
}

func insertReference(tx *sql.Tx, ref gen.Reference) (int, error) {
	var refId int
	err := tx.QueryRow(`INSERT INTO reference (type) VALUES ($1) RETURNING id`, ref.Type).Scan(&refId)
	if err != nil {
		return 0, err
	}
	for i, key := range ref.Keys {
		_, err = tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`,
			refId, i, key.Type, key.Value)
		if err != nil {
			return 0, err
		}
	}
	return refId, nil
}
