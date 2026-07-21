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
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
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

// Update modifies an existing File submodel element in the database.
// If the file value is changed and an OID exists, the old Large Object is deleted.
//
// Parameters:
//   - idShortOrPath: The idShort or path identifier of the element to update
//   - submodelElement: The updated File element data
//
// Returns:
//   - error: Error if the update operation fails
func (p PostgreSQLFileHandler) Update(submodelID string, idShortOrPath string, submodelElement types.ISubmodelElement, tx *sql.Tx, isPut bool) error {
	file, ok := submodelElement.(*types.File)
	if !ok {
		return common.NewErrBadRequest("submodelElement is not of type File")
	}

	var err error
	cu, localTx, err := common.StartTXIfNeeded(tx, err, p.db)
	if err != nil {
		return err
	}
	defer cu(&err)
	err = p.decorated.Update(submodelID, idShortOrPath, submodelElement, localTx, isPut)
	if err != nil {
		return err
	}

	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(localTx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("submodel not found")
		}
		return fmt.Errorf("failed to get submodel database ID: %w", err)
	}

	dialect := goqu.Dialect("postgres")

	// Get the current file element ID and value
	var elementID int64
	var currentValue sql.NullString
	query, args, err := dialect.From("submodel_element").
		InnerJoin(
			goqu.T("file_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("file_element.id"))),
		).
		Select("submodel_element.id", "file_element.value").
		Where(goqu.C("idshort_path").Eq(idShortOrPath)).
		Where(goqu.C("submodel_id").Eq(submodelDatabaseID)).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

	err = localTx.QueryRow(query, args...).Scan(&elementID, &currentValue)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("file element not found")
		}
		return fmt.Errorf("failed to get current file element: %w", err)
	}

	newValue := ""
	if file.Value() != nil {
		newValue = *file.Value()
	}
	hasFileValueChanged := currentValue.String != newValue
	if hasFileValueChanged {
		if err = deleteManagedFileReference(localTx, elementID); err != nil {
			return err
		}
		// Check if there's an OID in file_data for this element
		var oldOID sql.NullInt64
		fileDataQuery, fileDataArgs, err := dialect.From("file_data").
			Select("file_oid").
			Where(goqu.C("id").Eq(elementID)).
			ToSQL()
		if err != nil {
			return fmt.Errorf("failed to build file_data query: %w", err)
		}

		err = localTx.QueryRow(fileDataQuery, fileDataArgs...).Scan(&oldOID)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("failed to check existing file data: %w", err)
		}

		// If an OID exists, delete the Large Object
		if oldOID.Valid {
			err = removeLOFile(localTx, oldOID, dialect, elementID)
			if err != nil {
				return err
			}
		}

		// Update the file_element with the new value and content type
		updateQuery, updateArgs, err := dialect.Update("file_element").
			Set(goqu.Record{
				"value":        file.Value(),
				"content_type": file.ContentType(),
			}).
			Where(goqu.C("id").Eq(elementID)).
			ToSQL()
		if err != nil {
			return fmt.Errorf("failed to build update query: %w", err)
		}

		_, err = localTx.Exec(updateQuery, updateArgs...)
		if err != nil {
			return fmt.Errorf("failed to update file_element: %w", err)
		}
	} else {
		// Only Update content type if value hasn't changed
		updateQuery, updateArgs, err := dialect.Update("file_element").
			Set(goqu.Record{
				"content_type": file.ContentType(),
			}).
			Where(goqu.C("id").Eq(elementID)).
			ToSQL()
		if err != nil {
			return fmt.Errorf("failed to build update query: %w", err)
		}
		_, err = localTx.Exec(updateQuery, updateArgs...)
		if err != nil {
			return fmt.Errorf("failed to update file_element content type: %w", err)
		}
	}

	return common.CommitTransactionIfNeeded(tx, localTx)
}

