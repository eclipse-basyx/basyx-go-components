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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelqueries "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/queries"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
)

type submodelPageElementsError struct {
	err error
}

func (e submodelPageElementsError) Error() string {
	return e.err.Error()
}

func (e submodelPageElementsError) Unwrap() error {
	return e.err
}

// IsSubmodelPageElementsError reports whether a compound page read failed
// while loading SME trees rather than while selecting Submodels.
func IsSubmodelPageElementsError(err error) bool {
	var target submodelPageElementsError
	return errors.As(err, &target)
}

// GetSubmodelByID retrieves a submodel by identifier and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelByID(ctx context.Context, submodelIdentifier string, level string, metadataOnly bool, includeBlobValue bool) (types.ISubmodel, error) {
	var submodel types.ISubmodel
	err := common.ExecuteInReadTransaction(
		ctx,
		s.readDB(ctx),
		"SMREPO-GETSMBYID-STARTTX",
		"SMREPO-GETSMBYID-COMMIT",
		func(tx *sql.Tx) error {
			var txErr error
			submodel, txErr = s.getSubmodelByIDInTransaction(ctx, tx, submodelIdentifier, level, metadataOnly, includeBlobValue)
			return txErr
		},
	)
	if err != nil {
		return nil, err
	}
	if submodel == nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMBYID-NILSUBMODEL Loaded submodel is nil")
	}
	return submodel, nil
}

// GetSubmodels retrieves submodels and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodels(ctx context.Context, limit int32, cursor string, submodelIdentifier string, semanticID string, createdFrom time.Time, updatedFrom time.Time) ([]types.ISubmodel, string, error) {
	return s.getSubmodelsWithOptionalFilters(ctx, limit, cursor, submodelIdentifier, "", semanticID, createdFrom, updatedFrom)
}

// GetSubmodelsByListFilters retrieves submodels using public list filters.
func (s *SubmodelDatabase) GetSubmodelsByListFilters(ctx context.Context, limit int32, cursor string, idShort string, semanticID string, createdFrom time.Time, updatedFrom time.Time) ([]types.ISubmodel, string, error) {
	return s.getSubmodelsWithOptionalFilters(ctx, limit, cursor, "", idShort, semanticID, createdFrom, updatedFrom)
}

// GetSubmodelsWithElementsByListFilters reads a Submodel page and its element
// trees from one stable snapshot without per-Submodel database calls.
func (s *SubmodelDatabase) GetSubmodelsWithElementsByListFilters(
	ctx context.Context,
	limit int32,
	cursor string,
	idShort string,
	semanticID string,
	createdFrom time.Time,
	updatedFrom time.Time,
	level string,
	includeBlobValue bool,
) ([]types.ISubmodel, string, error) {
	var result []types.ISubmodel
	var nextCursor string
	err := common.ExecuteInReadTransaction(
		ctx,
		s.readDB(ctx),
		"SMREPO-GETSMSWITHELEMS-STARTTX",
		"SMREPO-GETSMSWITHELEMS-COMMIT",
		func(tx *sql.Tx) error {
			databaseIDs := make([]int64, 0)
			var txErr error
			result, nextCursor, txErr = s.getSubmodelsWithOptionalFiltersWithQueryer(
				ctx,
				tx,
				limit,
				cursor,
				"",
				idShort,
				semanticID,
				createdFrom,
				updatedFrom,
				&databaseIDs,
			)
			if txErr != nil {
				return txErr
			}

			elements, txErr := submodelelements.GetSubmodelElementsBySubmodelDatabaseIDsTx(
				ctx,
				tx,
				databaseIDs,
				includeBlobValue,
				level,
			)
			if txErr != nil {
				return submodelPageElementsError{err: txErr}
			}
			if len(result) != len(databaseIDs) {
				return common.NewInternalServerError("SMREPO-GETSMSWITHELEMS-PAGEMISMATCH Submodel and database id counts differ")
			}
			for index, submodel := range result {
				submodel.SetSubmodelElements(elements[databaseIDs[index]])
			}
			return nil
		},
	)
	return result, nextCursor, err
}

