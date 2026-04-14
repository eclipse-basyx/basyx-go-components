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
	"database/sql"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/doug-martin/goqu/v9"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence/utils"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

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
		err = tx.Commit()
		if err != nil {
			return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-COMMIT " + err.Error())
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

	err = tx.Commit()
	if err != nil {
		return nil, "", "", "", common.NewInternalServerError("AASREPO-GETTHUMBNAIL-COMMIT " + err.Error())
	}

	return fileContent, contentType.String, fileName.String, path, nil
}

// UploadThumbnailByAASID uploads thumbnail content for an AAS and persists metadata.
// nolint:revive // cyclomatic complexity of 33
func (h *PostgreSQLThumbnailFileHandler) UploadThumbnailByAASID(aasIdentifier string, fileName string, file *os.File) error {
	tx, cleanup, err := common.StartTransaction(h.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	err = h.uploadThumbnailByAASIDInTransaction(tx, aasIdentifier, fileName, file)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-COMMIT " + err.Error())
	}

	return nil
}

// nolint:revive // cyclomatic complexity of 33
func (h *PostgreSQLThumbnailFileHandler) uploadThumbnailByAASIDInTransaction(tx *sql.Tx, aasIdentifier string, fileName string, file *os.File) error {
	aasDBID, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("AASREPO-PUTTHUMBNAIL-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-GETAASDBID " + err.Error())
	}

	if file == nil {
		return common.NewErrBadRequest("AASREPO-PUTTHUMBNAIL-MISSINGFILE file payload is required")
	}

	dialect := goqu.Dialect("postgres")

	existingElementSQL, existingElementArgs, existingElementBuildErr := dialect.
		From("thumbnail_file_element").
		Select("content_type", "file_name").
		Where(goqu.I("id").Eq(aasDBID)).
		ToSQL()
	if existingElementBuildErr != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-BUILDEXISTINGELEMENTSQL " + existingElementBuildErr.Error())
	}

	var existingContentType sql.NullString
	var existingFileName sql.NullString
	existingElementErr := tx.QueryRow(existingElementSQL, existingElementArgs...).Scan(&existingContentType, &existingFileName)
	if existingElementErr != nil && existingElementErr != sql.ErrNoRows {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-EXECEXISTINGELEMENTSQL " + existingElementErr.Error())
	}

	if _, seekErr := file.Seek(0, 0); seekErr != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-SEEKFILE " + seekErr.Error())
	}

	contentTypeBuffer := make([]byte, 512)
	readBytes, readErr := file.Read(contentTypeBuffer)
	if readErr != nil && readErr != io.EOF {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-READCONTENTTYPE " + readErr.Error())
	}
	detectedContentType := "application/octet-stream"
	if readBytes > 0 {
		detectedContentType = http.DetectContentType(contentTypeBuffer[:readBytes])
	}

	resolvedFileName := strings.TrimSpace(fileName)
	if resolvedFileName == "" && existingFileName.Valid {
		resolvedFileName = existingFileName.String
	}

	resolvedContentType, mismatchDetectedVsDeclared := common.ResolveUploadedContentType(detectedContentType, existingContentType.String, resolvedFileName)
	if mismatchDetectedVsDeclared {
		log.Printf("[WARN] AASREPO-PUTTHUMBNAIL-RESOLVEMIME detected content type differs from declared content type; using detected content type")
	}

	if _, seekErr := file.Seek(0, 0); seekErr != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-SEEKFILE " + seekErr.Error())
	}

	oldOIDQuery, oldOIDArgs, oldOIDBuildErr := dialect.
		From("thumbnail_file_data").
		Select("file_oid").
		Where(goqu.I("id").Eq(aasDBID)).
		ToSQL()
	if oldOIDBuildErr != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-BUILDOIDSQL " + oldOIDBuildErr.Error())
	}

	var oldOID sql.NullInt64
	oldOIDErr := tx.QueryRow(oldOIDQuery, oldOIDArgs...).Scan(&oldOID)
	if oldOIDErr != nil && oldOIDErr != sql.ErrNoRows {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-EXECOIDSQL " + oldOIDErr.Error())
	}

	if oldOID.Valid {
		if _, unlinkErr := tx.Exec(`SELECT lo_unlink($1)`, oldOID.Int64); unlinkErr != nil {
			return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-UNLINKOLDLO " + unlinkErr.Error())
		}
	}

	var newOID int64
	if createErr := tx.QueryRow(`SELECT lo_create(0)`).Scan(&newOID); createErr != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-CREATELO " + createErr.Error())
	}

	var loFD int
	if loOpenErr := tx.QueryRow(`SELECT lo_open($1, $2)`, newOID, 0x00020000).Scan(&loFD); loOpenErr != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-OPENLO " + loOpenErr.Error())
	}

	buffer := make([]byte, 8192)
	for {
		readCount, chunkErr := file.Read(buffer)
		if readCount > 0 {
			if _, writeErr := tx.Exec(`SELECT lowrite($1, $2)`, loFD, buffer[:readCount]); writeErr != nil {
				_, _ = tx.Exec(`SELECT lo_close($1)`, loFD)
				return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-WRITELO " + writeErr.Error())
			}
		}

		if chunkErr != nil {
			if chunkErr == io.EOF {
				break
			}
			_, _ = tx.Exec(`SELECT lo_close($1)`, loFD)
			return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-READFILE " + chunkErr.Error())
		}
	}

	if _, loCloseErr := tx.Exec(`SELECT lo_close($1)`, loFD); loCloseErr != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-CLOSELO " + loCloseErr.Error())
	}

	if oldOID.Valid {
		updateOIDSQL, updateOIDArgs, updateOIDBuildErr := dialect.Update("thumbnail_file_data").
			Set(goqu.Record{"file_oid": newOID}).
			Where(goqu.I("id").Eq(aasDBID)).
			ToSQL()
		if updateOIDBuildErr != nil {
			return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-BUILDUPDATEOIDSQL " + updateOIDBuildErr.Error())
		}
		if _, updateOIDErr := tx.Exec(updateOIDSQL, updateOIDArgs...); updateOIDErr != nil {
			return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-EXECUPDATEOIDSQL " + updateOIDErr.Error())
		}
	} else {
		ensureElementSQL, ensureElementArgs, ensureElementBuildErr := dialect.Insert("thumbnail_file_element").
			Rows(goqu.Record{
				"id":           aasDBID,
				"content_type": resolvedContentType,
				"file_name":    resolvedFileName,
				"value":        "",
			}).
			OnConflict(goqu.DoNothing()).
			ToSQL()
		if ensureElementBuildErr != nil {
			return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-BUILDENSUREELEMENTSQL " + ensureElementBuildErr.Error())
		}
		if _, ensureElementErr := tx.Exec(ensureElementSQL, ensureElementArgs...); ensureElementErr != nil {
			return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-EXECENSUREELEMENTSQL " + ensureElementErr.Error())
		}

		insertOIDSQL, insertOIDArgs, insertOIDBuildErr := dialect.Insert("thumbnail_file_data").
			Rows(goqu.Record{"id": aasDBID, "file_oid": newOID}).
			ToSQL()
		if insertOIDBuildErr != nil {
			return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-BUILDINSERTOIDSQL " + insertOIDBuildErr.Error())
		}
		if _, insertOIDErr := tx.Exec(insertOIDSQL, insertOIDArgs...); insertOIDErr != nil {
			return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-EXECINSERTOIDSQL " + insertOIDErr.Error())
		}
	}

	upsertElementSQL, upsertElementArgs, upsertElementBuildErr := dialect.Insert("thumbnail_file_element").
		Rows(goqu.Record{
			"id":           aasDBID,
			"content_type": resolvedContentType,
			"file_name":    resolvedFileName,
			"value":        strconv.FormatInt(newOID, 10),
		}).
		OnConflict(goqu.DoUpdate("id", goqu.Record{
			"content_type": resolvedContentType,
			"file_name":    resolvedFileName,
			"value":        strconv.FormatInt(newOID, 10),
		})).
		ToSQL()
	if upsertElementBuildErr != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-BUILDELEMENTSQL " + upsertElementBuildErr.Error())
	}

	if _, upsertElementErr := tx.Exec(upsertElementSQL, upsertElementArgs...); upsertElementErr != nil {
		return common.NewInternalServerError("AASREPO-PUTTHUMBNAIL-EXECELEMENTSQL " + upsertElementErr.Error())
	}

	return nil
}

