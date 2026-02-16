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

// Package submodelelements provides persistence handlers for various submodel element types
// in the Eclipse BaSyx submodel repository. It implements CRUD operations for different
// submodel element types such as Range, Property, Collection, and others, with PostgreSQL
// as the underlying database.
//
// Author: Jannik Fried ( Fraunhofer IESE )
package submodelelements

import (
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	jsoniter "github.com/json-iterator/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLOperationHandler handles persistence operations for Operation submodel elements.
// It uses the decorator pattern to extend the base PostgreSQLSMECrudHandler with
// Operation-specific functionality. Operation elements represent callable functions with
// input, output, and in-output variables, each containing submodel elements as values.
type PostgreSQLOperationHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLOperationHandler creates a new PostgreSQLOperationHandler instance.
// It initializes the decorated PostgreSQLSMECrudHandler for base submodel element operations.
//
// Parameters:
//   - db: A PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLOperationHandler: A new handler instance
//   - error: An error if the decorated handler initialization fails
func NewPostgreSQLOperationHandler(db *sql.DB) (*PostgreSQLOperationHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLOperationHandler{db: db, decorated: decoratedHandler}, nil
}

// Update modifies an existing Operation submodel element in the database.
// This method handles both the common submodel element properties and the specific
// operation data including input, output, and in-output variables.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - submodelElement: The updated Operation element data
//   - tx: Active database transaction (can be nil, will create one if needed)
//   - isPut: true: Replaces the element with the body data; false: Updates only passed data
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLOperationHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	operation, ok := submodelElement.(*types.Operation)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type Operation")
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

	json := jsoniter.ConfigCompatibleWithStandardLibrary
	smDbID, err := persistenceutils.GetSubmodelDatabaseID(localTx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("submodel not found")
		}
		return err
	}

	// Build update record based on isPut flag
	// For PUT: always update all fields (even if nil/empty, which clears them)
	// For PATCH: only update fields that are provided (not nil)

	updateRecord, err := buildUpdateOperationRecordObject(isPut, operation, json)
	if err != nil {
		return err
	}

	// Only execute update if there are fields to update
	if anyFieldsToUpdate(updateRecord) {
		dialect := goqu.Dialect("postgres")
		updateQuery, updateArgs, err := dialect.Update("operation_element").
			Set(updateRecord).
			Where(goqu.C("id").In(
				dialect.From("submodel_element").
					Select("id").
					Where(goqu.Ex{
						"idshort_path": idShortOrPath,
						"submodel_id":  smDbID,
					}),
			)).
			ToSQL()
		if err != nil {
			return err
		}

		_, err = localTx.Exec(updateQuery, updateArgs...)
		if err != nil {
			return err
		}
	}

	return persistenceutils.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly updates only the value of an existing Operation submodel element identified by its idShort or path.
// Operation has no Value Only representation, so this method currently performs no action and returns nil.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set (must be of type types.ISubmodelElementValue)
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLOperationHandler) UpdateValueOnly(_ string, _ string, _ gen.SubmodelElementValue) error {
	return nil
}

