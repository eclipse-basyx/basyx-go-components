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

package submodelelements

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLSubmodelElementListHandler handles the persistence operations for SubmodelElementList submodel elements.
// It implements the SubmodelElementHandler interface and uses the decorator pattern
// to extend the base CRUD operations with SubmodelElementList-specific functionality.
type PostgreSQLSubmodelElementListHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLSubmodelElementListHandler creates a new PostgreSQLSubmodelElementListHandler instance.
// It initializes the handler with a database connection and creates the decorated
// base CRUD handler for common SubmodelElement operations.
//
// Parameters:
//   - db: Database connection to PostgreSQL
//
// Returns:
//   - *PostgreSQLSubmodelElementListHandler: Configured SubmodelElementList handler instance
//   - error: Error if the decorated handler creation fails
func NewPostgreSQLSubmodelElementListHandler(db *sql.DB) (*PostgreSQLSubmodelElementListHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLSubmodelElementListHandler{db: db, decorated: decoratedHandler}, nil
}

// Create persists a new SubmodelElementList submodel element to the database.
// It first creates the base SubmodelElement data using the decorated handler,
// then adds SubmodelElementList-specific data including order relevance, semantic ID,
// type value, and value type for list elements.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - submodelElement: The SubmodelElementList to create (must be of type *gen.SubmodelElementList)
//
// Returns:
//   - int: The database ID of the created list element
//   - error: Error if the element is not a SubmodelElementList or if database operations fail
func (p PostgreSQLSubmodelElementListHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	smeList, ok := submodelElement.(*gen.SubmodelElementList)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type SubmodelElementList")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// SubmodelElementList-specific database insertion
	err = insertSubmodelElementList(smeList, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested persists a new nested SubmodelElementList submodel element to the database.
// It creates the SubmodelElementList as a child element of another SubmodelElement with
// a specific position and idShortPath for hierarchical organization.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent SubmodelElement
//   - idShortPath: Path identifier for the nested element
//   - submodelElement: The SubmodelElementList to create (must be of type *gen.SubmodelElementList)
//   - pos: Position of the element within its parent
//
// Returns:
//   - int: The database ID of the created nested list element
//   - error: Error if the element is not a SubmodelElementList or if database operations fail
func (p PostgreSQLSubmodelElementListHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	smeList, ok := submodelElement.(*gen.SubmodelElementList)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type SubmodelElementList")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	// SubmodelElementList-specific database insertion
	err = insertSubmodelElementList(smeList, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing SubmodelElementList submodel element in the database.
// Currently delegates to the decorated handler for base SubmodelElement updates.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to update
//   - submodelElement: The updated SubmodelElementList data
//
// Returns:
//   - error: Error if the update operation fails
func (p PostgreSQLSubmodelElementListHandler) Update(submodelID string, idShortOrPath string, submodelElement gen.SubmodelElement, tx *sql.Tx, isPut bool) error {
	return p.decorated.Update(submodelID, idShortOrPath, submodelElement, tx, isPut)
}

// UpdateValueOnly updates only the value of an existing SubmodelElementList submodel element identified by its idShort or path.
// It processes the new value and updates nested elements accordingly.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set (must be of type gen.SubmodelElementValue)
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLSubmodelElementListHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	elems, err := persistenceutils.BuildElementsToProcessStackValueOnly(p.db, submodelID, idShortOrPath, valueOnly)
	if err != nil {
		return err
	}
	err = UpdateNestedElementsValueOnly(p.db, elems, idShortOrPath, submodelID)
	if err != nil {
		return err
	}
	return nil
}

// Delete removes a SubmodelElementList submodel element from the database.
// Currently delegates to the decorated handler for base SubmodelElement deletion.
// SubmodelElementList-specific data is automatically deleted due to foreign key constraints.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to delete
//
// Returns:
//   - error: Error if the delete operation fails
func (p PostgreSQLSubmodelElementListHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

func insertSubmodelElementList(smeList *gen.SubmodelElementList, tx *sql.Tx, id int) error {
	var semanticID sql.NullInt64
	if smeList.SemanticIdListElement != nil && !isEmptyReference(smeList.SemanticIdListElement) {
		refID, err := insertReference(tx, *smeList.SemanticIdListElement)
		if err != nil {
			return err
		}
		semanticID = sql.NullInt64{Int64: int64(refID), Valid: true}
	}

	var typeValue, valueType sql.NullString
	if smeList.TypeValueListElement != nil {
		typeValue = sql.NullString{String: string(*smeList.TypeValueListElement), Valid: true}
	}
	if smeList.ValueTypeListElement != "" {
		valueType = sql.NullString{String: string(smeList.ValueTypeListElement), Valid: true}
	}

	dialect := goqu.Dialect("postgres")
	insertQuery, insertArgs, err := dialect.Insert("submodel_element_list").
		Rows(goqu.Record{
			"id":                       id,
			"order_relevant":           smeList.OrderRelevant,
			"semantic_id_list_element": semanticID,
			"type_value_list_element":  typeValue,
			"value_type_list_element":  valueType,
		}).
		ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(insertQuery, insertArgs...)
	return err
}