// GetSubmodelReferences retrieves references and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelReferences(ctx context.Context, limit int32, cursor string, idShort string, semanticID string) ([]types.IReference, string, error) {
	selectDS := submodelqueries.SelectSubmodelIdentifierDataset(idShort, limit, cursor)
	selectDS = submodelqueries.ApplySubmodelSemanticIDFilter(selectDS, semanticID)
	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter != nil && queryFilter.Formula != nil {
		collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
		if err != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMREFS-BADCOLLECTOR " + err.Error())
		}
		selectDS, err = auth.AddFormulaQueryFromContext(ctx, selectDS, collector)
		if err != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMREFS-ABACFORMULA " + err.Error())
		}
	}

	query, args, err := selectDS.Prepared(true).ToSQL()
	if err != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMREFS-BUILDQ " + err.Error())
	}
	rows, err := s.readDB(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMREFS-EXECQ " + err.Error())
	}
	defer func() { _ = rows.Close() }()

	pageLimit := limit
	if pageLimit == 0 {
		pageLimit = 100
	}
	references := make([]types.IReference, 0)
	nextCursor := ""
	for rows.Next() {
		var identifier string
		if err := rows.Scan(&identifier); err != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMREFS-SCANROW " + err.Error())
		}
		if pageLimit > 0 && len(references) == int(pageLimit) {
			nextCursor = identifier
			continue
		}
		reference, referenceErr := buildSubmodelModelReference(identifier)
		if referenceErr != nil {
			return nil, "", referenceErr
		}
		references = append(references, reference)
	}
	if err := rows.Err(); err != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMREFS-ROWSERR " + err.Error())
	}

	return references, nextCursor, nil
}

// GetSubmodelReference retrieves the model reference for a single submodel
// while preserving ABAC visibility checks from ctx.
func (s *SubmodelDatabase) GetSubmodelReference(ctx context.Context, submodelIdentifier string) (types.IReference, error) {
	if submodelIdentifier == "" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMREFONE-EMPTYIDENTIFIER submodel identifier is required")
	}

	submodels, _, err := s.GetSubmodels(ctx, 1, "", submodelIdentifier, "", time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}

	if len(submodels) == 0 {
		return nil, common.NewErrNotFound(submodelIdentifier)
	}

	if submodels[0] == nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMREFONE-NILSUBMODEL loaded submodel is nil")
	}

	return buildSubmodelModelReference(submodels[0].ID())
}

