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

// Package descriptors contains the data‑access helpers that read and write
// Asset Administration Shell (AAS) and Submodel descriptor data to a
// PostgreSQL database.
// Author: Martin Stemmer ( Fraunhofer IESE )
package descriptors

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/queries"
	"github.com/lib/pq"
)

// row is an internal scan target for administrative information lookups.
// The Administration field carries JSON produced by the correlated subquery
// used by this package to materialize the administration block.
type row struct {
	ID             int64
	Administration json.RawMessage
}

// ReadAdministrativeInformationByID fetches a single AdministrativeInformation
// referenced by a nullable foreign key in the given table.
//
// Parameters:
//   - ctx: request context used for cancellation/deadlines
//   - db:  open SQL database handle
//   - tableName: name of the table that contains the administrative_information_id
//     column (e.g. aas_descriptor or submodel_descriptor)
//   - adminInfoID: nullable FK value pointing to the administration block
//
// Returns a zero value when the FK is NULL/invalid and a NotFound‑style error
// when the FK is valid but the referenced administration block is missing.
func ReadAdministrativeInformationByID(
	ctx context.Context,
	db *sql.DB,
	tableName string,
	adminInfoID sql.NullInt64,
) (*model.AdministrativeInformation, error) {
	if !adminInfoID.Valid {
		return &model.AdministrativeInformation{}, errors.New("administrative information ID is NULL/invalid")
	}

	m, err := ReadAdministrativeInformationByIDs(ctx, db, tableName, []int64{adminInfoID.Int64})
	if err != nil {
		return &model.AdministrativeInformation{}, err
	}
	v, ok := m[adminInfoID.Int64]
	if !ok {
		return &model.AdministrativeInformation{}, fmt.Errorf("administrative information with id %d not found", adminInfoID.Int64)
	}
	return v, nil
}

// ReadAdministrativeInformationByIDs fetches multiple AdministrativeInformation
// records for the provided FK values from the given table and returns them
// keyed by the FK (administrative_information_id). Missing IDs are omitted from
// the map.
//
// The function issues a single SQL SELECT over the given table, using a
// correlated subquery to produce the JSON shape expected by the builder, and
// then parses/assembles the Administration objects. Duplicate IDs in the input
// are de‑duplicated before querying.
func ReadAdministrativeInformationByIDs(
	ctx context.Context,
	db *sql.DB,
	tableName string,
	adminInfoIDs []int64,
) (map[int64]*model.AdministrativeInformation, error) {
	out := make(map[int64]*model.AdministrativeInformation, len(adminInfoIDs))
	if len(adminInfoIDs) == 0 {
		return out, nil
	}

	seen := make(map[int64]struct{}, len(adminInfoIDs))
	uniq := make([]int64, 0, len(adminInfoIDs))
	for _, id := range adminInfoIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}

	if len(uniq) == 0 {
		return out, nil
	}

	d := goqu.Dialect(dialect)

	// Correlated subquery that returns JSON for the administration block.
	adminJSON := queries.GetAdministrationSubquery(d, fmt.Sprintf("s.%s", colAdminInfoID))

	// SELECT only the requested IDs.
	arr := pq.Array(uniq)
	ds := d.From(goqu.T(tableName).As("s")).
		Select(
			goqu.I(fmt.Sprintf("s.%s", colAdminInfoID)).As("id"),
			goqu.L("COALESCE((?), '[]'::jsonb)", adminJSON),
		).
		Where(goqu.L(fmt.Sprintf("s.%s = ANY(?::bigint[])", colAdminInfoID), arr))

	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("building SQL failed: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying administrative information failed: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Administration); err != nil {
			return nil, fmt.Errorf("scanning administrative information row failed: %w", err)
		}

		if !common.IsArrayNotEmpty(r.Administration) {
			continue
		}

		adminRow, err := builders.ParseAdministrationRow(r.Administration)
		if err != nil {
			return nil, fmt.Errorf("parsing administration row (id %d) failed: %w", r.ID, err)
		}
		if adminRow == nil {
			continue
		}

		admin, err := builders.BuildAdministration(*adminRow)
		if err != nil {
			return nil, fmt.Errorf("building administration (id %d) failed: %w", r.ID, err)
		}

		out[r.ID] = &model.AdministrativeInformation{
			Version:                    admin.Version,
			Revision:                   admin.Revision,
			TemplateID:                 admin.TemplateID,
			Creator:                    admin.Creator,
			EmbeddedDataSpecifications: admin.EmbeddedDataSpecifications,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration failed: %w", err)
	}

	return out, nil
}
