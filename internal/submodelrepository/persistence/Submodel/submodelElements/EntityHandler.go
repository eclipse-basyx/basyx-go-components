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
	jsoniter "github.com/json-iterator/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLEntityHandler handles the persistence operations for Entity submodel elements.
// It implements the SubmodelElementHandler interface and uses the decorator pattern
// to extend the base CRUD operations with Entity-specific functionality.
type PostgreSQLEntityHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLEntityHandler creates a new PostgreSQLEntityHandler instance.
// It initializes the handler with a database connection and creates the decorated
// base CRUD handler for common SubmodelElement operations.
//
// Parameters:
//   - db: Database connection to PostgreSQL
//
// Returns:
//   - *PostgreSQLEntityHandler: Configured Entity handler instance
//   - error: Error if the decorated handler creation fails
func NewPostgreSQLEntityHandler(db *sql.DB) (*PostgreSQLEntityHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLEntityHandler{db: db, decorated: decoratedHandler}, nil
}

// Create persists a new Entity submodel element to the database.
// It first creates the base SubmodelElement data using the decorated handler,
// then adds Entity-specific data including entity type, global asset ID, and specific asset IDs.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - submodelElement: The Entity element to create (must be of type *gen.Entity)
//
// Returns:
//   - int: The database ID of the created entity
//   - error: Error if the element is not an Entity or if database operations fail
func (p PostgreSQLEntityHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	entity, ok := submodelElement.(*gen.Entity)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type Entity")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
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

// CreateNested persists a new nested Entity submodel element to the database.
// It creates the Entity as a child element of another SubmodelElement with a specific position
// and idShortPath for hierarchical organization.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent SubmodelElement
//   - idShortPath: Path identifier for the nested element
//   - submodelElement: The Entity element to create (must be of type *gen.Entity)
//   - pos: Position of the element within its parent
//
// Returns:
//   - int: The database ID of the created nested entity
//   - error: Error if the element is not an Entity or if database operations fail
func (p PostgreSQLEntityHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	entity, ok := submodelElement.(*gen.Entity)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type Entity")
	}

	// Create the nested entity with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
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

// Update modifies an existing Entity submodel element in the database.
// Handles both PUT (complete replacement) and PATCH (partial update) operations based on isPut flag.
//
// For PUT operations (isPut=true):
//   - Deletes all child elements (Statements) - complete replacement
//   - Replaces EntityType (optional field - cleared if empty)
//   - Replaces GlobalAssetID (optional field - cleared if empty)
//   - Replaces SpecificAssetIds (optional field - cleared if empty)
//
// For PATCH operations (isPut=false):
//   - Preserves existing child elements unless Statements field is provided
//   - Updates EntityType, GlobalAssetID, and SpecificAssetIds only if provided in the request
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifier of the element to update
//   - submodelElement: The updated Entity element data (must be of type *gen.Entity)
//   - tx: Optional database transaction (created if nil)
//   - isPut: true for PUT (replace all), false for PATCH (update only provided fields)
//
// Returns:
//   - error: Error if the update operation fails or element is not of correct type
func (p PostgreSQLEntityHandler) Update(submodelID string, idShortOrPath string, submodelElement gen.SubmodelElement, tx *sql.Tx, isPut bool) error {
	entity, ok := submodelElement.(*gen.Entity)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type Entity")
	}

	// Manage transaction
	var localTx *sql.Tx
	var err error
	if tx == nil {
		localTx, err = p.db.Begin()
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				_ = localTx.Rollback()
			}
		}()
		tx = localTx
	}

	// For PUT operations or when Statements are provided, delete all children
	if isPut || entity.Statements != nil {
		err = DeleteAllChildren(p.db, submodelID, idShortOrPath, tx)
		if err != nil {
			return err
		}
	}

	// Update base submodel element properties
	err = p.decorated.Update(submodelID, idShortOrPath, submodelElement, tx, isPut)
	if err != nil {
		return err
	}

	// Get element ID
	dialect := goqu.Dialect("postgres")
	idQuery, idArgs, err := dialect.From("submodel_element").
		Select("id").
		Where(goqu.And(
			goqu.C("submodel_id").Eq(submodelID),
			goqu.C("idshort_path").Eq(idShortOrPath),
		)).
		ToSQL()
	if err != nil {
		return err
	}

	var elementID int
	err = tx.QueryRow(idQuery, idArgs...).Scan(&elementID)
	if err != nil {
		return err
	}

	// Build update record for Entity-specific fields
	updateRecord := goqu.Record{}

	if isPut {
		// PUT: Always update all fields
		json := jsoniter.ConfigCompatibleWithStandardLibrary
		specificAssetIDs := "[]"
		if entity.SpecificAssetIds != nil {
			specificAssetIDsBytes, err := json.Marshal(entity.SpecificAssetIds)
			if err != nil {
				return err
			}
			specificAssetIDs = string(specificAssetIDsBytes)
		}

		updateRecord["entity_type"] = entity.EntityType
		updateRecord["global_asset_id"] = entity.GlobalAssetID
		updateRecord["specific_asset_ids"] = specificAssetIDs
	} else {
		// PATCH: Only update provided fields
		// Note: EntityType is a string enum, so we check if it's not empty
		if entity.EntityType != "" {
			updateRecord["entity_type"] = entity.EntityType
		}

		if entity.GlobalAssetID != "" {
			updateRecord["global_asset_id"] = entity.GlobalAssetID
		}

		if entity.SpecificAssetIds != nil {
			json := jsoniter.ConfigCompatibleWithStandardLibrary
			specificAssetIDsBytes, err := json.Marshal(entity.SpecificAssetIds)
			if err != nil {
				return err
			}
			updateRecord["specific_asset_ids"] = string(specificAssetIDsBytes)
		}
	}

	// Execute update if there are fields to update
	if len(updateRecord) > 0 {
		updateQuery, updateArgs, err := dialect.Update("entity_element").
			Set(updateRecord).
			Where(goqu.C("id").Eq(elementID)).
			ToSQL()
		if err != nil {
			return err
		}

		_, err = tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			return err
		}
	}

	// Commit transaction if we created it
	if localTx != nil {
		err = localTx.Commit()
	}

	return err
}

