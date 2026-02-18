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
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLPropertyHandler handles persistence operations for Property submodel elements.
// It uses the decorator pattern to extend the base PostgreSQLSMECrudHandler with
// Property-specific functionality. Property elements represent single data values with
// a defined value type (string, numeric, boolean, time, or datetime).
type PostgreSQLPropertyHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLPropertyHandler creates a new PostgreSQLPropertyHandler instance.
// It initializes the decorated PostgreSQLSMECrudHandler for base submodel element operations.
//
// Parameters:
//   - db: A PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLPropertyHandler: A new handler instance
//   - error: An error if the decorated handler initialization fails
func NewPostgreSQLPropertyHandler(db *sql.DB) (*PostgreSQLPropertyHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLPropertyHandler{db: db, decorated: decoratedHandler}, nil
}

// Update modifies an existing Property submodel element in the database.
// This method handles both the common submodel element properties and the specific
// property data including value type, value, and value ID reference.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - submodelElement: The updated Property element data
//   - tx: Active database transaction (can be nil, will create one if needed)
//   - isPut: true: Replaces the element with the body data; false: Updates only passed data
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLPropertyHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	property, ok := submodelElement.(*types.Property)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type Property")
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
	effectivePath := resolveUpdatedPath(idShortOrPath, submodelElement, isPut)

	// Get the element ID
	smDbID, err := persistenceutils.GetSubmodelDatabaseID(localTx, submodelID)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}
	elementID, err := p.decorated.GetDatabaseIDWithTx(localTx, smDbID, effectivePath)
	if err != nil {
		return err
	}

	// Build the update record
	updateRecord, err := buildUpdatePropertyRecordObject(property, isPut, localTx)
	if err != nil {
		return err
	}

	// Update property_element table
	updateQuery, updateArgs, err := goqu.Dialect("postgres").
		Update("property_element").
		Set(updateRecord).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = localTx.Exec(updateQuery, updateArgs...)
	if err != nil {
		return err
	}

	if isPut || property.ValueID() != nil {
		valueIDPayload := "[]"
		if property.ValueID() != nil && !isEmptyReference(property.ValueID()) {
			valueIDJSONString, serErr := serializeIClassSliceToJSON([]types.IClass{property.ValueID()}, "SMREPO-PROP-UPDATE-VALREFJSONIZATION")
			if serErr != nil {
				return serErr
			}
			valueIDPayload = valueIDJSONString
		}

		ensurePayloadQuery, ensurePayloadArgs, ensurePayloadErr := goqu.Dialect("postgres").Insert("property_element_payload").
			Rows(goqu.Record{"property_element_id": elementID}).
			OnConflict(goqu.DoNothing()).
			ToSQL()
		if ensurePayloadErr != nil {
			return ensurePayloadErr
		}

		_, err = localTx.Exec(ensurePayloadQuery, ensurePayloadArgs...)
		if err != nil {
			return err
		}

		updatePayloadQuery, updatePayloadArgs, updatePayloadErr := goqu.Dialect("postgres").Update("property_element_payload").
			Set(goqu.Record{
				"value_id_payload": goqu.L("?::jsonb", valueIDPayload),
			}).
			Where(goqu.C("property_element_id").Eq(elementID)).
			ToSQL()
		if updatePayloadErr != nil {
			return updatePayloadErr
		}

		_, err = localTx.Exec(updatePayloadQuery, updatePayloadArgs...)
		if err != nil {
			return err
		}
	}

	return persistenceutils.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly updates only the value of an existing Property submodel element identified by its idShort or path.
