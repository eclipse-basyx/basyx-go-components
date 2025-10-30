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

type PostgreSQLEntityHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLEntityHandler(db *sql.DB) (*PostgreSQLEntityHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLEntityHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLEntityHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	entity, ok := submodelElement.(*gen.Entity)
	if !ok {
		return 0, errors.New("submodelElement is not of type Entity")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
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

func (p PostgreSQLEntityHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	entity, ok := submodelElement.(*gen.Entity)
	if !ok {
		return 0, errors.New("submodelElement is not of type Entity")
	}

	// Create the nested entity with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
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

func (p PostgreSQLEntityHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLEntityHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertEntity(entity *gen.Entity, tx *sql.Tx, id int) error {
	_, err := tx.Exec(`INSERT INTO entity_element (id, entity_type, global_asset_id) VALUES ($1, $2, $3)`,
		id, entity.EntityType, entity.GlobalAssetId)
	if err != nil {
		return err
	}

	// Insert specific asset ids
	for _, sai := range entity.SpecificAssetIds {
		var extRef sql.NullInt64
		if !isEmptyReference(sai.ExternalSubjectId) {
			refId, err := insertReference(tx, *sai.ExternalSubjectId)
			if err != nil {
				return err
			}
			extRef = sql.NullInt64{Int64: int64(refId), Valid: true}
		}
		_, err = tx.Exec(`INSERT INTO entity_specific_asset_id (entity_id, name, value, external_subject_ref) VALUES ($1, $2, $3, $4)`,
			id, sai.Name, sai.Value, extRef)
		if err != nil {
			return err
		}
	}
	return nil
}
