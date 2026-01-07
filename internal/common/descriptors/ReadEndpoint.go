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
// Author: Martin Stemmer ( Fraunhofer IESE )

package descriptors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	// nolint:revive
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/lib/pq"
)

// ReadEndpointsByDescriptorID returns all endpoints that belong to a single
// descriptor identified by the given descriptorID.
//
// It is a convenience wrapper around ReadEndpointsByDescriptorIDs and simply
// returns the slice mapped to the provided ID. If the descriptor exists but has
// no endpoints, the returned slice is empty. If the descriptorID does not
// produce any rows, the returned slice is nil and no error is raised.
//
// The provided context is used for cancellation and deadline control of the
// underlying database call.
//
// Errors originate from ReadEndpointsByDescriptorIDs (SQL build/exec/scan or
// JSON decoding failures) and are returned verbatim.
func ReadEndpointsByDescriptorID(
	ctx context.Context,
	db *sql.DB,
	descriptorID int64,
	joinOnMainTable bool,
) ([]model.Endpoint, error) {
	v, err := ReadEndpointsByDescriptorIDs(ctx, db, []int64{descriptorID}, joinOnMainTable)
	return v[descriptorID], err
}

// ReadEndpointsByDescriptorIDs retrieves endpoints for the provided descriptorIDs
// in a single database round trip.
//
// Return value is a map keyed by descriptor ID, each value containing that
// descriptor's endpoints. When descriptorIDs is empty, an empty map is returned
// without querying the database.
//
// Result semantics and ordering:
// - Endpoints are ordered by descriptor_id ASC, then position ASC, then endpoint id ASC.
// - Protocol versions are aggregated per-endpoint and ordered by version row id.
// - Security attributes are aggregated per-endpoint and ordered by attribute row id.
// - Nullable text columns are COALESCE'd to empty strings; arrays default to empty.
//
// Implementation notes:
// - Uses pq.Array with SQL ANY for efficient multi-key filtering.
// - Uses LEFT JOINs so endpoints without versions or security attributes are still returned.
// - Prepared statements are enabled via goqu to allow DB plan caching.
//
// Errors may occur while building the SQL statement, executing the query,
// scanning columns, or decoding the aggregated JSON payload of security
// attributes.
func ReadEndpointsByDescriptorIDs(
	ctx context.Context,
	db *sql.DB,
	descriptorIDs []int64,
	joinOnMainTable bool,
) (map[int64][]model.Endpoint, error) {
	out := make(map[int64][]model.Endpoint, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}

	d := goqu.Dialect(dialect)
	arr := pq.Array(descriptorIDs)

	v := goqu.T(tblEndpointProtocolVersion).As("v")
	s := goqu.T(tblSecurityAttributes).As("s")

	ds := d.From(tDescriptor).
		InnerJoin(
			tAASDescriptor,
			goqu.On(tAASDescriptor.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		)
	var joinOn exp.AliasedExpression
	if joinOnMainTable {
		joinOn = aasDescriptorEndpointAlias
		ds = ds.LeftJoin(
			aasDescriptorEndpointAlias,
			goqu.On(aasDescriptorEndpointAlias.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		)
	} else {
		joinOn = submodelDescriptorEndpointAlias
		ds = ds.LeftJoin(
			submodelDescriptorEndpointAlias,
			goqu.On(submodelDescriptorEndpointAlias.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		)
	}

	ds = ds.LeftJoin(
		v,
		goqu.On(v.Col(colEndpointID).Eq(joinOn.Col(colID))),
	).
		LeftJoin(
			s,
			goqu.On(s.Col(colEndpointID).Eq(joinOn.Col(colID))),
		).
		Where(goqu.L("? = ANY(?::bigint[])", joinOn.Col(colDescriptorID), arr)).
		Select(
			joinOn.Col(colDescriptorID),
			joinOn.Col(colID),
			goqu.Func("COALESCE", joinOn.Col(colHref), "").As(colHref),
			goqu.Func("COALESCE", joinOn.Col(colEndpointProtocol), "").As(colEndpointProtocol),
			goqu.Func("COALESCE", joinOn.Col(colSubProtocol), "").As(colSubProtocol),
			goqu.Func("COALESCE", joinOn.Col(colSubProtocolBody), "").As(colSubProtocolBody),
			goqu.Func("COALESCE", joinOn.Col(colSubProtocolBodyEncoding), "").As(colSubProtocolBodyEncoding),
			goqu.Func("COALESCE", joinOn.Col(colInterface), "").As(colInterface),

			// versions
			goqu.L(
				fmt.Sprintf(
					"COALESCE(ARRAY_AGG(v.%s ORDER BY v.%s)\n                  FILTER (WHERE v.%s IS NOT NULL), '{}')",
					colEndpointProtocolVersion, colID, colEndpointProtocolVersion,
				),
			).As("versions"),

			// sec_attrs
			goqu.L(
				fmt.Sprintf(
					"COALESCE(JSON_AGG(JSON_BUILD_OBJECT(\n                    'type', s.%s,\n                    'key', s.%s,\n                    'value', s.%s\n                  ) ORDER BY s.%s)\n                  FILTER (WHERE s.%s IS NOT NULL), '[]')",
					colSecurityType, colSecurityKey, colSecurityValue, colID, colSecurityType,
				),
			).As("sec_attrs"),
		).
		GroupBy(
			joinOn.Col(colDescriptorID),
			joinOn.Col(colPosition),
			joinOn.Col(colID),
			joinOn.Col(colHref),
			joinOn.Col(colEndpointProtocol),
			joinOn.Col(colSubProtocol),
			joinOn.Col(colSubProtocolBody),
			joinOn.Col(colSubProtocolBodyEncoding),
			joinOn.Col(colInterface),
		).
		Order(
			joinOn.Col(colPosition).Asc(),
		).
		Prepared(true)

	ds, err := auth.AddFilterQueryFromContext(ctx, ds, "$aasdesc#endpoints[]")
	if err != nil {
		return nil, err
	}

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Println("endpoints")
	_, _ = fmt.Println(sqlStr)

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type secAttr struct {
		Type  string `json:"type"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	for rows.Next() {
		var (
			descID, endpointID                                int64
			href, proto, subProto, subBody, subBodyEnc, iface string
			versions                                          pq.StringArray
			secJSON                                           []byte
		)
		if err := rows.Scan(
			&descID, &endpointID,
			&href, &proto, &subProto, &subBody, &subBodyEnc, &iface,
			&versions, &secJSON,
		); err != nil {
			return nil, err
		}

		var secAttrs []secAttr
		if err := json.Unmarshal(secJSON, &secAttrs); err != nil {
			return nil, err
		}

		converted := make([]model.ProtocolInformationSecurityAttributes, len(secAttrs))
		for i, a := range secAttrs {
			converted[i] = model.ProtocolInformationSecurityAttributes{
				Type:  a.Type,
				Key:   a.Key,
				Value: a.Value,
			}
		}

		out[descID] = append(out[descID], model.Endpoint{
			Interface: iface,
			ProtocolInformation: model.ProtocolInformation{
				Href:                    href,
				EndpointProtocol:        proto,
				Subprotocol:             subProto,
				SubprotocolBody:         subBody,
				SubprotocolBodyEncoding: subBodyEnc,
				EndpointProtocolVersion: []string(versions),
				SecurityAttributes:      converted,
			},
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
