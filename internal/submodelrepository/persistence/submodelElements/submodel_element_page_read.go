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

package submodelelements

import (
	"context"
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

const getAllSubmodelRootLimit = 100

// SubmodelPathPage contains a flattened path page and the unencoded composite
// cursor boundary for the next request.
type SubmodelPathPage struct {
	Paths              []string
	NextSubmodelCursor string
	NextPathCursor     string
}

// GetSubmodelElementsBySubmodelDatabaseIDsTx loads the element trees for an
// ordered Submodel page without resolving identifiers or acquiring another
// connection.
func GetSubmodelElementsBySubmodelDatabaseIDsTx(
	ctx context.Context,
	tx *sql.Tx,
	submodelDatabaseIDs []int64,
	includeBlobValue bool,
	level string,
) (map[int64][]types.ISubmodelElement, error) {
	if tx == nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-NILTX transaction must not be nil")
	}
	if level != "" && level != "core" && level != "deep" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMEPAGE-BADLEVEL level must be core or deep")
	}

	result := make(map[int64][]types.ISubmodelElement, len(submodelDatabaseIDs))
	for _, databaseID := range submodelDatabaseIDs {
		result[databaseID] = []types.ISubmodelElement{}
	}
	if len(submodelDatabaseIDs) == 0 {
		return result, nil
	}

	query, args, err := buildSubmodelElementPageQuery(ctx, submodelDatabaseIDs, includeBlobValue, level)
	if err != nil {
		return nil, err
	}
	owners, loadedRows, err := readSubmodelElementPageRows(ctx, tx, query, args)
	if err != nil {
		return nil, err
	}
	if len(loadedRows) == 0 {
		return result, nil
	}
	forest, err := buildSubmodelElementForestFromRows(ctx, tx, loadedRows)
	if err != nil {
		return nil, err
	}
	for index, item := range loadedRows {
		if item.row.ParentID.Valid || !item.row.DbID.Valid {
			continue
		}
		if element, exists := forest[item.row.DbID.Int64]; exists {
			result[owners[index]] = append(result[owners[index]], element)
		}
	}
	return result, nil
}

// GetAllSubmodelElementPathsPageTx reads one flattened path page across all
// visible Submodels. visibleSubmodels must project submodel_id and
// submodel_identifier and must already contain Submodel filtering and ABAC.
func GetAllSubmodelElementPathsPageTx(
	ctx context.Context,
	tx *sql.Tx,
	visibleSubmodels *goqu.SelectDataset,
	limit int,
	submodelCursor string,
	pathCursor string,
	level string,
) (SubmodelPathPage, error) {
	if tx == nil {
		return SubmodelPathPage{}, common.NewInternalServerError("SMREPO-GETALLSMPATH-NILTX transaction must not be nil")
	}
	if visibleSubmodels == nil {
		return SubmodelPathPage{}, common.NewInternalServerError("SMREPO-GETALLSMPATH-NILQUERY visible Submodel query must not be nil")
	}
	if limit <= 0 {
		return SubmodelPathPage{}, common.NewErrBadRequest("SMREPO-GETALLSMPATH-BADLIMIT limit must be greater than zero")
	}

	query, args, err := buildAllSubmodelElementPathsPageQuery(
		ctx,
		visibleSubmodels,
		limit,
		submodelCursor,
		pathCursor,
		level,
	)
	if err != nil {
		return SubmodelPathPage{}, err
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return SubmodelPathPage{}, common.NewInternalServerError("SMREPO-GETALLSMPATH-EXECQ " + err.Error())
	}
	defer func() { _ = rows.Close() }()

	type pathRow struct {
		submodelIdentifier string
		path               string
		id                 int64
	}
	pathRows := make([]pathRow, 0, limit+1)
	for rows.Next() {
		var item pathRow
		if err := rows.Scan(&item.submodelIdentifier, &item.path, &item.id); err != nil {
			return SubmodelPathPage{}, common.NewInternalServerError("SMREPO-GETALLSMPATH-SCANROW " + err.Error())
		}
		pathRows = append(pathRows, item)
	}
	if err := rows.Err(); err != nil {
		return SubmodelPathPage{}, common.NewInternalServerError("SMREPO-GETALLSMPATH-ROWSERR " + err.Error())
	}

	page := SubmodelPathPage{Paths: make([]string, 0, min(limit, len(pathRows)))}
	returnedCount := min(limit, len(pathRows))
	for index := 0; index < returnedCount; index++ {
		page.Paths = append(page.Paths, pathRows[index].path)
	}
	if len(pathRows) <= limit {
		return page, nil
	}

	last := pathRows[limit-1]
	extra := pathRows[limit]
	if last.submodelIdentifier == extra.submodelIdentifier {
		page.NextSubmodelCursor = last.submodelIdentifier
		page.NextPathCursor = formatRootCursor(last.path, last.id)
	} else {
		page.NextSubmodelCursor = extra.submodelIdentifier
	}
	return page, nil
}

