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

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// GetReBACPackageManifest loads the immutable member identities for one package.
func (p *AASXFileServerDatabase) GetReBACPackageManifest(ctx context.Context, packageID string) (auth.ReBACPackageManifest, error) {
	var manifest auth.ReBACPackageManifest
	err := common.ExecuteInReadTransaction(ctx, p.readDB(ctx), "AASXFS-MANIFEST-STARTTX", "AASXFS-MANIFEST-COMMIT", func(tx *sql.Tx) error {
		return readPackageManifestTx(ctx, tx, packageID, &manifest)
	})
	return manifest, err
}

func readPackageManifestTx(ctx context.Context, tx *sql.Tx, packageID string, manifest *auth.ReBACPackageManifest) error {
	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.From("aasx_package").Join(goqu.T("aasx_package_manifest"), goqu.On(goqu.I("aasx_package.id").Eq(goqu.I("aasx_package_manifest.package_db_id")))).Select("aasx_package.id", "content_sha256").Where(goqu.C("package_id").Eq(packageID)).Prepared(true).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASXFS-MANIFEST-BUILD " + err.Error())
	}
	var dbID int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&dbID, &manifest.ContentSHA256); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return common.NewErrServiceUnavailable("AASXFS-MANIFEST-MISSING package has no ReBAC manifest")
		}
		return common.NewInternalServerError("AASXFS-MANIFEST-QUERY " + err.Error())
	}
	memberSQL, memberArgs, buildErr := dialect.From("aasx_package_manifest_resource").Select("resource_uuid", "resource_kind", "resource_identifier").Where(goqu.C("package_db_id").Eq(dbID)).Order(goqu.C("position").Asc()).Prepared(true).ToSQL()
	if buildErr != nil {
		return common.NewInternalServerError("AASXFS-MANIFEST-BUILDMEMBERS " + buildErr.Error())
	}
	rows, err := tx.QueryContext(ctx, memberSQL, memberArgs...)
	if err != nil {
		return common.NewInternalServerError("AASXFS-MANIFEST-MEMBERS " + err.Error())
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var member auth.ReBACPackageManifestMember
		if err = rows.Scan(&member.UUID, &member.Kind, &member.Identifier); err != nil {
			return common.NewInternalServerError("AASXFS-MANIFEST-SCAN " + err.Error())
		}
		manifest.Members = append(manifest.Members, member)
	}
	return rows.Err()
}

func replacePackageManifest(ctx context.Context, tx *sql.Tx, packageDBID int64, manifest *auth.ReBACPackageManifest) error {
	if manifest == nil {
		return nil
	}
	if manifest.ContentSHA256 == "" || len(manifest.Members) == 0 {
		return common.NewErrBadRequest("AASXFS-MANIFEST-INVALID manifest is incomplete")
	}
	dialect := goqu.Dialect("postgres")
	query, args, err := dialect.Insert("aasx_package_manifest").Rows(goqu.Record{"package_db_id": packageDBID, "content_sha256": manifest.ContentSHA256, "manifest_version": 1}).OnConflict(goqu.DoUpdate("package_db_id", goqu.Record{"content_sha256": manifest.ContentSHA256, "manifest_version": 1})).Prepared(true).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASXFS-MANIFEST-BUILDUPSERT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("AASXFS-MANIFEST-UPSERT " + err.Error())
	}
	deleteSQL, deleteArgs, err := dialect.Delete("aasx_package_manifest_resource").Where(goqu.C("package_db_id").Eq(packageDBID)).Prepared(true).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASXFS-MANIFEST-BUILDDELETE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, deleteSQL, deleteArgs...); err != nil {
		return common.NewInternalServerError("AASXFS-MANIFEST-DELETE " + err.Error())
	}
	records := make([]goqu.Record, 0, len(manifest.Members))
	for position, member := range manifest.Members {
		records = append(records, goqu.Record{"package_db_id": packageDBID, "resource_uuid": member.UUID, "resource_kind": member.Kind, "resource_identifier": member.Identifier, "position": position})
	}
	insertSQL, insertArgs, err := dialect.Insert("aasx_package_manifest_resource").Rows(records).Prepared(true).ToSQL()
	if err != nil {
		return common.NewInternalServerError("AASXFS-MANIFEST-BUILDINSERT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, insertSQL, insertArgs...); err != nil {
		return common.NewInternalServerError("AASXFS-MANIFEST-INSERT " + err.Error())
	}
	return nil
}
