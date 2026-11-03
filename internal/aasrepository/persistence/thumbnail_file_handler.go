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

package persistence

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence/utils"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/conditional"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const fallbackThumbnailBaseName = "thumbnail"

// PostgreSQLThumbnailFileHandler handles thumbnail file operations for AAS asset information.
type PostgreSQLThumbnailFileHandler struct {
	db *sql.DB
}

// NewPostgreSQLThumbnailFileHandler creates a handler for thumbnail file operations.
func NewPostgreSQLThumbnailFileHandler(db *sql.DB) (*PostgreSQLThumbnailFileHandler, error) {
	return &PostgreSQLThumbnailFileHandler{db: db}, nil
}

// DownloadThumbnailByAASID retrieves thumbnail content and metadata by AAS identifier.
func (h *PostgreSQLThumbnailFileHandler) DownloadThumbnailByAASID(aasIdentifier string) ([]byte, string, string, string, error) {
	tx, cleanup, err := common.StartTransaction(h.db)
	if err != nil {
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	aasDBID, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", "", "", common.NewErrNotFound("AASREPO-GETTHUMBNAIL-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-GETAASDBID " + err.Error())
	}

	dialect := goqu.Dialect("postgres")
	querySQL, queryArgs, buildErr := dialect.
		From("thumbnail_file_element").
		Select("value", "content_type", "file_name").
		Where(goqu.I("id").Eq(aasDBID)).
		ToSQL()
	if buildErr != nil {
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-BUILDELEMENTSQL " + buildErr.Error())
	}

	var filePath sql.NullString
	var contentType sql.NullString
	var fileName sql.NullString
	if queryErr := tx.QueryRow(querySQL, queryArgs...).Scan(&filePath, &contentType, &fileName); queryErr != nil {
		if queryErr == sql.ErrNoRows {
			return nil, "", "", "", common.NewErrNotFound("AASREPO-GETTHUMBNAIL-THUMBNAILNOTFOUND Thumbnail for Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-EXECELEMENTSQL " + queryErr.Error())
	}

	path := ""
	if filePath.Valid {
		path = filePath.String
	}

	if path == "" {
		return nil, "", "", "", common.NewErrNotFound("AASREPO-GETTHUMBNAIL-EMPTYPATH Thumbnail path is empty")
	}

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		err = common.CommitTransaction(tx)
		if err != nil {
			return nil, "", "", "", common.CommitError("AASREPO-GETTHUMBNAIL-COMMIT", err)
		}
		return nil, contentType.String, fileName.String, path, nil
	}

	dataSQL, dataArgs, dataBuildErr := dialect.
		From("thumbnail_file_data").
		Select("file_oid").
		Where(goqu.I("id").Eq(aasDBID)).
		ToSQL()
	if dataBuildErr != nil {
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-BUILDDATASQL " + dataBuildErr.Error())
	}

	var fileOID sql.NullInt64
	if dataErr := tx.QueryRow(dataSQL, dataArgs...).Scan(&fileOID); dataErr != nil {
		if dataErr == sql.ErrNoRows {
			return nil, "", "", "", common.NewErrNotFound("AASREPO-GETTHUMBNAIL-DATANOTFOUND Thumbnail data for Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-EXECDATASQL " + dataErr.Error())
	}

	if !fileOID.Valid {
		return nil, "", "", "", common.NewErrNotFound("AASREPO-GETTHUMBNAIL-NULLOID Thumbnail file OID is null")
	}

	var loFD int
	if openErr := tx.QueryRow(`SELECT lo_open($1, $2)`, fileOID.Int64, 0x00040000).Scan(&loFD); openErr != nil {
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-OPENLO " + openErr.Error())
	}

	fileContent := make([]byte, 0)
	for {
		var bytesRead []byte
		readErr := tx.QueryRow(`SELECT loread($1, $2)`, loFD, 8192).Scan(&bytesRead)
		if readErr != nil {
			_, _ = tx.Exec(`SELECT lo_close($1)`, loFD)
			return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-READLO " + readErr.Error())
		}
		if len(bytesRead) == 0 {
			break
		}
		fileContent = append(fileContent, bytesRead...)
	}

	if _, closeErr := tx.Exec(`SELECT lo_close($1)`, loFD); closeErr != nil {
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-CLOSELO " + closeErr.Error())
	}

	err = common.CommitTransaction(tx)
	if err != nil {
		return nil, "", "", "", common.CommitError("AASREPO-GETTHUMBNAIL-COMMIT", err)
	}

	return fileContent, contentType.String, fileName.String, path, nil
}

func (h *PostgreSQLThumbnailFileHandler) uploadManagedThumbnailTx(
	ctx context.Context,
	tx *sql.Tx,
	aasIdentifier string,
	file io.Reader,
	declaration binarycontent.UploadDeclaration,
) (binarycontent.Reference, string, error) {
	if file == nil {
		return binarycontent.Reference{}, "", common.NewErrBadRequest("AASREPO-PUTTHUMBNAIL-MISSINGFILE file payload is required")
	}
	metadata, err := loadManagedThumbnailMetadata(ctx, tx, aasIdentifier, true)
	if err != nil {
		return binarycontent.Reference{}, "", err
	}
	if err = conditional.RecordAddressedExistence(ctx, conditional.Ref(conditional.KindAAS, aasIdentifier), func() (bool, error) {
		return thumbnailExists(ctx, tx, metadata.AASDBID)
	}); err != nil {
		return binarycontent.Reference{}, "", err
	}
	upload, err := binarycontent.ResolveUpload(
		file,
		declaration,
		binarycontent.StoredMetadata{FileName: metadata.ExistingFileName.String, ContentType: metadata.ExistingContentType.String},
		fallbackThumbnailBaseName,
	)
	if err != nil {
		return binarycontent.Reference{}, "", err
	}
	if upload.TypeMismatch {
		slog.WarnContext(ctx, "detected thumbnail content type differs from declared content type; preserving declared content type", "error.code", "AASREPO-PUTTHUMBNAIL-RESOLVEMIME")
	}
	if err = ensureManagedThumbnailElement(ctx, tx, metadata.AASDBID); err != nil {
		return binarycontent.Reference{}, "", err
	}
	reference, err := binarycontent.StoreReferenceTx(
		ctx, tx, upload.Content, binarycontent.TableThumbnailReference, "thumbnail_element_id", metadata.AASDBID, upload.FileName,
	)
	if err != nil {
		return binarycontent.Reference{}, "", err
	}
	if err = upsertManagedThumbnailElement(ctx, tx, reference, upload.ContentType); err != nil {
		return binarycontent.Reference{}, "", err
	}
	if err = deleteLegacyThumbnailData(ctx, tx, metadata.AASDBID); err != nil {
		return binarycontent.Reference{}, "", err
	}
	return reference, upload.ContentType, touchAAS(ctx, tx, aasIdentifier, conditional.OpUpdate)
}

func ensureManagedThumbnailElement(ctx context.Context, tx *sql.Tx, aasDBID int64) error {
	query, args, err := goqu.Insert("thumbnail_file_element").Rows(goqu.Record{"id": aasDBID}).
		OnConflict(goqu.DoNothing()).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-BUILDENSUREELEMENT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-ENSUREELEMENT " + err.Error())
	}
	return nil
}