func buildAllSubmodelElementPathsPageQuery(
	ctx context.Context,
	visibleSubmodels *goqu.SelectDataset,
	limit int,
	submodelCursor string,
	pathCursor string,
	level string,
) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	authorizedPaths, recursiveVisible, err := buildAuthorizedSubmodelPaths(ctx, dialect, level)
	if err != nil {
		return "", nil, err
	}

	query := dialect.From(goqu.T("authorized_submodel_paths").As("authorized_path")).
		With("visible_submodels", visibleSubmodels).
		Select(
			goqu.I("authorized_path.submodel_identifier"),
			goqu.I("authorized_path.idshort_path"),
			goqu.I("authorized_path.sme_id"),
		)
	if recursiveVisible != nil {
		query = query.WithRecursive("visible_path_smes(sme_id,submodel_id)", recursiveVisible)
	}
	query = query.With("authorized_submodel_paths", authorizedPaths)

	if submodelCursor != "" && pathCursor != "" {
		cursorPath, cursorID, hasCursorID := parseRootCursor(pathCursor)
		cursorExists := dialect.From("authorized_submodel_paths").
			Select(goqu.L("1")).
			Where(
				goqu.I("submodel_identifier").Eq(submodelCursor),
				goqu.I("idshort_path").Eq(cursorPath),
			)
		if hasCursorID {
			cursorExists = cursorExists.Where(goqu.I("sme_id").Eq(cursorID))
		}

		var pathBoundary goqu.Expression = goqu.I("authorized_path.idshort_path").Gt(cursorPath)
		if hasCursorID {
			pathBoundary = goqu.Or(
				goqu.I("authorized_path.idshort_path").Gt(cursorPath),
				goqu.And(
					goqu.I("authorized_path.idshort_path").Eq(cursorPath),
					goqu.I("authorized_path.sme_id").Gt(cursorID),
				),
			)
		}
		query = query.Where(goqu.Or(
			goqu.I("authorized_path.submodel_identifier").Gt(submodelCursor),
			goqu.And(
				goqu.I("authorized_path.submodel_identifier").Eq(submodelCursor),
				goqu.Func("EXISTS", cursorExists),
				pathBoundary,
			),
		))
	}

	query = query.
		Order(
			goqu.I("authorized_path.submodel_identifier").Asc(),
			goqu.I("authorized_path.idshort_path").Asc(),
			goqu.I("authorized_path.sme_id").Asc(),
		).
		//nolint:gosec // limit is validated to be positive
		Limit(uint(limit) + 1)

	sqlQuery, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return "", nil, common.NewInternalServerError("SMREPO-GETALLSMPATH-BUILDQ " + err.Error())
	}
	return sqlQuery, args, nil
}

