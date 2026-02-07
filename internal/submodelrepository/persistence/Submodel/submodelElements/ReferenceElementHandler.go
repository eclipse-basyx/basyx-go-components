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
// Author: Jannik Fried ( Fraunhofer IESE )

// Package submodelelements provides persistence handlers for Asset Administration Shell (AAS) submodel elements
// in Eclipse BaSyx. This package implements the repository pattern for storing and retrieving various types of
// submodel elements in a PostgreSQL database, including their hierarchical relationships, metadata, and type-specific
// attributes.
//
// The package supports all AAS submodel element types defined in the specification, such as Properties, Collections,
// Lists, RelationshipElements, ReferenceElements, and more. Each handler uses a decorator pattern to add type-specific
// functionality on top of the base CRUD operations provided by PostgreSQLSMECrudHandler.
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

// PostgreSQLReferenceElementHandler is a persistence handler for ReferenceElement submodel elements.
// It implements the decorator pattern by wrapping PostgreSQLSMECrudHandler to add ReferenceElement-specific
// persistence logic for managing references to other AAS elements or external resources.
//
// A ReferenceElement contains a value that is a Reference, which consists of a type and a list of keys
// that together identify a specific element within an AAS environment or an external resource.
//
// The handler manages:
//   - Base SubmodelElement attributes (via decorated handler)
//   - Reference value with type and keys
//   - Null reference handling for empty values
//   - Key ordering and position management
//
// Database structure:
//   - reference_element table: Links submodel element ID to reference ID
//   - reference table: Stores reference type
//   - reference_key table: Stores ordered keys with positions
type PostgreSQLReferenceElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLReferenceElementHandler creates a new ReferenceElementHandler with the specified database connection.
// It initializes the decorated PostgreSQLSMECrudHandler for base SubmodelElement operations.
//
// Parameters:
//   - db: Active PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLReferenceElementHandler: Initialized handler instance
//   - error: Any error encountered during handler initialization
func NewPostgreSQLReferenceElementHandler(db *sql.DB) (*PostgreSQLReferenceElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLReferenceElementHandler{db: db, decorated: decoratedHandler}, nil
}

