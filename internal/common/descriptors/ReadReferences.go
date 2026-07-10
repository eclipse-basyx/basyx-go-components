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
	"sort"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// ReadSubmodelDescriptorSemanticReferencesByDescriptorIDs loads semantic
// references for submodel descriptors keyed by descriptor ID.
func ReadSubmodelDescriptorSemanticReferencesByDescriptorIDs(
	ctx context.Context,
	db DBQueryer,
	descriptorIDs []int64,
) (map[int64]types.IReference, error) {
	out := make(map[int64]types.IReference, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}

	rows, err := queryReferenceRowsByOwnerIDs(
		ctx,
		db,
		descriptorIDs,
		referenceQuerySpec{
			ownerTable:        "submodel_descriptor",
			ownerIDColumn:     "descriptor_id",
			referenceTable:    "submodel_descriptor_semantic_id_reference",
			referenceKeyTable: "submodel_descriptor_semantic_id_reference_key",
			ownerAlias:        common.AliasSubmodelDescriptor,
			referenceAlias:    common.AliasSubmodelDescriptorSemanticIDReference,
			referenceKeyAlias: common.AliasSubmodelDescriptorSemanticIDReferenceKey,
			filterSpecs: []referenceFilterSpec{
				{
					fragment:  "$aasdesc#submodelDescriptors[].semanticId.keys[]",
					collector: nil,
				},
				{
					fragment:  "$smdesc#semanticId.keys[]",
					collector: nil,
				},
			},
		},
	)
	if err != nil {
		return nil, err
	}

	for _, id := range descriptorIDs {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	for ownerID, ref := range rows {
		out[ownerID] = ref
	}

	return out, nil
}

// ReadSpecificAssetExternalSubjectReferencesBySpecificAssetIDs loads external
// subject references for specific asset IDs keyed by specific asset ID.
func ReadSpecificAssetExternalSubjectReferencesBySpecificAssetIDs(
	ctx context.Context,
	db DBQueryer,
	specificAssetIDs []int64,
) (map[int64]types.IReference, error) {
	out := make(map[int64]types.IReference, len(specificAssetIDs))
	if len(specificAssetIDs) == 0 {
		return out, nil
	}

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAASDesc)
	if err != nil {
		return nil, fmt.Errorf("REFREAD-EXTSUBJECT-COLLECTOR: %w", err)
	}
	collector.AllowInlineAliases(
		"descriptor",
		"aas_descriptor",
		common.AliasSpecificAssetID,
		common.AliasExternalSubjectReference,
		common.AliasExternalSubjectReferenceKey,
	)

	rows, err := queryReferenceRowsByOwnerIDs(
		ctx,
		db,
		specificAssetIDs,
		referenceQuerySpec{
			ownerTable:        "specific_asset_id",
			ownerIDColumn:     "id",
			referenceTable:    "specific_asset_id_external_subject_id_reference",
			referenceKeyTable: "specific_asset_id_external_subject_id_reference_key",
			ownerAlias:        common.AliasSpecificAssetID,
			referenceAlias:    common.AliasExternalSubjectReference,
			referenceKeyAlias: common.AliasExternalSubjectReferenceKey,
			filterSpecs: []referenceFilterSpec{
				{
					fragment:  "$aas#assetInformation.specificAssetIds[].externalSubjectId",
					collector: nil,
				},
				{
					fragment:  "$aas#assetInformation.specificAssetIds[].externalSubjectId.keys[]",
					collector: nil,
				},
				{
					fragment:  "$aasdesc#specificAssetIds[].externalSubjectId.keys[]",
					collector: collector,
				},
				{
					fragment:  "$bd#specificAssetIds[].externalSubjectId.keys[]",
					collector: nil,
				},
			},
		},
	)
	if err != nil {
		return nil, err
	}

	for _, id := range specificAssetIDs {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	for ownerID, ref := range rows {
		out[ownerID] = ref
	}

	return out, nil
}

// ReadSpecificAssetSupplementalSemanticReferencesBySpecificAssetIDs loads
// supplemental semantic references for specific asset IDs keyed by specific
// asset ID.
func ReadSpecificAssetSupplementalSemanticReferencesBySpecificAssetIDs(
	ctx context.Context,
	db DBQueryer,
	specificAssetIDs []int64,
) (map[int64][]types.IReference, error) {
	return readContextReferences1ToManyByOwnerIDs(
		ctx,
		db,
		specificAssetIDs,
		contextReferences1ToManyQuerySpec{
			ownerIDColumn:  common.ColSpecificAssetIDID,
			referenceTable: common.TblSpecificAssetIDSuppSemantic,
			errPrefix:      "REFREAD-SUPPSPEC",
		},
	)
}

// ReadSubmodelDescriptorSupplementalSemanticReferencesByDescriptorIDs loads
// supplemental semantic references for submodel descriptors keyed by descriptor
// ID.
func ReadSubmodelDescriptorSupplementalSemanticReferencesByDescriptorIDs(
	ctx context.Context,
	db DBQueryer,
	descriptorIDs []int64,
) (map[int64][]types.IReference, error) {
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSMDesc)
	if err != nil {
		return nil, fmt.Errorf("REFREAD-SUPPSMDESC-COLLECTOR: %w", err)
	}
	collector.AllowInlineAliases(
		"submodel_descriptor",
		"aasdesc_submodel_descriptor_supplemental_semantic_id_reference",
		"aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key",
	)

	return readContextReferences1ToManyByOwnerIDs(
		ctx,
		db,
		descriptorIDs,
		contextReferences1ToManyQuerySpec{
			ownerTable:        common.TblSubmodelDescriptor,
			ownerIDColumn:     common.ColDescriptorID,
			referenceTable:    common.TblSubmodelDescriptorSuppSemantic,
			ownerAlias:        common.AliasSubmodelDescriptor,
			referenceAlias:    "aasdesc_submodel_descriptor_supplemental_semantic_id_reference",
			referenceKeyAlias: "aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key",
			filterSpecs: []referenceFilterSpec{
				{
					fragment:  "$aasdesc#submodelDescriptors[].supplementalSemanticIds[]",
					collector: collector,
				},
				{
					fragment:  "$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[]",
					collector: collector,
				},
				{
					fragment:  "$smdesc#supplementalSemanticIds[]",
					collector: collector,
				},
				{
					fragment:  "$smdesc#supplementalSemanticIds[].keys[]",
					collector: collector,
				},
			},
			errPrefix: "REFREAD-SUPPSMDESC",
		},
	)
}

