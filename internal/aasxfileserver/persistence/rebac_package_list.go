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

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func (p *AASXFileServerDatabase) listVisiblePackages(ctx context.Context, limit int32, cursorID int64, aasID string) ([]PackageRecord, int64, error) {
	if limit <= 0 {
		limit = 100
	}
	visible := make([]PackageRecord, 0)
	for {
		candidates, more, err := p.packageCandidates(ctx, cursorID, aasID)
		if err != nil {
			return nil, 0, err
		}
		batch, err := p.visiblePackageBatch(ctx, candidates, int(limit)+1-len(visible))
		if err != nil {
			return nil, 0, err
		}
		visible = append(visible, batch...)
		if len(visible) > int(limit) {
			return visible[:limit], visible[limit-1].DBID, nil
		}
		if more == 0 || len(candidates) == 0 {
			return visible, 0, nil
		}
		cursorID = candidates[len(candidates)-1].DBID
	}
}

func (p *AASXFileServerDatabase) packageCandidates(ctx context.Context, cursorID int64, aasID string) ([]PackageRecord, int64, error) {
	var records []PackageRecord
	var more int64
	err := common.ExecuteInReadTransaction(ctx, p.readDB(ctx), "AASXFS-CANDIDATES-STARTTX", "AASXFS-CANDIDATES-COMMIT", func(tx *sql.Tx) error {
		var err error
		records, more, err = p.listPackagesInTransaction(ctx, tx, 50, cursorID, aasID)
		if err != nil {
			return err
		}
		for i := range records {
			records[i].manifest = &auth.ReBACPackageManifest{}
			records[i].manifestErr = readPackageManifestTx(ctx, tx, records[i].PackageID, records[i].manifest)
		}
		return nil
	})
	return records, more, err
}

func (p *AASXFileServerDatabase) packageVisible(ctx context.Context, record PackageRecord) (bool, error) {
	authorized, err := auth.AuthorizeReBACReadResource(ctx, "package", record.PackageID)
	if common.IsErrDenied(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if record.manifestErr != nil {
		return false, record.manifestErr
	}
	if record.manifest == nil {
		return false, common.NewErrServiceUnavailable("AASXFS-LIST-MANIFEST manifest unavailable")
	}
	err = auth.AuthorizeReBACPackageManifest(authorized, *record.manifest)
	if common.IsErrDenied(err) {
		return false, nil
	}
	return err == nil, err
}

func (p *AASXFileServerDatabase) visiblePackageBatch(ctx context.Context, candidates []PackageRecord, limit int) ([]PackageRecord, error) {
	var visible []PackageRecord
	for _, candidate := range candidates {
		allowed, err := p.packageVisible(ctx, candidate)
		if err != nil {
			return nil, err
		}
		if allowed {
			visible = append(visible, candidate)
		}
		if len(visible) == limit {
			break
		}
	}
	return visible, nil
}
