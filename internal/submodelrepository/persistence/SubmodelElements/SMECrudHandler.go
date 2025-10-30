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
	"fmt"
	"reflect"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLSMECrudHandler struct {
	db *sql.DB
}

// isEmptyReference checks if a Reference is empty (zero value)
func isEmptyReference(ref *gen.Reference) bool {
	if ref == nil {
		return true
	}
	return reflect.DeepEqual(ref, gen.Reference{})
}

func NewPostgreSQLSMECrudHandler(db *sql.DB) (*PostgreSQLSMECrudHandler, error) {
	return &PostgreSQLSMECrudHandler{db: db}, nil
}

// Create performs the base SubmodelElement operations within an existing transaction
func (p *PostgreSQLSMECrudHandler) CreateAndPath(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, position int) (int, error) {
	referenceID, err := persistence_utils.CreateReference(tx, submodelElement.GetSemanticId(), sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		return 0, err
	}
	// Check if a SubmodelElement with the same submodelId and idshort_path already exists
	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM submodel_element WHERE submodel_id = $1 AND idshort_path = $2)`,
		submodelId, idShortPath).Scan(&exists)
	if err != nil {
		return 0, err
	}

	if exists {
		return 0, fmt.Errorf("SubmodelElement with submodelId '%s' and idshort_path '%s' already exists",
			submodelId, idShortPath)
	}
	var id int
	err = tx.QueryRow(`	INSERT INTO
	 					submodel_element(submodel_id, parent_sme_id, position, id_short, category, model_type, semantic_id, idshort_path)
						VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		submodelId,
		parentId,
		position,
		submodelElement.GetIdShort(),
		submodelElement.GetCategory(),
		submodelElement.GetModelType(),
		referenceID, // This will be NULL if no semantic ID was provided
		idShortPath, // Use the provided idShortPath instead of just GetIdShort()
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	//println("Inserted SubmodelElement with idShort: " + submodelElement.GetIdShort())

	return id, nil
}

func (p *PostgreSQLSMECrudHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	referenceID, err := persistence_utils.CreateReference(tx, submodelElement.GetSemanticId(), sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		return 0, err
	}

	// Check if a SubmodelElement with the same submodelId and idshort_path already exists
	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM submodel_element WHERE submodel_id = $1 AND idshort_path = $2)`,
		submodelId, submodelElement.GetIdShort()).Scan(&exists)
	if err != nil {
		return 0, err
	}

	if exists {
		return 0, fmt.Errorf("SubmodelElement with submodelId '%s' and idshort_path '%s' already exists",
			submodelId, submodelElement.GetIdShort())
	}
	var id int
	err = tx.QueryRow(`	INSERT INTO
	 					submodel_element(submodel_id, parent_sme_id, position, id_short, category, model_type, semantic_id, idshort_path)
						VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		submodelId,
		nil,
		0,
		submodelElement.GetIdShort(),
		submodelElement.GetCategory(),
		submodelElement.GetModelType(),
		referenceID, // This will be NULL if no semantic ID was provided
		submodelElement.GetIdShort(),
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	//println("Inserted SubmodelElement with idShort: " + submodelElement.GetIdShort())

	supplSId := submodelElement.GetSupplementalSemanticIds()
	if len(supplSId) > 0 {
		err := persistence_utils.InsertSupplementalSemanticIdsSME(tx, int64(id), supplSId)
		if err != nil {
			return 0, err
		}
	}
	return id, nil
}

func (p *PostgreSQLSMECrudHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	return nil
}

func (p *PostgreSQLSMECrudHandler) Delete(idShortOrPath string) error {
	return nil
}

func (p *PostgreSQLSMECrudHandler) GetDatabaseId(idShortPath string) (int, error) {
	var id int
	err := p.db.QueryRow(`SELECT id FROM submodel_element WHERE idshort_path = $1`, idShortPath).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (p *PostgreSQLSMECrudHandler) GetNextPosition(parentId int) (int, error) {
	var position sql.NullInt64
	err := p.db.QueryRow(`SELECT MAX(position) FROM submodel_element WHERE parent_sme_id = $1`, parentId).Scan(&position)
	if err != nil {
		return 0, err
	}
	if position.Valid {
		return int(position.Int64) + 1, nil
	}
	return 0, nil // If no children exist, start at position 0
}

func (p *PostgreSQLSMECrudHandler) GetSubmodelElementType(idShortPath string) (string, error) {
	var modelType string
	err := p.db.QueryRow(`SELECT model_type FROM submodel_element WHERE idshort_path = $1`, idShortPath).Scan(&modelType)
	if err != nil {
		return "", err
	}
	return modelType, nil
}