// DeleteThumbnailByAASID deletes thumbnail content and metadata for an AAS.
func (h *PostgreSQLThumbnailFileHandler) DeleteThumbnailByAASID(aasIdentifier string) error {
	tx, cleanup, err := common.StartTransaction(h.db)
	if err != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	err = h.deleteThumbnailByAASIDInTransaction(tx, aasIdentifier)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-COMMIT " + err.Error())
	}

	return nil
}

func (h *PostgreSQLThumbnailFileHandler) deleteThumbnailByAASIDInTransaction(tx *sql.Tx, aasIdentifier string) error {
	aasDBID, err := persistenceutils.GetAssetAdministrationShellDatabaseID(tx, aasIdentifier)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("AASREPO-DELTHUMBNAIL-AASNOTFOUND Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-GETAASDBID " + err.Error())
	}

	dialect := goqu.Dialect("postgres")
	oidSQL, oidArgs, oidBuildErr := dialect.
		From("thumbnail_file_data").
		Select("file_oid").
		Where(goqu.I("id").Eq(aasDBID)).
		ToSQL()
	if oidBuildErr != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-BUILDOIDSQL " + oidBuildErr.Error())
	}

	var fileOID sql.NullInt64
	oidErr := tx.QueryRow(oidSQL, oidArgs...).Scan(&fileOID)
	if oidErr != nil {
		if oidErr == sql.ErrNoRows {
			return common.NewErrNotFound("AASREPO-DELTHUMBNAIL-DATANOTFOUND Thumbnail data for Asset Administration Shell with ID '" + aasIdentifier + "' not found")
		}
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-EXECOIDSQL " + oidErr.Error())
	}

	if !fileOID.Valid {
		return common.NewErrNotFound("AASREPO-DELTHUMBNAIL-NULLOID Thumbnail file OID is null")
	}

	if _, unlinkErr := tx.Exec(`SELECT lo_unlink($1)`, fileOID.Int64); unlinkErr != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-UNLINKLO " + unlinkErr.Error())
	}

	deleteDataSQL, deleteDataArgs, deleteDataBuildErr := dialect.Delete("thumbnail_file_data").
		Where(goqu.I("id").Eq(aasDBID)).
		ToSQL()
	if deleteDataBuildErr != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-BUILDDELETEDATASQL " + deleteDataBuildErr.Error())
	}
	if _, deleteDataErr := tx.Exec(deleteDataSQL, deleteDataArgs...); deleteDataErr != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-EXECDELETEDATASQL " + deleteDataErr.Error())
	}

	updateElementSQL, updateElementArgs, updateElementBuildErr := dialect.Update("thumbnail_file_element").
		Set(goqu.Record{
			"value":     "",
			"file_name": nil,
		}).
		Where(goqu.I("id").Eq(aasDBID)).
		ToSQL()
	if updateElementBuildErr != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-BUILDELEMENTSQL " + updateElementBuildErr.Error())
	}

	updateResult, updateErr := tx.Exec(updateElementSQL, updateElementArgs...)
	if updateErr != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-EXECELEMENTSQL " + updateErr.Error())
	}

	rowsAffected, rowsErr := updateResult.RowsAffected()
	if rowsErr != nil {
		return common.NewInternalServerError("AASREPO-DELTHUMBNAIL-GETROWCOUNT " + rowsErr.Error())
	}
	if rowsAffected == 0 {
		return common.NewErrNotFound("AASREPO-DELTHUMBNAIL-THUMBNAILNOTFOUND Thumbnail for Asset Administration Shell with ID '" + aasIdentifier + "' not found")
	}

	return nil
}