// ReadSubmodelSupplementalSemanticReferencesBySubmodelIDs loads filtered
// supplemental semantic references for submodels keyed by database ID.
func ReadSubmodelSupplementalSemanticReferencesBySubmodelIDs(
	ctx context.Context,
	db DBQueryer,
	submodelIDs []int64,
) (map[int64][]types.IReference, error) {
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
	if err != nil {
		return nil, fmt.Errorf("REFREAD-SUPPSM-COLLECTOR: %w", err)
	}
	collector.AllowInlineAliases(
		"s",
		"sm_supplemental_semantic_id_reference",
		"sm_supplemental_semantic_id_reference_key",
	)
	filterCtx, filterSpecs := supplementalSemanticIDFilterContext(ctx, "$sm#", collector)

	return readContextReferences1ToManyByOwnerIDs(
		filterCtx,
		db,
		submodelIDs,
		contextReferences1ToManyQuerySpec{
			ownerTable:        "submodel",
			ownerJoinColumn:   common.ColID,
			ownerIDColumn:     common.ColSubmodelID,
			referenceTable:    common.TblSubmodelSuppSemantic,
			ownerAlias:        "s",
			referenceAlias:    "sm_supplemental_semantic_id_reference",
			referenceKeyAlias: "sm_supplemental_semantic_id_reference_key",
			filterSpecs:       filterSpecs,
			errPrefix:         "REFREAD-SUPPSM",
		},
	)
}