// UpdateValueOnly updates only the value of an existing File submodel element identified by its idShort or path.
// It processes the new value and updates nested elements accordingly.
//
// Parameters:
//   - submodelID: The ID of the parent submodel
//   - idShortOrPath: The idShort or path identifying the element to update
//   - valueOnly: The new value to set (must be of type gen.FileValue)
//
// Returns:
//   - error: An error if the update operation fails
func (p PostgreSQLFileHandler) UpdateValueOnly(submodelID string, idShortOrPath string, valueOnly gen.SubmodelElementValue, tx *sql.Tx) error {
	fileValueOnly, ok := valueOnly.(gen.FileValue)
	if !ok {
		return common.NewErrBadRequest("valueOnly is not of type FileValue")
	}
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("submodel not found")
		}
		return fmt.Errorf("failed to get submodel database ID: %w", err)
	}

	dialect := goqu.Dialect("postgres")

	var elementID int
	var currentValue sql.NullString
	query, args, err := dialect.From("submodel_element").
		InnerJoin(
			goqu.T("file_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("file_element.id"))),
		).
		Select("submodel_element.id", "file_element.value").
		Where(
			goqu.C("idshort_path").Eq(idShortOrPath),
			goqu.C("submodel_id").Eq(submodelDatabaseID),
		).
		ToSQL()
	if err != nil {
		return err
	}

	err = tx.QueryRow(query, args...).Scan(&elementID, &currentValue)
	if err != nil {
		return err
	}

	if currentValue.String != fileValueOnly.Value {
		if err = deleteLegacyFileData(tx, dialect, int64(elementID)); err != nil {
			return err
		}
		if err = deleteManagedFileReference(tx, int64(elementID)); err != nil {
			return err
		}
	}

	// Build the update query
	updateQuery, args, err := dialect.Update("file_element").
		Set(goqu.Record{
			"content_type": fileValueOnly.ContentType,
			"value":        fileValueOnly.Value,
		}).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	_, err = tx.Exec(updateQuery, args...)
	if err != nil {
		return common.NewInternalServerError(fmt.Sprintf("failed to execute update query: %s", err))
	}

	return nil
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

// GetInsertQueryPart returns the type-specific insert query part for batch insertion of File elements.
// It returns the table name and record for inserting into the file_element table.
//
// Parameters:
//   - tx: Active database transaction (not used for File)
//   - id: The database ID of the base submodel_element record
//   - element: The File element to insert
//
// Returns:
//   - *InsertQueryPart: The table name and record for file_element insert
//   - error: An error if the element is not of type File
func (p PostgreSQLFileHandler) GetInsertQueryPart(_ *sql.Tx, id int, element types.ISubmodelElement) (*InsertQueryPart, error) {
	file, ok := element.(*types.File)
	if !ok {
		return nil, common.NewErrBadRequest("submodelElement is not of type File")
	}

	return &InsertQueryPart{
		TableName: "file_element",
		Record: goqu.Record{
			"id":           id,
			"content_type": file.ContentType(),
			"value":        file.Value(),
		},
	}, nil
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
	return withReopenedUploadFile(file, func(uploadFile io.Reader) error {
		return p.UploadFileAttachmentReader(submodelID, idShortPath, uploadFile, fileName)
	})
}

// UploadFileAttachmentReader uploads attachment content from a reader.
func (p PostgreSQLFileHandler) UploadFileAttachmentReader(submodelID string, idShortPath string, file io.Reader, fileName string) error {
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := p.UploadFileAttachmentReaderTx(tx, submodelID, idShortPath, file, fileName); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true
	return nil
}

// UploadFileAttachmentTx uploads attachment content using the provided transaction.
func (p PostgreSQLFileHandler) UploadFileAttachmentTx(tx *sql.Tx, submodelID string, idShortPath string, file *os.File, fileName string) error {
	return withReopenedUploadFile(file, func(uploadFile io.Reader) error {
		return p.UploadFileAttachmentReaderTx(tx, submodelID, idShortPath, uploadFile, fileName)
	})
}

// UploadFileAttachmentReaderTx uploads attachment content from a reader using the provided transaction.
func (p PostgreSQLFileHandler) UploadFileAttachmentReaderTx(tx *sql.Tx, submodelID string, idShortPath string, file io.Reader, fileName string) error {
	dialect := goqu.Dialect("postgres")
	if file == nil {
		return common.NewErrBadRequest("SMREPO-UPLOADATTACHMENT-MISSINGFILE file payload is required")
	}

	metadata, err := readFileElementUploadMetadata(tx, dialect, submodelID, idShortPath)
	if err != nil {
		return err
	}

	resolvedFileName, resolvedContentType, uploadContent, err := resolveUploadFileMetadata(file, fileName, metadata)
	if err != nil {
		return err
	}

	oldOID, err := readExistingFileOID(tx, dialect, metadata.elementID)
	if err != nil {
		return err
	}

	if oldOID.Valid {
		if err := unlinkLargeObject(tx, oldOID.Int64); err != nil {
			return err
		}
	}

	newOID, err := createAndWriteLargeObject(tx, uploadContent)
	if err != nil {
		return err
	}
	if err := upsertFileDataOID(tx, dialect, metadata.elementID, newOID, oldOID.Valid); err != nil {
		return err
	}
	return updateFileElementAttachment(tx, dialect, metadata.elementID, newOID, resolvedFileName, resolvedContentType)
}

// UploadManagedFileAttachmentReaderTx stores a deduplicated internal attachment.
func (p PostgreSQLFileHandler) UploadManagedFileAttachmentReaderTx(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string, file io.Reader, fileName string) (binarycontent.Reference, string, error) {
	if file == nil {
		return binarycontent.Reference{}, "", common.NewErrBadRequest("SMREPO-UPLOADATTACHMENT-MISSINGFILE file payload is required")
	}
	dialect := goqu.Dialect("postgres")
	metadata, err := readFileElementUploadMetadata(tx, dialect, submodelID, idShortPath)
	if err != nil {
		return binarycontent.Reference{}, "", err
	}
	resolvedFileName, resolvedContentType, uploadContent, err := resolveUploadFileMetadata(file, fileName, metadata)
	if err != nil {
		return binarycontent.Reference{}, "", err
	}
	reference, err := binarycontent.StoreReferenceTx(
		ctx, tx, uploadContent, binarycontent.TableFileReference, "file_element_id", metadata.elementID, resolvedFileName,
	)
	if err != nil {
		return binarycontent.Reference{}, "", err
	}
	if err = deleteLegacyFileData(tx, dialect, metadata.elementID); err != nil {
		return binarycontent.Reference{}, "", err
	}
	if err = updateManagedFileElementAttachment(tx, dialect, reference, resolvedContentType); err != nil {
		return binarycontent.Reference{}, "", err
	}
	return reference, resolvedContentType, nil
}

// UploadManagedFileAttachmentTx stores a deduplicated attachment from a file.
func (p PostgreSQLFileHandler) UploadManagedFileAttachmentTx(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string, file *os.File, fileName string) (binarycontent.Reference, string, error) {
	var reference binarycontent.Reference
	var contentType string
	err := withReopenedUploadFile(file, func(uploadFile io.Reader) error {
		var uploadErr error
		reference, contentType, uploadErr = p.UploadManagedFileAttachmentReaderTx(ctx, tx, submodelID, idShortPath, uploadFile, fileName)
		return uploadErr
	})
	return reference, contentType, err
}

type fileElementUploadMetadata struct {
	elementID           int64
	existingContentType sql.NullString
	existingFileName    sql.NullString
}

func withReopenedUploadFile(file *os.File, useFile func(io.Reader) error) error {
	reopenedFile, err := reopenUploadedFile(file)
	if err != nil {
		return err
	}
	defer func() {
		_ = reopenedFile.Close()
	}()

	return useFile(reopenedFile)
}

func reopenUploadedFile(file *os.File) (*os.File, error) {
	if file == nil {
		return nil, common.NewErrBadRequest("SMREPO-UPLOADATTACHMENT-MISSINGFILE file payload is required")
	}

	filePath := filepath.Clean(file.Name())
	// #nosec G703 -- path comes from server-created temporary file and is normalized with filepath.Clean.
	reopenedFile, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to reopen file: %w", err)
	}
	return reopenedFile, nil
}

