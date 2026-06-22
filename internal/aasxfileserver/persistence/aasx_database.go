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

// Package persistence provides Postgres-backed storage for the AASX file server.
package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	aasx "github.com/aas-core-works/aas-package3-golang"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const (
	largeObjectWriteMode      = 0x00020000
	largeObjectReadMode       = 0x00040000
	defaultPackageContentType = "application/asset-administration-shell-package"
)

// PackageRecord contains package metadata returned by list and write operations.
type PackageRecord struct {
	DBID        int64
	PackageID   string
	FileName    string
	ContentType string
	AASIDs      []string
}

// PackageBinary combines package metadata with package bytes.
type PackageBinary struct {
	PackageRecord
	Content []byte
}

// AASXFileServerDatabase exposes package persistence operations.
type AASXFileServerDatabase struct {
	db *sql.DB
}

// NewAASXFileServerDatabaseFromDB creates an AASX database backend from an existing SQL handle.
func NewAASXFileServerDatabaseFromDB(db *sql.DB) (*AASXFileServerDatabase, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("AASXFS-NEWFROMDB-NILDB database handle must not be nil")
	}

	return &AASXFileServerDatabase{db: db}, nil
}

// ListPackages returns package metadata for a page and the next cursor identifier, if any.
func (p *AASXFileServerDatabase) ListPackages(ctx context.Context, limit int32, cursorID int64, aasID string) ([]PackageRecord, int64, error) {
	if limit <= 0 {
		limit = 100
	}

	dialect := goqu.Dialect("postgres")
	ds := dialect.From("aasx_package").
		Select("id", "package_id", "file_name", "content_type").
		Order(goqu.I("id").Asc())

	if cursorID > 0 {
		ds = ds.Where(goqu.I("id").Gt(cursorID))
	}

	trimmedAASID := strings.TrimSpace(aasID)
	if trimmedAASID != "" {
		ds = ds.Where(
			goqu.L(
				"EXISTS (SELECT 1 FROM aasx_package_aas_id aa WHERE aa.package_db_id = aasx_package.id AND aa.aas_id = ?)",
				trimmedAASID,
			),
		)
	}

	// #nosec G115 -- limit is normalized to a positive int32 value above.
	ds = ds.Limit(uint(limit + 1))

	sqlQuery, args, err := ds.ToSQL()
	if err != nil {
		return nil, 0, common.NewInternalServerError("AASXFS-LISTPACKAGES-BUILDSQL " + err.Error())
	}

	rows, err := p.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, 0, common.NewInternalServerError("AASXFS-LISTPACKAGES-QUERY " + err.Error())
	}
	defer func() { _ = rows.Close() }()

	records := make([]PackageRecord, 0, limit+1)
	for rows.Next() {
		var row PackageRecord
		if scanErr := rows.Scan(&row.DBID, &row.PackageID, &row.FileName, &row.ContentType); scanErr != nil {
			return nil, 0, common.NewInternalServerError("AASXFS-LISTPACKAGES-SCAN " + scanErr.Error())
		}
		records = append(records, row)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, common.NewInternalServerError("AASXFS-LISTPACKAGES-ROWS " + err.Error())
	}

	hasMore := len(records) > int(limit)
	nextCursor := int64(0)
	if hasMore {
		nextCursor = records[limit].DBID
		records = records[:limit]
	}

	for idx := range records {
		aasIDs, aasErr := p.getAASIDs(ctx, records[idx].DBID)
		if aasErr != nil {
			return nil, 0, aasErr
		}
		records[idx].AASIDs = aasIDs
	}

	return records, nextCursor, nil
}

// CreatePackage creates a new package and fails if the package ID already exists.
func (p *AASXFileServerDatabase) CreatePackage(ctx context.Context, packageID string, file *os.File, aasIDs []string, fileName string) (*PackageRecord, error) {
	record, _, err := p.putPackage(ctx, strings.TrimSpace(packageID), file, aasIDs, fileName, false)
	if err != nil {
		return nil, err
	}
	return record, nil
}