func (h *PostgreSQLThumbnailFileHandler) downloadManagedThumbnail(ctx context.Context, aasIdentifier string) ([]byte, string, string, string, error) {
	tx, err := common.BeginReadTransaction(ctx, h.db)
	if err != nil {
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-STARTTX " + err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	metadata, err := loadManagedThumbnailMetadata(ctx, tx, aasIdentifier, false)
	if err != nil {
		return nil, "", "", "", err
	}
	thumbnailPath := metadata.Path.String
	if strings.HasPrefix(thumbnailPath, "http://") || strings.HasPrefix(thumbnailPath, "https://") {
		if err = common.CommitTransaction(tx); err != nil {
			return nil, "", "", "", common.CommitError("AASREPO-GETTHUMBNAIL-COMMIT", err)
		}
		committed = true
		return nil, metadata.ExistingContentType.String, metadata.ExistingFileName.String, thumbnailPath, nil
	}
	reference, err := binarycontent.LoadReferenceTx(ctx, tx, binarycontent.TableThumbnailReference, "thumbnail_element_id", metadata.AASDBID)
	var content []byte
	if err == nil {
		content, err = binarycontent.ReadAllTx(ctx, tx, reference.Content)
	} else if errors.Is(err, sql.ErrNoRows) {
		content, err = readLegacyThumbnailContent(ctx, tx, metadata.AASDBID)
	}
	if err != nil {
		return nil, "", "", "", err
	}
	if err = common.CommitTransaction(tx); err != nil {
		return nil, "", "", "", common.CommitError("AASREPO-GETTHUMBNAIL-COMMIT", err)
	}
	committed = true
	return content, metadata.ExistingContentType.String, metadata.ExistingFileName.String, thumbnailPath, nil
}

func (h *PostgreSQLThumbnailFileHandler) streamManagedThumbnailTx(
	ctx context.Context,
	tx *sql.Tx,
	aasIdentifier string,
	consume func(string, string, string, int64, io.Reader) error,
) error {
	if tx == nil || consume == nil {
		return common.NewInternalServerError("AASREPO-STREAMTHUMBNAILTX-INVALID transaction and stream consumer are required")
	}
	metadata, err := loadManagedThumbnailMetadata(ctx, tx, aasIdentifier, false)
	if err != nil {
		return err
	}
	thumbnailPath := metadata.Path.String
	if strings.HasPrefix(thumbnailPath, "http://") || strings.HasPrefix(thumbnailPath, "https://") {
		return common.NewErrNotFound("AASREPO-STREAMTHUMBNAIL-EXTERNAL external thumbnail has no managed content")
	}
	reference, err := binarycontent.LoadReferenceTx(ctx, tx, binarycontent.TableThumbnailReference, "thumbnail_element_id", metadata.AASDBID)
	if err == nil {
		err = binarycontent.StreamTx(ctx, tx, reference.Content, func(reader io.Reader) error {
			return consume(metadata.ExistingContentType.String, metadata.ExistingFileName.String, thumbnailPath, reference.Content.SizeBytes, reader)
		})
	} else if errors.Is(err, sql.ErrNoRows) {
		var oid int64
		oid, err = readLegacyThumbnailOID(ctx, tx, metadata.AASDBID)
		if err == nil {
			err = binarycontent.StreamOIDTx(ctx, tx, oid, func(reader io.Reader) error {
				return consume(metadata.ExistingContentType.String, metadata.ExistingFileName.String, thumbnailPath, 0, reader)
			})
		}
	}
	if err != nil {
		return err
	}
	return nil
}

func loadManagedThumbnailMetadata(ctx context.Context, tx *sql.Tx, aasIdentifier string, forUpdate bool) (commonmodel.ManagedThumbnailMetadata, error) {
	dialect := goqu.Dialect("postgres")
	dataset := dialect.From(goqu.T("aas").As("aas")).
		LeftJoin(goqu.T("thumbnail_file_element").As("thumbnail"), goqu.On(goqu.I("thumbnail.id").Eq(goqu.I("aas.id")))).
		Select(goqu.I("aas.id"), goqu.I("thumbnail.content_type"), goqu.I("thumbnail.file_name"), goqu.I("thumbnail.value")).
		Where(goqu.I("aas.aas_id").Eq(aasIdentifier))
	if forUpdate {
		dataset = dataset.ForUpdate(exp.Wait, goqu.I("aas"))
	}
	query, args, err := dataset.ToSQL()
	if err != nil {
		return commonmodel.ManagedThumbnailMetadata{}, common.NewInternalServerError("AASREPO-THUMBNAIL-BUILDMETADATA " + err.Error())
	}
	var metadata commonmodel.ManagedThumbnailMetadata
	if err = tx.QueryRowContext(ctx, query, args...).Scan(
		&metadata.AASDBID, &metadata.ExistingContentType, &metadata.ExistingFileName, &metadata.Path,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return commonmodel.ManagedThumbnailMetadata{}, common.NewErrNotFound("AASREPO-THUMBNAIL-AASNOTFOUND Asset Administration Shell not found")
		}
		return commonmodel.ManagedThumbnailMetadata{}, common.NewInternalServerError("AASREPO-THUMBNAIL-METADATA " + err.Error())
	}
	if !forUpdate && !metadata.Path.Valid {
		return commonmodel.ManagedThumbnailMetadata{}, common.NewErrNotFound("AASREPO-THUMBNAIL-NOTFOUND Thumbnail not found")
	}
	return metadata, nil
}