func readFileElementUploadMetadata(tx *sql.Tx, dialect goqu.DialectWrapper, submodelID string, idShortPath string) (fileElementUploadMetadata, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseIDForUpdate(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fileElementUploadMetadata{}, common.NewErrNotFound("submodel not found")
		}
		return fileElementUploadMetadata{}, fmt.Errorf("failed to get submodel database ID: %w", err)
	}

	query, args, err := dialect.From("submodel_element").
		InnerJoin(
			goqu.T("file_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("file_element.id"))),
		).
		Select("submodel_element.id", "file_element.content_type", "file_element.file_name").
		Where(goqu.C("submodel_id").Eq(submodelDatabaseID), goqu.C("idshort_path").Eq(idShortPath)).
		ToSQL()
	if err != nil {
		return fileElementUploadMetadata{}, fmt.Errorf("failed to build query: %w", err)
	}

	var metadata fileElementUploadMetadata
	err = tx.QueryRow(query, args...).Scan(&metadata.elementID, &metadata.existingContentType, &metadata.existingFileName)
	if err != nil {
		if err == sql.ErrNoRows {
			return fileElementUploadMetadata{}, common.NewErrNotFound("submodel element not found")
		}
		return fileElementUploadMetadata{}, fmt.Errorf("failed to get submodel element ID: %w", err)
	}
	return metadata, nil
}