// ReadSubmodelElementSupplementalSemanticReferencesByElementIDs loads filtered
// supplemental semantic references for submodel elements keyed by database ID.
func ReadSubmodelElementSupplementalSemanticReferencesByElementIDs(
	ctx context.Context,
	db DBQueryer,
	submodelElementIDs []int64,
) (map[int64][]types.IReference, error) {
	collector, err := grammar.NewResolvedFieldPathCollectorForSMERow("submodel_element")
	if err != nil {
		return nil, fmt.Errorf("REFREAD-SUPPSME-COLLECTOR: %w", err)
	}
	collector.AllowInlineAliases(
		"submodel_element",
		"sme_supplemental_semantic_id_reference",
		"sme_supplemental_semantic_id_reference_key",
	)
	filterCtx, filterSpecs := supplementalSemanticIDFilterContext(ctx, "$sme", collector)

	return readContextReferences1ToManyByOwnerIDs(
		filterCtx,
		db,
		submodelElementIDs,
		contextReferences1ToManyQuerySpec{
			ownerTable:        "submodel_element",
			ownerJoinColumn:   common.ColID,
			ownerIDColumn:     common.ColSubmodelElementID,
			referenceTable:    common.TblSubmodelElementSuppSemantic,
			ownerAlias:        "submodel_element",
			referenceAlias:    "sme_supplemental_semantic_id_reference",
			referenceKeyAlias: "sme_supplemental_semantic_id_reference_key",
			filterSpecs:       filterSpecs,
			errPrefix:         "REFREAD-SUPPSME",
		},
	)
}

func supplementalSemanticIDFilterContext(
	ctx context.Context,
	prefix string,
	collector *grammar.ResolvedFieldPathCollector,
) (context.Context, []referenceFilterSpec) {
	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter == nil {
		return ctx, nil
	}

	fragments := make([]grammar.FragmentStringPattern, 0)
	for fragment := range queryFilter.Filters {
		value := string(fragment)
		if strings.HasPrefix(value, prefix) && strings.Contains(value, "#supplementalSemanticIds") {
			fragments = append(fragments, fragment)
		}
	}
	sort.Slice(fragments, func(i, j int) bool {
		return fragments[i] < fragments[j]
	})

	specs := make([]referenceFilterSpec, 0, len(fragments))
	filteredQueryFilter := &auth.QueryFilter{
		Filters:     make(auth.FragmentFilters, len(fragments)),
		FilterMatch: make(auth.FragmentMatchModes, len(fragments)),
	}
	for _, fragment := range fragments {
		filteredQueryFilter.Filters[fragment] = queryFilter.Filters[fragment]
		if queryFilter.FilterMatch != nil {
			filteredQueryFilter.FilterMatch[fragment] = queryFilter.FilterMatch[fragment]
		}
		specs = append(specs, referenceFilterSpec{
			fragment:  fragment,
			collector: collector,
		})
	}
	return auth.WithQueryFilter(ctx, filteredQueryFilter), specs
}

type contextReferenceRow struct {
	ownerID                int64
	refType                sql.NullInt64
	keyID                  sql.NullInt64
	keyType                sql.NullInt64
	keyVal                 sql.NullString
	parentReferencePayload []byte
}

type referenceFilterSpec struct {
	fragment  grammar.FragmentStringPattern
	collector *grammar.ResolvedFieldPathCollector
}

type referenceQuerySpec struct {
	ownerTable        string
	ownerIDColumn     string
	referenceTable    string
	referenceKeyTable string
	ownerAlias        string
	referenceAlias    string
	referenceKeyAlias string
	filterSpecs       []referenceFilterSpec
}

type contextReferences1ToManyQuerySpec struct {
	ownerTable        string
	ownerJoinColumn   string
	ownerIDColumn     string
	referenceTable    string
	ownerAlias        string
	referenceAlias    string
	referenceKeyAlias string
	filterSpecs       []referenceFilterSpec
	errPrefix         string
}

