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

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// ListGlobalAssetIDsBySubmodelID returns the distinct, non-empty globalAssetIds of every
// Asset Administration Shell that references the given submodel via a submodel reference.
//
// The Submodel Repository does not own the Asset Administration Shell tables, but both
// services share the same database schema, so this reads across into the AAS reference
// tables directly instead of round-tripping through the AAS Repository API.
func (s *SubmodelDatabase) ListGlobalAssetIDsBySubmodelID(ctx context.Context, submodelIdentifier string) ([]string, error) {
	dialect := goqu.Dialect("postgres")
	sqlQuery, args, buildErr := dialect.
		From(goqu.T("aas_submodel_reference_key").As("k")).
		InnerJoin(goqu.T("aas_submodel_reference").As("r"), goqu.On(goqu.I("r.id").Eq(goqu.I("k.reference_id")))).
		InnerJoin(goqu.T("asset_information").As("ai"), goqu.On(goqu.I("ai.asset_information_id").Eq(goqu.I("r.aas_id")))).
		SelectDistinct(goqu.I("ai.global_asset_id")).
		Where(
			goqu.I("k.value").Eq(submodelIdentifier),
			goqu.I("ai.global_asset_id").IsNotNull(),
			goqu.I("ai.global_asset_id").Neq(""),
		).
		Order(goqu.I("ai.global_asset_id").Asc()).
		Prepared(true).
		ToSQL()
	if buildErr != nil {
		return nil, common.NewInternalServerError("SMREPO-LISTGLOBALASSETIDS-BUILDSQL " + buildErr.Error())
	}

	rows, queryErr := s.readDB(ctx).QueryContext(ctx, sqlQuery, args...)
	if queryErr != nil {
		return nil, common.NewInternalServerError("SMREPO-LISTGLOBALASSETIDS-EXECSQL " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	globalAssetIDs := make([]string, 0, 4)
	for rows.Next() {
		var globalAssetID string
		if scanErr := rows.Scan(&globalAssetID); scanErr != nil {
			return nil, common.NewInternalServerError("SMREPO-LISTGLOBALASSETIDS-SCANROW " + scanErr.Error())
		}
		globalAssetIDs = append(globalAssetIDs, globalAssetID)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("SMREPO-LISTGLOBALASSETIDS-ITERROWS " + rowsErr.Error())
	}

	return globalAssetIDs, nil
}
