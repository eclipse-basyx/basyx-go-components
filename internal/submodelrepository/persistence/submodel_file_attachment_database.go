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
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
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

// ManagedFileAttachmentPaths returns all canonical idShort paths backed by managed attachment bytes.
func (s *SubmodelDatabase) ManagedFileAttachmentPaths(ctx context.Context, submodelID string) (map[string]struct{}, error) {
	pathsBySubmodel, err := s.ManagedFileAttachmentPathsBySubmodelIDs(ctx, []string{submodelID})
	if err != nil {
		return nil, err
	}
	return pathsBySubmodel[submodelID], nil
}

// ManagedFileAttachmentPathsBySubmodelIDs returns canonical managed attachment paths grouped by Submodel ID.
func (s *SubmodelDatabase) ManagedFileAttachmentPathsBySubmodelIDs(
	ctx context.Context,
	submodelIDs []string,
) (map[string]map[string]struct{}, error) {
	pathsBySubmodel := make(map[string]map[string]struct{}, len(submodelIDs))
	for _, submodelID := range submodelIDs {
		pathsBySubmodel[submodelID] = make(map[string]struct{})
	}
	if len(submodelIDs) == 0 {
		return pathsBySubmodel, nil
	}
	query, args, err := submodelqueries.BuildManagedFileAttachmentPathsBySubmodelIDsSQL(submodelIDs)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-MANAGEDFILEPATHS-BUILDSQL " + err.Error())
	}
	rows, err := s.readDB(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-MANAGEDFILEPATHS-QUERY " + err.Error())
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var submodelID string
		var path string
		if err = rows.Scan(&submodelID, &path); err != nil {
			return nil, common.NewInternalServerError("SMREPO-MANAGEDFILEPATHS-SCAN " + err.Error())
		}
		paths := pathsBySubmodel[submodelID]
		if paths == nil {
			paths = make(map[string]struct{})
			pathsBySubmodel[submodelID] = paths
		}
		paths[path] = struct{}{}
	}
	if err = rows.Err(); err != nil {
		return nil, common.NewInternalServerError("SMREPO-MANAGEDFILEPATHS-ITERATE " + err.Error())
	}
	return pathsBySubmodel, nil
}

// UploadFileAttachment uploads attachment content for a File submodel element.
func (s *SubmodelDatabase) UploadFileAttachment(submodelID string, idShortPath string, file *os.File, fileName string) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return err
	}

	return fileHandler.UploadFileAttachment(submodelID, idShortPath, file, fileName)
}

// UploadFileAttachmentReader streams attachment content from a reader into a
// File submodel element.
//
// Parameters:
//   - submodelID: Identifier of the attachment's parent submodel.
//   - idShortPath: Path of the target File submodel element.
//   - file: Attachment source consumed before the method returns.
//   - fileName: Original attachment filename.
//
// Returns:
//   - error: Handler construction, validation, transaction, or persistence error.
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
		if visibilityErr := s.ensureFileAttachmentMutationVisible(ctx, tx, submodelID, idShortPath, "SMREPO-UPLOADFILEHIST", false); visibilityErr != nil {
			return visibilityErr
		}
		previousSnapshot, snapshotErr := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
		if snapshotErr != nil {
			return snapshotErr
		}
		reference, contentType, uploadErr := fileHandler.UploadManagedFileAttachmentTx(ctx, tx, submodelID, idShortPath, file, fileName)
		if uploadErr != nil {
			return uploadErr
		}
		if visibilityErr := s.ensureFileAttachmentMutationVisible(ctx, tx, submodelID, idShortPath, "SMREPO-UPLOADFILEHIST-PROSPECTIVE", true); visibilityErr != nil {
			return visibilityErr
		}
		return s.recordFileUploadMutationTx(ctx, tx, submodelID, idShortPath, previousSnapshot, reference, contentType)
	})
}

// UploadFileAttachmentReaderWithHistory streams attachment content and appends
// the current Submodel snapshot in the same transaction.
//
// Parameters:
//   - ctx: Request context preserving authorization, history, and cancellation data.
//   - submodelID: Identifier of the attachment's parent submodel.
//   - idShortPath: Path of the target File submodel element.
//   - file: Attachment source consumed before the method returns.
//   - fileName: Original attachment filename.
//   - contentType: Primary content type declared by the upload source.
//   - fallbackContentType: Secondary content type declaration used after the primary source.
//
// Returns:
//   - error: Visibility, history, validation, transaction, or persistence error.
func (s *SubmodelDatabase) UploadFileAttachmentReaderWithHistory(
	ctx context.Context,
	submodelID string,
	idShortPath string,
	file io.Reader,
	fileName string,
	contentType string,
	fallbackContentType string,
) error {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.db)
	if err != nil {
		return err
	}

	return common.ExecuteInTransaction(s.db, "SMREPO-UPLOADFILEHIST-STARTTX", "SMREPO-UPLOADFILEHIST-COMMIT", func(tx *sql.Tx) error {
		if visibilityErr := s.ensureFileAttachmentMutationVisible(ctx, tx, submodelID, idShortPath, "SMREPO-UPLOADFILEHIST", false); visibilityErr != nil {
			return visibilityErr
		}
		previousSnapshot, snapshotErr := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
		if snapshotErr != nil {
			return snapshotErr
		}
		reference, resolvedContentType, uploadErr := fileHandler.UploadManagedFileAttachmentReaderTx(
			ctx,
			tx,
			submodelID,
			idShortPath,
			file,
			fileName,
			contentType,
			fallbackContentType,
		)
		if uploadErr != nil {
			return uploadErr
		}
		if visibilityErr := s.ensureFileAttachmentMutationVisible(ctx, tx, submodelID, idShortPath, "SMREPO-UPLOADFILEHIST-PROSPECTIVE", true); visibilityErr != nil {
			return visibilityErr
		}
		return s.recordFileUploadMutationTx(ctx, tx, submodelID, idShortPath, previousSnapshot, reference, resolvedContentType)
	})
}

