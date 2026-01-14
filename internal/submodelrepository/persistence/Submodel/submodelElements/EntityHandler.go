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
// Currently delegates to the decorated handler for base SubmodelElement updates.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to update
//   - submodelElement: The updated Entity element data
//
// Returns:
//   - error: Error if the update operation fails
func (p PostgreSQLEntityHandler) Update(submodelID string, idShortOrPath string, submodelElement gen.SubmodelElement, tx *sql.Tx, isPut bool) error {
	return p.decorated.Update(submodelID, idShortOrPath, submodelElement, tx, isPut)
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

	_, err := tx.Exec(`INSERT INTO entity_element (id, entity_type, global_asset_id, specific_asset_ids) VALUES ($1, $2, $3, $4)`,
		id, entity.EntityType, entity.GlobalAssetID, specificAssetIDs)
	if err != nil {
		return err
	}
	return nil
}