// It categorizes the new value based on the property's value type and updates the corresponding database columns.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set (must be of type gen.SubmodelElementValue)
//
// Returns:
//   - error: An error if the update operation fails or if the valueOnly type is incorrect
func (p PostgreSQLPropertyHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	smDbID, err := persistenceutils.GetSubmodelDatabaseIDFromDB(p.db, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound(fmt.Sprintf("Submodel with ID %s not found", submodelID))
		}
		return err
	}

	var elementID int
	goquQuery, args, err := goqu.From("submodel_element").
		Select("id").
		Where(goqu.Ex{
			"submodel_id":  smDbID,
			"idshort_path": idShortOrPath,
		}).ToSQL()
	if err != nil {
		return err
	}

	row := p.db.QueryRow(goquQuery, args...)
	err = row.Scan(&elementID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound(fmt.Sprintf("Property element not found for the given idShortOrPath %s", idShortOrPath))
		}
		return err
	}

	goquQuery, args, err = goqu.From("property_element").Select("value_type").Where(goqu.C("id").Eq(elementID)).ToSQL()
	if err != nil {
		return err
	}
	var valueType types.DataTypeDefXSD
	row = p.db.QueryRow(goquQuery, args...)
	err = row.Scan(&valueType)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound(fmt.Sprintf("Property element not found for the given idShortOrPath %s", idShortOrPath))
		}
		return err
	}
	// Update based on valueType using centralized value type mapper
	propertyValue, ok := valueOnly.(gen.PropertyValue)
	if !ok {
		return common.NewErrBadRequest("valueOnly is not of type PropertyValue")
	}

	typedValue := MapValueByType(valueType, &propertyValue.Value)

	dialect := goqu.Dialect("postgres")
	updateQuery, updateArgs, err := dialect.Update("property_element").
		Set(goqu.Record{
			"value_text":     typedValue.Text,
			"value_num":      typedValue.Numeric,
			"value_bool":     typedValue.Boolean,
			"value_time":     typedValue.Time,
			"value_date":     typedValue.Date,
			"value_datetime": typedValue.DateTime,
		}).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = p.db.Exec(updateQuery, updateArgs...)
	if err != nil {
		return err
	}

	return nil
}

// Delete removes a Property submodel element from the database.
// Currently delegates all delete operations to the decorated handler. Property-specific data
// is typically removed via database cascade constraints.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifying the element to delete
//
// Returns:
//   - error: An error if the delete operation fails
func (p PostgreSQLPropertyHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

// GetInsertQueryPart returns the type-specific insert query part for batch insertion of Property elements.
//
// Parameters:
//   - tx: Active database transaction (needed for creating value references)
//   - id: The database ID of the base submodel_element record
//   - element: The submodel element to insert (must be of type *types.Property)
//
// Returns:
//   - *InsertQueryPart: The table name and record for property_element table insert
//   - error: An error if the element is not a Property or value reference creation fails
func (p PostgreSQLPropertyHandler) GetInsertQueryPart(_ *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error) {
	property, ok := element.(*types.Property)
	if !ok {
		return nil, common.NewErrBadRequest("element is not of type Property")
	}

	// Use centralized value type mapper
	typedValue := MapValueByType(property.ValueType(), property.Value())

	return &InsertQueryPart{
		TableName: "property_element",
		Record: goqu.Record{
			"id":             id,
			"value_type":     property.ValueType(),
			"value_text":     typedValue.Text,
			"value_num":      typedValue.Numeric,
			"value_bool":     typedValue.Boolean,
			"value_time":     typedValue.Time,
			"value_date":     typedValue.Date,
			"value_datetime": typedValue.DateTime,
		},
	}, nil
}

func buildUpdatePropertyRecordObject(property *types.Property, isPut bool, localTx *sql.Tx) (goqu.Record, error) {
	updateRecord := goqu.Record{}

	// Required field - always update
	updateRecord["value_type"] = property.ValueType()

	// Map value by type - always update based on isPut or if value is provided
	if isPut || property.Value() != nil {
		typedValue := MapValueByType(property.ValueType(), property.Value())
		updateRecord["value_text"] = typedValue.Text
		updateRecord["value_num"] = typedValue.Numeric
		updateRecord["value_bool"] = typedValue.Boolean
		updateRecord["value_time"] = typedValue.Time
		updateRecord["value_datetime"] = typedValue.DateTime
	}

	_ = localTx
	return updateRecord, nil
}