// Update modifies an existing ReferenceElement identified by its idShort or full path.
// This method handles both the common submodel element properties and the specific
// reference element data including the reference value.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: The idShort or full path of the ReferenceElement to update
//   - submodelElement: The updated ReferenceElement with new values
//   - tx: Active database transaction (can be nil, will create one if needed)
//   - isPut: true: Replaces the element with the body data; false: Updates only passed data
//
// Returns:
//   - error: Any error encountered during the update operation
func (p PostgreSQLReferenceElementHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	refElem, ok := submodelElement.(*types.ReferenceElement)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type ReferenceElement")
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

	elementID, err := p.decorated.GetDatabaseID(submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	// Handle optional Value field based on isPut flag
	// For PUT: always update (even if nil, which clears the field)
	// For PATCH: only update if provided (not nil)
	if isPut || refElem.Value() != nil {
		var referenceJSONString sql.NullString
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		if refElem.Value() != nil && !isEmptyReference(refElem.Value()) {
			jsonable, err := jsonization.ToJsonable(refElem.Value())
			if err != nil {
				return common.NewErrBadRequest("SMREPO-REFU-JSONABLE Failed to convert reference to jsonable: " + err.Error())
			}
			bytes, err := json.Marshal(jsonable)
			if err != nil {
				return err
			}
			referenceJSONString = sql.NullString{String: string(bytes), Valid: true}
		} else {
			referenceJSONString = sql.NullString{Valid: false}
		}

		// Update reference_element table
		dialect := goqu.Dialect("postgres")
		updateQuery, updateArgs, err := dialect.Update("reference_element").
			Set(goqu.Record{
				"value": referenceJSONString,
			}).
			Where(goqu.C("id").Eq(elementID)).
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

// UpdateValueOnly updates only the value of an existing ReferenceElement identified by its idShort or full path.
// This method specifically updates the reference value (type and keys) without modifying other attributes.
//
// The method performs the following operations:
//  1. Type assertion to ensure the valueOnly is a ReferenceElementValue
//  2. Marshals the reference value to JSON for database storage
//  3. Constructs and executes an update query to modify only the value field
//
// Parameters:
//   - idShortOrPath: The idShort or full path of the ReferenceElement to update
//   - valueOnly: The new ReferenceElementValue containing updated reference data
//
// Returns:
//   - error: Any error encountered during type assertion, marshaling, or database operations
func (p PostgreSQLReferenceElementHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	refElemVal, ok := valueOnly.(gen.ReferenceElementValue)
	if !ok {
		return common.NewErrBadRequest("valueOnly is not of type ReferenceElementValue")
	}

	// Marshal reference value to JSON using helper function
	referenceJSONString, err := marshalReferenceValueToJSON(refElemVal)
	if err != nil {
		return err
	}

	// Build and execute update query using GoQu
	query, args, err := goqu.Update("reference_element").
		Set(goqu.Record{
			"value": referenceJSONString,
		}).
		Where(goqu.I("id").Eq(
			goqu.From("submodel_element").
				Select("id").
				Where(goqu.Ex{
					"submodel_id":  submodelID,
					"idshort_path": idShortOrPath,
				}),
		)).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = p.db.Exec(query, args...)
	return err
}

// Delete removes a ReferenceElement identified by its idShort or full path from the database.
// Parameters:
//   - idShortOrPath: The idShort or full path of the ReferenceElement to delete
//
// Returns:
//   - error: Any error encountered during the deletion operation
//
// Note: Database foreign key constraints ensure cascading deletion of related records.
func (p PostgreSQLReferenceElementHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

// GetInsertQueryPart returns the type-specific insert query part for batch insertion of ReferenceElement elements.
// It returns the table name and record for inserting into the reference_element table.
//
// Parameters:
//   - tx: Active database transaction (not used for ReferenceElement)
//   - id: The database ID of the base submodel_element record
//   - element: The ReferenceElement element to insert
//
// Returns:
//   - *InsertQueryPart: The table name and record for reference_element insert
//   - error: An error if the element is not of type ReferenceElement
func (p PostgreSQLReferenceElementHandler) GetInsertQueryPart(_ *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error) {
	refElem, ok := element.(*types.ReferenceElement)
	if !ok {
		return nil, common.NewErrBadRequest("submodelElement is not of type ReferenceElement")
	}

	var referenceJSONString sql.NullString
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if !isEmptyReference(refElem.Value()) {
		jsonable, err := jsonization.ToJsonable(refElem.Value())
		if err != nil {
			return nil, common.NewErrBadRequest("SMREPO-GIQP-REFELEM-JSONABLE Failed to convert reference to jsonable: " + err.Error())
		}
		bytes, err := json.Marshal(jsonable)
		if err != nil {
			return nil, err
		}
		referenceJSONString = sql.NullString{String: string(bytes), Valid: true}
	} else {
		referenceJSONString = sql.NullString{Valid: false}
	}

	return &InsertQueryPart{
		TableName: "reference_element",
		Record: goqu.Record{
			"id":    id,
			"value": referenceJSONString,
		},
	}, nil
}

// marshalReferenceValueToJSON converts a ReferenceElementValue to a JSON string for database storage.
// Returns a sql.NullString with Valid=true if the reference has keys, otherwise Valid=false.
func marshalReferenceValueToJSON(refElemVal gen.ReferenceElementValue) (sql.NullString, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var referenceJSONString sql.NullString

	if len(refElemVal.Keys) > 0 {
		bytes, err := json.Marshal(refElemVal)
		if err != nil {
			return sql.NullString{}, err
		}
		referenceJSONString = sql.NullString{String: string(bytes), Valid: true}
	} else {
		referenceJSONString = sql.NullString{Valid: false}
	}

	return referenceJSONString, nil
}