// UpdateValueOnly updates only the value of an existing Entity submodel element identified by its idShort or path.
// It updates the entity type, global asset ID, and specific asset IDs based on the provided value.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set (must be of type gen.EntityValue)
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLEntityHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	entityValueOnly, ok := valueOnly.(gen.EntityValue)
	if !ok {
		return common.NewErrBadRequest("valueOnly is not of type EntityValue")
	}
	elems, err := persistenceutils.BuildElementsToProcessStackValueOnly(p.db, submodelID, idShortOrPath, valueOnly)
	if err != nil {
		return err
	}
	err = UpdateNestedElementsValueOnly(p.db, elems, idShortOrPath, submodelID)
	if err != nil {
		return err
	}

	dialect := goqu.Dialect("postgres")

	var elementID int
	query, args, err := dialect.From("submodel_element").
		InnerJoin(
			goqu.T("entity_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("entity_element.id"))),
		).
		Select("submodel_element.id").
		Where(
			goqu.C("idshort_path").Eq(idShortOrPath),
			goqu.C("submodel_id").Eq(submodelID),
		).
		ToSQL()
	if err != nil {
		return err
	}
	err = tx.QueryRow(query, args...).Scan(&elementID)
	if err != nil {
		return err
	}

	var specificAssetIDs string
	if entityValueOnly.SpecificAssetIds != nil {
		json := jsoniter.ConfigCompatibleWithStandardLibrary
		specificAssetIDsBytes, err := json.Marshal(entityValueOnly.SpecificAssetIds)
		if err != nil {
			return err
		}
		specificAssetIDs = string(specificAssetIDsBytes)
	} else {
		specificAssetIDs = "[]"
	}

	updateQuery, args, err := dialect.Update("entity_element").
		Set(
			goqu.Record{
				"entity_type":        entityValueOnly.EntityType,
				"global_asset_id":    entityValueOnly.GlobalAssetID,
				"specific_asset_ids": specificAssetIDs,
			},
		).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(updateQuery, args...)
	if err != nil {
		return err
	}

	err = tx.Commit()
	return err
}

// Delete removes an Entity submodel element from the database.
// Currently delegates to the decorated handler for base SubmodelElement deletion.
// Entity-specific data is automatically deleted due to foreign key constraints.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to delete
//
// Returns:
//   - error: Error if the delete operation fails
func (p PostgreSQLEntityHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

func insertEntity(entity *gen.Entity, tx *sql.Tx, id int) error {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	specificAssetIDs := "[]"
	if entity.SpecificAssetIds != nil {
		specificAssetIDsBytes, err := json.Marshal(entity.SpecificAssetIds)
		if err != nil {
			return err
		}
		specificAssetIDs = string(specificAssetIDsBytes)
	}

	dialect := goqu.Dialect("postgres")
	insertQuery, insertArgs, err := dialect.Insert("entity_element").
		Rows(goqu.Record{
			"id":                 id,
			"entity_type":        entity.EntityType,
			"global_asset_id":    entity.GlobalAssetID,
			"specific_asset_ids": specificAssetIDs,
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