// Delete removes an Operation submodel element from the database.
// Currently delegates all delete operations to the decorated handler. Operation-specific data
// (including variables and their values) is typically removed via database cascade constraints.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifying the element to delete
//
// Returns:
//   - error: An error if the delete operation fails
func (p PostgreSQLOperationHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

// GetInsertQueryPart returns the type-specific insert query part for batch insertion of Operation elements.
// It returns the table name and record for inserting into the operation_element table.
//
// Parameters:
//   - tx: Active database transaction (not used for Operation)
//   - id: The database ID of the base submodel_element record
//   - element: The Operation element to insert
//
// Returns:
//   - *InsertQueryPart: The table name and record for operation_element insert
//   - error: An error if the element is not of type Operation
func (p PostgreSQLOperationHandler) GetInsertQueryPart(_ *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error) {
	operation, ok := element.(*types.Operation)
	if !ok {
		return nil, common.NewErrBadRequest("submodelElement is not of type Operation")
	}

	json := jsoniter.ConfigCompatibleWithStandardLibrary

	var inputVars, outputVars, inoutputVars string
	if operation.InputVariables() != nil {
		var jsonables []map[string]any
		for _, v := range operation.InputVariables() {
			jsonable, err := jsonization.ToJsonable(v)
			if err != nil {
				return nil, err
			}
			jsonables = append(jsonables, jsonable)
		}
		inputVarBytes, err := json.Marshal(jsonables)
		if err != nil {
			return nil, err
		}
		inputVars = string(inputVarBytes)
	} else {
		inputVars = "[]"
	}

	if operation.OutputVariables() != nil {
		var jsonables []map[string]any
		for _, v := range operation.OutputVariables() {
			jsonable, err := jsonization.ToJsonable(v)
			if err != nil {
				return nil, err
			}
			jsonables = append(jsonables, jsonable)
		}
		outputVarBytes, err := json.Marshal(jsonables)
		if err != nil {
			return nil, err
		}
		outputVars = string(outputVarBytes)
	} else {
		outputVars = "[]"
	}

	if operation.InoutputVariables() != nil {
		var jsonables []map[string]any
		for _, v := range operation.InoutputVariables() {
			jsonable, err := jsonization.ToJsonable(v)
			if err != nil {
				return nil, err
			}
			jsonables = append(jsonables, jsonable)
		}
		inoutputVarBytes, err := json.Marshal(jsonables)
		if err != nil {
			return nil, err
		}
		inoutputVars = string(inoutputVarBytes)
	} else {
		inoutputVars = "[]"
	}

	return &InsertQueryPart{
		TableName: "operation_element",
		Record: goqu.Record{
			"id":                 id,
			"input_variables":    inputVars,
			"output_variables":   outputVars,
			"inoutput_variables": inoutputVars,
		},
	}, nil
}

func buildUpdateOperationRecordObject(isPut bool, operation *types.Operation, json jsoniter.API) (goqu.Record, error) {
	updateRecord := goqu.Record{}

	if isPut || operation.InputVariables() != nil {
		var inputVars string
		if operation.InputVariables() != nil {
			var jsonables []map[string]any
			for _, v := range operation.InputVariables() {
				jsonable, err := jsonization.ToJsonable(v)
				if err != nil {
					return nil, err
				}
				jsonables = append(jsonables, jsonable)
			}
			inputVarBytes, err := json.Marshal(jsonables)
			if err != nil {
				return nil, err
			}
			inputVars = string(inputVarBytes)
		} else {
			inputVars = "[]"
		}
		updateRecord["input_variables"] = inputVars
	}

	if isPut || operation.OutputVariables() != nil {
		var outputVars string
		if operation.OutputVariables() != nil {
			var jsonables []map[string]any
			for _, v := range operation.OutputVariables() {
				jsonable, err := jsonization.ToJsonable(v)
				if err != nil {
					return nil, err
				}
				jsonables = append(jsonables, jsonable)
			}
			outputVarBytes, err := json.Marshal(jsonables)
			if err != nil {
				return nil, err
			}
			outputVars = string(outputVarBytes)
		} else {
			outputVars = "[]"
		}
		updateRecord["output_variables"] = outputVars
	}

	if isPut || operation.InoutputVariables() != nil {
		var inoutputVars string
		if operation.InoutputVariables() != nil {
			var jsonables []map[string]any
			for _, v := range operation.InoutputVariables() {
				jsonable, err := jsonization.ToJsonable(v)
				if err != nil {
					return nil, err
				}
				jsonables = append(jsonables, jsonable)
			}
			inoutputVarBytes, err := json.Marshal(jsonables)
			if err != nil {
				return nil, err
			}
			inoutputVars = string(inoutputVarBytes)
		} else {
			inoutputVars = "[]"
		}
		updateRecord["inoutput_variables"] = inoutputVars
	}
	return updateRecord, nil
}