// PutPackage upserts a package and reports whether an existing package was updated.
func (p *AASXFileServerDatabase) PutPackage(ctx context.Context, packageID string, file *os.File, aasIDs []string, fileName string) (bool, *PackageRecord, error) {
	record, updated, err := p.putPackage(ctx, strings.TrimSpace(packageID), file, aasIDs, fileName, true)
	if err != nil {
		return false, nil, err
	}
	return updated, record, nil
}

func (p *AASXFileServerDatabase) putPackage(ctx context.Context, packageID string, file *os.File, aasIDs []string, fileName string, allowUpdate bool) (*PackageRecord, bool, error) {
	if strings.TrimSpace(packageID) == "" {
		return nil, false, common.NewErrBadRequest("AASXFS-PUTPACKAGE-EMPTYPACKAGEID packageId must not be empty")
	}
	if file == nil {
		return nil, false, common.NewErrBadRequest("AASXFS-PUTPACKAGE-NILFILE package file must not be nil")
	}

	normalizedAASIDs := normalizeAASIDs(aasIDs)

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, common.NewInternalServerError("AASXFS-PUTPACKAGE-STARTTX " + err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	dialect := goqu.Dialect("postgres")
	selectSQL, selectArgs, err := dialect.From("aasx_package").
		Select("id", "file_oid", "file_name").
		Where(goqu.C("package_id").Eq(packageID)).
		ToSQL()
	if err != nil {
		return nil, false, common.NewInternalServerError("AASXFS-PUTPACKAGE-BUILDSELECT " + err.Error())
	}

	var existingID int64
	var existingOID int64
	var existingFileName string
	// #nosec G701 -- SQL is generated by goqu with fixed table/column names and bound arguments.
	scanErr := tx.QueryRowContext(ctx, selectSQL, selectArgs...).Scan(&existingID, &existingOID, &existingFileName)
	exists := scanErr == nil
	if scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
		return nil, false, common.NewInternalServerError("AASXFS-PUTPACKAGE-SELECT " + scanErr.Error())
	}
	if exists && !allowUpdate {
		return nil, false, common.NewErrConflict("AASXFS-PUTPACKAGE-CONFLICT packageId already exists")
	}

	resolvedFileName := normalizeFileName(fileName, file)
	if resolvedFileName == "" {
		resolvedFileName = strings.TrimSpace(existingFileName)
	}
	if resolvedFileName == "" {
		resolvedFileName = packageID + ".aasx"
	}

	newOID, detectedContentType, err := writeLargeObjectFromFile(tx, file, resolvedFileName)
	if err != nil {
		return nil, false, err
	}

	if exists {
		updateSQL, updateArgs, buildErr := dialect.Update("aasx_package").
			Set(goqu.Record{
				"file_oid":     newOID,
				"file_name":    resolvedFileName,
				"content_type": detectedContentType,
			}).
			Where(goqu.C("id").Eq(existingID)).
			ToSQL()
		if buildErr != nil {
			return nil, false, common.NewInternalServerError("AASXFS-PUTPACKAGE-BUILDUPDATE " + buildErr.Error())
		}
		// #nosec G701 -- SQL is generated by goqu with fixed table/column names and bound arguments.
		if _, execErr := tx.ExecContext(ctx, updateSQL, updateArgs...); execErr != nil {
			return nil, false, common.NewInternalServerError("AASXFS-PUTPACKAGE-UPDATE " + execErr.Error())
		}

		if err = replaceAASIDs(ctx, tx, existingID, normalizedAASIDs); err != nil {
			return nil, false, err
		}

		// #nosec G701 -- Query text is static and the OID is passed as a bound argument.
		if _, unlinkErr := tx.ExecContext(ctx, `SELECT lo_unlink($1)`, existingOID); unlinkErr != nil {
			return nil, false, common.NewInternalServerError("AASXFS-PUTPACKAGE-UNLINKOLDLO " + unlinkErr.Error())
		}

		if commitErr := tx.Commit(); commitErr != nil {
			return nil, false, common.NewInternalServerError("AASXFS-PUTPACKAGE-COMMIT " + commitErr.Error())
		}
		committed = true

		return &PackageRecord{
			PackageID:   packageID,
			FileName:    resolvedFileName,
			ContentType: detectedContentType,
			AASIDs:      normalizedAASIDs,
		}, true, nil
	}

	insertSQL, insertArgs, err := dialect.Insert("aasx_package").
		Rows(goqu.Record{
			"package_id":   packageID,
			"file_oid":     newOID,
			"file_name":    resolvedFileName,
			"content_type": detectedContentType,
		}).
		Returning("id").
		ToSQL()
	if err != nil {
		return nil, false, common.NewInternalServerError("AASXFS-PUTPACKAGE-BUILDINSERT " + err.Error())
	}

	var newID int64
	// #nosec G701 -- SQL is generated by goqu with fixed table/column names and bound arguments.
	if err = tx.QueryRowContext(ctx, insertSQL, insertArgs...).Scan(&newID); err != nil {
		if isUniqueViolation(err) {
			return nil, false, common.NewErrConflict("AASXFS-PUTPACKAGE-CONFLICT packageId already exists")
		}
		return nil, false, common.NewInternalServerError("AASXFS-PUTPACKAGE-INSERT " + err.Error())
	}

	if err = replaceAASIDs(ctx, tx, newID, normalizedAASIDs); err != nil {
		return nil, false, err
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return nil, false, common.NewInternalServerError("AASXFS-PUTPACKAGE-COMMIT " + commitErr.Error())
	}
	committed = true

	return &PackageRecord{
		PackageID:   packageID,
		FileName:    resolvedFileName,
		ContentType: detectedContentType,
		AASIDs:      normalizedAASIDs,
	}, false, nil
}

