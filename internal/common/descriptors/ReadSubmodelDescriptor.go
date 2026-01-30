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

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
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
			Exp: submodelDescriptorAlias.Col(colAdminInfoID),
		},
		{
			Exp: submodelDescriptorAlias.Col(colDescriptionID),
		},
		{
			Exp: submodelDescriptorAlias.Col(colDisplayNameID),
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
		Where(goqu.L("? = ANY(?::bigint[])", submodelDescriptorAlias.Col(colAASDescriptorID), arr)).
		GroupBy(expressions[0], expressions[1])
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
	cteWhere := goqu.L(fmt.Sprintf("%s.%s = ANY(?::bigint[])", aliasSubmodelDescriptor, colAASDescriptorID), arr)
	ds, err = auth.ApplyResolvedFieldPathCTEs(ds, collector, cteWhere)
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
	defer func() {
		_ = rows.Close()
	}()

	perAAS := make(map[int64][]model.SubmodelDescriptorRow, len(uniqAASDesc))
	allSmdDescIDs := make([]int64, 0, 10000)
	semRefIDs := make([]int64, 0, 10000)
	adminInfoIDs := make([]int64, 0, 10000)
	descIDs := make([]int64, 0, 10000)
	displayNameIDs := make([]int64, 0, 10000)

	for rows.Next() {
		var r model.SubmodelDescriptorRow
		if err := rows.Scan(
			&r.AasDescID,
			&r.SmdDescID,
			&r.IDShort,
			&r.ID,
			&r.SemanticRefID,
			&r.AdminInfoID,
			&r.DescriptionID,
			&r.DisplayNameID,
		); err != nil {
			return nil, err
		}
		perAAS[r.AasDescID] = append(perAAS[r.AasDescID], r)
		allSmdDescIDs = append(allSmdDescIDs, r.SmdDescID)
		if r.SemanticRefID.Valid {
			semRefIDs = append(semRefIDs, r.SemanticRefID.Int64)
		}
		if r.AdminInfoID.Valid {
			adminInfoIDs = append(adminInfoIDs, r.AdminInfoID.Int64)
		}
		if r.DescriptionID.Valid {
			descIDs = append(descIDs, r.DescriptionID.Int64)
		}
		if r.DisplayNameID.Valid {
			displayNameIDs = append(displayNameIDs, r.DisplayNameID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(allSmdDescIDs) == 0 {
		for _, id := range uniqAASDesc {
			if _, ok := out[id]; !ok {
				out[id] = nil
			}
		}
		return out, nil
	}

	uniqSmdDescIDs := allSmdDescIDs
	uniqSemRefIDs := semRefIDs
	uniqAdminInfoIDs := adminInfoIDs
	uniqDescIDs := descIDs
	uniqDisplayNameIDs := displayNameIDs

	semRefByID := map[int64]types.IReference{}
	admByID := map[int64]types.IAdministrativeInformation{}
	nameByID := map[int64][]types.ILangStringNameType{}
	descByID := map[int64][]types.ILangStringTextType{}
	suppBySmdDesc := map[int64][]types.IReference{}
	endpointsByDesc := map[int64][]model.Endpoint{}
	extensionsByDesc := map[int64][]types.Extension{}

	if allowParallel {
		g, gctx := errgroup.WithContext(ctx)

		if len(uniqSemRefIDs) > 0 {
			ids := uniqSemRefIDs
			GoAssign(g, func() (map[int64]types.IReference, error) {
				return GetReferencesByIDsBatch(db, ids)
			}, &semRefByID)
		}

		if len(uniqAdminInfoIDs) > 0 {
			ids := uniqAdminInfoIDs
			GoAssign(g, func() (map[int64]types.IAdministrativeInformation, error) {
				return ReadAdministrativeInformationByIDs(gctx, db, tblSubmodelDescriptor, ids)
			}, &admByID)
		}

		if len(uniqDisplayNameIDs) > 0 {
			ids := uniqDisplayNameIDs
			GoAssign(g, func() (map[int64][]types.ILangStringNameType, error) {
				return GetLangStringNameTypesByIDs(db, ids)
			}, &nameByID)
		}

		if len(uniqDescIDs) > 0 {
			ids := uniqDescIDs
			GoAssign(g, func() (map[int64][]types.ILangStringTextType, error) {
				return GetLangStringTextTypesByIDs(db, ids)
			}, &descByID)
		}

		if len(uniqSmdDescIDs) > 0 {
			smdIDs := uniqSmdDescIDs

			GoAssign(g, func() (map[int64][]types.IReference, error) {
				return readEntityReferences1ToMany(
					gctx, db, smdIDs,
					tblSubmodelDescriptorSuppSemantic,
					colDescriptorID,
					colReferenceID,
				)
			}, &suppBySmdDesc)

			// Endpoints
			GoAssign(g, func() (map[int64][]model.Endpoint, error) {
				return ReadEndpointsByDescriptorIDs(gctx, db, smdIDs, false)
			}, &endpointsByDesc)

			// Extensions
			GoAssign(g, func() (map[int64][]types.Extension, error) {
				return ReadExtensionsByDescriptorIDs(gctx, db, smdIDs)
			}, &extensionsByDesc)
		}

		if err := g.Wait(); err != nil {
			return nil, err
		}
	} else {
		var err error
		if len(uniqSemRefIDs) > 0 {
			semRefByID, err = GetReferencesByIDsBatch(db, uniqSemRefIDs)
			if err != nil {
				return nil, err
			}
		}
		if len(uniqAdminInfoIDs) > 0 {
			admByID, err = ReadAdministrativeInformationByIDs(ctx, db, tblSubmodelDescriptor, uniqAdminInfoIDs)
			if err != nil {
				return nil, err
			}
		}
		if len(uniqDisplayNameIDs) > 0 {
			nameByID, err = GetLangStringNameTypesByIDs(db, uniqDisplayNameIDs)
			if err != nil {
				return nil, err
			}
		}
		if len(uniqDescIDs) > 0 {
			descByID, err = GetLangStringTextTypesByIDs(db, uniqDescIDs)
			if err != nil {
				return nil, err
			}
		}
		if len(uniqSmdDescIDs) > 0 {
			suppBySmdDesc, err = readEntityReferences1ToMany(
				ctx, db, uniqSmdDescIDs,
				tblSubmodelDescriptorSuppSemantic,
				colDescriptorID,
				colReferenceID,
			)
			if err != nil {
				return nil, err
			}
			endpointsByDesc, err = ReadEndpointsByDescriptorIDs(ctx, db, uniqSmdDescIDs, false)
			if err != nil {
				return nil, err
			}
			extensionsByDesc, err = ReadExtensionsByDescriptorIDs(ctx, db, uniqSmdDescIDs)
			if err != nil {
				return nil, err
			}
		}
	}

	// Assemble
	for aasID, rowsForAAS := range perAAS {
		for _, r := range rowsForAAS {
			var semanticRef types.IReference
			if r.SemanticRefID.Valid {
				semanticRef = semRefByID[r.SemanticRefID.Int64]
			}
			var adminInfo types.IAdministrativeInformation
			if r.AdminInfoID.Valid {
				adminInfo = admByID[r.AdminInfoID.Int64]
			}

			var displayName []types.ILangStringNameType
			if r.DisplayNameID.Valid {
				displayName = nameByID[r.DisplayNameID.Int64]
			}
			var description []types.ILangStringTextType
			if r.DescriptionID.Valid {
				description = descByID[r.DescriptionID.Int64]
			}

			out[aasID] = append(out[aasID], model.SubmodelDescriptor{
				IdShort:                r.IDShort.String,
				Id:                     r.ID.String,
				SemanticId:             semanticRef,
				Administration:         adminInfo,
				DisplayName:            displayName,
				Description:            description,
				Endpoints:              endpointsByDesc[r.SmdDescID],
				Extensions:             extensionsByDesc[r.SmdDescID],
				SupplementalSemanticId: suppBySmdDesc[r.SmdDescID],
			})
		}
	}

	for _, id := range uniqAASDesc {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	return out, nil
}
