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

// Create persists a new Operation submodel element to the database within a transaction.
// It first creates the base submodel element using the decorated handler, then inserts
// Operation-specific data including all input, output, and in-output variables. Each
// variable's value (which is itself a submodel element) is recursively persisted.
//
// Parameters:
//   - tx: The database transaction
//   - submodelID: The ID of the parent submodel
//   - submodelElement: The Operation element to create (must be of type *gen.Operation)
//
// Returns:
//   - int: The database ID of the created element
//   - error: An error if the element is not an Operation type or if database operations fail
func (p PostgreSQLOperationHandler) Create(tx *sql.Tx, submodelID string, submodelElement types.ISubmodelElement) (int, error) {
	operation, ok := submodelElement.(*types.Operation)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type Operation")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// Operation-specific database insertion
	err = insertOperation(operation, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested persists a new nested Operation submodel element to the database within a transaction.
// This method is used when creating Operation elements within collection-like structures (e.g., SubmodelElementCollection).
// It creates the base nested element with the provided idShortPath and position, then inserts Operation-specific
// data including all variables.
//
// Parameters:
//   - tx: The database transaction
//   - submodelID: The ID of the parent submodel
//   - parentID: The database ID of the parent collection element
//   - idShortPath: The path identifying the element's location within the hierarchy
//   - submodelElement: The Operation element to create (must be of type *gen.Operation)
//   - pos: The position of the element within the parent collection
//
// Returns:
//   - int: The database ID of the created element
//   - error: An error if the element is not an Operation type or if database operations fail
func (p PostgreSQLOperationHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement types.ISubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	operation, ok := submodelElement.(*types.Operation)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type Operation")
	}

	// Create the nested operation with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	// Operation-specific database insertion for nested element
	err = insertOperation(operation, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
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

	// Build update record based on isPut flag
	// For PUT: always update all fields (even if nil/empty, which clears them)
	// For PATCH: only update fields that are provided (not nil)

	updateRecord, err := buildUpdateOperationRecordObject(isPut, operation, json)
	if err != nil {
		return err
	}

	// Only execute update if there are fields to update
	if persistenceutils.AnyFieldsToUpdate(updateRecord) {
		dialect := goqu.Dialect("postgres")
		updateQuery, updateArgs, err := dialect.Update("operation_element").
			Set(updateRecord).
			Where(goqu.C("id").In(
				dialect.From("submodel_element").
					Select("id").
					Where(goqu.Ex{
						"idshort_path": idShortOrPath,
						"submodel_id":  submodelID,
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

// insertOperation is a helper function that inserts Operation-specific data into the operation_element table.
// It creates the operation record and then inserts all associated variables (input, output, and in-output).
// Each variable's value is persisted as a nested submodel element.
//
// Parameters:
//   - operation: The Operation element containing the data to insert
//   - tx: The database transaction
//   - id: The database ID of the parent submodel element
//   - submodelID: The ID of the parent submodel (needed for variable value persistence)
//   - db: The database connection (needed to get appropriate handlers for variable values)
//
// Returns:
//   - error: An error if the database insert operation fails
func insertOperation(operation *types.Operation, tx *sql.Tx, id int) error {
	json := jsoniter.ConfigCompatibleWithStandardLibrary

	var inputVars, outputVars, inoutputVars string
	if operation.InputVariables() != nil {
		inputVarBytes, err := json.Marshal(operation.InputVariables())
		if err != nil {
			return err
		}
		inputVars = string(inputVarBytes)
	} else {
		inputVars = "[]"
	}

	if operation.OutputVariables() != nil {
		outputVarBytes, err := json.Marshal(operation.OutputVariables())
		if err != nil {
			return err
		}
		outputVars = string(outputVarBytes)
	} else {
		outputVars = "[]"
	}

	if operation.InoutputVariables() != nil {
		inoutputVarBytes, err := json.Marshal(operation.InoutputVariables())
		if err != nil {
			return err
		}
		inoutputVars = string(inoutputVarBytes)
	} else {
		inoutputVars = "[]"
	}

	dialect := goqu.Dialect("postgres")
	insertQuery, insertArgs, err := dialect.Insert("operation_element").
		Rows(goqu.Record{
			"id":                 id,
			"input_variables":    inputVars,
			"output_variables":   outputVars,
			"inoutput_variables": inoutputVars,
		}).
		ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(insertQuery, insertArgs...)
	if err != nil {
		return err
	}
	return nil
}

func buildUpdateOperationRecordObject(isPut bool, operation *types.Operation, json jsoniter.API) (goqu.Record, error) {
	updateRecord := goqu.Record{}

	if isPut || operation.InputVariables() != nil {
		var inputVars string
		if operation.InputVariables() != nil {
			inputVarBytes, err := json.Marshal(operation.InputVariables())
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
			outputVarBytes, err := json.Marshal(operation.OutputVariables())
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
			inoutputVarBytes, err := json.Marshal(operation.InoutputVariables())
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
