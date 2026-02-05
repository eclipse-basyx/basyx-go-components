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

// PostgreSQLMultiLanguagePropertyHandler handles persistence operations for MultiLanguageProperty submodel elements.
// It uses the decorator pattern to extend the base PostgreSQLSMECrudHandler with
// MultiLanguageProperty-specific functionality. MultiLanguageProperty elements represent text values
// that can be expressed in multiple languages, with each language variant stored as a separate value.
type PostgreSQLMultiLanguagePropertyHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLMultiLanguagePropertyHandler creates a new PostgreSQLMultiLanguagePropertyHandler instance.
// It initializes the decorated PostgreSQLSMECrudHandler for base submodel element operations.
//
// Parameters:
//   - db: A PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLMultiLanguagePropertyHandler: A new handler instance
//   - error: An error if the decorated handler initialization fails
func NewPostgreSQLMultiLanguagePropertyHandler(db *sql.DB) (*PostgreSQLMultiLanguagePropertyHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLMultiLanguagePropertyHandler{db: db, decorated: decoratedHandler}, nil
}

// Update modifies an existing MultiLanguageProperty submodel element in the database.
// This method handles both the common submodel element properties and the specific
// multi-language property data including language-text pairs and valueId reference.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - submodelElement: The updated MultiLanguageProperty element data
//   - tx: Active database transaction (can be nil, will create one if needed)
//   - isPut: true: Replaces the element with the body data; false: Updates only passed data
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLMultiLanguagePropertyHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	mlp, ok := submodelElement.(*types.MultiLanguageProperty)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type MultiLanguageProperty")
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

	dialect := goqu.Dialect("postgres")

	elementID, err := p.decorated.GetDatabaseID(submodelID, idShortOrPath)
	if err != nil {
		return err
	}

	// Handle optional valueId field
	// For PUT: always update (even if nil, which clears the field)
	// For PATCH: only update if provided (not nil)
	if isPut || mlp.ValueID() != nil {
		var valueIdRef sql.NullInt64
		if mlp.ValueID() != nil && !isEmptyReference(mlp.ValueID()) {
			// Insert the reference and get the ID
			refID, err := insertReference(localTx, mlp.ValueID())
			if err != nil {
				return err
			}
			valueIdRef = sql.NullInt64{Int64: int64(refID), Valid: true}
		}

		// Update multilanguage_property table with valueId
		updateQuery, updateArgs, err := dialect.Update("multilanguage_property").
			Set(goqu.Record{
				"value_id": valueIdRef,
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

	// Handle Value field - delete existing values and insert new ones
	// For PUT: always replace (delete all and insert new)
	// For PATCH: only update if provided (not nil)
	if isPut || mlp.Value() != nil {
		deleteQuery, deleteArgs, err := dialect.Delete("multilanguage_property_value").
			Where(goqu.C("mlp_id").Eq(elementID)).
			ToSQL()
		if err != nil {
			return err
		}

		_, err = localTx.Exec(deleteQuery, deleteArgs...)
		if err != nil {
			return err
		}

		// Insert new values if provided
		if mlp.Value() != nil {
			for _, val := range mlp.Value() {
				insertQuery, insertArgs, err := dialect.Insert("multilanguage_property_value").
					Rows(goqu.Record{
						"mlp_id":   elementID,
						"language": val.Language(),
						"text":     val.Text(),
					}).
					ToSQL()
				if err != nil {
					return err
				}

				_, err = localTx.Exec(insertQuery, insertArgs...)
				if err != nil {
					return err
				}
			}
		}
	}

	return persistenceutils.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly updates only the value of an existing MultiLanguageProperty submodel element identified by its idShort or path.
// It deletes existing language-text pairs and inserts the new set of values provided.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set (must be of type gen.MultiLanguagePropertyValue)
//
// Returns:
//   - error: An error if the update operation fails or if the valueOnly type is incorrect
func (p PostgreSQLMultiLanguagePropertyHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	mlp, ok := valueOnly.(gen.MultiLanguagePropertyValue)
	if !ok {
		ambiguous, isAmbiguous := valueOnly.(gen.AmbiguousSubmodelElementValue)
		if !isAmbiguous {
			return common.NewErrBadRequest("valueOnly is not of type MultiLanguagePropertyValue")
		}
		var err error
		mlp, err = ambiguous.ConvertToMultiLanguagePropertyValue()
		if err != nil {
			return common.NewErrBadRequest("valueOnly contains non-MultiLanguagePropertyValue entries")
		}
	}

	dialect := goqu.Dialect("postgres")

	// Build subquery to get the submodel element ID
	subquery := dialect.From("submodel_element").
		Select("id").
		Where(
			goqu.C("submodel_id").Eq(submodelID),
			goqu.C("idshort_path").Eq(idShortOrPath),
		)

	// Delete existing values
	deleteQuery, deleteArgs, err := dialect.Delete("multilanguage_property_value").
		Where(goqu.C("mlp_id").Eq(subquery)).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	_, err = p.db.Exec(deleteQuery, deleteArgs...)
	if err != nil {
		return fmt.Errorf("failed to delete existing values: %w", err)
	}

	// Insert new values
	for _, val := range mlp {
		for lang, text := range val {
			insertQuery, insertArgs, err := dialect.Insert("multilanguage_property_value").
				Rows(goqu.Record{
					"mlp_id":   subquery,
					"language": lang,
					"text":     text,
				}).
				ToSQL()
			if err != nil {
				return fmt.Errorf("failed to build insert query: %w", err)
			}

			_, err = p.db.Exec(insertQuery, insertArgs...)
			if err != nil {
				return fmt.Errorf("failed to insert value: %w", err)
			}
		}
	}
	return nil
}

// Delete removes a MultiLanguageProperty submodel element from the database.
// Currently delegates all delete operations to the decorated handler. MultiLanguageProperty-specific data
// (including all language values) is typically removed via database cascade constraints.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifying the element to delete
//
// Returns:
//   - error: An error if the delete operation fails
func (p PostgreSQLMultiLanguagePropertyHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

// GetInsertQueryPart returns the type-specific insert query part for batch insertion of MultiLanguageProperty elements.
// It returns the table name and record for inserting into the multilanguage_property table.
// Note: The language values in multilanguage_property_value are inserted separately by BatchInsert
// after the main table insert, because they require the multilanguage_property record to exist first.
//
// Parameters:
//   - tx: Active database transaction (needed for creating value references)
//   - id: The database ID of the base submodel_element record
//   - element: The MultiLanguageProperty element to insert
//
// Returns:
//   - *InsertQueryPart: The table name and record for multilanguage_property insert
//   - error: An error if the element is not of type MultiLanguageProperty
func (p PostgreSQLMultiLanguagePropertyHandler) GetInsertQueryPart(tx *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error) {
	mlp, ok := element.(*types.MultiLanguageProperty)
	if !ok {
		return nil, common.NewErrBadRequest("submodelElement is not of type MultiLanguageProperty")
	}

	valueIDDbID, err := persistenceutils.CreateReference(tx, mlp.ValueID(), sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		return nil, common.NewInternalServerError("Failed to create ValueID reference: " + err.Error())
	}

	return &InsertQueryPart{
		TableName: "multilanguage_property",
		Record: goqu.Record{
			"id":       id,
			"value_id": valueIDDbID,
		},
	}, nil
}
