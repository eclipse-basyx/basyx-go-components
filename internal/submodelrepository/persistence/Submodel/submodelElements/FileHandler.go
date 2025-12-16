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

package submodelelements

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

// PostgreSQLFileHandler handles the persistence operations for File submodel elements.
// It implements the SubmodelElementHandler interface and uses the decorator pattern
// to extend the base CRUD operations with File-specific functionality.
type PostgreSQLFileHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

// NewPostgreSQLFileHandler creates a new PostgreSQLFileHandler instance.
// It initializes the handler with a database connection and creates the decorated
// base CRUD handler for common SubmodelElement operations.
//
// Parameters:
//   - db: Database connection to PostgreSQL
//
// Returns:
//   - *PostgreSQLFileHandler: Configured File handler instance
//   - error: Error if the decorated handler creation fails
func NewPostgreSQLFileHandler(db *sql.DB) (*PostgreSQLFileHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLFileHandler{db: db, decorated: decoratedHandler}, nil
}

// Create persists a new File submodel element to the database.
// It first creates the base SubmodelElement data using the decorated handler,
// then adds File-specific data including content type and value.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - submodelElement: The File element to create (must be of type *gen.File)
//
// Returns:
//   - int: The database ID of the created file element
//   - error: Error if the element is not a File or if database operations fail
func (p PostgreSQLFileHandler) Create(tx *sql.Tx, submodelID string, submodelElement gen.SubmodelElement) (int, error) {
	file, ok := submodelElement.(*gen.File)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type File")
	}
	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelID, submodelElement)
	if err != nil {
		return 0, err
	}

	// File-specific database insertion
	_, err = tx.Exec(`INSERT INTO file_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, file.ContentType, file.Value)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// CreateNested persists a new nested File submodel element to the database.
// It creates the File as a child element of another SubmodelElement with a specific position
// and idShortPath for hierarchical organization.
//
// Parameters:
//   - tx: Database transaction to use for the operation
//   - submodelID: ID of the parent submodel
//   - parentID: Database ID of the parent SubmodelElement
//   - idShortPath: Path identifier for the nested element
//   - submodelElement: The File element to create (must be of type *gen.File)
//   - pos: Position of the element within its parent
//
// Returns:
//   - int: The database ID of the created nested file element
//   - error: Error if the element is not a File or if database operations fail
func (p PostgreSQLFileHandler) CreateNested(tx *sql.Tx, submodelID string, parentID int, idShortPath string, submodelElement gen.SubmodelElement, pos int, rootSubmodelElementID int) (int, error) {
	file, ok := submodelElement.(*gen.File)
	if !ok {
		return 0, common.NewErrBadRequest("submodelElement is not of type File")
	}

	// Create the nested file with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateWithPath(tx, submodelID, parentID, idShortPath, submodelElement, pos, rootSubmodelElementID)
	if err != nil {
		return 0, err
	}

	// File-specific database insertion for nested element
	_, err = tx.Exec(`INSERT INTO file_element (id, content_type, value) VALUES ($1, $2, $3)`,
		id, file.ContentType, file.Value)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// Update modifies an existing File submodel element in the database.
// If the file value is changed and an OID exists, the old Large Object is deleted.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to update
//   - submodelElement: The updated File element data
//
// Returns:
//   - error: Error if the update operation fails
func (p PostgreSQLFileHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	file, ok := submodelElement.(*gen.File)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type File")
	}

	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		} else {
			_ = tx.Commit()
		}
	}()

	dialect := goqu.Dialect("postgres")

	// Get the current file element ID and value
	var elementID int64
	var currentValue string
	query, args, err := dialect.From("submodel_element").
		InnerJoin(
			goqu.T("file_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("file_element.id"))),
		).
		Select("submodel_element.id", "file_element.value").
		Where(goqu.C("idshort_path").Eq(idShortOrPath)).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	err = tx.QueryRow(query, args...).Scan(&elementID, &currentValue)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("file element not found")
		}
		return fmt.Errorf("failed to get current file element: %w", err)
	}

	// Check if the value has changed
	if currentValue != file.Value {
		// Check if there's an OID in file_data for this element
		var oldOID sql.NullInt64
		fileDataQuery, fileDataArgs, err := dialect.From("file_data").
			Select("file_oid").
			Where(goqu.C("id").Eq(elementID)).
			ToSQL()
		if err != nil {
			return fmt.Errorf("failed to build file_data query: %w", err)
		}

		err = tx.QueryRow(fileDataQuery, fileDataArgs...).Scan(&oldOID)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("failed to check existing file data: %w", err)
		}

		// If an OID exists, delete the Large Object
		if oldOID.Valid {
			_, err = tx.Exec(`SELECT lo_unlink($1)`, oldOID.Int64)
			if err != nil {
				return fmt.Errorf("failed to delete large object: %w", err)
			}

			// Delete the file_data entry
			deleteQuery, deleteArgs, err := dialect.Delete("file_data").
				Where(goqu.C("id").Eq(elementID)).
				ToSQL()
			if err != nil {
				return fmt.Errorf("failed to build delete query: %w", err)
			}

			_, err = tx.Exec(deleteQuery, deleteArgs...)
			if err != nil {
				return fmt.Errorf("failed to delete file_data: %w", err)
			}
		}

		// Update the file_element with the new value and content type
		updateQuery, updateArgs, err := dialect.Update("file_element").
			Set(goqu.Record{
				"value":        file.Value,
				"content_type": file.ContentType,
			}).
			Where(goqu.C("id").Eq(elementID)).
			ToSQL()
		if err != nil {
			return fmt.Errorf("failed to build update query: %w", err)
		}

		_, err = tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			return fmt.Errorf("failed to update file_element: %w", err)
		}
	}

	// Update base SubmodelElement properties
	return p.decorated.Update(idShortOrPath, submodelElement)
}

// Delete removes a File submodel element from the database.
// Currently delegates to the decorated handler for base SubmodelElement deletion.
// File-specific data is automatically deleted due to foreign key constraints.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to delete
//
// Returns:
//   - error: Error if the delete operation fails
func (p PostgreSQLFileHandler) Delete(idShortOrPath string) error {
	return p.decorated.Delete(idShortOrPath)
}

// UploadFileAttachment uploads a file to PostgreSQL's Large Object system and stores the OID reference.
// This method handles the complete upload process including cleaning up any existing file data.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortPath: Path to the file element within the submodel
//   - file: The file to upload
//
// Returns:
//   - error: Error if the upload operation fails
//
//nolint:revive // cyclomatic complexity is acceptable for this function as the SQL process is complex and requires multiple steps, refactoring would not improve readability
func (p PostgreSQLFileHandler) UploadFileAttachment(submodelID string, idShortPath string, file *os.File, fileName string) error {
	dialect := goqu.Dialect("postgres")

	// Validate and clean the file path
	filePath := filepath.Clean(file.Name())

	// Optional: Ensure it's within an expected base directory
	expectedBaseDir := "/tmp" // or your configured upload directory
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to resolve file path: %w", err)
	}
	absBaseDir, err := filepath.Abs(expectedBaseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve base directory: %w", err)
	}
	// Ensure base directory ends with separator for proper prefix matching
	if !strings.HasSuffix(absBaseDir, string(filepath.Separator)) {
		absBaseDir += string(filepath.Separator)
	}
	if !strings.HasPrefix(absPath+string(filepath.Separator), absBaseDir) {
		return fmt.Errorf("file path outside allowed directory")
	}

	// Reopen the file since it might be closed by the OpenAPI framework
	reopenedFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to reopen file: %w", err)
	}
	defer func() {
		_ = reopenedFile.Close()
		// Clean up the temporary file
		_ = os.Remove(filePath)
	}()

	// Detect content type from file content
	contentTypeBuffer := make([]byte, 512)
	n, err := reopenedFile.Read(contentTypeBuffer)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read file for content type detection: %w", err)
	}
	detectedContentType := "application/octet-stream" // Default
	if n > 0 {
		detectedContentType = http.DetectContentType(contentTypeBuffer[:n])
	}

	// Seek back to the beginning of the file
	_, err = reopenedFile.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to seek file: %w", err)
	}

	// Start a transaction for atomic operation
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Get the submodel element ID
	var submodelElementID int64
	query, args, err := dialect.From("submodel_element").
		Select("id").
		Where(goqu.C("submodel_id").Eq(submodelID), goqu.C("idshort_path").Eq(idShortPath)).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	err = tx.QueryRow(query, args...).Scan(&submodelElementID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("submodel element not found")
		}
		return fmt.Errorf("failed to get submodel element ID: %w", err)
	}

	// Check for existing file_data and delete old Large Object if it exists
	var oldOID sql.NullInt64
	fileDataQuery, fileDataArgs, err := dialect.From("file_data").
		Select("file_oid").
		Where(goqu.C("id").Eq(submodelElementID)).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build file_data query: %w", err)
	}

	err = tx.QueryRow(fileDataQuery, fileDataArgs...).Scan(&oldOID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check existing file data: %w", err)
	}

	// Delete old Large Object if it exists
	if oldOID.Valid {
		_, err = tx.Exec(`SELECT lo_unlink($1)`, oldOID.Int64)
		if err != nil {
			return fmt.Errorf("failed to delete old large object: %w", err)
		}
	}

	// Create a new Large Object
	var newOID int64
	err = tx.QueryRow(`SELECT lo_create(0)`).Scan(&newOID)
	if err != nil {
		return fmt.Errorf("failed to create large object: %w", err)
	}

	// Open the Large Object for writing (0x00020000 = INV_WRITE mode)
	var loFD int
	err = tx.QueryRow(`SELECT lo_open($1, $2)`, newOID, 0x00020000).Scan(&loFD)
	if err != nil {
		return fmt.Errorf("failed to open large object: %w", err)
	}

	// Read file content and write to Large Object in chunks
	buffer := make([]byte, 8192) // 8KB chunks
	for {
		n, readErr := reopenedFile.Read(buffer)
		if n > 0 {
			_, err = tx.Exec(`SELECT lowrite($1, $2)`, loFD, buffer[:n])
			if err != nil {
				_, _ = tx.Exec(`SELECT lo_close($1)`, loFD)
				return fmt.Errorf("failed to write to large object: %w", err)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			_, _ = tx.Exec(`SELECT lo_close($1)`, loFD)
			return fmt.Errorf("failed to read file: %w", readErr)
		}
	}

	// Close the Large Object
	_, err = tx.Exec(`SELECT lo_close($1)`, loFD)
	if err != nil {
		return fmt.Errorf("failed to close large object: %w", err)
	}

	// Update or insert file_data entry with the new OID
	if oldOID.Valid {
		// Update existing entry using GoQu
		updateQuery, updateArgs, err := dialect.Update("file_data").
			Set(goqu.Record{"file_oid": newOID}).
			Where(goqu.C("id").Eq(submodelElementID)).
			ToSQL()
		if err != nil {
			return fmt.Errorf("failed to build update query: %w", err)
		}
		_, err = tx.Exec(updateQuery, updateArgs...)
		if err != nil {
			return fmt.Errorf("failed to update file_oid: %w", err)
		}
	} else {
		// Insert new entry using GoQu
		insertQuery, insertArgs, err := dialect.Insert("file_data").
			Rows(goqu.Record{"id": submodelElementID, "file_oid": newOID}).
			ToSQL()
		if err != nil {
			return fmt.Errorf("failed to build insert query: %w", err)
		}
		_, err = tx.Exec(insertQuery, insertArgs...)
		if err != nil {
			return fmt.Errorf("failed to insert file_oid: %w", err)
		}
	}

	// Update file_element.value to reference the OID and content_type
	updateFileElementQuery, updateFileElementArgs, err := dialect.Update("file_element").
		Set(goqu.Record{
			"value":        fmt.Sprintf("%d", newOID),
			"file_name":    fileName,
			"content_type": detectedContentType,
		}).
		Where(goqu.C("id").Eq(submodelElementID)).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build file_element update query: %w", err)
	}
	_, err = tx.Exec(updateFileElementQuery, updateFileElementArgs...)
	if err != nil {
		return fmt.Errorf("failed to update file_element value: %w", err)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DownloadFileAttachment retrieves a file from PostgreSQL's Large Object system.
// This method reads the file content based on the OID stored in file_data.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortPath: Path to the file element within the submodel
//
// Returns:
//   - []byte: The file content
//   - string: The content type
//   - error: Error if the download operation fails
func (p PostgreSQLFileHandler) DownloadFileAttachment(submodelID string, idShortPath string) ([]byte, string, string, error) {
	dialect := goqu.Dialect("postgres")

	// Get the submodel element ID and content type
	var submodelElementID int64
	var contentType string
	var fileName string
	tx, err := p.db.Begin()
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		} else {
			_ = tx.Commit()
		}
	}()

	query, args, err := dialect.From("submodel_element").
		InnerJoin(
			goqu.T("file_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("file_element.id"))),
		).
		Select("submodel_element.id", "file_element.content_type", "file_element.file_name").
		Where(
			goqu.C("submodel_id").Eq(submodelID),
			goqu.C("idshort_path").Eq(idShortPath),
		).
		ToSQL()
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to build query: %w", err)
	}

	err = tx.QueryRow(query, args...).Scan(&submodelElementID, &contentType, &fileName)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", "", common.NewErrNotFound("file element not found")
		}
		return nil, "", "", fmt.Errorf("failed to get file element: %w", err)
	}

	// Get the file OID from file_data
	var fileOID sql.NullInt64
	fileDataQuery, fileDataArgs, err := dialect.From("file_data").
		Select("file_oid").
		Where(goqu.C("id").Eq(submodelElementID)).
		ToSQL()
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to build file_data query: %w", err)
	}

	err = tx.QueryRow(fileDataQuery, fileDataArgs...).Scan(&fileOID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", "", common.NewErrNotFound("file data not found")
		}
		return nil, "", "", fmt.Errorf("failed to get file OID: %w", err)
	}

	if !fileOID.Valid {
		return nil, "", "", common.NewErrNotFound("file OID is null")
	}

	// Open the Large Object for reading (0x00040000 = INV_READ mode)
	var loFD int
	err = tx.QueryRow(`SELECT lo_open($1, $2)`, fileOID.Int64, 0x00040000).Scan(&loFD)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to open large object: %w", err)
	}

	// Read the Large Object content in chunks
	var fileContent []byte
	for {
		var bytesRead []byte
		err = tx.QueryRow(`SELECT loread($1, $2)`, loFD, 8192).Scan(&bytesRead)
		if err != nil {
			_, _ = tx.Exec(`SELECT lo_close($1)`, loFD)
			return nil, "", "", fmt.Errorf("failed to read large object: %w", err)
		}
		if len(bytesRead) == 0 {
			break
		}
		fileContent = append(fileContent, bytesRead...)
	}

	// Close the Large Object
	_, err = tx.Exec(`SELECT lo_close($1)`, loFD)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to close large object: %w", err)
	}

	return fileContent, contentType, fileName, nil
}

// DeleteFileAttachment deletes a file from PostgreSQL's Large Object system.
// This method removes the Large Object and clears the file_data entry, setting the File SME value to empty.
//
// Parameters:
//   - submodelID: ID of the parent submodel
//   - idShortPath: Path to the file element within the submodel
//
// Returns:
//   - error: Error if the deletion operation fails
func (p PostgreSQLFileHandler) DeleteFileAttachment(submodelID string, idShortPath string) error {
	dialect := goqu.Dialect("postgres")

	// Get the submodel element ID
	var submodelElementID int64
	query, args, err := dialect.From("submodel_element").
		Select("id").
		Where(
			goqu.C("submodel_id").Eq(submodelID),
			goqu.C("idshort_path").Eq(idShortPath),
		).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		} else {
			_ = tx.Commit()
		}
	}()

	err = tx.QueryRow(query, args...).Scan(&submodelElementID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("file element not found")
		}
		return fmt.Errorf("failed to get file element: %w", err)
	}

	// Get the file OID from file_data
	var fileOID sql.NullInt64
	fileDataQuery, fileDataArgs, err := dialect.From("file_data").
		Select("file_oid").
		Where(goqu.C("id").Eq(submodelElementID)).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build file_data query: %w", err)
	}

	err = tx.QueryRow(fileDataQuery, fileDataArgs...).Scan(&fileOID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get file OID: %w", err)
	}

	// If an OID exists, delete the Large Object
	if fileOID.Valid {
		_, err = tx.Exec(`SELECT lo_unlink($1)`, fileOID.Int64)
		if err != nil {
			return fmt.Errorf("failed to delete large object: %w", err)
		}

		// Delete the file_data entry
		deleteQuery, deleteArgs, err := dialect.Delete("file_data").
			Where(goqu.C("id").Eq(submodelElementID)).
			ToSQL()
		if err != nil {
			return fmt.Errorf("failed to build delete query: %w", err)
		}

		_, err = tx.Exec(deleteQuery, deleteArgs...)
		if err != nil {
			return fmt.Errorf("failed to delete file_data: %w", err)
		}
	}

	// Clear the value in file_element (set to empty string)
	updateQuery, updateArgs, err := dialect.Update("file_element").
		Set(goqu.Record{"value": ""}).
		Where(goqu.C("id").Eq(submodelElementID)).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	_, err = tx.Exec(updateQuery, updateArgs...)
	if err != nil {
		return fmt.Errorf("failed to update file_element: %w", err)
	}

	return nil
}