func upsertManagedThumbnailElement(ctx context.Context, tx *sql.Tx, reference binarycontent.Reference, contentType string) error {
	record := goqu.Record{
		"id": reference.OwnerID, "content_type": contentType,
		"file_name": reference.SafeFileName, "value": reference.ManagedPath(),
	}
	query, args, err := goqu.Insert("thumbnail_file_element").Rows(record).
		OnConflict(goqu.DoUpdate("id", record)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-BUILDELEMENT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-ELEMENT " + err.Error())
	}
	return nil
}

func readLegacyThumbnailContent(ctx context.Context, tx *sql.Tx, aasDBID int64) ([]byte, error) {
	oid, err := readLegacyThumbnailOID(ctx, tx, aasDBID)
	if err != nil {
		return nil, err
	}
	return binarycontent.ReadOIDTx(ctx, tx, oid)
}

func readLegacyThumbnailOID(ctx context.Context, tx *sql.Tx, aasDBID int64) (int64, error) {
	query, args, err := goqu.From("thumbnail_file_data").Select("file_oid").Where(goqu.C("id").Eq(aasDBID)).ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("AASREPO-GETTHUMBNAIL-BUILDLEGACY " + err.Error())
	}
	var oid int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&oid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, common.NewErrNotFound("AASREPO-GETTHUMBNAIL-DATANOTFOUND Thumbnail data not found")
		}
		return 0, common.NewInternalServerError("AASREPO-GETTHUMBNAIL-LEGACY " + err.Error())
	}
	return oid, nil
}

