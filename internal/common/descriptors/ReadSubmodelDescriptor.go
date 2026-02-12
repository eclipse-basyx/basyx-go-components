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
	"fmt"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/lib/pq"
	"golang.org/x/sync/errgroup"
)

// ReadSubmodelDescriptorsByAASDescriptorID returns all submodel descriptors that
// belong to a single Asset Administration Shell (AAS) identified by its internal
// descriptor id (not the AAS Id string).
//
// The function delegates to ReadSubmodelDescriptorsByAASDescriptorIDs for the
// heavy lifting and unwraps the single-entry map. The returned slice contains
// fully materialized submodel descriptors including optional fields such as
// SemanticId, Administration, DisplayName, Description, Endpoints, Extensions
// and SupplementalSemanticId where available. The order of results is by
// internal descriptor id, then the submodel descriptor position, and finally
// submodel descriptor id ascending.
//
// Parameters:
//   - ctx: request-scoped context used for cancellation and deadlines
//   - db:  open SQL database handle
//   - aasDescriptorID: internal descriptor id of the owning AAS
//
// Returns the submodel descriptors slice for the given AAS or an error if the
// query or any of the dependent lookups fail.
func ReadSubmodelDescriptorsByAASDescriptorID(
	ctx context.Context,
	db DBQueryer,
	aasDescriptorID int64,
	isMain bool,
) ([]model.SubmodelDescriptor, error) {
	v, err := ReadSubmodelDescriptorsByAASDescriptorIDs(ctx, db, []int64{aasDescriptorID}, isMain)
	return v[aasDescriptorID], err
}