func buildAuthorizedSubmodelPaths(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	level string,
) (*goqu.SelectDataset, *goqu.SelectDataset, error) {
	query := dialect.From(goqu.T("visible_submodels").As("visible_submodel")).
		Join(
			goqu.T("submodel_element").As("sme"),
			goqu.On(goqu.I("sme.submodel_id").Eq(goqu.I("visible_submodel.submodel_id"))),
		).
		Select(
			goqu.I("visible_submodel.submodel_identifier"),
			goqu.I("sme.idshort_path"),
			goqu.I("sme.id").As("sme_id"),
		)

	var recursiveVisible *goqu.SelectDataset
	var err error
	if level == "core" {
		query = query.Where(goqu.I("sme.parent_sme_id").IsNull())
		query, err = addSMERowFilterQueries(ctx, query)
	} else {
		recursiveVisible, err = buildVisiblePathSMEs(ctx, dialect)
		query = query.Join(
			goqu.T("visible_path_smes").As("visible_path_sme"),
			goqu.On(goqu.I("visible_path_sme.sme_id").Eq(goqu.I("sme.id"))),
		)
	}
	if err != nil {
		return nil, nil, common.NewInternalServerError("SMREPO-GETALLSMPATH-ROWFILTER " + err.Error())
	}

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if err != nil {
		return nil, nil, common.NewInternalServerError("SMREPO-GETALLSMPATH-BADCOLLECTOR " + err.Error())
	}
	shouldEnforce, err := auth.ShouldEnforceFormula(ctx)
	if err != nil {
		return nil, nil, common.NewInternalServerError("SMREPO-GETALLSMPATH-SHOULDENFORCE " + err.Error())
	}
	if shouldEnforce {
		query, err = auth.AddFormulaQueryFromContext(ctx, query, collector)
		if err != nil {
			return nil, nil, common.NewInternalServerError("SMREPO-GETALLSMPATH-ABACFORMULA " + err.Error())
		}
	}
	return query, recursiveVisible, nil
}

func buildVisiblePathSMEs(ctx context.Context, dialect goqu.DialectWrapper) (*goqu.SelectDataset, error) {
	filterCtx, fragments, err := normalizeSMERowFilters(ctx)
	if err != nil {
		return nil, err
	}

	rootAlias := "visible_path_root"
	rootQuery := dialect.From(goqu.T("visible_submodels").As("visible_submodel")).
		Join(
			goqu.T("submodel_element").As(rootAlias),
			goqu.On(goqu.I(rootAlias+".submodel_id").Eq(goqu.I("visible_submodel.submodel_id"))),
		).
		Select(goqu.I(rootAlias+".id"), goqu.I(rootAlias+".submodel_id")).
		Where(goqu.I(rootAlias + ".parent_sme_id").IsNull())
	if len(fragments) > 0 {
		rootQuery, err = addNormalizedSMERowFilterQueries(filterCtx, rootQuery, fragments, rootAlias)
		if err != nil {
			return nil, err
		}
	}

	childAlias := "visible_path_child"
	parentAlias := "visible_path_parent"
	childQuery := dialect.From(goqu.T("submodel_element").As(childAlias)).
		Join(
			goqu.T("visible_path_smes").As(parentAlias),
			goqu.On(
				goqu.I(childAlias+".parent_sme_id").Eq(goqu.I(parentAlias+".sme_id")),
				goqu.I(childAlias+".submodel_id").Eq(goqu.I(parentAlias+".submodel_id")),
			),
		).
		Select(goqu.I(childAlias+".id"), goqu.I(childAlias+".submodel_id"))
	if len(fragments) > 0 {
		childQuery, err = addNormalizedSMERowFilterQueries(filterCtx, childQuery, fragments, childAlias)
		if err != nil {
			return nil, err
		}
	}
	return rootQuery.UnionAll(childQuery), nil
}