// GetPackageByID returns metadata and binary content for one package ID.
func (p *AASXFileServerDatabase) GetPackageByID(ctx context.Context, packageID string) (*PackageBinary, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, common.NewInternalServerError("AASXFS-GETPACKAGE-STARTTX " + err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.From("aasx_package").
		Select("id", "file_oid", "file_name", "content_type").
		Where(goqu.C("package_id").Eq(packageID)).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("AASXFS-GETPACKAGE-BUILDSQL " + err.Error())
	}

	var packageDBID int64
	var fileOID int64
	var fileName string
	var contentType string
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&packageDBID, &fileOID, &fileName, &contentType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("AASXFS-GETPACKAGE-NOTFOUND package not found")
		}
		return nil, common.NewInternalServerError("AASXFS-GETPACKAGE-QUERY " + err.Error())
	}

	content, err := readLargeObject(tx, fileOID)
	if err != nil {
		return nil, err
	}

	aasIDs, err := p.getAASIDsTx(ctx, tx, packageDBID)
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, common.NewInternalServerError("AASXFS-GETPACKAGE-COMMIT " + err.Error())
	}
	committed = true

	return &PackageBinary{
		PackageRecord: PackageRecord{
			DBID:        packageDBID,
			PackageID:   packageID,
			FileName:    fileName,
			ContentType: contentType,
			AASIDs:      aasIDs,
		},
		Content: content,
	}, nil
}

