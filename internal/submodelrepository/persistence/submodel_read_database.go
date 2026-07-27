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
	"fmt"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelqueries "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/queries"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
	"golang.org/x/sync/errgroup"
)

// GetSubmodelByID retrieves a submodel by identifier and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelByID(ctx context.Context, submodelIdentifier string, level string, metadataOnly bool, includeBlobValue bool) (types.ISubmodel, error) {
	eg := errgroup.Group{}
	var submodels []types.ISubmodel
	eg.Go(func() error {
		var err error
		submodels, _, err = s.GetSubmodels(ctx, 0, "", submodelIdentifier, "", time.Time{}, time.Time{})
		if err != nil {
			return err
		}
		if len(submodels) == 0 {
			return common.NewErrNotFound(submodelIdentifier)
		}
		if len(submodels) > 1 {
			return fmt.Errorf("multiple submodels found with identifier '%s'", submodelIdentifier)
		}
		return nil
	})
	submodelElements := make([]types.ISubmodelElement, 0)
	if !metadataOnly {
		eg.Go(func() error {
			unlimited := -1
			smes, _, err := s.GetSubmodelElements(ctx, submodelIdentifier, &unlimited, "", includeBlobValue, level)
			if err != nil {
				return err
			}
			submodelElements = smes
			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		return nil, err
	}
	if len(submodels) == 0 {
		return nil, common.NewErrNotFound(submodelIdentifier)
	}
	if submodels[0] == nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMBYID-NILSUBMODEL Loaded submodel is nil")
	}

	submodels[0].SetSubmodelElements(submodelElements)

	return submodels[0], nil
}

// GetSubmodels retrieves submodels and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodels(ctx context.Context, limit int32, cursor string, submodelIdentifier string, semanticID string, createdFrom time.Time, updatedFrom time.Time) ([]types.ISubmodel, string, error) {
	return s.getSubmodelsWithOptionalFilters(ctx, limit, cursor, submodelIdentifier, "", semanticID, createdFrom, updatedFrom)
}

// GetSubmodelsByListFilters retrieves submodels using public list filters.
func (s *SubmodelDatabase) GetSubmodelsByListFilters(ctx context.Context, limit int32, cursor string, idShort string, semanticID string, createdFrom time.Time, updatedFrom time.Time) ([]types.ISubmodel, string, error) {
	return s.getSubmodelsWithOptionalFilters(ctx, limit, cursor, "", idShort, semanticID, createdFrom, updatedFrom)
}