func buildSubmodelElementPageQuery(
	ctx context.Context,
	submodelDatabaseIDs []int64,
	includeBlobValue bool,
	level string,
) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	visibleRoots, err := buildVisibleSubmodelRoots(ctx, dialect, submodelDatabaseIDs)
	if err != nil {
		return "", nil, err
	}
	selectedRoots := dialect.From("visible_submodel_roots").
		Select("root_id", "submodel_id", "root_rank").
		Where(goqu.I("root_rank").Lte(getAllSubmodelRootLimit))
	selectedElements := buildSelectedSubmodelElements(dialect, level)
	selectedElementValues := buildSubmodelElementPageValueDataset(dialect, includeBlobValue)

	collector, err := grammar.NewResolvedFieldPathCollectorForSMERow("sme")
	if err != nil {
		return "", nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-BADCOLLECTOR " + err.Error())
	}
	maskRuntime, maskGroups, err := buildSMEMaskRuntime(ctx, collector)
	if err != nil {
		return "", nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-MASKRUNTIME " + err.Error())
	}

	const dataAlias = "submodel_element_page_data"
	idShortVisible, err := buildSharedMaskVisibilityExpr(dataAlias, maskRuntime, maskGroups.idShort)
	if err != nil {
		return "", nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-IDSHORTMASK " + err.Error())
	}
	semanticVisible, err := buildSharedMaskVisibilityExpr(dataAlias, maskRuntime, maskGroups.semantic)
	if err != nil {
		return "", nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-SEMANTICMASK " + err.Error())
	}
	valueVisible, err := buildSharedMaskVisibilityExpr(dataAlias, maskRuntime, maskGroups.value)
	if err != nil {
		return "", nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-VALUEMASK " + err.Error())
	}

	inner := buildSubmodelElementPageInnerQuery(dialect, submodelDatabaseIDs, maskRuntime)
	inner, err = addSMERowFilterQueries(ctx, inner)
	if err != nil {
		return "", nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-ROWFILTER " + err.Error())
	}

	query := dialect.From(inner.As(dataAlias)).
		With("visible_submodel_roots", visibleRoots).
		With("selected_submodel_roots", selectedRoots).
		With("selected_submodel_elements", selectedElements).
		With(submodelElementPageValuesCTE, selectedElementValues).
		Select(buildSubmodelElementPageOuterProjections(dataAlias, idShortVisible, semanticVisible, valueVisible)...).
		Order(
			goqu.I(dataAlias+".sort_submodel_order").Asc(),
			goqu.I(dataAlias+".sort_root_rank").Asc(),
			goqu.I(dataAlias+".c_position").Asc(),
			goqu.I(dataAlias+".c_idshort_path").Asc(),
			goqu.I(dataAlias+".sort_id").Asc(),
		)

	sqlQuery, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return "", nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-BUILDQ " + err.Error())
	}
	return sqlQuery, args, nil
}

func buildVisibleSubmodelRoots(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	submodelDatabaseIDs []int64,
) (*goqu.SelectDataset, error) {
	query := dialect.From(goqu.T("submodel_element").As("sme")).
		Select(
			goqu.I("sme.id").As("root_id"),
			goqu.I("sme.submodel_id").As("submodel_id"),
			goqu.ROW_NUMBER().Over(
				goqu.W().PartitionBy(goqu.I("sme.submodel_id")).OrderBy(
					goqu.I("sme.idshort_path").Asc(),
					goqu.I("sme.id").Asc(),
				),
			).As("root_rank"),
		).
		Where(
			common.PostgreSQLBigIntArrayContains(goqu.I("sme.submodel_id"), submodelDatabaseIDs),
			goqu.I("sme.parent_sme_id").IsNull(),
		)

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-ROOTCOLLECTOR " + err.Error())
	}
	shouldEnforce, err := auth.ShouldEnforceFormula(ctx)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-SHOULDENFORCE " + err.Error())
	}
	if shouldEnforce {
		query, err = auth.AddFormulaQueryFromContext(ctx, query, collector)
		if err != nil {
			return nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-ROOTFORMULA " + err.Error())
		}
	}
	query, err = addSMERowFilterQueries(ctx, query)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-ROOTFILTER " + err.Error())
	}
	return query, nil
}

