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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	submodelqueries "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/queries"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
)

// FileAttachmentExists reports whether a File submodel element currently has attachment data stored in file_data.file_oid.
func (s *SubmodelDatabase) FileAttachmentExists(submodelID string, idShortPath string) (bool, error) {
	query, args, err := submodelqueries.BuildFileAttachmentExistsSQL(submodelID, idShortPath)
	if err != nil {
		return false, common.NewInternalServerError("SMREPO-FILEATTEXISTS-BUILDSQL " + err.Error())
	}

	var fileElementID sql.NullInt64
	var fileOID sql.NullInt64
	if scanErr := s.db.QueryRow(query, args...).Scan(&fileElementID, &fileOID); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return false, common.NewErrNotFound("SMREPO-FILEATTEXISTS-NOTFOUND Submodel element not found")
		}
		return false, common.NewInternalServerError("SMREPO-FILEATTEXISTS-QUERY " + scanErr.Error())
	}

	if !fileElementID.Valid {
		return false, common.NewErrMethodNotAllowed("SMREPO-FILEATTEXISTS-NOTFILE Submodel element is not of type File")
	}

	return fileOID.Valid, nil
}

// UploadFileAttachment uploads attachment content for a File submodel element.
func (s *SubmodelDatabase) UploadFileAttachment(submodelID string, idShortPath string, file *os.File, fileName string) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return err
	}

	return fileHandler.UploadFileAttachment(submodelID, idShortPath, file, fileName)
}

// UploadFileAttachmentReader uploads attachment content for a File submodel element from a reader.
func (s *SubmodelDatabase) UploadFileAttachmentReader(submodelID string, idShortPath string, file io.Reader, fileName string) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return err
	}

	return fileHandler.UploadFileAttachmentReader(submodelID, idShortPath, file, fileName)
}

// UploadFileAttachmentWithHistory uploads attachment content and appends the current Submodel snapshot atomically.
func (s *SubmodelDatabase) UploadFileAttachmentWithHistory(ctx context.Context, submodelID string, idShortPath string, file *os.File, fileName string) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return err
	}

	return common.ExecuteInTransaction(s.db, "SMREPO-UPLOADFILEHIST-STARTTX", "SMREPO-UPLOADFILEHIST-COMMIT", func(tx *sql.Tx) error {
		reference, contentType, uploadErr := fileHandler.UploadManagedFileAttachmentTx(ctx, tx, submodelID, idShortPath, file, fileName)
		if uploadErr != nil {
			return uploadErr
		}
		return s.recordFileUploadMutationTx(ctx, tx, submodelID, idShortPath, reference, contentType)
	})
}

// UploadFileAttachmentReaderWithHistory uploads attachment content from a reader and appends the current Submodel snapshot atomically.
func (s *SubmodelDatabase) UploadFileAttachmentReaderWithHistory(ctx context.Context, submodelID string, idShortPath string, file io.Reader, fileName string) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return err
	}

	return common.ExecuteInTransaction(s.db, "SMREPO-UPLOADFILEHIST-STARTTX", "SMREPO-UPLOADFILEHIST-COMMIT", func(tx *sql.Tx) error {
		reference, contentType, uploadErr := fileHandler.UploadManagedFileAttachmentReaderTx(ctx, tx, submodelID, idShortPath, file, fileName)
		if uploadErr != nil {
			return uploadErr
		}
		return s.recordFileUploadMutationTx(ctx, tx, submodelID, idShortPath, reference, contentType)
	})
}

func (s *SubmodelDatabase) recordFileUploadMutationTx(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string, reference binarycontent.Reference, contentType string) error {
	binaryReceipt, err := history.EnsureBinaryEvidenceTx(ctx, tx, reference.Content, contentType)
	if err != nil {
		return err
	}
	mutationCtx := history.WithBinaryReferenceExpected(ctx, reference.ManagedPath())
	if err = s.appendChangedSubmodelElementHistoryTx(mutationCtx, tx, submodelID, submodelElementRootMutation{
		previousPath: idShortPath,
		currentPath:  idShortPath,
	}); err != nil {
		return err
	}
	return history.RecordBinaryReferenceEvidenceTx(
		mutationCtx, tx, history.TableSubmodel, submodelID, reference.Content,
		reference.ManagedPath(), reference.SafeFileName, contentType, binaryReceipt,
	)
}

// DownloadFileAttachment downloads attachment content for a File submodel element.
func (s *SubmodelDatabase) DownloadFileAttachment(submodelID string, idShortPath string) ([]byte, string, string, error) {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return nil, "", "", err
	}

	return fileHandler.DownloadFileAttachment(submodelID, idShortPath)
}

// DownloadFileAttachmentWithContext resolves canonical content through the owning File SME.
func (s *SubmodelDatabase) DownloadFileAttachmentWithContext(ctx context.Context, submodelID string, idShortPath string) ([]byte, string, string, error) {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return nil, "", "", err
	}
	return fileHandler.DownloadManagedFileAttachment(ctx, submodelID, idShortPath)
}

// DeleteFileAttachment deletes attachment content of a File submodel element.
func (s *SubmodelDatabase) DeleteFileAttachment(submodelID string, idShortPath string) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return err
	}

	return fileHandler.DeleteFileAttachment(submodelID, idShortPath)
}

// DeleteFileAttachmentWithHistory deletes attachment content and appends the current Submodel snapshot atomically.
func (s *SubmodelDatabase) DeleteFileAttachmentWithHistory(ctx context.Context, submodelID string, idShortPath string) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return err
	}

	return common.ExecuteInTransaction(s.db, "SMREPO-DELETEFILEHIST-STARTTX", "SMREPO-DELETEFILEHIST-COMMIT", func(tx *sql.Tx) error {
		if err := fileHandler.DeleteManagedFileAttachmentTx(ctx, tx, submodelID, idShortPath); err != nil {
			return err
		}
		return s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, submodelElementRootMutation{
			previousPath: idShortPath,
			currentPath:  idShortPath,
		})
	})
}
