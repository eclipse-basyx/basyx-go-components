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

func (p PostgreSQLOperationHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	// First, get the base submodel element
	var baseSME gen.SubmodelElement
	id, err := p.decorated.Read(tx, submodelId, idShortOrPath, &baseSME)
	if err != nil {
		return nil, err
	}

	// Check if it's an operation
	operation, ok := baseSME.(*gen.Operation)
	if !ok {
		return nil, errors.New("submodelElement is not of type Operation")
	}

	// Query operation variables
	rows, err := tx.Query(`SELECT role, position, value_sme FROM operation_variable WHERE operation_id = $1 ORDER BY position`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var inputVars, outputVars, inoutputVars []gen.OperationVariable
	for rows.Next() {
		var role string
		var position int
		var valueSmeId int
		err := rows.Scan(&role, &position, &valueSmeId)
		if err != nil {
			return nil, err
		}

		// Get the idshort_path and model_type for the value SME
		var valueIdShortPath, valueModelType string
		err = tx.QueryRow(`SELECT idshort_path, model_type FROM submodel_element WHERE id = $1`, valueSmeId).Scan(&valueIdShortPath, &valueModelType)
		if err != nil {
			return nil, err
		}

		// Get the handler for the value SME
		handler, err := GetSMEHandlerByModelType(valueModelType, p.db)
		if err != nil {
			return nil, err
		}

		// Read the value submodel element
		valueSme, err := handler.Read(tx, submodelId, valueIdShortPath)
		if err != nil {
			return nil, err
		}

		ov := gen.OperationVariable{Value: valueSme}
		switch role {
		case "in":
			inputVars = append(inputVars, ov)
		case "out":
			outputVars = append(outputVars, ov)
		case "inout":
			inoutputVars = append(inoutputVars, ov)
		}
	}

	operation.InputVariables = inputVars
	operation.OutputVariables = outputVars
	operation.InoutputVariables = inoutputVars

	return operation, nil
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