func queryReferenceRowsByOwnerIDs(
	ctx context.Context,
	db DBQueryer,
	ownerIDs []int64,
	spec referenceQuerySpec,
) (map[int64]types.IReference, error) {
	if len(ownerIDs) == 0 {
		return map[int64]types.IReference{}, nil
	}

	d := goqu.Dialect(common.Dialect)

	ot := goqu.T(spec.ownerTable).As(spec.ownerAlias)
	rt := goqu.T(spec.referenceTable).As(spec.referenceAlias)
	rkt := goqu.T(spec.referenceKeyTable).As(spec.referenceKeyAlias)
	rpt := goqu.T(spec.referenceTable + "_payload").As("rpt")

	ds := d.From(ot).
		LeftJoin(rt, goqu.On(rt.Col(common.ColID).Eq(ot.Col(spec.ownerIDColumn)))).
		LeftJoin(rpt, goqu.On(rpt.Col(common.ColReferenceID).Eq(rt.Col(common.ColID)))).
		LeftJoin(rkt, goqu.On(rkt.Col(common.ColReferenceID).Eq(rt.Col(common.ColID)))).
		Select(
			ot.Col(spec.ownerIDColumn).As("owner_id"),
			rt.Col(common.ColType).As("ref_type"),
			rkt.Col(common.ColID).As("key_id"),
			rkt.Col(common.ColType).As("key_type"),
			rkt.Col(common.ColValue).As("key_value"),
			rpt.Col("parent_reference_payload").As("parent_reference_payload"),
		).
		Where(ot.Col(spec.ownerIDColumn).In(ownerIDs)).
		Order(
			ot.Col(spec.ownerIDColumn).Asc(),
			rkt.Col(common.ColPosition).Asc(),
			rkt.Col(common.ColID).Asc(),
		)

	var err error
	for _, filterSpec := range spec.filterSpecs {
		ds, err = auth.AddFilterQueryFromContext(ctx, ds, filterSpec.fragment, filterSpec.collector)
		if err != nil {
			return nil, fmt.Errorf("REFREAD-ADDFILTER: %w", err)
		}
	}

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REFREAD-BUILDQUERY: %w", err)
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("REFREAD-QUERYDB: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	builders := make(map[int64]*builder.ReferenceBuilder, len(ownerIDs))
	refs := make(map[int64]types.IReference, len(ownerIDs))

	for rows.Next() {
		var row contextReferenceRow
		if err := rows.Scan(
			&row.ownerID,
			&row.refType,
			&row.keyID,
			&row.keyType,
			&row.keyVal,
			&row.parentReferencePayload,
		); err != nil {
			return nil, fmt.Errorf("REFREAD-SCANROW: %w", err)
		}

		if !row.refType.Valid {
			continue
		}

		b, ok := builders[row.ownerID]
		if !ok {
			ref, rb := builder.NewReferenceBuilder(types.ReferenceTypes(row.refType.Int64), row.ownerID)
			parentReference, err := parseReferencePayload(row.parentReferencePayload)
			if err != nil {
				return nil, fmt.Errorf("REFREAD-PARSEPARENTPAYLOAD: %w", err)
			}
			ref.SetReferredSemanticID(parentReference)
			refs[row.ownerID] = ref
			builders[row.ownerID] = rb
			b = rb
		}

		if row.keyID.Valid && row.keyType.Valid && row.keyVal.Valid {
			b.CreateKey(row.keyID.Int64, types.KeyTypes(row.keyType.Int64), row.keyVal.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("REFREAD-ITERATEROWS: %w", err)
	}

	for _, b := range builders {
		b.BuildNestedStructure()
	}

	return refs, nil
}

func readContextReferences1ToManyByOwnerIDs(
	ctx context.Context,
	db DBQueryer,
	ownerIDs []int64,
	spec contextReferences1ToManyQuerySpec,
) (map[int64][]types.IReference, error) {
	out := make(map[int64][]types.IReference, len(ownerIDs))
	if len(ownerIDs) == 0 {
		return out, nil
	}

	d := goqu.Dialect(common.Dialect)

	referenceAlias := firstNonEmpty(spec.referenceAlias, "rt")
	referenceKeyAlias := firstNonEmpty(spec.referenceKeyAlias, "rkt")

	rt := goqu.T(spec.referenceTable).As(referenceAlias)
	rkt := goqu.T(spec.referenceTable + "_key").As(referenceKeyAlias)
	rpt := goqu.T(spec.referenceTable + "_payload").As("rpt")

	ds := d.From(rt).
		LeftJoin(rpt, goqu.On(rpt.Col(common.ColReferenceID).Eq(rt.Col(common.ColID)))).
		LeftJoin(rkt, goqu.On(rkt.Col(common.ColReferenceID).Eq(rt.Col(common.ColID)))).
		Select(
			rt.Col(spec.ownerIDColumn).As("owner_id"),
			rt.Col(common.ColID).As("ref_id"),
			rt.Col(common.ColType).As("ref_type"),
			rkt.Col(common.ColID).As("key_id"),
			rkt.Col(common.ColType).As("key_type"),
			rkt.Col(common.ColValue).As("key_value"),
			rpt.Col("parent_reference_payload").As("parent_reference_payload"),
		).
		Where(rt.Col(spec.ownerIDColumn).In(ownerIDs)).
		Order(
			rt.Col(spec.ownerIDColumn).Asc(),
			rt.Col(common.ColPosition).Asc(),
			rkt.Col(common.ColPosition).Asc(),
			rkt.Col(common.ColID).Asc(),
		)

	if spec.ownerTable != "" && spec.ownerAlias != "" {
		ot := goqu.T(spec.ownerTable).As(spec.ownerAlias)
		ownerJoinColumn := firstNonEmpty(spec.ownerJoinColumn, spec.ownerIDColumn)
		ds = ds.Join(
			ot,
			goqu.On(ot.Col(ownerJoinColumn).Eq(rt.Col(spec.ownerIDColumn))),
		)
	}

	var err error
	for _, filterSpec := range spec.filterSpecs {
		ds, err = auth.AddCorrelatedFilterQueryFromContext(ctx, ds, filterSpec.fragment, filterSpec.collector)
		if err != nil {
			return nil, fmt.Errorf("%s-ADDFILTER: %w", spec.errPrefix, err)
		}
	}

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("%s-BUILDQUERY: %w", spec.errPrefix, err)
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("%s-QUERYDB: %w", spec.errPrefix, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	type contextReferenceRow1ToMany struct {
		ownerID                sql.NullInt64
		referenceID            sql.NullInt64
		refType                sql.NullInt64
		keyID                  sql.NullInt64
		keyType                sql.NullInt64
		keyVal                 sql.NullString
		parentReferencePayload []byte
	}

	refBuilders := map[int64]*builder.ReferenceBuilder{}
	refByID := map[int64]types.IReference{}
	refIDsByOwner := map[int64][]int64{}
	seenRefByOwner := map[int64]map[int64]struct{}{}

	for rows.Next() {
		var row contextReferenceRow1ToMany
		if err := rows.Scan(
			&row.ownerID,
			&row.referenceID,
			&row.refType,
			&row.keyID,
			&row.keyType,
			&row.keyVal,
			&row.parentReferencePayload,
		); err != nil {
			return nil, fmt.Errorf("%s-SCANROW: %w", spec.errPrefix, err)
		}

		if !row.ownerID.Valid || !row.referenceID.Valid || !row.refType.Valid {
			continue
		}
		ownerID := row.ownerID.Int64
		referenceID := row.referenceID.Int64

		if _, ok := refBuilders[referenceID]; !ok {
			ref, rb := builder.NewReferenceBuilder(types.ReferenceTypes(row.refType.Int64), referenceID)
			parentReference, err := parseReferencePayload(row.parentReferencePayload)
			if err != nil {
				return nil, fmt.Errorf("%s-PARSEPARENTPAYLOAD: %w", spec.errPrefix, err)
			}
			ref.SetReferredSemanticID(parentReference)
			refBuilders[referenceID] = rb
			refByID[referenceID] = ref
		}

		if _, ok := seenRefByOwner[ownerID]; !ok {
			seenRefByOwner[ownerID] = map[int64]struct{}{}
		}
		if _, ok := seenRefByOwner[ownerID][referenceID]; !ok {
			seenRefByOwner[ownerID][referenceID] = struct{}{}
			refIDsByOwner[ownerID] = append(refIDsByOwner[ownerID], referenceID)
		}

		if row.keyID.Valid && row.keyType.Valid && row.keyVal.Valid {
			refBuilders[referenceID].CreateKey(
				row.keyID.Int64,
				types.KeyTypes(row.keyType.Int64),
				row.keyVal.String,
			)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s-ITERATEROWS: %w", spec.errPrefix, err)
	}

	for _, b := range refBuilders {
		b.BuildNestedStructure()
	}

	for ownerID, referenceIDs := range refIDsByOwner {
		refs := make([]types.IReference, 0, len(referenceIDs))
		for _, referenceID := range referenceIDs {
			if ref, ok := refByID[referenceID]; ok {
				refs = append(refs, ref)
			}
		}
		out[ownerID] = refs
	}

	for _, ownerID := range ownerIDs {
		if _, ok := out[ownerID]; !ok {
			out[ownerID] = nil
		}
	}

	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