// ReadSubmodelDescriptorsByDescriptorIDs returns submodel descriptors addressed
// by their own descriptor IDs (i.e., submodel_descriptor.descriptor_id). This
// is used for the Submodel Registry Service, where descriptors are not tied to
// a specific AAS (aas_descriptor_id IS NULL).
func ReadSubmodelDescriptorsByDescriptorIDs(
	ctx context.Context,
	db DBQueryer,
	descriptorIDs []int64,
) (map[int64][]model.SubmodelDescriptor, error) {
	if debugEnabled(ctx) {
		defer func(start time.Time) {
			_, _ = fmt.Printf("ReadSubmodelDescriptorsByDescriptorIDs took %s\n", time.Since(start))
		}(time.Now())
	}
	if len(descriptorIDs) == 0 {
		return map[int64][]model.SubmodelDescriptor{}, nil
	}

	allowParallel := true
	if _, ok := db.(*sql.Tx); ok {
		allowParallel = false
	}
	uniqDesc := descriptorIDs

	d := goqu.Dialect(dialect)
	payloadAlias := tDescriptorPayload.As("smd_payload")
	var mapper = []auth.ExpressionIdentifiableMapper{
		{
			Exp: submodelDescriptorAlias.Col(colDescriptorID),
		},
		{
			Exp: submodelDescriptorAlias.Col(colDescriptorID),
		},
		{
			Exp:      submodelDescriptorAlias.Col(colIDShort),
			Fragment: fragPtr("$smdesc#idShort"),
		},
		{
			Exp: submodelDescriptorAlias.Col(colAASID),
		},
		{
			Exp:      submodelDescriptorAlias.Col(colSemanticID),
			Fragment: fragPtr("$smdesc#semanticId"),
		},
		{
			Exp: payloadAlias.Col(colAdministrativeInfoPayload),
		},
		{
			Exp: payloadAlias.Col(colDescriptionPayload),
		},
		{
			Exp: payloadAlias.Col(colDisplayNamePayload),
		},
	}

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSMDesc)
	if err != nil {
		return nil, err
	}
	expressions, err := auth.GetColumnSelectStatement(ctx, mapper, collector)
	if err != nil {
		return nil, err
	}

	arr := pq.Array(uniqDesc)
	ds := d.From(submodelDescriptorAlias).
		LeftJoin(
			payloadAlias,
			goqu.On(payloadAlias.Col(colDescriptorID).Eq(submodelDescriptorAlias.Col(colDescriptorID))),
		).
		Select(
			expressions[0],
			expressions[1],
			expressions[2],
			expressions[3],
			expressions[4],
			expressions[5],
			expressions[6],
			expressions[7],
		).
		Where(
			goqu.And(
				goqu.L("? = ANY(?::bigint[])", submodelDescriptorAlias.Col(colDescriptorID), arr),
				submodelDescriptorAlias.Col(colAASDescriptorID).IsNull(),
			),
		)
	if auth.NeedsGroupBy(ctx, mapper) {
		ds = ds.GroupBy(
			submodelDescriptorAlias.Col(colDescriptorID),
			submodelDescriptorAlias.Col(colIDShort),
			submodelDescriptorAlias.Col(colAASID),
			submodelDescriptorAlias.Col(colSemanticID),
			payloadAlias.Col(colAdministrativeInfoPayload),
			payloadAlias.Col(colDescriptionPayload),
			payloadAlias.Col(colDisplayNamePayload),
		)
	}

	ds = ds.Order(
		submodelDescriptorAlias.Col(colPosition).Asc(),
	)

	seenFragments := map[grammar.FragmentStringPattern]struct{}{}
	for _, m := range mapper {
		if m.Fragment == nil {
			continue
		}
		if _, ok := seenFragments[*m.Fragment]; ok {
			continue
		}
		seenFragments[*m.Fragment] = struct{}{}
		ds, err = auth.AddFilterQueryFromContext(ctx, ds, *m.Fragment, collector)
		if err != nil {
			return nil, err
		}
	}
	ds, err = auth.AddFormulaQueryFromContext(ctx, ds, collector)
	if err != nil {
		return nil, err
	}

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}
	if debugEnabled(ctx) {
		_, _ = fmt.Println(sqlStr)
	}
	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	perDesc := make(map[int64][]model.SubmodelDescriptorRow, len(uniqDesc))
	allSmdDescIDs := make([]int64, 0, len(uniqDesc))
	semRefIDs := make([]int64, 0, len(uniqDesc))

	for rows.Next() {
		var r model.SubmodelDescriptorRow
		if err := rows.Scan(
			&r.AasDescID,
			&r.SmdDescID,
			&r.IDShort,
			&r.ID,
			&r.SemanticRefID,
			&r.AdministrativeInfoPayload,
			&r.DescriptionPayload,
			&r.DisplayNamePayload,
		); err != nil {
			return nil, err
		}
		perDesc[r.AasDescID] = append(perDesc[r.AasDescID], r)
		allSmdDescIDs = append(allSmdDescIDs, r.SmdDescID)
		if r.SemanticRefID.Valid {
			semRefIDs = append(semRefIDs, r.SemanticRefID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return materializeSubmodelDescriptors(
		ctx,
		db,
		uniqDesc,
		perDesc,
		allSmdDescIDs,
		semRefIDs,
		allowParallel,
	)
}

// ReadSubmodelDescriptorsByAASDescriptorIDs returns all submodel descriptors for
// a set of AAS descriptor ids (internal ids, not AAS Id strings). Results are
// grouped by AAS descriptor id in the returned map. The function performs a
// single base query to collect submodel rows and then issues batched lookups to
// materialize related data (semantic references, administrative information,
// display name and description language strings, endpoints, extensions and
// supplemental semantic references). Batched queries are executed concurrently
// using errgroup to reduce latency.
//
// If an AAS descriptor id from the input has no submodel descriptors, the map
// will contain that key with a nil slice to signal an empty result explicitly.
// When the input is empty, an empty map is returned.
//
// Parameters:
//   - ctx: request-scoped context used for cancellation and deadlines
//   - db:  open SQL database handle
//   - aasDescriptorIDs: list of internal AAS descriptor ids to fetch for
//
// Returns a map keyed by AAS descriptor id with the corresponding submodel
// descriptors or an error if any query fails.
//
//nolint:revive // This method is already refactored and further changes would not improve readability.
func ReadSubmodelDescriptorsByAASDescriptorIDs(
	ctx context.Context,
	db DBQueryer,
	aasDescriptorIDs []int64,
	isMain bool,
) (map[int64][]model.SubmodelDescriptor, error) {

	if debugEnabled(ctx) {
		defer func(start time.Time) {
			_, _ = fmt.Printf("ReadSubmodelDescriptorsByAASDescriptorIDs took %s\n", time.Since(start))
		}(time.Now())
	}
	out := make(map[int64][]model.SubmodelDescriptor, len(aasDescriptorIDs))
	if len(aasDescriptorIDs) == 0 {
		return out, nil
	}

	allowParallel := true
	if _, ok := db.(*sql.Tx); ok {
		allowParallel = false
	}
	uniqAASDesc := aasDescriptorIDs

	d := goqu.Dialect(dialect)
	payloadAlias := tDescriptorPayload.As("smd_payload")
	var mapper = []auth.ExpressionIdentifiableMapper{
		{
			Exp: submodelDescriptorAlias.Col(colAASDescriptorID),
		},
		{
			Exp: submodelDescriptorAlias.Col(colDescriptorID),
		},
		{
			Exp:      submodelDescriptorAlias.Col(colIDShort),
			Fragment: fragPtr("$aasdesc#submodelDescriptors[].idShort"),
		},
		{
			Exp: submodelDescriptorAlias.Col(colAASID),
		},
		{
			Exp:      submodelDescriptorAlias.Col(colSemanticID),
			Fragment: fragPtr("$aasdesc#submodelDescriptors[].semanticId"),
		},
		{
			Exp: payloadAlias.Col(colAdministrativeInfoPayload),
		},
		{
			Exp: payloadAlias.Col(colDescriptionPayload),
		},
		{
			Exp: payloadAlias.Col(colDisplayNamePayload),
		},
	}
	var root grammar.CollectorRoot
	if isMain {
		root = grammar.CollectorRootSMDesc
	} else {
		root = grammar.CollectorRootAASDesc
	}
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(root)
	if err != nil {
		return nil, err
	}
	expressions, err := auth.GetColumnSelectStatement(ctx, mapper, collector)
	if err != nil {
		return nil, err
	}
	arr := pq.Array(uniqAASDesc)
	ds := d.From(tDescriptor).
		InnerJoin(
			tAASDescriptor,
			goqu.On(tAASDescriptor.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		).
		LeftJoin(
			submodelDescriptorAlias,
			goqu.On(submodelDescriptorAlias.Col(colAASDescriptorID).Eq(tAASDescriptor.Col(colDescriptorID))),
		).
		LeftJoin(
			payloadAlias,
			goqu.On(payloadAlias.Col(colDescriptorID).Eq(submodelDescriptorAlias.Col(colDescriptorID))),
		).
		Select(
			expressions[0],
			expressions[1],
			expressions[2],
			expressions[3],
			expressions[4],
			expressions[5],
			expressions[6],
			expressions[7],
		).
		Where(goqu.L("? = ANY(?::bigint[])", submodelDescriptorAlias.Col(colAASDescriptorID), arr))
	if auth.NeedsGroupBy(ctx, mapper) {
		ds = ds.GroupBy(
			submodelDescriptorAlias.Col(colAASDescriptorID),
			submodelDescriptorAlias.Col(colDescriptorID),
			submodelDescriptorAlias.Col(colIDShort),
			submodelDescriptorAlias.Col(colAASID),
			submodelDescriptorAlias.Col(colSemanticID),
			payloadAlias.Col(colAdministrativeInfoPayload),
			payloadAlias.Col(colDescriptionPayload),
			payloadAlias.Col(colDisplayNamePayload),
		)
	}
	ds = ds.Order(
		submodelDescriptorAlias.Col(colPosition).Asc(),
	)

	ds, err = auth.AddFilterQueryFromContext(ctx, ds, "$aasdesc#submodelDescriptors[]", collector)
	if err != nil {
		return nil, err
	}
	if isMain {
		ds, err = auth.AddFormulaQueryFromContext(ctx, ds, collector)
		if err != nil {
			return nil, err
		}
	}

	sqlStr, args, err := ds.ToSQL()

	if err != nil {
		return nil, err
	}
	if debugEnabled(ctx) {
		_, _ = fmt.Println(sqlStr)
	}
	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	perAAS := make(map[int64][]model.SubmodelDescriptorRow, len(uniqAASDesc))
	allSmdDescIDs := make([]int64, 0, 10000)
	semRefIDs := make([]int64, 0, 10000)

	for rows.Next() {
		var r model.SubmodelDescriptorRow
		if err := rows.Scan(
			&r.AasDescID,
			&r.SmdDescID,
			&r.IDShort,
			&r.ID,
			&r.SemanticRefID,
			&r.AdministrativeInfoPayload,
			&r.DescriptionPayload,
			&r.DisplayNamePayload,
		); err != nil {
			return nil, err
		}
		perAAS[r.AasDescID] = append(perAAS[r.AasDescID], r)
		allSmdDescIDs = append(allSmdDescIDs, r.SmdDescID)
		if r.SemanticRefID.Valid {
			semRefIDs = append(semRefIDs, r.SemanticRefID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return materializeSubmodelDescriptors(
		ctx,
		db,
		uniqAASDesc,
		perAAS,
		allSmdDescIDs,
		semRefIDs,
		allowParallel,
	)
}

func materializeSubmodelDescriptors(
	ctx context.Context,
	db DBQueryer,
	groupIDs []int64,
	perGroup map[int64][]model.SubmodelDescriptorRow,
	allSmdDescIDs []int64,
	semRefIDs []int64,
	allowParallel bool,
) (map[int64][]model.SubmodelDescriptor, error) {
	out := make(map[int64][]model.SubmodelDescriptor, len(groupIDs))
	if len(allSmdDescIDs) == 0 {
		ensureSubmodelDescriptorGroups(out, groupIDs)
		return out, nil
	}

	lookups, err := loadSubmodelDescriptorLookups(
		ctx,
		db,
		semRefIDs,
		allSmdDescIDs,
		allowParallel,
	)
	if err != nil {
		return nil, err
	}

	if err := assembleSubmodelDescriptors(out, perGroup, lookups); err != nil {
		return nil, err
	}
	ensureSubmodelDescriptorGroups(out, groupIDs)
	return out, nil
}

type submodelDescriptorLookups struct {
	semRefByID       map[int64]types.IReference
	suppBySmdDesc    map[int64][]types.IReference
	endpointsByDesc  map[int64][]model.Endpoint
	extensionsByDesc map[int64][]types.Extension
}

func newSubmodelDescriptorLookups() submodelDescriptorLookups {
	return submodelDescriptorLookups{
		semRefByID:       map[int64]types.IReference{},
		suppBySmdDesc:    map[int64][]types.IReference{},
		endpointsByDesc:  map[int64][]model.Endpoint{},
		extensionsByDesc: map[int64][]types.Extension{},
	}
}

func loadSubmodelDescriptorLookups(
	ctx context.Context,
	db DBQueryer,
	semRefIDs []int64,
	smdDescIDs []int64,
	allowParallel bool,
) (submodelDescriptorLookups, error) {
	if allowParallel {
		return loadSubmodelDescriptorLookupsParallel(ctx, db, semRefIDs, smdDescIDs)
	}
	return loadSubmodelDescriptorLookupsSerial(ctx, db, semRefIDs, smdDescIDs)
}

func loadSubmodelDescriptorLookupsParallel(
	ctx context.Context,
	db DBQueryer,
	semRefIDs []int64,
	smdDescIDs []int64,
) (submodelDescriptorLookups, error) {
	lookups := newSubmodelDescriptorLookups()
	g, gctx := errgroup.WithContext(ctx)

	if len(semRefIDs) > 0 {
		ids := semRefIDs
		GoAssign(g, func() (map[int64]types.IReference, error) {
			return GetReferencesByIDsBatch(db, ids)
		}, &lookups.semRefByID)
	}

	if len(smdDescIDs) > 0 {
		ids := smdDescIDs
		GoAssign(g, func() (map[int64][]types.IReference, error) {
			return readEntityReferences1ToMany(
				gctx, db, ids,
				tblSubmodelDescriptorSuppSemantic,
				colDescriptorID,
				colReferenceID,
			)
		}, &lookups.suppBySmdDesc)

		GoAssign(g, func() (map[int64][]model.Endpoint, error) {
			return ReadEndpointsByDescriptorIDs(gctx, db, ids, "submodel")
		}, &lookups.endpointsByDesc)

		GoAssign(g, func() (map[int64][]types.Extension, error) {
			return ReadExtensionsByDescriptorIDs(gctx, db, ids)
		}, &lookups.extensionsByDesc)
	}

	if err := g.Wait(); err != nil {
		return submodelDescriptorLookups{}, err
	}
	return lookups, nil
}

func loadSubmodelDescriptorLookupsSerial(
	ctx context.Context,
	db DBQueryer,
	semRefIDs []int64,
	smdDescIDs []int64,
) (submodelDescriptorLookups, error) {
	lookups := newSubmodelDescriptorLookups()
	var err error

	if len(semRefIDs) > 0 {
		lookups.semRefByID, err = GetReferencesByIDsBatch(db, semRefIDs)
		if err != nil {
			return submodelDescriptorLookups{}, err
		}
	}
	if len(smdDescIDs) > 0 {
		lookups.suppBySmdDesc, err = readEntityReferences1ToMany(
			ctx, db, smdDescIDs,
			tblSubmodelDescriptorSuppSemantic,
			colDescriptorID,
			colReferenceID,
		)
		if err != nil {
			return submodelDescriptorLookups{}, err
		}
		lookups.endpointsByDesc, err = ReadEndpointsByDescriptorIDs(ctx, db, smdDescIDs, "submodel")
		if err != nil {
			return submodelDescriptorLookups{}, err
		}
		lookups.extensionsByDesc, err = ReadExtensionsByDescriptorIDs(ctx, db, smdDescIDs)
		if err != nil {
			return submodelDescriptorLookups{}, err
		}
	}

	return lookups, nil
}

func assembleSubmodelDescriptors(
	out map[int64][]model.SubmodelDescriptor,
	perGroup map[int64][]model.SubmodelDescriptorRow,
	lookups submodelDescriptorLookups,
) error {
	for groupID, rows := range perGroup {
		for _, r := range rows {
			admin, err := parseAdministrativeInfoPayload(r.AdministrativeInfoPayload)
			if err != nil {
				return common.NewInternalServerError("SMDESC-READ-ADMINPAYLOAD")
			}
			displayName, err := parseLangStringNamePayload(r.DisplayNamePayload)
			if err != nil {
				return common.NewInternalServerError("SMDESC-READ-DISPLAYNAMEPAYLOAD")
			}
			description, err := parseLangStringTextPayload(r.DescriptionPayload)
			if err != nil {
				return common.NewInternalServerError("SMDESC-READ-DESCRIPTIONPAYLOAD")
			}

			out[groupID] = append(out[groupID], model.SubmodelDescriptor{
				IdShort:                r.IDShort.String,
				Id:                     r.ID.String,
				SemanticId:             lookupSubmodelSemanticRef(lookups.semRefByID, r.SemanticRefID),
				Administration:         admin,
				DisplayName:            displayName,
				Description:            description,
				Endpoints:              lookups.endpointsByDesc[r.SmdDescID],
				Extensions:             lookups.extensionsByDesc[r.SmdDescID],
				SupplementalSemanticId: lookups.suppBySmdDesc[r.SmdDescID],
			})
		}
	}
	return nil
}

func ensureSubmodelDescriptorGroups(out map[int64][]model.SubmodelDescriptor, groupIDs []int64) {
	for _, id := range groupIDs {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
}

func lookupSubmodelSemanticRef(
	semRefByID map[int64]types.IReference,
	refID sql.NullInt64,
) types.IReference {
	if refID.Valid {
		return semRefByID[refID.Int64]
	}
	return nil
}