func buildSelectedSubmodelElements(dialect goqu.DialectWrapper, level string) *goqu.SelectDataset {
	roots := dialect.From(goqu.T("selected_submodel_roots").As("selected_root")).
		Select(
			goqu.I("selected_root.root_id").As("element_id"),
			goqu.I("selected_root.submodel_id").As("submodel_id"),
			goqu.I("selected_root.root_rank").As("root_rank"),
		)

	joinColumn := "selected_sme.root_sme_id"
	if level == "core" {
		joinColumn = "selected_sme.parent_sme_id"
	}
	children := dialect.From(goqu.T("submodel_element").As("selected_sme")).
		Join(
			goqu.T("selected_submodel_roots").As("selected_root"),
			goqu.On(
				goqu.I("selected_sme.submodel_id").Eq(goqu.I("selected_root.submodel_id")),
				goqu.I(joinColumn).Eq(goqu.I("selected_root.root_id")),
			),
		).
		Select(
			goqu.I("selected_sme.id").As("element_id"),
			goqu.I("selected_root.submodel_id").As("submodel_id"),
			goqu.I("selected_root.root_rank").As("root_rank"),
		)
	if level != "core" {
		children = children.Where(goqu.I("selected_sme.id").Neq(goqu.I("selected_root.root_id")))
	}

	return roots.UnionAll(children)
}

func buildSubmodelElementPageInnerQuery(
	dialect goqu.DialectWrapper,
	submodelDatabaseIDs []int64,
	maskRuntime *auth.SharedFragmentMaskRuntime,
) *goqu.SelectDataset {
	value := goqu.T(submodelElementPageValuesCTE).As("sme_value")
	projections := []interface{}{
		goqu.I("selected_element.submodel_id").As("page_submodel_id"),
		common.PostgreSQLBigIntArrayPosition(submodelDatabaseIDs, goqu.I("selected_element.submodel_id")).As("sort_submodel_order"),
		goqu.I("selected_element.root_rank").As("sort_root_rank"),
		goqu.I("sme.id").As("c_id"),
		goqu.I("sme.parent_sme_id").As("c_parent_sme_id"),
		goqu.I("sme.root_sme_id").As("c_root_sme_id"),
		goqu.I("sme.id_short").As("c_id_short"),
		goqu.I("sme.idshort_path").As("c_idshort_path"),
		goqu.I("sme.category").As("c_category"),
		goqu.I("sme.model_type").As("c_model_type"),
		goqu.COALESCE(goqu.I("sme.position"), 0).As("c_position"),
		goqu.L("COALESCE(sme_p.embedded_data_specification_payload, '[]'::jsonb)").As("raw_embedded_data_specification_payload"),
		goqu.L("COALESCE(sme_p.supplemental_semantic_ids_payload, '[]'::jsonb)").As("raw_supplemental_semantic_ids_payload"),
		goqu.L("COALESCE(sme_p.extensions_payload, '[]'::jsonb)").As("raw_extensions_payload"),
		goqu.L("COALESCE(sme_p.displayname_payload, '[]'::jsonb)").As("raw_displayname_payload"),
		goqu.L("COALESCE(sme_p.description_payload, '[]'::jsonb)").As("raw_description_payload"),
		value.Col("value_payload").As("raw_value_payload"),
		goqu.L("'[]'::jsonb").As("raw_semantic_id_referred_payload"),
		goqu.L("'[]'::jsonb").As("raw_supplemental_semantic_ids_referred_payload"),
		goqu.L("COALESCE(sme_p.qualifiers_payload, '[]'::jsonb)").As("raw_qualifiers_payload"),
		goqu.L("COALESCE(sme_sem_payload.parent_reference_payload, '{}'::jsonb)").As("raw_semantic_payload"),
		goqu.I("sme.id").As("sort_id"),
	}
	projections = append(projections, maskRuntime.Projections()...)

	return dialect.From(goqu.T("submodel_element").As("sme")).
		Join(
			goqu.T("selected_submodel_elements").As("selected_element"),
			goqu.On(goqu.I("sme.id").Eq(goqu.I("selected_element.element_id"))),
		).
		LeftJoin(
			goqu.T("submodel_element_payload").As("sme_p"),
			goqu.On(goqu.I("sme.id").Eq(goqu.I("sme_p.submodel_element_id"))),
		).
		LeftJoin(
			goqu.T("submodel_element_semantic_id_reference_payload").As("sme_sem_payload"),
			goqu.On(goqu.I("sme_sem_payload.reference_id").Eq(goqu.I("sme.id"))),
		).
		LeftJoin(
			value,
			goqu.On(value.Col("element_id").Eq(goqu.I("sme.id"))),
		).
		Select(projections...)
}