// DeletePackageByID removes a package and unlinks its PostgreSQL large object.
func (p *AASXFileServerDatabase) DeletePackageByID(ctx context.Context, packageID string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("AASXFS-DELETEPACKAGE-STARTTX " + err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	dialect := goqu.Dialect("postgres")
	selectSQL, selectArgs, err := dialect.From("aasx_package").
		Select("id", "file_oid").
		Where(goqu.C("package_id").Eq(packageID)).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASXFS-DELETEPACKAGE-BUILDSELECT " + err.Error())
	}

	var packageDBID int64
	var fileOID int64
	if err = tx.QueryRowContext(ctx, selectSQL, selectArgs...).Scan(&packageDBID, &fileOID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return common.NewErrNotFound("AASXFS-DELETEPACKAGE-NOTFOUND package not found")
		}
		return common.NewInternalServerError("AASXFS-DELETEPACKAGE-SELECT " + err.Error())
	}

	deleteSQL, deleteArgs, err := dialect.Delete("aasx_package").
		Where(goqu.C("id").Eq(packageDBID)).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASXFS-DELETEPACKAGE-BUILDDELETE " + err.Error())
	}
	// #nosec G701 -- SQL is generated by goqu with fixed table/column names and bound arguments.
	if _, err = tx.ExecContext(ctx, deleteSQL, deleteArgs...); err != nil {
		return common.NewInternalServerError("AASXFS-DELETEPACKAGE-DELETE " + err.Error())
	}

	if _, err = tx.ExecContext(ctx, `SELECT lo_unlink($1)`, fileOID); err != nil {
		return common.NewInternalServerError("AASXFS-DELETEPACKAGE-UNLINK " + err.Error())
	}

	if err = tx.Commit(); err != nil {
		return common.NewInternalServerError("AASXFS-DELETEPACKAGE-COMMIT " + err.Error())
	}
	committed = true
	return nil
}

func (p *AASXFileServerDatabase) getAASIDs(ctx context.Context, packageDBID int64) ([]string, error) {
	return p.getAASIDsTx(ctx, p.db, packageDBID)
}

func (p *AASXFileServerDatabase) getAASIDsTx(ctx context.Context, queryable interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}, packageDBID int64) ([]string, error) {
	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.From("aasx_package_aas_id").
		Select("aas_id").
		Where(goqu.C("package_db_id").Eq(packageDBID)).
		Order(goqu.I("position").Asc()).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("AASXFS-GETAASIDS-BUILDSQL " + err.Error())
	}

	rows, err := queryable.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("AASXFS-GETAASIDS-QUERY " + err.Error())
	}
	defer func() { _ = rows.Close() }()

	aasIDs := make([]string, 0, 4)
	for rows.Next() {
		var aasID string
		if scanErr := rows.Scan(&aasID); scanErr != nil {
			return nil, common.NewInternalServerError("AASXFS-GETAASIDS-SCAN " + scanErr.Error())
		}
		aasIDs = append(aasIDs, aasID)
	}
	if err = rows.Err(); err != nil {
		return nil, common.NewInternalServerError("AASXFS-GETAASIDS-ROWS " + err.Error())
	}

	return aasIDs, nil
}

func replaceAASIDs(ctx context.Context, tx *sql.Tx, packageDBID int64, aasIDs []string) error {
	dialect := goqu.Dialect("postgres")
	deleteSQL, deleteArgs, err := dialect.Delete("aasx_package_aas_id").
		Where(goqu.C("package_db_id").Eq(packageDBID)).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASXFS-REPLACEAASIDS-BUILDDELETE " + err.Error())
	}
	//nolint:gosec // SQL is generated by goqu with fixed table/column names and bound arguments.
	if _, err = tx.ExecContext(ctx, deleteSQL, deleteArgs...); err != nil {
		return common.NewInternalServerError("AASXFS-REPLACEAASIDS-DELETE " + err.Error())
	}

	if len(aasIDs) == 0 {
		return nil
	}

	records := make([]goqu.Record, 0, len(aasIDs))
	for idx, aasID := range aasIDs {
		records = append(records, goqu.Record{
			"package_db_id": packageDBID,
			"aas_id":        aasID,
			"position":      idx,
		})
	}

	insertSQL, insertArgs, err := dialect.Insert("aasx_package_aas_id").Rows(records).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASXFS-REPLACEAASIDS-BUILDINSERT " + err.Error())
	}
	// #nosec G701 -- SQL is generated by goqu with fixed table/column names and bound arguments.
	if _, err = tx.ExecContext(ctx, insertSQL, insertArgs...); err != nil {
		if isUniqueViolation(err) {
			return common.NewErrConflict("AASXFS-REPLACEAASIDS-CONFLICT duplicate aasIds in request")
		}
		return common.NewInternalServerError("AASXFS-REPLACEAASIDS-INSERT " + err.Error())
	}

	return nil
}