func resolveUploadFileMetadata(file io.Reader, fileName string, metadata fileElementUploadMetadata) (string, string, io.Reader, error) {
	detectedContentType, uploadContent, err := common.SniffContentTypeReader(file)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to read file for content type detection: %w", err)
	}

	resolvedFileName := strings.TrimSpace(fileName)
	if resolvedFileName == "" && metadata.existingFileName.Valid {
		resolvedFileName = metadata.existingFileName.String
	}

	resolvedContentType, mismatchDetectedVsDeclared := common.ResolveUploadedContentType(detectedContentType, metadata.existingContentType.String, resolvedFileName)
	if mismatchDetectedVsDeclared {
		log.Printf("[WARN] SMREPO-UPLOADATTACHMENT-RESOLVEMIME detected content type differs from declared content type; using detected content type")
	}

	return resolvedFileName, resolvedContentType, uploadContent, nil
}

func readExistingFileOID(tx *sql.Tx, dialect goqu.DialectWrapper, elementID int64) (sql.NullInt64, error) {
	fileDataQuery, fileDataArgs, err := dialect.From("file_data").
		Select("file_oid").
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return sql.NullInt64{}, fmt.Errorf("failed to build file_data query: %w", err)
	}

	var oldOID sql.NullInt64
	err = tx.QueryRow(fileDataQuery, fileDataArgs...).Scan(&oldOID)
	if err != nil && err != sql.ErrNoRows {
		return sql.NullInt64{}, fmt.Errorf("failed to check existing file data: %w", err)
	}
	return oldOID, nil
}

func unlinkLargeObject(tx *sql.Tx, oid int64) error {
	if _, err := tx.Exec(`SELECT lo_unlink($1)`, oid); err != nil {
		return fmt.Errorf("failed to delete old large object: %w", err)
	}
	return nil
}

func createAndWriteLargeObject(tx *sql.Tx, file io.Reader) (int64, error) {
	var newOID int64
	if err := tx.QueryRow(`SELECT lo_create(0)`).Scan(&newOID); err != nil {
		return 0, fmt.Errorf("failed to create large object: %w", err)
	}

	var loFD int
	if err := tx.QueryRow(`SELECT lo_open($1, $2)`, newOID, 0x00020000).Scan(&loFD); err != nil {
		return 0, fmt.Errorf("failed to open large object: %w", err)
	}
	if err := writeLargeObject(tx, file, loFD); err != nil {
		return 0, err
	}
	return newOID, nil
}