func buildSubmodelElementPageOuterProjections(
	dataAlias string,
	idShortVisible goqu.Expression,
	semanticVisible goqu.Expression,
	valueVisible goqu.Expression,
) []interface{} {
	return []interface{}{
		goqu.I(dataAlias + ".page_submodel_id"),
		goqu.I(dataAlias + ".c_id"),
		goqu.I(dataAlias + ".c_parent_sme_id"),
		goqu.I(dataAlias + ".c_root_sme_id"),
		goqu.Case().When(idShortVisible, goqu.I(dataAlias+".c_id_short")).Else(nil),
		goqu.I(dataAlias + ".c_idshort_path"),
		goqu.I(dataAlias + ".c_category"),
		goqu.I(dataAlias + ".c_model_type"),
		goqu.I(dataAlias + ".c_position"),
		goqu.I(dataAlias + ".raw_embedded_data_specification_payload"),
		goqu.I(dataAlias + ".raw_supplemental_semantic_ids_payload"),
		goqu.I(dataAlias + ".raw_extensions_payload"),
		goqu.I(dataAlias + ".raw_displayname_payload"),
		goqu.I(dataAlias + ".raw_description_payload"),
		goqu.Case().When(valueVisible, goqu.I(dataAlias+".raw_value_payload")).Else(buildMaskedSMEValuePayloadExpr(dataAlias + ".raw_value_payload")),
		goqu.I(dataAlias + ".raw_semantic_id_referred_payload"),
		goqu.I(dataAlias + ".raw_supplemental_semantic_ids_referred_payload"),
		goqu.I(dataAlias + ".raw_qualifiers_payload"),
		goqu.Case().When(semanticVisible, goqu.I(dataAlias+".raw_semantic_payload")).Else(nil),
		goqu.Case().When(semanticVisible, goqu.L("TRUE")).Else(goqu.L("FALSE")),
		goqu.Case().When(valueVisible, goqu.L("TRUE")).Else(goqu.L("FALSE")),
	}
}

func readSubmodelElementPageRows(
	ctx context.Context,
	db DBQueryer,
	query string,
	args []any,
) ([]int64, []loadedSMERow, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-EXECQ " + err.Error())
	}
	defer func() { _ = rows.Close() }()

	owners := make([]int64, 0, 64)
	loadedRows := make([]loadedSMERow, 0, 64)
	for rows.Next() {
		var owner int64
		row, scanErr := scanLoadedSMERow(rows, &owner)
		if scanErr != nil {
			return nil, nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-SCANROW " + scanErr.Error())
		}
		owners = append(owners, owner)
		loadedRows = append(loadedRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-ROWSERR " + err.Error())
	}

	if hasSMESemanticIDKeyFilter(ctx) {
		if err := applyFilteredSMESemanticIDs(ctx, db, loadedRows); err != nil {
			return nil, nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-FILTERSEMANTIC " + err.Error())
		}
	}
	if hasSMESupplementalSemanticIDFilter(ctx) {
		if err := applyFilteredSMESupplementalSemanticIDs(ctx, db, loadedRows); err != nil {
			return nil, nil, common.NewInternalServerError("SMREPO-GETSMEPAGE-FILTERSUPPSEM " + err.Error())
		}
	}
	return owners, loadedRows, nil
}