// GetAllSubmodelPathsPage reads a flattened path page across visible
// Submodels in one statement and one stable snapshot.
func (s *SubmodelDatabase) GetAllSubmodelPathsPage(
	ctx context.Context,
	limit int,
	submodelCursor string,
	pathCursor string,
	idShort string,
	semanticID string,
	level string,
) (submodelelements.SubmodelPathPage, error) {
	if level != "" && level != "core" && level != "deep" {
		return submodelelements.SubmodelPathPage{}, common.NewErrBadRequest("SMREPO-GETSMEPATHSPAGE-BADLEVEL level must be one of '', 'core', or 'deep'")
	}
	dialect := goqu.Dialect(common.Dialect)
	visibleSubmodels := dialect.From("submodel").
		Join(goqu.T("submodel_payload"), goqu.On(goqu.Ex{"submodel.id": goqu.I("submodel_payload.submodel_id")})).
		Select(
			goqu.I("submodel.id").As("submodel_id"),
			goqu.I("submodel.submodel_identifier").As("submodel_identifier"),
		)
	if idShort != "" {
		visibleSubmodels = visibleSubmodels.Where(goqu.Ex{"submodel.id_short": idShort})
	}
	visibleSubmodels = submodelqueries.ApplySubmodelSemanticIDFilter(visibleSubmodels, semanticID)
	if submodelCursor != "" {
		cursorExists := dialect.From(goqu.T("submodel").As("cursor_submodel")).
			Select(goqu.L("1")).
			Where(goqu.I("cursor_submodel.submodel_identifier").Eq(submodelCursor))
		visibleSubmodels = visibleSubmodels.Where(
			goqu.Func("EXISTS", cursorExists),
			goqu.I("submodel.submodel_identifier").Gte(submodelCursor),
		)
	}

	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter != nil && queryFilter.Formula != nil {
		collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
		if err != nil {
			return submodelelements.SubmodelPathPage{}, common.NewInternalServerError("SMREPO-GETALLSMPATH-BADCOLLECTOR " + err.Error())
		}
		visibleSubmodels, err = auth.AddFormulaQueryFromContext(ctx, visibleSubmodels, collector)
		if err != nil {
			return submodelelements.SubmodelPathPage{}, common.NewInternalServerError("SMREPO-GETALLSMPATH-ABACFORMULA " + err.Error())
		}
	}

	var result submodelelements.SubmodelPathPage
	err := common.ExecuteInReadTransaction(
		ctx,
		s.readDB(ctx),
		"SMREPO-GETALLSMPATH-STARTTX",
		"SMREPO-GETALLSMPATH-COMMIT",
		func(tx *sql.Tx) error {
			var txErr error
			result, txErr = submodelelements.GetAllSubmodelElementPathsPageTx(
				ctx,
				tx,
				visibleSubmodels,
				limit,
				submodelCursor,
				pathCursor,
				level,
			)
			return txErr
		},
	)
	return result, err
}

func (s *SubmodelDatabase) getSubmodelByIDInTransaction(ctx context.Context, tx *sql.Tx, submodelIdentifier string, level string, metadataOnly bool, includeBlobValue bool) (types.ISubmodel, error) {
	if tx == nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMBYIDTX-NILTX transaction must not be nil")
	}

	submodel, err := s.getSubmodelMetadataByIDInTransaction(ctx, tx, submodelIdentifier)
	if err != nil {
		return nil, err
	}

	if metadataOnly {
		return submodel, nil
	}

	unlimited := -1
	submodelElements, _, err := submodelelements.GetSubmodelElementsBySubmodelIDTx(ctx, tx, submodelIdentifier, &unlimited, "", includeBlobValue, level)
	if err != nil {
		return nil, err
	}
	submodel.SetSubmodelElements(submodelElements)
	return submodel, nil
}

func (s *SubmodelDatabase) getSubmodelMetadataByIDInTransaction(ctx context.Context, tx *sql.Tx, submodelIdentifier string) (types.ISubmodel, error) {
	limit := int32(1)
	selectDS, err := submodelqueries.SelectSubmodelDataset(&submodelIdentifier, nil, &limit, nil, time.Time{}, time.Time{}, nil)
	if err != nil {
		return nil, err
	}

	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter != nil && queryFilter.Formula != nil {
		collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
		if collectorErr != nil {
			return nil, common.NewInternalServerError("SMREPO-GETSMBYIDTX-BADCOLLECTOR " + collectorErr.Error())
		}
		selectDS, err = auth.AddFormulaQueryFromContext(ctx, selectDS, collector)
		if err != nil {
			return nil, common.NewInternalServerError("SMREPO-GETSMBYIDTX-ABACFORMULA " + err.Error())
		}
	}

	query, args, err := selectDS.Prepared(true).ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMBYIDTX-BUILDSQL " + err.Error())
	}

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMBYIDTX-EXECSQL " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	if !rows.Next() {
		if rowsErr := rows.Err(); rowsErr != nil {
			return nil, common.NewInternalServerError("SMREPO-GETSMBYIDTX-ROWS " + rowsErr.Error())
		}
		return nil, common.NewErrNotFound(submodelIdentifier)
	}

	submodel, scanErr := scanSubmodelMetadataRow(rows)
	if scanErr != nil {
		return nil, scanErr
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMBYIDTX-CLOSEROWS " + closeErr.Error())
	}
	return submodel, nil
}