func writeLargeObject(tx *sql.Tx, file io.Reader, loFD int) error {
	buffer := make([]byte, 8192)
	for {
		n, readErr := file.Read(buffer)
		if n > 0 {
			if _, err := tx.Exec(`SELECT lowrite($1, $2)`, loFD, buffer[:n]); err != nil {
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
	if _, err := tx.Exec(`SELECT lo_close($1)`, loFD); err != nil {
		return fmt.Errorf("failed to close large object: %w", err)
	}
	return nil
}

func upsertFileDataOID(tx *sql.Tx, dialect goqu.DialectWrapper, elementID int64, oid int64, exists bool) error {
	if exists {
		updateQuery, updateArgs, err := dialect.Update("file_data").
			Set(goqu.Record{"file_oid": oid}).
			Where(goqu.C("id").Eq(elementID)).
			ToSQL()
		if err != nil {
			return fmt.Errorf("failed to build update query: %w", err)
		}
		if _, err = tx.Exec(updateQuery, updateArgs...); err != nil {
			return fmt.Errorf("failed to update file_oid: %w", err)
		}
		return nil
	}

	insertQuery, insertArgs, err := dialect.Insert("file_data").
		Rows(goqu.Record{"id": elementID, "file_oid": oid}).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build insert query: %w", err)
	}
	if _, err = tx.Exec(insertQuery, insertArgs...); err != nil {
		return fmt.Errorf("failed to insert file_oid: %w", err)
	}
	return nil
}

func updateFileElementAttachment(tx *sql.Tx, dialect goqu.DialectWrapper, elementID int64, oid int64, fileName string, contentType string) error {
	updateFileElementQuery, updateFileElementArgs, err := dialect.Update("file_element").
		Set(goqu.Record{
			"value":        fmt.Sprintf("%d", oid),
			"file_name":    fileName,
			"content_type": contentType,
		}).
		Where(goqu.C("id").Eq(elementID)).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build file_element update query: %w", err)
	}
	if _, err = tx.Exec(updateFileElementQuery, updateFileElementArgs...); err != nil {
		return fmt.Errorf("failed to update file_element value: %w", err)
	}
	return nil
}

func updateManagedFileElementAttachment(tx *sql.Tx, dialect goqu.DialectWrapper, reference binarycontent.Reference, contentType string) error {
	query, args, err := dialect.Update("file_element").Set(goqu.Record{
		"value": reference.ManagedPath(), "file_name": reference.SafeFileName, "content_type": contentType,
	}).Where(goqu.C("id").Eq(reference.OwnerID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-UPLOADATTACHMENT-BUILDUPDATE " + err.Error())
	}
	if _, err = tx.Exec(query, args...); err != nil {
		return common.NewInternalServerError("SMREPO-UPLOADATTACHMENT-UPDATE " + err.Error())
	}
	return nil
}

func deleteLegacyFileData(tx *sql.Tx, dialect goqu.DialectWrapper, elementID int64) error {
	oldOID, err := readExistingFileOID(tx, dialect, elementID)
	if err != nil {
		return err
	}
	if !oldOID.Valid {
		return nil
	}
	return removeLOFile(tx, oldOID, dialect, elementID)
}

func deleteManagedFileReference(tx *sql.Tx, elementID int64) error {
	query, args, err := goqu.Delete(binarycontent.TableFileReference).
		Where(goqu.C("file_element_id").Eq(elementID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-FILEREFERENCE-BUILDDELETE " + err.Error())
	}
	if _, err = tx.Exec(query, args...); err != nil {
		return common.NewInternalServerError("SMREPO-FILEREFERENCE-DELETE " + err.Error())
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

	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", "", common.NewErrNotFound("submodel not found")
		}
		return nil, "", "", fmt.Errorf("failed to get submodel database ID: %w", err)
	}

	query, args, err := dialect.From("submodel_element").
		InnerJoin(
			goqu.T("file_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("file_element.id"))),
		).
		Select("submodel_element.id", "file_element.content_type", "file_element.file_name").
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
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

// DownloadManagedFileAttachment reads canonical content through its owning File SME.
func (p PostgreSQLFileHandler) DownloadManagedFileAttachment(ctx context.Context, submodelID string, idShortPath string) ([]byte, string, string, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", "", common.NewInternalServerError("SMREPO-DOWNLOADATTACHMENT-STARTTX " + err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	metadata, err := readDownloadFileMetadata(ctx, tx, submodelID, idShortPath)
	if err != nil {
		return nil, "", "", err
	}
	reference, err := binarycontent.LoadReferenceTx(ctx, tx, binarycontent.TableFileReference, "file_element_id", metadata.elementID)
	if err == nil {
		content, readErr := binarycontent.ReadAllTx(ctx, tx, reference.Content)
		if readErr != nil {
			return nil, "", "", readErr
		}
		if err = tx.Commit(); err != nil {
			return nil, "", "", common.NewInternalServerError("SMREPO-DOWNLOADATTACHMENT-COMMIT " + err.Error())
		}
		committed = true
		return content, metadata.contentType, metadata.fileName, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, "", "", err
	}
	content, err := readLegacyFileContent(ctx, tx, metadata.elementID)
	if err != nil {
		return nil, "", "", err
	}
	if err = tx.Commit(); err != nil {
		return nil, "", "", common.NewInternalServerError("SMREPO-DOWNLOADATTACHMENT-COMMIT " + err.Error())
	}
	committed = true
	return content, metadata.contentType, metadata.fileName, nil
}

// StreamManagedFileAttachment supplies attachment metadata and a bounded-memory reader.
//
// Parameters:
//   - ctx: Request context preserving authorization and cancellation.
//   - submodelID: Identifier of the attachment's parent submodel.
//   - idShortPath: Path of the File submodel element.
//   - consume: Callback receiving content type, filename, known size, and a scoped reader.
//
// Returns:
//   - error: Lookup, consumer, stream, or transaction error.
func (p PostgreSQLFileHandler) StreamManagedFileAttachment(
	ctx context.Context,
	submodelID string,
	idShortPath string,
	consume func(string, string, int64, io.Reader) error,
) error {
	if consume == nil {
		return common.NewInternalServerError("SMREPO-STREAMATTACHMENT-NILCONSUMER stream consumer is required")
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("SMREPO-STREAMATTACHMENT-STARTTX " + err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	metadata, err := readDownloadFileMetadata(ctx, tx, submodelID, idShortPath)
	if err != nil {
		return err
	}
	reference, err := binarycontent.LoadReferenceTx(ctx, tx, binarycontent.TableFileReference, "file_element_id", metadata.elementID)
	if err == nil {
		err = binarycontent.StreamTx(ctx, tx, reference.Content, func(reader io.Reader) error {
			return consume(metadata.contentType, metadata.fileName, reference.Content.SizeBytes, reader)
		})
	} else if errors.Is(err, sql.ErrNoRows) {
		var oid int64
		oid, err = readLegacyFileOID(ctx, tx, metadata.elementID)
		if err == nil {
			err = binarycontent.StreamOIDTx(ctx, tx, oid, func(reader io.Reader) error {
				return consume(metadata.contentType, metadata.fileName, 0, reader)
			})
		}
	}
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return common.NewInternalServerError("SMREPO-STREAMATTACHMENT-COMMIT " + err.Error())
	}
	committed = true
	return nil
}

type downloadFileMetadata struct {
	elementID   int64
	contentType string
	fileName    string
}

func readDownloadFileMetadata(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string) (downloadFileMetadata, error) {
	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.From(goqu.T("submodel").As("sm")).
		Join(goqu.T("submodel_element").As("sme"), goqu.On(goqu.I("sme.submodel_id").Eq(goqu.I("sm.id")))).
		Join(goqu.T("file_element").As("fe"), goqu.On(goqu.I("fe.id").Eq(goqu.I("sme.id")))).
		Select(goqu.I("sme.id"), goqu.I("fe.content_type"), goqu.I("fe.file_name")).
		Where(goqu.I("sm.submodel_identifier").Eq(submodelID), goqu.I("sme.idshort_path").Eq(idShortPath)).ToSQL()
	if err != nil {
		return downloadFileMetadata{}, common.NewInternalServerError("SMREPO-DOWNLOADATTACHMENT-BUILDMETADATA " + err.Error())
	}
	var metadata downloadFileMetadata
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&metadata.elementID, &metadata.contentType, &metadata.fileName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return downloadFileMetadata{}, common.NewErrNotFound("SMREPO-DOWNLOADATTACHMENT-NOTFOUND File SME not found")
		}
		return downloadFileMetadata{}, common.NewInternalServerError("SMREPO-DOWNLOADATTACHMENT-METADATA " + err.Error())
	}
	return metadata, nil
}

func readLegacyFileContent(ctx context.Context, tx *sql.Tx, elementID int64) ([]byte, error) {
	oid, err := readLegacyFileOID(ctx, tx, elementID)
	if err != nil {
		return nil, err
	}
	return binarycontent.ReadOIDTx(ctx, tx, oid)
}

func readLegacyFileOID(ctx context.Context, tx *sql.Tx, elementID int64) (int64, error) {
	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.From("file_data").Select("file_oid").Where(goqu.C("id").Eq(elementID)).ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("SMREPO-DOWNLOADATTACHMENT-BUILDLEGACY " + err.Error())
	}
	var oid int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&oid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, common.NewErrNotFound("SMREPO-DOWNLOADATTACHMENT-NODATA attachment content not found")
		}
		return 0, common.NewInternalServerError("SMREPO-DOWNLOADATTACHMENT-LEGACY " + err.Error())
	}
	return oid, nil
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
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := p.DeleteFileAttachmentTx(tx, submodelID, idShortPath); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true
	return nil
}