// GetSubmodelReferences retrieves references and applies optional ABAC formula filters from ctx.
func (s *SubmodelDatabase) GetSubmodelReferences(ctx context.Context, limit int32, cursor string, idShort string, semanticID string) ([]types.IReference, string, error) {
	submodels, nextCursor, err := s.getSubmodelsWithOptionalFilters(ctx, limit, cursor, "", idShort, semanticID, time.Time{}, time.Time{})
	if err != nil {
		return nil, "", err
	}

	references := make([]types.IReference, 0, len(submodels))
	for _, submodel := range submodels {
		if submodel == nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMREF-NILSUBMODEL loaded submodel is nil")
		}

		reference, referenceErr := buildSubmodelModelReference(submodel.ID())
		if referenceErr != nil {
			return nil, "", referenceErr
		}

		references = append(references, reference)
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

func (s *SubmodelDatabase) getSubmodelByIDInTransaction(ctx context.Context, tx *sql.Tx, submodelIdentifier string, level string, metadataOnly bool) (types.ISubmodel, error) {
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
	submodelElements, _, err := submodelelements.GetSubmodelElementsBySubmodelIDTx(ctx, tx, submodelIdentifier, &unlimited, "", true, level)
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

	query, args, err := selectDS.ToSQL()
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
	return submodel, nil
}

// GetSubmodelByIDAndDate returns the Submodel version valid at the requested instant.
func (s *SubmodelDatabase) GetSubmodelByIDAndDate(ctx context.Context, submodelIdentifier string, at time.Time) (types.ISubmodel, error) {
	snapshot, err := history.SnapshotByDate(ctx, s.db, history.TableSubmodel, submodelIdentifier, at)
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
	var limitFilter *int32

	if limit == 0 {
		limit = 100
	}

	if limit > 0 {
		limitFilter = &limit
	}

	var cursorFilter *string
	if cursor != "" {
		cursorExists, cursorErr := s.submodelCursorExists(ctx, cursor)
		if cursorErr != nil {
			return nil, "", cursorErr
		}
		if !cursorExists {
			return []types.ISubmodel{}, "", nil
		}
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
	maskedColumns := []auth.MaskedInnerColumnSpec{
		{Fragment: "$sm#idShort", FlagAlias: "flag_idshort", RawAlias: "c1"},
		{Fragment: "$sm#semanticId", FlagAlias: "flag_semanticid", RawAlias: "raw_semantic_id_payload"},
	}
	maskRuntime, maskRuntimeErr := auth.BuildSharedFragmentMaskRuntime(ctx, collector, maskedColumns)
	if maskRuntimeErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMS-MASKRUNTIME " + maskRuntimeErr.Error())
	}
	maskedExpressions, maskedExprErr := maskRuntime.MaskedInnerAliasExprs(dataAlias, maskedColumns)
	if maskedExprErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMS-MASKEXPR " + maskedExprErr.Error())
	}

	filterSupplementalSemanticIDs := hasFragmentFilterPrefix(ctx, "$sm#supplementalSemanticIds")
	additionalProjections := maskRuntime.Projections()
	if filterSupplementalSemanticIDs {
		additionalProjections = append(additionalProjections, goqu.I("submodel.id").As("supplemental_owner_id"))
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
	query, args, err := submodelqueries.BuildSubmodelListSQLWithSupplementalOwnerID(
		selectDS,
		dataAlias,
		maskedExpressions,
		filterSupplementalSemanticIDs,
	)
	if err != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMS-BUILDSQL " + err.Error())
	}

	var identifier, rawIDShort, category, descriptionJsonString, displayNameJsonString, administrativeInformationJsonString, embeddedDataSpecificationJsonString, supplementalSemanticIDsJsonString, extensionsJsonString, qualifiersJsonString, semanticIDJSONString sql.NullString
	var kind sql.NullInt64

	rows, err := s.db.QueryContext(ctx, query, args...)
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
	supplementalOwnerIDs := make([]int64, 0)
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
		var supplementalOwnerID int64
		if filterSupplementalSemanticIDs {
			scanTargets = append(scanTargets, &supplementalOwnerID)
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return nil, "", err
		}

		if pageLimit > 0 && len(submodels) == pageLimit {
			nextCursor = identifier.String
			break
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
		if filterSupplementalSemanticIDs {
			supplementalOwnerIDs = append(supplementalOwnerIDs, supplementalOwnerID)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	if filterSupplementalSemanticIDs {
		filteredReferences, readErr := descriptors.ReadSubmodelSupplementalSemanticReferencesBySubmodelIDs(
			ctx,
			s.db,
			supplementalOwnerIDs,
		)
		if readErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMS-READSUPPSEM " + readErr.Error())
		}
		for index, ownerID := range supplementalOwnerIDs {
			submodels[index].SetSupplementalSemanticIDs(filteredReferences[ownerID])
		}
	}

	return submodels, nextCursor, nil
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

func (s *SubmodelDatabase) submodelCursorExists(ctx context.Context, cursor string) (bool, error) {
	query, args, buildErr := submodelqueries.BuildSubmodelCursorExistsSQL(cursor)
	if buildErr != nil {
		return false, common.NewInternalServerError("SMREPO-CHECKSMCURSOR-BUILDSQL " + buildErr.Error())
	}

	var one int
	if queryErr := s.db.QueryRowContext(ctx, query, args...).Scan(&one); queryErr != nil {
		if errors.Is(queryErr, sql.ErrNoRows) {
			return false, nil
		}
		return false, common.NewInternalServerError("SMREPO-CHECKSMCURSOR-EXECSQL " + queryErr.Error())
	}
	return true, nil
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
