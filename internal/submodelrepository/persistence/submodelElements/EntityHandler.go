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
	"context"
	"database/sql"
	"log/slog"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	jsoniter "github.com/json-iterator/go"
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

// Update modifies an existing Entity element identified by its idShort or path.
// This method delegates the update operation to the decorated CRUD handler which handles
// the common submodel element update logic and additionally manages Entity-specific fields.
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
func (p PostgreSQLEntityHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	entity, ok := submodelElement.(*types.Entity)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type Entity")
	}

	var err error
	cu, localTx, err := common.StartTXIfNeeded(tx, err, p.db)
	if err != nil {
		return err
	}
	defer cu(&err)

	// Update base submodel element properties
	err = p.decorated.Update(submodelID, idShortOrPath, submodelElement, localTx, isPut)
	if err != nil {
		return err
	}
	effectivePath := resolveUpdatedPath(idShortOrPath, submodelElement, isPut)

	smDbID, err := persistenceutils.GetSubmodelDatabaseID(localTx, submodelID)
	if err != nil {
		slog.Error("submodel database ID lookup failed", "error.code", "SMREPO-SMEUPDATE-GETSMDBID", "error", err)
		return common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}
	elementID, err := p.decorated.GetDatabaseIDWithTx(localTx, smDbID, effectivePath)
	if err != nil {
		return err
	}

	// Build update record for Entity-specific fields
	updateRecord, err := buildUpdateEntityRecordObject(isPut, entity)
	if err != nil {
		return err
	}

	dialect := goqu.Dialect("postgres")

	// Execute update if there are fields to update
	if anyFieldsToUpdate(updateRecord) {
		updateQuery, updateArgs, err := dialect.Update("entity_element").
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
	}

	return common.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly stores the decoded ValueOnly attributes of an Entity; absent attributes stay untouched.
// Statements are applied as separate updates.
func (p PostgreSQLEntityHandler) UpdateValueOnly(ctx context.Context, tx *sql.Tx, update gen.ValueOnlyUpdate) error {
	patch, err := valueOnlyPatchAs[gen.EntityValuePatch](update)
	if err != nil {
		return err
	}
	record := goqu.Record{}
	if patch.EntityType != nil {
		record["entity_type"] = int(*patch.EntityType)
	}
	if patch.GlobalAssetID != nil {
		record["global_asset_id"] = *patch.GlobalAssetID
	}
	if patch.SpecificAssetIDs != nil {
		specificAssetIDs, err := serializeIClassSliceToJSON(toIClassSlice(*patch.SpecificAssetIDs), "SMREPO-UPDENTITYVALONLY-SPECIFICASSETIDS")
		if err != nil {
			return err
		}
		record["specific_asset_ids"] = specificAssetIDs
	}
	return execValueOnlyUpdate(ctx, tx, "entity_element", record, update.ElementID, "SMREPO-UPDENTITYVALONLY")
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

// GetInsertQueryPart returns the type-specific insert query part for batch insertion of Entity elements.
// It returns the table name and record for inserting into the entity_element table.
//
// Parameters:
//   - tx: Active database transaction (not used for Entity)
//   - id: The database ID of the base submodel_element record
//   - element: The Entity element to insert
//
// Returns:
//   - *InsertQueryPart: The table name and record for entity_element insert
//   - error: An error if the element is not of type Entity
func (p PostgreSQLEntityHandler) GetInsertQueryPart(_ *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error) {
	entity, ok := element.(*types.Entity)
	if !ok {
		return nil, common.NewErrBadRequest("submodelElement is not of type Entity")
	}

	json := jsoniter.ConfigCompatibleWithStandardLibrary
	specificAssetIDs := "[]"
	if entity.SpecificAssetIDs() != nil {
		var jsonable []map[string]any
		for _, saa := range entity.SpecificAssetIDs() {
			jsonableSaa, err := jsonization.ToJsonable(saa)
			if err != nil {
				return nil, common.NewErrBadRequest("SMREPO-GIQP-ENTITY-SAATJSONABLE Failed to convert Specific Asset ID to JSONABLE: " + err.Error())
			}
			jsonable = append(jsonable, jsonableSaa)
		}
		specificAssetIDsBytes, err := json.Marshal(jsonable)
		if err != nil {
			return nil, err
		}
		specificAssetIDs = string(specificAssetIDsBytes)
	}

	return &InsertQueryPart{
		TableName: "entity_element",
		Record: goqu.Record{
			"id":                 id,
			"entity_type":        entity.EntityType(),
			"global_asset_id":    entity.GlobalAssetID(),
			"specific_asset_ids": specificAssetIDs,
		},
	}, nil
}

func buildUpdateEntityRecordObject(isPut bool, entity *types.Entity) (goqu.Record, error) {
	updateRecord := goqu.Record{}

	if isPut {
		// PUT: Always update all fields
		json := jsoniter.ConfigCompatibleWithStandardLibrary
		specificAssetIDs := "[]"
		if entity.SpecificAssetIDs() != nil {
			var jsonable []map[string]any
			for _, saa := range entity.SpecificAssetIDs() {
				jsonableSaa, err := jsonization.ToJsonable(saa)
				if err != nil {
					return nil, common.NewErrBadRequest("SMREPO-UPDENTITY-SAATJSONABLE Failed to convert Specific Asset ID to JSONABLE: " + err.Error())
				}
				jsonable = append(jsonable, jsonableSaa)
			}
			specificAssetIDsBytes, err := json.Marshal(jsonable)
			if err != nil {
				return nil, err
			}
			specificAssetIDs = string(specificAssetIDsBytes)
		}

		updateRecord["entity_type"] = entity.EntityType()
		updateRecord["global_asset_id"] = entity.GlobalAssetID()
		updateRecord["specific_asset_ids"] = specificAssetIDs
	} else {
		// PATCH: Only update provided fields
		// Note: EntityType is a string enum, so we check if it's not empty
		if entity.EntityType() != nil {
			updateRecord["entity_type"] = entity.EntityType()
		}

		if entity.GlobalAssetID() != nil {
			updateRecord["global_asset_id"] = entity.GlobalAssetID()
		}

		if entity.SpecificAssetIDs() != nil {
			json := jsoniter.ConfigCompatibleWithStandardLibrary
			var jsonable []map[string]any
			for _, saa := range entity.SpecificAssetIDs() {
				jsonableSaa, err := jsonization.ToJsonable(saa)
				if err != nil {
					return nil, common.NewErrBadRequest("SMREPO-UPDENTITY-SAATJSONABLE Failed to convert Specific Asset ID to JSONABLE: " + err.Error())
				}
				jsonable = append(jsonable, jsonableSaa)
			}
			specificAssetIDsBytes, err := json.Marshal(jsonable)
			if err != nil {
				return nil, err
			}
			updateRecord["specific_asset_ids"] = string(specificAssetIDsBytes)
		}
	}
	return updateRecord, nil
}