// DeleteFileAttachmentTx deletes attachment content using the provided transaction.
func (p PostgreSQLFileHandler) DeleteFileAttachmentTx(tx *sql.Tx, submodelID string, idShortPath string) error {
	dialect := goqu.Dialect("postgres")

	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseIDForUpdate(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("submodel not found")
		}
		return fmt.Errorf("failed to get submodel database ID: %w", err)
	}

	// Get the submodel element ID
	var submodelElementID int64
	query, args, err := dialect.From("submodel_element").
		Select("id").
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("idshort_path").Eq(idShortPath),
		).
		ToSQL()
	if err != nil {
		return fmt.Errorf("failed to build query: %w", err)
	}

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

// DeleteManagedFileAttachmentTx removes canonical and legacy attachment associations.
func (p PostgreSQLFileHandler) DeleteManagedFileAttachmentTx(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string) error {
	dialect := goqu.Dialect("postgres")
	metadata, err := readFileElementUploadMetadata(tx, dialect, submodelID, idShortPath)
	if err != nil {
		return err
	}
	if err = binarycontent.DeleteReferenceTx(ctx, tx, binarycontent.TableFileReference, "file_element_id", metadata.elementID); err != nil {
		return err
	}
	if err = deleteLegacyFileData(tx, dialect, metadata.elementID); err != nil {
		return err
	}
	query, args, err := dialect.Update("file_element").Set(goqu.Record{"value": "", "file_name": nil}).
		Where(goqu.C("id").Eq(metadata.elementID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELETEATTACHMENT-BUILDUPDATE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("SMREPO-DELETEATTACHMENT-UPDATE " + err.Error())
	}
	return nil
}

func removeLOFile(tx *sql.Tx, oldOID sql.NullInt64, dialect goqu.DialectWrapper, elementID int64) error {
	_, err := tx.Exec(`SELECT lo_unlink($1)`, oldOID.Int64)
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
	return nil
}