func (s *SubmodelDatabase) recordFileUploadMutationTx(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string, previousSnapshot map[string]any, reference binarycontent.Reference, contentType string) error {
	binaryReceipt, err := history.EnsureBinaryEvidenceTx(ctx, tx, reference.Content, contentType)
	if err != nil {
		return err
	}
	expectation, err := history.NewBinaryReferenceExpectation(
		reference.Content, reference.ManagedPath(), reference.SafeFileName, contentType, binaryReceipt,
	)
	if err != nil {
		return err
	}
	mutationCtx := history.WithBinaryReferenceExpected(ctx, expectation)
	if err = s.appendChangedSubmodelElementHistoryTx(mutationCtx, tx, submodelID, previousSnapshot, submodelElementRootMutation{
		previousPath: idShortPath,
		currentPath:  idShortPath,
	}); err != nil {
		return err
	}
	return history.RecordBinaryReferenceEvidenceTx(
		mutationCtx, tx, history.TableSubmodel, submodelID, expectation,
	)
}

// DownloadFileAttachment downloads attachment content for a File submodel element.
func (s *SubmodelDatabase) DownloadFileAttachment(submodelID string, idShortPath string) ([]byte, string, string, error) {
	readDB := s.readerDB
	if readDB == nil {
		readDB = s.db
	}
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(readDB)
	if err != nil {
		return nil, "", "", err
	}

	return fileHandler.DownloadFileAttachment(submodelID, idShortPath)
}

// DownloadFileAttachmentWithContext resolves canonical content through the owning File SME.
func (s *SubmodelDatabase) DownloadFileAttachmentWithContext(ctx context.Context, submodelID string, idShortPath string) ([]byte, string, string, error) {
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(s.readDB(ctx))
	if err != nil {
		return nil, "", "", err
	}
	return fileHandler.DownloadManagedFileAttachment(ctx, submodelID, idShortPath)
}

// StreamFileAttachmentWithContext streams an attachment while preserving ABAC visibility from ctx.
//
// Parameters:
//   - ctx: Request context preserving authorization and cancellation.
//   - submodelID: Identifier of the attachment's parent submodel.
//   - idShortPath: Path of the File submodel element.
//   - consume: Callback receiving content type, filename, known size, and a scoped reader.
//
// Returns:
//   - error: Handler construction, lookup, consumer, stream, or transaction error.
func (s *SubmodelDatabase) StreamFileAttachmentWithContext(ctx context.Context, submodelID string, idShortPath string, consume func(string, string, int64, io.Reader) error) error {
	readDB := s.readDB(ctx)
	fileHandler, err := submodelelements.NewPostgreSQLFileHandler(readDB)
	if err != nil {
		return err
	}
	tx, err := readDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return common.NewInternalServerError("SMREPO-STREAMATTACHMENT-STARTTX " + err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err = s.ensureFileAttachmentReadVisible(ctx, tx, submodelID, idShortPath); err != nil {
		return err
	}
	if err = fileHandler.StreamManagedFileAttachmentTx(ctx, tx, submodelID, idShortPath, consume); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return common.NewInternalServerError("SMREPO-STREAMATTACHMENT-COMMIT " + err.Error())
	}
	committed = true
	return nil
}

func (s *SubmodelDatabase) ensureFileAttachmentReadVisible(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string) error {
	exists, visible, err := s.checkSubmodelElementVisibilityInTx(ctx, tx, submodelID, idShortPath)
	if err != nil {
		return err
	}
	if !exists || !visible {
		return common.NewErrNotFound("SMREPO-STREAMATTACHMENT-NOTVISIBLE File SME not found")
	}
	return nil
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
		if visibilityErr := s.ensureFileAttachmentMutationVisible(ctx, tx, submodelID, idShortPath, "SMREPO-DELETEFILEHIST", false); visibilityErr != nil {
			return visibilityErr
		}
		previousSnapshot, snapshotErr := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
		if snapshotErr != nil {
			return snapshotErr
		}
		if err := fileHandler.DeleteManagedFileAttachmentTx(ctx, tx, submodelID, idShortPath); err != nil {
			return err
		}
		return s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, previousSnapshot, submodelElementRootMutation{
			previousPath: idShortPath,
			currentPath:  idShortPath,
		})
	})
}

func (s *SubmodelDatabase) ensureFileAttachmentMutationVisible(ctx context.Context, tx *sql.Tx, submodelID string, idShortPath string, errorPrefix string, prospective bool) error {
	shouldEnforce, err := shouldEnforceFormula(ctx, errorPrefix+"-SHOULDENFORCE")
	if err != nil || !shouldEnforce {
		return err
	}
	readCtx := ctx
	if right, selected := auth.SelectedFormulaRight(ctx); selected && right == grammar.RightsEnumCREATE && !prospective {
		readCtx = auth.ContextWithoutQueryFilter(ctx)
	}
	exists, visible, err := s.checkSubmodelElementVisibilityInTx(readCtx, tx, submodelID, idShortPath)
	if err != nil {
		return err
	}
	if !exists {
		return common.NewErrNotFound(errorPrefix + "-NOTFOUND Submodel element not found")
	}
	if !visible {
		return common.NewErrDenied(errorPrefix + "-ABACDENIED Mutating this file attachment is not allowed")
	}
	return nil
}