// GetSubmodelByIDAndDate returns the Submodel version valid at the requested instant.
func (s *SubmodelDatabase) GetSubmodelByIDAndDate(ctx context.Context, submodelIdentifier string, at time.Time) (types.ISubmodel, error) {
	snapshot, err := history.SnapshotByDate(ctx, s.readDB(ctx), history.TableSubmodel, submodelIdentifier, at)
	if err != nil {
		return nil, err
	}
	submodel, err := jsonization.SubmodelFromJsonable(snapshot)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-HISTORY-FROMJSON " + err.Error())
	}
	return submodel, nil
}

// RecordCurrentSubmodelVersion appends a full snapshot of the current Submodel state.
func (s *SubmodelDatabase) RecordCurrentSubmodelVersion(ctx context.Context, submodelIdentifier string, changeType string) error {
	return common.ExecuteInTransaction(s.db, "SMREPO-HISTORY-STARTTX", "SMREPO-HISTORY-COMMIT", func(tx *sql.Tx) error {
		previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelIdentifier)
		if err != nil {
			return err
		}
		return s.appendCurrentSubmodelHistoryTx(ctx, tx, submodelIdentifier, previousSnapshot, changeType)
	})
}

// QuerySubmodels applies query conditions to the context and reuses the regular submodel listing logic.
func (s *SubmodelDatabase) QuerySubmodels(ctx context.Context, limit int32, cursor string, queryWrapper *grammar.QueryWrapper, _ bool) ([]types.ISubmodel, string, error) {
	if queryWrapper == nil || queryWrapper.Query.Condition == nil {
		return nil, "", common.NewErrBadRequest("SMREPO-QUERYSMS-INVALIDQUERY query condition is required")
	}

	ctx = auth.MergeQueryFilter(ctx, queryWrapper.Query)
	return s.GetSubmodels(ctx, limit, cursor, "", "", time.Time{}, time.Time{})
}

//nolint:revive // cyclomatic complexity is acceptable for this function due to query/filter orchestration in one flow
func (s *SubmodelDatabase) getSubmodelsWithOptionalFilters(ctx context.Context, limit int32, cursor string, submodelIdentifier string, idShort string, semanticID string, createdFrom time.Time, updatedFrom time.Time) ([]types.ISubmodel, string, error) {
	if !hasFragmentFilterPrefix(ctx, "$sm#semanticId.keys") &&
		!hasFragmentFilterPrefix(ctx, "$sm#supplementalSemanticIds") {
		return s.getSubmodelsWithOptionalFiltersWithQueryer(ctx, s.readDB(ctx), limit, cursor, submodelIdentifier, idShort, semanticID, createdFrom, updatedFrom, nil)
	}

	var result []types.ISubmodel
	var nextCursor string
	err := common.ExecuteInReadTransaction(ctx, s.readDB(ctx), "SMREPO-GETSMS-STARTTX", "SMREPO-GETSMS-COMMIT", func(tx *sql.Tx) error {
		var txErr error
		result, nextCursor, txErr = s.getSubmodelsWithOptionalFiltersWithQueryer(ctx, tx, limit, cursor, submodelIdentifier, idShort, semanticID, createdFrom, updatedFrom, nil)
		return txErr
	})
	return result, nextCursor, err
}

