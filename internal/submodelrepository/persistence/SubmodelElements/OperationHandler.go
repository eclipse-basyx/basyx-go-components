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

type PostgreSQLOperationHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLOperationHandler(db *sql.DB) (*PostgreSQLOperationHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLOperationHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLOperationHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	operation, ok := submodelElement.(*gen.Operation)
	if !ok {
		return 0, errors.New("submodelElement is not of type Operation")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// Operation-specific database insertion
	err = insertOperation(operation, tx, id, submodelId, p.db)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLOperationHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	operation, ok := submodelElement.(*gen.Operation)
	if !ok {
		return 0, errors.New("submodelElement is not of type Operation")
	}

	// Create the nested operation with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// Operation-specific database insertion for nested element
	err = insertOperation(operation, tx, id, submodelId, p.db)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLOperationHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLOperationHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertOperation(operation *gen.Operation, tx *sql.Tx, id int, submodelId string, db *sql.DB) error {
	_, err := tx.Exec(`INSERT INTO operation_element (id) VALUES ($1)`, id)
	if err != nil {
		return err
	}

	// Insert variables
	err = insertOperationVariables(tx, operation.InputVariables, "in", id, submodelId, db)
	if err != nil {
		return err
	}
	err = insertOperationVariables(tx, operation.OutputVariables, "out", id, submodelId, db)
	if err != nil {
		return err
	}
	err = insertOperationVariables(tx, operation.InoutputVariables, "inout", id, submodelId, db)
	if err != nil {
		return err
	}
	return nil
}

func insertOperationVariables(tx *sql.Tx, variables []gen.OperationVariable, role string, operationId int, submodelId string, db *sql.DB) error {
	for i, ov := range variables {
		// Create the value submodel element
		handler, err := GetSMEHandler(ov.Value, db)
		if err != nil {
			return err
		}
		valueId, err := handler.Create(tx, submodelId, ov.Value)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO operation_variable (operation_id, role, position, value_sme) VALUES ($1, $2, $3, $4)`,
			operationId, role, i, valueId)
		if err != nil {
			return err
		}
	}
	return nil
}
