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

// Package submodelelements provides handlers for different types of submodel elements in the BaSyx framework.
// This package contains PostgreSQL-based persistence implementations for various submodel element types
// including blob elements for binary data storage.
package submodelelements

import (
	"database/sql"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	smrepoconfig "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/config"
	smrepoerrors "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/errors"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLBlobHandler provides PostgreSQL-based persistence operations for Blob submodel elements.
// It implements CRUD operations and handles binary data storage with content type information.
// Blob elements are used to store binary data such as images, documents, or other files within submodels.
type PostgreSQLBlobHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLBlobHandler creates a new handler for Blob element persistence.
// It initializes the handler with a database connection and sets up the decorated CRUD handler
// for common submodel element operations.
//
// Parameters:
//   - db: PostgreSQL database connection
//
// Returns:
//   - *PostgreSQLBlobHandler: Configured handler instance
//   - error: Error if handler initialization fails
func NewPostgreSQLBlobHandler(db *sql.DB) (*PostgreSQLBlobHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLBlobHandler{db: db, decorated: decoratedHandler}, nil
}

// Update modifies an existing Blob element identified by its idShort or path.
// This method handles both the common submodel element properties and the specific blob
// data including content type and binary value storage.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortOrPath: idShort or hierarchical path to the element to update
//   - submodelElement: Updated element data
//   - tx: Active database transaction (can be nil, will create one if needed)
//   - isPut: true: Replaces the Submodel Element with the Body Data (Deletes non-specified fields); false: Updates only passed request body data, unspecified is ignored
//
// Returns:
//   - error: Error if update fails
func (p PostgreSQLBlobHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	blob, ok := submodelElement.(*types.Blob)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type Blob")
	}

	var err error
	cu, localTx, err := persistenceutils.StartTXIfNeeded(tx, err, p.db)
	if err != nil {
		return err
	}
	defer cu(&err)
	if isBlobSizeExceeded(blob) {
		return smrepoerrors.ErrBlobTooLarge
	}

	smDbID, err := persistenceutils.GetSubmodelDatabaseID(localTx, submodelID)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}
	elementID, err := p.decorated.GetDatabaseID(smDbID, idShortOrPath)
	if err != nil {
		return err
	}

	dialect := goqu.Dialect("postgres")

	// Build the update record based on isPut flag
	// For PUT: always update all fields (even if empty, which clears them)
	// For PATCH: only update fields that are provided (not empty)
	updateRecord := buildUpdateBlobRecordObject(isPut, blob)

	if anyFieldsToUpdate(updateRecord) {
		updateQuery, updateArgs, err := dialect.Update("blob_element").
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

	err = p.decorated.Update(submodelID, idShortOrPath, submodelElement, localTx, isPut)
	if err != nil {
		return err
	}

	return persistenceutils.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly updates only the value of an existing Blob submodel element identified by its idShort or path.
// It updates the content type and binary value in the database.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set (must be of type gen.BlobValue)
//
// Returns:
//   - error: An error if the update operation fails or if the valueOnly type is incorrect
func (p PostgreSQLBlobHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue) error {
	blobValueOnly, ok := valueOnly.(gen.BlobValue)
	if !ok {
		var fileValueOnly gen.FileValue
		var isMistakenAsFileValue bool
		if fileValueOnly, isMistakenAsFileValue = valueOnly.(gen.FileValue); !isMistakenAsFileValue {
			return common.NewErrBadRequest("valueOnly is not of type BlobValue")
		}

		bytea := []byte(fileValueOnly.Value)

		blobValueOnly = gen.BlobValue{
			ContentType: fileValueOnly.ContentType,
			Value:       bytea,
		}
	}

	// Check if blob value is larger than 1GB
	if len(blobValueOnly.Value) > 1<<30 {
		return common.NewErrBadRequest("blob value exceeds maximum size of 1GB - for files larger than 1GB, you must use File submodel element instead - Postgres Limitation")
	}

	// Start transaction
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Update only the blob-specific fields in the database
	dialect := goqu.Dialect("postgres")
	smDbID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("submodel not found")
		}
		return err
	}

	var elementID int
	query, args, err := dialect.From("submodel_element").
		InnerJoin(
			goqu.T("blob_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("blob_element.id"))),
		).
		Select("submodel_element.id").
		Where(
			goqu.C("idshort_path").Eq(idShortOrPath),
			goqu.C("submodel_id").Eq(smDbID),
		).
		ToSQL()
	if err != nil {
		return err
	}
	err = tx.QueryRow(query, args...).Scan(&elementID)
	if err != nil {
		return err
	}

	updateQuery, updateArgs, err := dialect.Update("blob_element").
		Set(goqu.Record{"content_type": blobValueOnly.ContentType, "value": []byte(blobValueOnly.Value)}).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(updateQuery, updateArgs...)
	if err != nil {
		return err
	}

	err = tx.Commit()
	return err
}

// Delete removes a Blob element identified by its idShort or path from the database.
// This method delegates the deletion operation to the decorated CRUD handler which handles
// the cascading deletion of all related data and child elements.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the element to delete
//
// Returns:
//   - error: Error if deletion fails
func (p PostgreSQLBlobHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

// GetInsertQueryPart returns the type-specific insert query part for batch insertion of Blob elements.
// It returns the table name and record for inserting into the blob_element table.
//
// Parameters:
//   - tx: Active database transaction (not used for Blob)
//   - id: The database ID of the base submodel_element record
//   - element: The Blob element to insert
//
// Returns:
//   - *InsertQueryPart: The table name and record for blob_element insert
//   - error: An error if the element is not of type Blob
func (p PostgreSQLBlobHandler) GetInsertQueryPart(_ *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error) {
	blob, ok := element.(*types.Blob)
	if !ok {
		return nil, common.NewErrBadRequest("submodelElement is not of type Blob")
	}

	contentType := ""
	if blob.ContentType() != nil {
		contentType = *blob.ContentType()
	}

	// encode value to base64
	encoded := common.Encode(blob.Value())

	return &InsertQueryPart{
		TableName: "blob_element",
		Record: goqu.Record{
			"id":           id,
			"content_type": contentType,
			"value":        encoded,
		},
	}, nil
}

func isBlobSizeExceeded(blob *types.Blob) bool {
	return len(blob.Value()) > smrepoconfig.MaxBlobSizeBytes
}

func buildUpdateBlobRecordObject(isPut bool, blob *types.Blob) goqu.Record {
	updateRecord := goqu.Record{}

	contentType := ""
	if blob.ContentType() != nil {
		contentType = *blob.ContentType()
	}
	if isPut || contentType != "" {
		var contentTypeNull sql.NullString
		if contentType != "" {
			contentTypeNull = sql.NullString{String: contentType, Valid: true}
		}
		updateRecord["content_type"] = contentTypeNull
	}

	value := blob.Value()
	if isPut || len(value) > 0 {
		updateRecord["value"] = value
	}
	return updateRecord
}
