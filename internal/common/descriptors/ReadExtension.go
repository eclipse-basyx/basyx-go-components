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
// Author: Martin Stemmer ( Fraunhofer IESE )

package descriptors

import (
	"context"
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/lib/pq"
)

// ReadExtensionsByDescriptorID returns all extensions that belong to a single
// descriptor identified by the given descriptorID.
//
// It is a convenience wrapper around ReadExtensionsByDescriptorIDs and simply
// returns the slice mapped to the provided ID. If the descriptor exists but has
// no extensions, the returned slice is empty. If the descriptorID does not
// produce any rows, the returned slice is nil and no error is raised.
//
// The provided context is used for cancellation and deadline control of the
// underlying database call.
//
// Errors originate from ReadExtensionsByDescriptorIDs (SQL build/exec/scan
// failures or type conversion issues) and are returned verbatim.
func ReadExtensionsByDescriptorID(
	ctx context.Context,
	db *sql.DB,
	descriptorID int64,
) ([]types.Extension, error) {
	v, err := ReadExtensionsByDescriptorIDs(ctx, db, []int64{descriptorID})
	return v[descriptorID], err
}

// ReadExtensionsByDescriptorIDs retrieves extensions for the provided
// descriptorIDs in a single database round trip.
//
// Return value is a map keyed by descriptor ID, each value containing that
// descriptor's extensions. When descriptorIDs is empty, an empty map is
// returned without querying the database.
//
// Result semantics and ordering:
//   - Extensions are ordered by descriptor_id ASC, then extension id ASC.
//   - The extension Value is selected from one of the typed columns based on the
//     stored ValueType (xs:string/URI->text; numeric types->num; xs:boolean->bool;
//     xs:time->time; date/datetime/duration/g*->datetime). When no explicit
//     match exists, falls back to text if present.
//   - SemanticID may be nil when not set; supplemental semantic IDs and RefersTo
//     references are loaded via the respective link tables.
//
// Implementation notes:
//   - Uses pq.Array with SQL ANY for efficient multi-key filtering.
//   - Performs a single join to fetch base extension rows, then batches lookups
//     for references to minimize round trips.
//   - Converts ValueType strings to model.DataTypeDefXsd via
//     model.NewDataTypeDefXsdFromValue; invalid values propagate an error.
//
// Errors may occur while building the SQL statement, executing the query,
// scanning columns, or converting types.
func ReadExtensionsByDescriptorIDs(
	ctx context.Context,
	db DBQueryer,
	descriptorIDs []int64,
) (map[int64][]types.Extension, error) {
	out := make(map[int64][]types.Extension, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}

	d := goqu.Dialect(common.Dialect)
	dp := goqu.T(common.TblDescriptorPayload).As("dp")

	arr := pq.Array(descriptorIDs)
	sqlStr, args, err := d.
		From(dp).
		Select(
			dp.Col(common.ColDescriptorID),
			dp.Col(common.ColExtensionsPayload),
		).
		Where(goqu.L("dp.descriptor_id = ANY(?::bigint[])", arr)).
		Order(dp.Col(common.ColDescriptorID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var descriptorID int64
		var extensionsPayload []byte
		if err := rows.Scan(&descriptorID, &extensionsPayload); err != nil {
			return nil, err
		}
		extensions, err := parseExtensionsPayload(extensionsPayload)
		if err != nil {
			return nil, err
		}
		out[descriptorID] = extensions
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, descriptorID := range descriptorIDs {
		if _, ok := out[descriptorID]; !ok {
			out[descriptorID] = nil
		}
	}

	return out, nil
}