//nolint:revive // cyclomatic complexity is acceptable for this function due to query/filter orchestration in one flow
func (s *SubmodelDatabase) getSubmodelsWithOptionalFiltersWithQueryer(ctx context.Context, db descriptors.DBQueryer, limit int32, cursor string, submodelIdentifier string, idShort string, semanticID string, createdFrom time.Time, updatedFrom time.Time, databaseIDs *[]int64) ([]types.ISubmodel, string, error) {
	var limitFilter *int32

	if limit == 0 {
		limit = 100
	}

	if limit > 0 {
		limitFilter = &limit
	}

	var cursorFilter *string
	if cursor != "" {
		cursorFilter = &cursor
	}

	var submodelIdentifierFilter *string
	if submodelIdentifier != "" {
		submodelIdentifierFilter = &submodelIdentifier
	}
	var idShortFilter *string
	if idShort != "" {
		idShortFilter = &idShort
	}
	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
	if collectorErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMS-BADCOLLECTOR " + collectorErr.Error())
	}

	const dataAlias = "submodel_list_data"
	const semanticIDFragment grammar.FragmentStringPattern = "$sm#semanticId"
	filterSemanticIDKeys := hasFragmentFilterPrefix(ctx, "$sm#semanticId.keys")
	filterSupplementalSemanticIDs := hasFragmentFilterPrefix(ctx, "$sm#supplementalSemanticIds")
	filterReferenceRows := filterSemanticIDKeys || filterSupplementalSemanticIDs
	maskedColumns := []auth.MaskedInnerColumnSpec{
		{Fragment: "$sm#idShort", FlagAlias: "flag_idshort", RawAlias: "c1"},
		{Fragment: semanticIDFragment, FlagAlias: "flag_semanticid", RawAlias: "raw_semantic_id_payload"},
	}
	maskRuntime, maskRuntimeErr := auth.BuildSharedFragmentMaskRuntime(ctx, collector, maskedColumns)
	if maskRuntimeErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMS-MASKRUNTIME " + maskRuntimeErr.Error())
	}
	maskedExpressions, maskedExprErr := maskRuntime.MaskedInnerAliasExprs(dataAlias, maskedColumns)
	if maskedExprErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMS-MASKEXPR " + maskedExprErr.Error())
	}

	additionalProjections := maskRuntime.Projections()
	if filterReferenceRows {
		additionalProjections = append(additionalProjections, goqu.I("submodel.id").As("reference_owner_id"))
	}
	if databaseIDs != nil {
		additionalProjections = append(additionalProjections, goqu.I("submodel.id").As("page_submodel_id"))
		*databaseIDs = (*databaseIDs)[:0]
	}
	selectDS, err := submodelqueries.SelectSubmodelDataset(submodelIdentifierFilter, idShortFilter, limitFilter, cursorFilter, createdFrom, updatedFrom, additionalProjections)
	if err != nil {
		return nil, "", err
	}
	selectDS = submodelqueries.ApplySubmodelSemanticIDFilter(selectDS, semanticID)

	queryFilter := auth.GetQueryFilter(ctx)
	hasFormulaInContext := queryFilter != nil && queryFilter.Formula != nil
	if hasFormulaInContext {
		collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
		if collectorErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMS-BADCOLLECTOR " + collectorErr.Error())
		}
		selectDS, err = auth.AddFormulaQueryFromContext(ctx, selectDS, collector)
		if err != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMS-ABACFORMULA " + err.Error())
		}
	}
	outerProjections := make([]exp.Expression, 0, 1)
	if filterSemanticIDKeys {
		semanticIDFlagAlias, flagErr := maskRuntime.FlagAlias(semanticIDFragment)
		if flagErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMS-SEMANTICFLAG " + flagErr.Error())
		}
		outerProjections = append(
			outerProjections,
			goqu.I(dataAlias+"."+semanticIDFlagAlias).As("semantic_id_visible"),
		)
	}
	if databaseIDs != nil {
		outerProjections = append(outerProjections, goqu.I(dataAlias+".page_submodel_id"))
	}
	query, args, err := submodelqueries.BuildSubmodelListSQLWithReferenceOwnerID(
		selectDS,
		dataAlias,
		maskedExpressions,
		filterReferenceRows,
		outerProjections...,
	)
	if err != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMS-BUILDSQL " + err.Error())
	}

	var identifier, rawIDShort, category, descriptionJsonString, displayNameJsonString, administrativeInformationJsonString, embeddedDataSpecificationJsonString, supplementalSemanticIDsJsonString, extensionsJsonString, qualifiersJsonString, semanticIDJSONString sql.NullString
	var kind sql.NullInt64

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		_ = rows.Close()
	}()

	pageLimit := 0
	if limitFilter != nil {
		pageLimit = int(*limitFilter)
	}

	submodels := make([]types.ISubmodel, 0)
	referenceStates := make([]submodelReferenceReadState, 0)
	nextCursor := ""
	for rows.Next() {
		scanTargets := []any{
			&identifier,
			&rawIDShort,
			&category,
			&kind,
			&descriptionJsonString,
			&displayNameJsonString,
			&administrativeInformationJsonString,
			&embeddedDataSpecificationJsonString,
			&supplementalSemanticIDsJsonString,
			&extensionsJsonString,
			&qualifiersJsonString,
			&semanticIDJSONString,
		}
		var referenceState submodelReferenceReadState
		if filterReferenceRows {
			scanTargets = append(scanTargets, &referenceState.ownerID)
		}
		if filterSemanticIDKeys {
			scanTargets = append(scanTargets, &referenceState.semanticIDVisible)
		}
		var submodelDatabaseID int64
		if databaseIDs != nil {
			scanTargets = append(scanTargets, &submodelDatabaseID)
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return nil, "", err
		}

		if pageLimit > 0 && len(submodels) == pageLimit {
			nextCursor = identifier.String
			continue
		}

		var submodel types.ISubmodel
		submodel = types.NewSubmodel(identifier.String)
		if rawIDShort.Valid {
			idShortValue := rawIDShort.String
			submodel.SetIDShort(&idShortValue)
		}
		if category.Valid {
			categoryValue := category.String
			submodel.SetCategory(&categoryValue)
		}
		if kind.Valid {
			modellingKind := types.ModellingKind(kind.Int64)
			submodel.SetKind(&modellingKind)
		}

		submodel, err = jsonPayloadToInstance(descriptionJsonString, displayNameJsonString, administrativeInformationJsonString, embeddedDataSpecificationJsonString, supplementalSemanticIDsJsonString, extensionsJsonString, qualifiersJsonString, submodel)
		if err != nil {
			return nil, "", err
		}

		if semanticIDJSONString.Valid {
			semanticID, parseSemanticErr := common.ParseReferenceJSON([]byte(semanticIDJSONString.String))
			if parseSemanticErr != nil {
				return nil, "", parseSemanticErr
			}
			if semanticID != nil {
				submodel.SetSemanticID(semanticID)
			}
		}

		submodels = append(submodels, submodel)
		if databaseIDs != nil {
			*databaseIDs = append(*databaseIDs, submodelDatabaseID)
		}
		if filterReferenceRows {
			referenceStates = append(referenceStates, referenceState)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	if filterSemanticIDKeys {
		if applyErr := applyFilteredSubmodelSemanticIDs(ctx, db, submodels, referenceStates); applyErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMS-READSEMANTICID " + applyErr.Error())
		}
	}

	if filterSupplementalSemanticIDs {
		referenceOwnerIDs := submodelReferenceOwnerIDs(referenceStates)
		filteredReferences, readErr := descriptors.ReadSubmodelSupplementalSemanticReferencesBySubmodelIDs(
			ctx,
			db,
			referenceOwnerIDs,
		)
		if readErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMS-READSUPPSEM " + readErr.Error())
		}
		for index, ownerID := range referenceOwnerIDs {
			submodels[index].SetSupplementalSemanticIDs(filteredReferences[ownerID])
		}
	}

	return submodels, nextCursor, nil
}

