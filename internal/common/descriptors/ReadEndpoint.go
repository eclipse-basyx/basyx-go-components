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
	"encoding/json"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	// nolint:revive
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
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
	db DBQueryer,
	descriptorID int64,
	joinOnMainTable string,
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
//   - Endpoints are ordered by position ASC.
//   - Protocol versions and security attributes are stored as JSONB arrays on the
//     endpoint row and are decoded per endpoint.
//   - Nullable text columns are COALESCE'd to empty strings; JSON arrays default to empty.
//
// Implementation notes:
// - Uses pq.Array with SQL ANY for efficient multi-key filtering.
// - Uses LEFT JOINs so descriptors without endpoints are still handled.
// - Prepared statements are enabled via goqu to allow DB plan caching.
//
// Errors may occur while building the SQL statement, executing the query,
// scanning columns, or decoding the aggregated JSON payload of security
// attributes.
func ReadEndpointsByDescriptorIDs(
	ctx context.Context,
	db DBQueryer,
	descriptorIDs []int64,
	joinOnMainTable string,
) (map[int64][]model.Endpoint, error) {
	if debugEnabled(ctx) {
		defer func(start time.Time) {
			_, _ = fmt.Printf("ReadEndpointsByDescriptorIDs took %s\n", time.Since(start))
		}(time.Now())
	}
	out := make(map[int64][]model.Endpoint, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}

	d := goqu.Dialect(common.Dialect)
	arr := pq.Array(descriptorIDs)

	ds := d.From(common.TDescriptor)
	var joinOn exp.AliasedExpression
	switch joinOnMainTable {
	case "aas":
		joinOn = aasDescriptorEndpointAlias
		ds = ds.InnerJoin(
			common.TAASDescriptor,
			goqu.On(common.TAASDescriptor.Col(common.ColDescriptorID).Eq(common.TDescriptor.Col(common.ColID))),
		)
		ds = ds.LeftJoin(
			aasDescriptorEndpointAlias,
			goqu.On(aasDescriptorEndpointAlias.Col(common.ColDescriptorID).Eq(common.TDescriptor.Col(common.ColID))),
		)
	case "submodel":
		joinOn = submodelDescriptorEndpointAlias
		ds = ds.InnerJoin(
			submodelDescriptorAlias,
			goqu.On(submodelDescriptorAlias.Col(common.ColDescriptorID).Eq(common.TDescriptor.Col(common.ColID))),
		).LeftJoin(
			submodelDescriptorEndpointAlias,
			goqu.On(submodelDescriptorEndpointAlias.Col(common.ColDescriptorID).Eq(submodelDescriptorAlias.Col(common.ColDescriptorID))),
		)
	case "infrastructure":
		joinOn = infrastructureDescriptorEndpointAlias
		ds = ds.InnerJoin(
			common.TInfrastructureDescriptor,
			goqu.On(common.TInfrastructureDescriptor.Col(common.ColDescriptorID).Eq(common.TDescriptor.Col(common.ColID))),
		)
		ds = ds.LeftJoin(
			infrastructureDescriptorEndpointAlias,
			goqu.On(infrastructureDescriptorEndpointAlias.Col(common.ColDescriptorID).Eq(infrastructureDescriptorAlias.Col(common.ColDescriptorID))),
		)
	}

	ds = ds.
		Where(goqu.L("? = ANY(?::bigint[])", joinOn.Col(common.ColDescriptorID), arr)).
		Select(
			joinOn.Col(common.ColDescriptorID),
			joinOn.Col(common.ColID),
			goqu.Func("COALESCE", joinOn.Col(common.ColHref), "").As(common.ColHref),
			goqu.Func("COALESCE", joinOn.Col(common.ColEndpointProtocol), "").As(common.ColEndpointProtocol),
			goqu.Func("COALESCE", joinOn.Col(common.ColSubProtocol), "").As(common.ColSubProtocol),
			goqu.Func("COALESCE", joinOn.Col(common.ColSubProtocolBody), "").As(common.ColSubProtocolBody),
			goqu.Func("COALESCE", joinOn.Col(common.ColSubProtocolBodyEncoding), "").As(common.ColSubProtocolBodyEncoding),
			goqu.Func("COALESCE", joinOn.Col(common.ColInterface), "").As(common.ColInterface),
			goqu.Func("COALESCE", joinOn.Col(common.ColEndpointProtocolVersion), goqu.L("'[]'::jsonb")).As("versions"),
			goqu.Func("COALESCE", joinOn.Col(common.ColSecurityAttributes), goqu.L("'[]'::jsonb")).As("sec_attrs"),
		).
		Order(
			joinOn.Col(common.ColPosition).Asc(),
		).
		Prepared(true)

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAASDesc)
	if err != nil {
		return nil, err
	}
	ds, err = auth.AddFilterQueryFromContext(ctx, ds, "$aasdesc#endpoints[]", collector)
	if err != nil {
		return nil, err
	}

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			descID, endpointID                                int64
			href, proto, subProto, subBody, subBodyEnc, iface string
			versionsJSON                                      []byte
			secJSON                                           []byte
		)
		if err := rows.Scan(
			&descID, &endpointID,
			&href, &proto, &subProto, &subBody, &subBodyEnc, &iface,
			&versionsJSON, &secJSON,
		); err != nil {
			return nil, err
		}

		var versions []string
		if err := json.Unmarshal(versionsJSON, &versions); err != nil {
			return nil, err
		}

		var secAttrs []model.ProtocolInformationSecurityAttributes
		if err := json.Unmarshal(secJSON, &secAttrs); err != nil {
			return nil, err
		}

		out[descID] = append(out[descID], model.Endpoint{
			Interface: iface,
			ProtocolInformation: model.ProtocolInformation{
				Href:                    href,
				EndpointProtocol:        proto,
				Subprotocol:             subProto,
				SubprotocolBody:         subBody,
				SubprotocolBodyEncoding: subBodyEnc,
				EndpointProtocolVersion: versions,
				SecurityAttributes:      secAttrs,
			},
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