func writeLargeObjectFromFile(tx *sql.Tx, file *os.File, fileName string) (int64, string, error) {
	filePath := filepath.Clean(file.Name())
	// #nosec G703 -- file.Name() points to server-created temp files from multipart parsing.
	reopenedFile, err := os.Open(filePath)
	if err != nil {
		return 0, "", common.NewErrBadRequest("AASXFS-WRITELO-OPENFILE " + err.Error())
	}
	defer func() { _ = reopenedFile.Close() }()

	contentType, err := resolvePackageContentTypeForUpload(reopenedFile, fileName)
	if err != nil {
		return 0, "", err
	}

	if _, err = reopenedFile.Seek(0, 0); err != nil {
		return 0, "", common.NewInternalServerError("AASXFS-WRITELO-SEEK " + err.Error())
	}

	var fileOID int64
	if err = tx.QueryRow(`SELECT lo_create(0)`).Scan(&fileOID); err != nil {
		return 0, "", common.NewInternalServerError("AASXFS-WRITELO-CREATE " + err.Error())
	}

	var loFD int
	if err = tx.QueryRow(`SELECT lo_open($1, $2)`, fileOID, largeObjectWriteMode).Scan(&loFD); err != nil {
		return 0, "", common.NewInternalServerError("AASXFS-WRITELO-OPEN " + err.Error())
	}

	buffer := make([]byte, 8192)
	for {
		chunkSize, readErr := reopenedFile.Read(buffer)
		if chunkSize > 0 {
			if _, writeErr := tx.Exec(`SELECT lowrite($1, $2)`, loFD, buffer[:chunkSize]); writeErr != nil {
				_, _ = tx.Exec(`SELECT lo_close($1)`, loFD)
				return 0, "", common.NewInternalServerError("AASXFS-WRITELO-WRITE " + writeErr.Error())
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			_, _ = tx.Exec(`SELECT lo_close($1)`, loFD)
			return 0, "", common.NewInternalServerError("AASXFS-WRITELO-READ " + readErr.Error())
		}
	}

	if _, err = tx.Exec(`SELECT lo_close($1)`, loFD); err != nil {
		return 0, "", common.NewInternalServerError("AASXFS-WRITELO-CLOSE " + err.Error())
	}

	return fileOID, contentType, nil
}

func resolvePackageContentTypeForUpload(file *os.File, fileName string) (string, error) {
	if file == nil {
		return "", common.NewErrBadRequest("AASXFS-RESOLVEMIME-NILFILE package file must not be nil")
	}

	signature := make([]byte, 512)
	readBytes, err := file.Read(signature)
	if err != nil && err != io.EOF {
		return "", common.NewInternalServerError("AASXFS-RESOLVEMIME-READSIGNATURE " + err.Error())
	}

	detectedContentType := ""
	if readBytes > 0 {
		detectedContentType = strings.TrimSpace(http.DetectContentType(signature[:readBytes]))
	}

	resolvedContentType, _ := common.ResolveUploadedContentType(detectedContentType, "", fileName)

	aasxContentType, aasxErr := detectAASXEnvironmentContentType(file)
	if aasxErr == nil && aasxContentType != "" {
		resolvedContentType = aasxContentType
	}

	if resolvedContentType == "" || resolvedContentType == "application/octet-stream" || resolvedContentType == "application/zip" {
		resolvedContentType = defaultPackageContentType
	}

	return resolvedContentType, nil
}

func detectAASXEnvironmentContentType(file *os.File) (string, error) {
	if file == nil {
		return "", common.NewErrBadRequest("AASXFS-DETECTAASX-NILFILE package file must not be nil")
	}

	if _, err := file.Seek(0, 0); err != nil {
		return "", common.NewInternalServerError("AASXFS-DETECTAASX-SEEK " + err.Error())
	}

	packaging := aasx.NewPackaging()
	packageReader, err := packaging.OpenReadFromStream(file)
	if err != nil {
		return "", err
	}
	defer func() { _ = packageReader.Close() }()

	specs, err := packageReader.Specs()
	if err != nil {
		return "", err
	}

	hasJSONSpec := false
	hasXMLSpec := false
	for _, spec := range specs {
		if aasxSpecLooksJSON(spec) {
			hasJSONSpec = true
		}
		if aasxSpecLooksXML(spec) {
			hasXMLSpec = true
		}
	}

	if hasJSONSpec && !hasXMLSpec {
		return "application/aasx+json", nil
	}
	if hasXMLSpec && !hasJSONSpec {
		return "application/aasx+xml", nil
	}

	return "", fmt.Errorf("AASXFS-DETECTAASX-NOSINGLEFORMAT no unambiguous JSON/XML AASX spec found")
}

func aasxSpecLooksJSON(specPart *aasx.Part) bool {
	if specPart == nil {
		return false
	}
	partPath := normalizeAASXPartPath(specPart)
	contentType := strings.ToLower(strings.TrimSpace(specPart.ContentType))
	return strings.HasSuffix(partPath, ".json") || strings.Contains(contentType, "json")
}

func aasxSpecLooksXML(specPart *aasx.Part) bool {
	if specPart == nil {
		return false
	}
	partPath := normalizeAASXPartPath(specPart)
	contentType := strings.ToLower(strings.TrimSpace(specPart.ContentType))
	return strings.HasSuffix(partPath, ".xml") || strings.Contains(contentType, "xml")
}

func normalizeAASXPartPath(specPart *aasx.Part) string {
	if specPart == nil || specPart.URI == nil {
		return ""
	}

	uriPath := strings.TrimSpace(specPart.URI.Path)
	if uriPath == "" {
		uriPath = strings.TrimSpace(specPart.URI.String())
	}
	return strings.ToLower(uriPath)
}

func readLargeObject(tx *sql.Tx, fileOID int64) ([]byte, error) {
	var loFD int
	if err := tx.QueryRow(`SELECT lo_open($1, $2)`, fileOID, largeObjectReadMode).Scan(&loFD); err != nil {
		return nil, common.NewInternalServerError("AASXFS-READLO-OPEN " + err.Error())
	}

	content := make([]byte, 0, 8192)
	for {
		var bytesRead []byte
		err := tx.QueryRow(`SELECT loread($1, $2)`, loFD, 8192).Scan(&bytesRead)
		if err != nil {
			_, _ = tx.Exec(`SELECT lo_close($1)`, loFD)
			return nil, common.NewInternalServerError("AASXFS-READLO-READ " + err.Error())
		}
		if len(bytesRead) == 0 {
			break
		}
		content = append(content, bytesRead...)
	}

	if _, err := tx.Exec(`SELECT lo_close($1)`, loFD); err != nil {
		return nil, common.NewInternalServerError("AASXFS-READLO-CLOSE " + err.Error())
	}

	return content, nil
}

func normalizeAASIDs(aasIDs []string) []string {
	seen := make(map[string]struct{}, len(aasIDs))
	result := make([]string, 0, len(aasIDs))

	for _, raw := range aasIDs {
		parts := strings.Split(raw, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			result = append(result, trimmed)
		}
	}

	return result
}

func normalizeFileName(providedFileName string, tempFile *os.File) string {
	trimmed := strings.TrimSpace(providedFileName)
	if trimmed != "" {
		return filepath.Base(trimmed)
	}
	if tempFile == nil {
		return ""
	}

	base := filepath.Base(tempFile.Name())
	if idx := strings.LastIndex(base, "."); idx > 0 {
		base = base[:idx]
	}
	return strings.TrimSpace(base)
}

func isUniqueViolation(err error) bool {
	return common.IsPostgresUniqueViolation(err)
}

// ParseCursorID parses a cursor string into a non-negative database identifier.
func ParseCursorID(cursor string) (int64, error) {
	trimmed := strings.TrimSpace(cursor)
	if trimmed == "" {
		return 0, nil
	}

	cursorID, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor %q: %w", trimmed, err)
	}
	if cursorID < 0 {
		return 0, fmt.Errorf("invalid cursor %q: must be non-negative", trimmed)
	}
	return cursorID, nil
}