func deleteLegacyThumbnailData(ctx context.Context, tx *sql.Tx, aasDBID int64) error {
	query, args, err := goqu.From("thumbnail_file_data").Select("file_oid").Where(goqu.C("id").Eq(aasDBID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASREPO-THUMBNAIL-BUILDLEGACYDELETE " + err.Error())
	}
	var oid sql.NullInt64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&oid); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return common.NewInternalServerError("AASREPO-THUMBNAIL-READLEGACYDELETE " + err.Error())
	}
	if oid.Valid {
		unlinkQuery, unlinkArgs, buildErr := goqu.Select(goqu.Func("lo_unlink", oid.Int64)).ToSQL()
		if buildErr != nil {
			return common.NewInternalServerError("AASREPO-THUMBNAIL-BUILDUNLINKLEGACY " + buildErr.Error())
		}
		if _, err = tx.ExecContext(ctx, unlinkQuery, unlinkArgs...); err != nil {
			return common.NewInternalServerError("AASREPO-THUMBNAIL-UNLINKLEGACY " + err.Error())
		}
	}
	deleteQuery, deleteArgs, err := goqu.Delete("thumbnail_file_data").Where(goqu.C("id").Eq(aasDBID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASREPO-THUMBNAIL-BUILDDELETELEGACY " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, deleteQuery, deleteArgs...); err != nil {
		return common.NewInternalServerError("AASREPO-THUMBNAIL-DELETELEGACY " + err.Error())
	}
	return nil
}

func (h *PostgreSQLThumbnailFileHandler) deleteManagedThumbnailTx(ctx context.Context, tx *sql.Tx, aasIdentifier string) error {
	metadata, err := loadManagedThumbnailMetadata(ctx, tx, aasIdentifier, true)
	if err != nil {
		return err
	}
	hasThumbnail, err := thumbnailExists(ctx, tx, metadata.AASDBID)
	if err != nil {
		return err
	}
	if !hasThumbnail {
		return common.NewErrNotFound("AASREPO-DELTHUMBNAIL-DATANOTFOUND Thumbnail data not found")
	}
	if err = binarycontent.DeleteReferenceTx(ctx, tx, binarycontent.TableThumbnailReference, "thumbnail_element_id", metadata.AASDBID); err != nil {
		return err
	}
	if err = deleteLegacyThumbnailData(ctx, tx, metadata.AASDBID); err != nil {
		return err
	}
	query, args, err := goqu.Update("thumbnail_file_element").Set(goqu.Record{"value": "", "file_name": nil, "content_type": nil}).
		Where(goqu.C("id").Eq(metadata.AASDBID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-BUILDELEMENT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-ELEMENT " + err.Error())
	}
	return touchAAS(ctx, tx, aasIdentifier, conditional.OpUpdate)
}

func thumbnailExists(ctx context.Context, tx *sql.Tx, aasDBID int64) (bool, error) {
	return binarycontent.ContentExistsTx(ctx, tx, binarycontent.TableThumbnailReference, "thumbnail_element_id", aasDBID)
}
