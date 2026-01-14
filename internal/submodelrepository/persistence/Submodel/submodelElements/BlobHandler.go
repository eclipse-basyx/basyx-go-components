/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	smrepoconfig "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/config"
	smrepoerrors "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/errors"
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

// Create inserts a new Blob element into the database as a top-level submodel element.
// This method handles both the common submodel element properties and the specific blob
// data including content type and binary value storage.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the parent submodel
//   - submodelElement: The Blob element to create
//
// Returns:
//   - int: Database ID of the created element
//   - error: Error if creation fails or element is not of correct type
func (p PostgreSQLBlobHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	blob, ok := submodelElement.(*gen.Blob)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type Blob")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// Check if blob value is larger than maximum allowed size
	if len(blob.Value) > smrepoconfig.MaxBlobSizeBytes {
		return 0, smrepoerrors.ErrBlobTooLarge
	}

	// Blob-specific database insertion
	_, err = tx.Exec(`INSERT INTO blob_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, blob.ContentType, []byte(blob.Value))
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested inserts a new Blob element as a nested element within a collection or list.
// This method creates the element at a specific hierarchical path and position within its parent container.
// It handles both the parent-child relationship and the specific blob data storage.
//
// Parameters:
//   - tx: Active database transaction
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent element
//   - idShortPath: Hierarchical path where the element should be created
//   - submodelElement: The Blob element to create
//   - pos: Position within the parent container
//
// Returns:
//   - int: Database ID of the created nested element
//   - error: Error if creation fails or element is not of correct type
func (p PostgreSQLBlobHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	blob, ok := submodelElement.(*gen.Blob)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type Blob")
	}

	// Create the nested blob with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	// Blob-specific database insertion for nested element
	_, err = tx.Exec(`INSERT INTO blob_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, blob.ContentType, []byte(blob.Value))
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing Blob element identified by its idShort or path.
// This method delegates the update operation to the decorated CRUD handler which handles
// the common submodel element update logic.
//
// Parameters:
//   - idShortOrPath: idShort or hierarchical path to the element to update
//   - submodelElement: Updated element data
//
// Returns:
//   - error: Error if update fails
func (p PostgreSQLBlobHandler) Update(submodelID string, idShortOrPath string, submodelElement gen.SubmodelElement, tx *sql.Tx, isPut bool) error {
	return p.decorated.Update(submodelID, idShortOrPath, submodelElement, tx, isPut)
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
		blobValueOnly = gen.BlobValue(fileValueOnly)
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

	var elementID int
	query, args, err := dialect.From("submodel_element").
		InnerJoin(
			goqu.T("blob_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("blob_element.id"))),
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