type submodelReferenceReadState struct {
	ownerID           int64
	semanticIDVisible bool
}

func applyFilteredSubmodelSemanticIDs(
	ctx context.Context,
	db descriptors.DBQueryer,
	submodels []types.ISubmodel,
	referenceStates []submodelReferenceReadState,
) error {
	if len(submodels) != len(referenceStates) {
		return common.NewInternalServerError("SMREPO-APPLYFILTEREDSEM-STATEMISMATCH submodel and reference state counts differ")
	}
	visibleOwnerIDs := make([]int64, 0, len(referenceStates))
	for _, state := range referenceStates {
		if state.semanticIDVisible {
			visibleOwnerIDs = append(visibleOwnerIDs, state.ownerID)
		}
	}
	filteredReferences, err := descriptors.ReadSubmodelSemanticReferencesBySubmodelIDs(ctx, db, visibleOwnerIDs)
	if err != nil {
		return err
	}
	for index, state := range referenceStates {
		if !state.semanticIDVisible {
			submodels[index].SetSemanticID(nil)
			continue
		}
		submodels[index].SetSemanticID(filteredReferences[state.ownerID])
	}
	return nil
}

func submodelReferenceOwnerIDs(referenceStates []submodelReferenceReadState) []int64 {
	ownerIDs := make([]int64, 0, len(referenceStates))
	for _, state := range referenceStates {
		ownerIDs = append(ownerIDs, state.ownerID)
	}
	return ownerIDs
}

func hasFragmentFilterPrefix(ctx context.Context, prefix string) bool {
	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter == nil {
		return false
	}
	for fragment := range queryFilter.Filters {
		if strings.HasPrefix(string(fragment), prefix) {
			return true
		}
	}
	return false
}

func buildSubmodelModelReference(submodelIdentifier string) (types.IReference, error) {
	if submodelIdentifier == "" {
		return nil, common.NewErrBadRequest("SMREPO-BUILDSMREF-INVALIDIDENTIFIER submodel identifier is required")
	}

	key := types.NewKey(types.KeyTypesSubmodel, submodelIdentifier)

	reference := types.NewReference(types.ReferenceTypesModelReference, []types.IKey{key})

	return reference, nil
}

func scanSubmodelMetadataRow(rows *sql.Rows) (types.ISubmodel, error) {
	var identifier, idShort, category, descriptionJSON, displayNameJSON, administrationJSON, edsJSON, supplementalSemanticIDsJSON, extensionsJSON, qualifiersJSON, semanticIDJSON, sortIdentifier sql.NullString
	var kind sql.NullInt64

	if err := rows.Scan(&identifier, &idShort, &category, &kind, &descriptionJSON, &displayNameJSON, &administrationJSON, &edsJSON, &supplementalSemanticIDsJSON, &extensionsJSON, &qualifiersJSON, &semanticIDJSON, &sortIdentifier); err != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMBYIDTX-SCAN " + err.Error())
	}

	var submodel types.ISubmodel
	submodel = types.NewSubmodel(identifier.String)
	idShortValue := idShort.String
	submodel.SetIDShort(&idShortValue)
	if category.Valid {
		categoryValue := category.String
		submodel.SetCategory(&categoryValue)
	}
	if kind.Valid {
		modellingKind := types.ModellingKind(kind.Int64)
		submodel.SetKind(&modellingKind)
	}

	var err error
	submodel, err = jsonPayloadToInstance(descriptionJSON, displayNameJSON, administrationJSON, edsJSON, supplementalSemanticIDsJSON, extensionsJSON, qualifiersJSON, submodel)
	if err != nil {
		return nil, err
	}

	if semanticIDJSON.Valid {
		semanticID, parseSemanticErr := common.ParseReferenceJSON([]byte(semanticIDJSON.String))
		if parseSemanticErr != nil {
			return nil, parseSemanticErr
		}
		if semanticID != nil {
			submodel.SetSemanticID(semanticID)
		}
	}
	return submodel, nil
}
