package submodelelements

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/aas-core-works/aas-core3.1-golang/stringification"
	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

// GetSubmodelElementByIDShortOrPath loads a submodel element by path and applies optional ABAC formula filters from ctx.
func GetSubmodelElementByIDShortOrPath(ctx context.Context, db *sql.DB, submodelID string, idShortOrPath string, level string) (types.ISubmodelElement, error) {
	if submodelID == "" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMEBYPATH-EMPTYSMID Submodel id must not be empty")
	}
	if idShortOrPath == "" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMEBYPATH-EMPTYPATH idShort or path must not be empty")
	}
	if level != "" && level != "core" && level != "deep" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMEBYPATH-BADLEVEL level must be one of '', 'core', or 'deep'")
	}

	submodelDatabaseID, submodelIDErr := persistenceutils.GetSubmodelDatabaseIDFromDB(db, submodelID)
	if submodelIDErr != nil {
		if errors.Is(submodelIDErr, sql.ErrNoRows) {
			return nil, common.NewErrNotFound(submodelID)
		}
		return nil, common.NewInternalServerError("SMREPO-GETSMEBYPATH-GETSMDATABASEID " + submodelIDErr.Error())
	}

	return getSubmodelElementByIDShortOrPathWithSubmodelDBID(ctx, db, submodelID, int64(submodelDatabaseID), idShortOrPath, level)
}

func getSubmodelElementByIDShortOrPathWithSubmodelDBID(ctx context.Context, db *sql.DB, submodelID string, submodelDatabaseID int64, idShortOrPath string, level string) (types.ISubmodelElement, error) {
	if formulaCheckErr := ensureSubmodelElementPathMatchesFormula(ctx, db, submodelID, submodelDatabaseID, idShortOrPath); formulaCheckErr != nil {
		return nil, formulaCheckErr
	}

	includeChildren := level != "core"
	parsedRows, readRowsErr := readSubmodelElementRowsByPath(ctx, db, submodelDatabaseID, idShortOrPath, includeChildren)

	if readRowsErr != nil {
		return nil, readRowsErr
	}
	if len(parsedRows) == 0 {
		return nil, common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortOrPath + "' not found in submodel '" + submodelID + "'")
	}

	rootElement, buildTreeErr := buildSubmodelElementTreeFromRows(db, parsedRows, submodelID, idShortOrPath)
	if buildTreeErr != nil {
		return nil, buildTreeErr
	}

	return rootElement, nil
}

// GetSubmodelElementPathsBySubmodelID returns submodel element paths directly from persisted idshort_path values.
func GetSubmodelElementPathsBySubmodelID(ctx context.Context, db *sql.DB, submodelID string, level string) ([]string, error) {
	if submodelID == "" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMEPATHS-EMPTYSMID Submodel id must not be empty")
	}
	if level != "" && level != "core" && level != "deep" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMEPATHS-BADLEVEL level must be one of '', 'core', or 'deep'")
	}

	submodelDatabaseID, submodelIDErr := persistenceutils.GetSubmodelDatabaseIDFromDB(db, submodelID)
	if submodelIDErr != nil {
		if errors.Is(submodelIDErr, sql.ErrNoRows) {
			return nil, common.NewErrNotFound(submodelID)
		}
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHS-GETSMDATABASEID " + submodelIDErr.Error())
	}

	dialect := goqu.Dialect("postgres")
	query := dialect.
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.idshort_path")).
		Where(goqu.I("sme.submodel_id").Eq(submodelDatabaseID))

	if level == "core" {
		query = query.Where(goqu.I("sme.parent_sme_id").IsNull())
	}

	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if collectorErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHS-BADCOLLECTOR " + collectorErr.Error())
	}

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHS-SHOULDENFORCE " + enforceErr.Error())
	}
	if shouldEnforceFormula {
		var addFormulaErr error
		query, addFormulaErr = auth.AddFormulaQueryFromContext(ctx, query, collector)
		if addFormulaErr != nil {
			return nil, common.NewInternalServerError("SMREPO-GETSMEPATHS-ABACFORMULA " + addFormulaErr.Error())
		}
	}

	query = query.Order(goqu.I("sme.idshort_path").Asc(), goqu.I("sme.id").Asc())

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHS-BUILDQ " + toSQLErr.Error())
	}

	rows, queryErr := db.Query(sqlQuery, args...)
	if queryErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHS-EXECQ " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	paths := make([]string, 0)
	for rows.Next() {
		var path string
		if scanErr := rows.Scan(&path); scanErr != nil {
			return nil, common.NewInternalServerError("SMREPO-GETSMEPATHS-SCANQ " + scanErr.Error())
		}
		paths = append(paths, path)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHS-ROWSERR " + rowsErr.Error())
	}

	return paths, nil
}

// GetSubmodelElementPathsPageBySubmodelID returns paged submodel element paths directly from persisted idshort_path values.
func GetSubmodelElementPathsPageBySubmodelID(ctx context.Context, db *sql.DB, submodelID string, limit *int, cursor string, level string) ([]string, string, error) {
	if submodelID == "" {
		return nil, "", common.NewErrBadRequest("SMREPO-GETSMEPATHSPAGE-EMPTYSMID Submodel id must not be empty")
	}
	if level != "" && level != "core" && level != "deep" {
		return nil, "", common.NewErrBadRequest("SMREPO-GETSMEPATHSPAGE-BADLEVEL level must be one of '', 'core', or 'deep'")
	}
	if limit != nil && *limit < 0 {
		return nil, "", common.NewErrBadRequest("SMREPO-GETSMEPATHSPAGE-BADLIMIT limit must be >= 0")
	}

	pageLimit := 100
	if limit != nil {
		pageLimit = *limit
	}
	if pageLimit == 0 {
		return []string{}, "", nil
	}

	submodelDatabaseID, submodelIDErr := persistenceutils.GetSubmodelDatabaseIDFromDB(db, submodelID)
	if submodelIDErr != nil {
		if errors.Is(submodelIDErr, sql.ErrNoRows) {
			return nil, "", common.NewErrNotFound(submodelID)
		}
		return nil, "", common.NewInternalServerError("SMREPO-GETSMEPATHSPAGE-GETSMDATABASEID " + submodelIDErr.Error())
	}

	dialect := goqu.Dialect("postgres")
	query := dialect.
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.idshort_path"), goqu.I("sme.id")).
		Where(goqu.I("sme.submodel_id").Eq(submodelDatabaseID))

	if level == "core" {
		query = query.Where(goqu.I("sme.parent_sme_id").IsNull())
	}
	if cursor != "" {
		cursorPath, cursorID, hasCursorID := parseRootCursor(cursor)
		if hasCursorID {
			query = query.Where(
				goqu.Or(
					goqu.I("sme.idshort_path").Gt(cursorPath),
					goqu.And(
						goqu.I("sme.idshort_path").Eq(cursorPath),
						goqu.I("sme.id").Gt(cursorID),
					),
				),
			)
		} else {
			query = query.Where(goqu.I("sme.idshort_path").Gt(cursorPath))
		}
	}

	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if collectorErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMEPATHSPAGE-BADCOLLECTOR " + collectorErr.Error())
	}

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMEPATHSPAGE-SHOULDENFORCE " + enforceErr.Error())
	}
	if shouldEnforceFormula {
		var addFormulaErr error
		query, addFormulaErr = auth.AddFormulaQueryFromContext(ctx, query, collector)
		if addFormulaErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMEPATHSPAGE-ABACFORMULA " + addFormulaErr.Error())
		}
	}

	query = query.
		Order(goqu.I("sme.idshort_path").Asc(), goqu.I("sme.id").Asc()).
		//nolint:gosec // pageLimit is validated to be >= 0
		Limit(uint(pageLimit) + 1)

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMEPATHSPAGE-BUILDQ " + toSQLErr.Error())
	}

	rows, queryErr := db.Query(sqlQuery, args...)
	if queryErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMEPATHSPAGE-EXECQ " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	pathRows := make([]rootElementCursorRow, 0, pageLimit+1)
	for rows.Next() {
		var path string
		var id int64
		if scanErr := rows.Scan(&path, &id); scanErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMEPATHSPAGE-SCANQ " + scanErr.Error())
		}
		pathRows = append(pathRows, rootElementCursorRow{id: id, path: path})
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMEPATHSPAGE-ROWSERR " + rowsErr.Error())
	}

	nextCursor := ""
	if len(pathRows) > pageLimit {
		lastPath := pathRows[pageLimit-1]
		nextCursor = formatRootCursor(lastPath.path, lastPath.id)
		pathRows = pathRows[:pageLimit]
	}

	paths := make([]string, 0, len(pathRows))
	for _, pathRow := range pathRows {
		paths = append(paths, pathRow.path)
	}

	return paths, nextCursor, nil
}

// GetSubmodelElementPathsByPath returns persisted idshort_path values for a submodel element path and, for deep level, its descendants.
func GetSubmodelElementPathsByPath(ctx context.Context, db *sql.DB, submodelID string, idShortPath string, level string) ([]string, error) {
	if submodelID == "" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMEPATHSBYPATH-EMPTYSMID Submodel id must not be empty")
	}
	if idShortPath == "" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMEPATHSBYPATH-EMPTYPATH idShort path must not be empty")
	}
	if level != "" && level != "core" && level != "deep" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMEPATHSBYPATH-BADLEVEL level must be one of '', 'core', or 'deep'")
	}

	submodelDatabaseID, submodelIDErr := persistenceutils.GetSubmodelDatabaseIDFromDB(db, submodelID)
	if submodelIDErr != nil {
		if errors.Is(submodelIDErr, sql.ErrNoRows) {
			return nil, common.NewErrNotFound(submodelID)
		}
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHSBYPATH-GETSMDATABASEID " + submodelIDErr.Error())
	}

	if formulaCheckErr := ensureSubmodelElementPathMatchesFormula(ctx, db, submodelID, int64(submodelDatabaseID), idShortPath); formulaCheckErr != nil {
		return nil, formulaCheckErr
	}

	dialect := goqu.Dialect("postgres")
	query := dialect.
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.idshort_path")).
		Where(goqu.I("sme.submodel_id").Eq(submodelDatabaseID))

	if level == "core" {
		query = query.Where(goqu.I("sme.idshort_path").Eq(idShortPath))
	} else {
		descendantPattern := "^" + regexp.QuoteMeta(idShortPath) + `(\.|\[)`
		query = query.Where(
			goqu.Or(
				goqu.I("sme.idshort_path").Eq(idShortPath),
				goqu.L(`"sme"."idshort_path" ~ ?`, descendantPattern),
			),
		)
	}

	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if collectorErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHSBYPATH-BADCOLLECTOR " + collectorErr.Error())
	}

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHSBYPATH-SHOULDENFORCE " + enforceErr.Error())
	}
	if shouldEnforceFormula {
		var addFormulaErr error
		query, addFormulaErr = auth.AddFormulaQueryFromContext(ctx, query, collector)
		if addFormulaErr != nil {
			return nil, common.NewInternalServerError("SMREPO-GETSMEPATHSBYPATH-ABACFORMULA " + addFormulaErr.Error())
		}
	}

	query = query.Order(goqu.I("sme.idshort_path").Asc(), goqu.I("sme.id").Asc())

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHSBYPATH-BUILDQ " + toSQLErr.Error())
	}

	rows, queryErr := db.Query(sqlQuery, args...)
	if queryErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHSBYPATH-EXECQ " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	paths := make([]string, 0)
	for rows.Next() {
		var path string
		if scanErr := rows.Scan(&path); scanErr != nil {
			return nil, common.NewInternalServerError("SMREPO-GETSMEPATHSBYPATH-SCANQ " + scanErr.Error())
		}
		paths = append(paths, path)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEPATHSBYPATH-ROWSERR " + rowsErr.Error())
	}

	if len(paths) == 0 {
		return nil, common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortPath + "' not found in submodel '" + submodelID + "'")
	}

	return paths, nil
}

func ensureSubmodelElementPathMatchesFormula(ctx context.Context, db *sql.DB, submodelID string, submodelDatabaseID int64, idShortOrPath string) error {
	dialect := goqu.Dialect("postgres")
	query := dialect.
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.id")).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			goqu.I("sme.idshort_path").Eq(idShortOrPath),
		).
		Limit(1)

	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if collectorErr != nil {
		return common.NewInternalServerError("SMREPO-GETSMEBYPATH-BADCOLLECTOR " + collectorErr.Error())
	}

	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return common.NewInternalServerError("SMREPO-GETSMEBYPATH-SHOULDENFORCE " + enforceErr.Error())
	}
	if shouldEnforceFormula {
		var addFormulaErr error
		query, addFormulaErr = auth.AddFormulaQueryFromContext(ctx, query, collector)
		if addFormulaErr != nil {
			return common.NewInternalServerError("SMREPO-GETSMEBYPATH-ABACFORMULA " + addFormulaErr.Error())
		}
	}

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return common.NewInternalServerError("SMREPO-GETSMEBYPATH-BUILDQ " + toSQLErr.Error())
	}

	var elementID int64
	scanErr := db.QueryRow(sqlQuery, args...).Scan(&elementID)
	if scanErr == nil {
		return nil
	}
	if errors.Is(scanErr, sql.ErrNoRows) {
		return common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortOrPath + "' not found in submodel '" + submodelID + "'")
	}
	return common.NewInternalServerError("SMREPO-GETSMEBYPATH-EXECQ " + scanErr.Error())
}

// GetSubmodelElementsBySubmodelID loads top-level submodel elements and reconstructs
// each complete subtree in original hierarchy.
func GetSubmodelElementsBySubmodelID(ctx context.Context, db *sql.DB, submodelID string, limit *int, cursor string, level string) ([]types.ISubmodelElement, string, error) {
	if submodelID == "" {
		return nil, "", common.NewErrBadRequest("SMREPO-GETSMES-EMPTYSMID Submodel id must not be empty")
	}
	if limit != nil {
		if *limit < -1 {
			return nil, "", common.NewErrBadRequest("SMREPO-GETSMES-BADLIMIT limit must be >= -1")
		}
	}
	if limit == nil {
		limit = new(int)
		*limit = 100
	}
	submodelDatabaseID, submodelIDErr := persistenceutils.GetSubmodelDatabaseIDFromDB(db, submodelID)
	if submodelIDErr != nil {
		if errors.Is(submodelIDErr, sql.ErrNoRows) {
			return nil, "", common.NewErrNotFound(submodelID)
		}
		return nil, "", common.NewInternalServerError("SMREPO-GETSMES-GETSMDATABASEID " + submodelIDErr.Error())
	}

	rootElements, nextCursor, rootPathErr := getRootElementPage(ctx, db, int64(submodelDatabaseID), limit, cursor)
	if rootPathErr != nil {
		return nil, "", rootPathErr
	}
	if len(rootElements) == 0 {
		return []types.ISubmodelElement{}, nextCursor, nil
	}

	rootIDs := make([]int64, 0, len(rootElements))
	for _, rootElement := range rootElements {
		rootIDs = append(rootIDs, rootElement.id)
	}

	includeChildren := level != "core"
	isGetSubmodelElements := true
	parsedRows, readRowsErr := readSubmodelElementRowsByRootIDs(ctx, db, int64(submodelDatabaseID), rootIDs, includeChildren, isGetSubmodelElements)
	if readRowsErr != nil {
		return nil, "", readRowsErr
	}

	forest, buildForestErr := buildSubmodelElementForestFromRows(db, parsedRows)
	if buildForestErr != nil {
		return nil, "", buildForestErr
	}

	result := make([]types.ISubmodelElement, 0, len(rootElements))
	for _, rootElement := range rootElements {
		element, exists := forest[rootElement.id]
		if !exists {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMES-BUILDFOREST-MISSINGROOT Missing root element for path '" + rootElement.path + "'")
		}
		result = append(result, element)
	}

	return result, nextCursor, nil
}

// GetSubmodelElementReferencesBySubmodelID retrieves references for top-level submodel elements of a submodel with optional pagination.
func GetSubmodelElementReferencesBySubmodelID(ctx context.Context, db *sql.DB, submodelID string, limit *int, cursor string) ([]types.IReference, string, error) {
	if submodelID == "" {
		return nil, "", common.NewErrBadRequest("SMREPO-GETSMEREFS-EMPTYSMID Submodel id must not be empty")
	}
	if limit != nil {
		if *limit < -1 {
			return nil, "", common.NewErrBadRequest("SMREPO-GETSMEREFS-BADLIMIT limit must be >= -1")
		}
	}
	if limit == nil {
		limit = new(int)
		*limit = 100
	}

	submodelDatabaseID, submodelIDErr := persistenceutils.GetSubmodelDatabaseIDFromDB(db, submodelID)
	if submodelIDErr != nil {
		if errors.Is(submodelIDErr, sql.ErrNoRows) {
			return nil, "", common.NewErrNotFound(submodelID)
		}
		return nil, "", common.NewInternalServerError("SMREPO-GETSMEREFS-GETSMDATABASEID " + submodelIDErr.Error())
	}

	rootElements, nextCursor, rootPathErr := getRootElementPage(ctx, db, int64(submodelDatabaseID), limit, cursor)
	if rootPathErr != nil {
		return nil, "", rootPathErr
	}
	if len(rootElements) == 0 {
		return []types.IReference{}, nextCursor, nil
	}

	rootIDs := make([]int64, 0, len(rootElements))
	for _, rootElement := range rootElements {
		rootIDs = append(rootIDs, rootElement.id)
	}

	dialect := goqu.Dialect("postgres")
	modelTypesQuery, modelTypesArgs, modelTypesSQLErr := dialect.
		From(goqu.T("submodel_element").As("sme")).
		Select(
			goqu.I("sme.id"),
			goqu.I("sme.model_type"),
		).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			goqu.I("sme.id").In(rootIDs),
		).
		ToSQL()
	if modelTypesSQLErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMEREFS-BUILDMODELTYPESQ " + modelTypesSQLErr.Error())
	}

	rows, modelTypesQueryErr := db.Query(modelTypesQuery, modelTypesArgs...)
	if modelTypesQueryErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMEREFS-EXECMODELTYPESQ " + modelTypesQueryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	modelTypesByID := make(map[int64]types.ModelType, len(rootElements))
	for rows.Next() {
		var elementID int64
		var modelTypeInt int64
		if scanErr := rows.Scan(&elementID, &modelTypeInt); scanErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMEREFS-SCANMODELTYPESQ " + scanErr.Error())
		}
		modelTypesByID[elementID] = types.ModelType(modelTypeInt)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETSMEREFS-ROWSERRMODELTYPESQ " + rowsErr.Error())
	}

	references := make([]types.IReference, 0, len(rootElements))
	for _, rootElement := range rootElements {
		modelType, modelTypeExists := modelTypesByID[rootElement.id]
		if !modelTypeExists {
			return nil, "", common.NewInternalServerError("SMREPO-GETSMEREFS-MISSINGMODELTYPE Missing model type for root element id")
		}

		reference, referenceErr := buildSubmodelElementReference(submodelID, modelType, rootElement.path)
		if referenceErr != nil {
			return nil, "", referenceErr
		}

		references = append(references, reference)
	}

	return references, nextCursor, nil
}

func buildSubmodelElementReference(submodelID string, modelType types.ModelType, idShortPath string) (types.IReference, error) {
	if submodelID == "" || idShortPath == "" {
		return nil, common.NewErrBadRequest("SMREPO-BUILDSMEREF-INVALIDPARAMS Invalid reference parameters")
	}

	modelTypeLiteral, ok := stringification.ModelTypeToString(modelType)
	if !ok {
		return nil, common.NewInternalServerError("SMREPO-BUILDSMEREF-MODELTYPE Unknown model type for reference")
	}
	modelTypeKeyType, ok := stringification.KeyTypesFromString(modelTypeLiteral)
	if !ok {
		return nil, common.NewInternalServerError("SMREPO-BUILDSMEREF-KEYTYPE Unknown key type for model type")
	}

	pathSegments, parsePathErr := parseReferencePathSegments(idShortPath)
	if parsePathErr != nil {
		return nil, parsePathErr
	}

	keys := make([]types.IKey, 0, len(pathSegments)+1)

	firstKey := types.Key{}
	firstKey.SetType(types.KeyTypesSubmodel)
	firstKey.SetValue(submodelID)
	keys = append(keys, &firstKey)

	for i, segment := range pathSegments {
		key := types.Key{}
		isLast := i == len(pathSegments)-1

		switch {
		case segment.isIndex:
			key.SetType(types.KeyTypesSubmodelElementList)
		case isLast:
			key.SetType(modelTypeKeyType)
		default:
			key.SetType(types.KeyTypesSubmodelElementCollection)
		}

		key.SetValue(segment.value)
		keys = append(keys, &key)
	}

	reference := types.Reference{}
	reference.SetType(types.ReferenceTypesModelReference)
	reference.SetKeys(keys)

	return &reference, nil
}

type referencePathSegment struct {
	value   string
	isIndex bool
}

func parseReferencePathSegments(idShortPath string) ([]referencePathSegment, error) {
	segments := make([]referencePathSegment, 0, 4)
	current := strings.Builder{}

	flushCurrent := func() {
		if current.Len() == 0 {
			return
		}
		segments = append(segments, referencePathSegment{value: current.String()})
		current.Reset()
	}

	for i := 0; i < len(idShortPath); i++ {
		switch idShortPath[i] {
		case '.':
			flushCurrent()
		case '[':
			flushCurrent()
			endIndex := strings.IndexByte(idShortPath[i+1:], ']')
			if endIndex < 0 {
				return nil, common.NewErrBadRequest("SMREPO-BUILDSMEREF-INVALIDPATH Invalid idShort path syntax")
			}

			start := i + 1
			end := start + endIndex
			indexValue := idShortPath[start:end]
			if indexValue == "" {
				return nil, common.NewErrBadRequest("SMREPO-BUILDSMEREF-INVALIDPATH Empty list index in idShort path")
			}

			segments = append(segments, referencePathSegment{value: indexValue, isIndex: true})
			i = end
		default:
			err := current.WriteByte(idShortPath[i])
			if err != nil {
				return nil, common.NewErrBadRequest("SMREPO-BUILDSMEREF-INVALIDPATH Invalid idShort path syntax")
			}
		}
	}

	flushCurrent()

	if len(segments) == 0 {
		return nil, common.NewErrBadRequest("SMREPO-BUILDSMEREF-INVALIDPATH Invalid idShort path syntax")
	}

	for _, segment := range segments {
		if !segment.isIndex && segment.value == "" {
			return nil, common.NewErrBadRequest("SMREPO-BUILDSMEREF-INVALIDPATH Invalid idShort segment in path")
		}
	}

	return segments, nil
}

type rootElementCursorRow struct {
	id   int64
	path string
}

func getRootElementPage(ctx context.Context, db *sql.DB, submodelDatabaseID int64, limit *int, cursor string) ([]rootElementCursorRow, string, error) {
	if limit != nil && *limit == 0 {
		return []rootElementCursorRow{}, "", nil
	}

	dialect := goqu.Dialect("postgres")

	query := dialect.
		From(goqu.T("submodel_element").As("sme")).
		Select(
			goqu.I("sme.id"),
			goqu.I("sme.idshort_path"),
		).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			goqu.I("sme.parent_sme_id").IsNull(),
		)

	query = query.Order(goqu.I("sme.idshort_path").Asc(), goqu.I("sme.id").Asc())

	if cursor != "" {
		cursorPath, cursorID, hasCursorID := parseRootCursor(cursor)
		if hasCursorID {
			query = query.Where(
				goqu.Or(
					goqu.I("sme.idshort_path").Gt(cursorPath),
					goqu.And(
						goqu.I("sme.idshort_path").Eq(cursorPath),
						goqu.I("sme.id").Gt(cursorID),
					),
				),
			)
		} else {
			query = query.Where(goqu.I("sme.idshort_path").Gt(cursorPath))
		}
	}

	if limit != nil && *limit > 0 {
		//nolint:gosec // limit is validated to be > 0 before conversion
		query = query.Limit(uint(*limit + 1))
	}

	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if collectorErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETROOTPATHS-BADCOLLECTOR " + collectorErr.Error())
	}
	shouldEnforceFormula, enforceErr := auth.ShouldEnforceFormula(ctx)
	if enforceErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETROOTPATHS-SHOULDENFORCE " + enforceErr.Error())
	}
	if shouldEnforceFormula {
		var addFormulaErr error
		query, addFormulaErr = auth.AddFormulaQueryFromContext(ctx, query, collector)
		if addFormulaErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETROOTPATHS-ABACFORMULA " + addFormulaErr.Error())
		}
	}

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETROOTPATHS-BUILDQ " + toSQLErr.Error())
	}

	rows, queryErr := db.Query(sqlQuery, args...)
	if queryErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETROOTPATHS-EXECQ " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	paths := make([]rootElementCursorRow, 0, 32)
	nextCursor := ""

	for rows.Next() {
		var id int64
		var path string
		if scanErr := rows.Scan(&id, &path); scanErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-GETROOTPATHS-SCANROW " + scanErr.Error())
		}

		paths = append(paths, rootElementCursorRow{id: id, path: path})
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-GETROOTPATHS-ROWSERR " + rowsErr.Error())
	}

	if limit != nil && *limit > 0 && len(paths) > *limit {
		paths = paths[:*limit]
		lastPath := paths[len(paths)-1]
		nextCursor = formatRootCursor(lastPath.path, lastPath.id)
	}

	if limit != nil && *limit == -1 && cursor == "" {
		sort.SliceStable(paths, func(i, j int) bool {
			if paths[i].path == paths[j].path {
				return paths[i].id < paths[j].id
			}
			return paths[i].path < paths[j].path
		})
	}

	return paths, nextCursor, nil
}

func parseRootCursor(cursor string) (string, int64, bool) {
	separatorIndex := strings.LastIndex(cursor, "|")
	if separatorIndex <= 0 || separatorIndex == len(cursor)-1 {
		return cursor, 0, false
	}

	path := cursor[:separatorIndex]
	idPart := cursor[separatorIndex+1:]
	id, parseErr := strconv.ParseInt(idPart, 10, 64)
	if parseErr != nil {
		return cursor, 0, false
	}

	return path, id, true
}

func formatRootCursor(path string, id int64) string {
	return path + "|" + strconv.FormatInt(id, 10)
}

type loadedSMERow struct {
	row             model.SubmodelElementRow
	semanticPayload []byte
	qualifiers      []byte
	semanticVisible bool
	valueVisible    bool
}

type smeMaskFragmentGroups struct {
	idShort  []grammar.FragmentStringPattern
	semantic []grammar.FragmentStringPattern
	value    []grammar.FragmentStringPattern
}

func buildSMEMaskRuntime(ctx context.Context, collector *grammar.ResolvedFieldPathCollector) (*auth.SharedFragmentMaskRuntime, smeMaskFragmentGroups, error) {
	idShortFragments := map[grammar.FragmentStringPattern]struct{}{
		"$sme#idShort": {},
	}
	semanticFragments := map[grammar.FragmentStringPattern]struct{}{
		"$sme#semanticId": {},
	}
	valueFragments := map[grammar.FragmentStringPattern]struct{}{
		"$sme#value":     {},
		"$sme#valueType": {},
		"$sme#language":  {},
	}

	qf := auth.GetQueryFilter(ctx)
	if qf != nil {
		for fragment := range qf.Filters {
			suffix := fragmentSuffix(fragment)
			switch suffix {
			case "idShort":
				idShortFragments[fragment] = struct{}{}
			case "semanticId":
				semanticFragments[fragment] = struct{}{}
			case "value", "valueType", "language":
				valueFragments[fragment] = struct{}{}
			default:
				if strings.HasPrefix(suffix, "semanticId.") {
					semanticFragments[fragment] = struct{}{}
				}
			}
		}
	}

	groups := smeMaskFragmentGroups{
		idShort:  sortedFragments(idShortFragments),
		semantic: sortedFragments(semanticFragments),
		value:    sortedFragments(valueFragments),
	}

	maskedColumns := make([]auth.MaskedInnerColumnSpec, 0, len(groups.idShort)+len(groups.semantic)+len(groups.value))
	for i, fragment := range groups.idShort {
		maskedColumns = append(maskedColumns, auth.MaskedInnerColumnSpec{
			Fragment:  fragment,
			FlagAlias: "flag_idshort_" + strconv.Itoa(i+1),
			RawAlias:  "c_id_short",
		})
	}
	for i, fragment := range groups.semantic {
		maskedColumns = append(maskedColumns, auth.MaskedInnerColumnSpec{
			Fragment:  fragment,
			FlagAlias: "flag_semantic_" + strconv.Itoa(i+1),
			RawAlias:  "raw_semantic_payload",
		})
	}
	for i, fragment := range groups.value {
		maskedColumns = append(maskedColumns, auth.MaskedInnerColumnSpec{
			Fragment:  fragment,
			FlagAlias: "flag_value_" + strconv.Itoa(i+1),
			RawAlias:  "raw_value_payload",
		})
	}

	runtime, err := auth.BuildSharedFragmentMaskRuntime(ctx, collector, maskedColumns)
	if err != nil {
		return nil, smeMaskFragmentGroups{}, err
	}

	return runtime, groups, nil
}

func fragmentSuffix(fragment grammar.FragmentStringPattern) string {
	fragmentStr := string(fragment)
	fragmentIndex := strings.LastIndex(fragmentStr, "#")
	if fragmentIndex < 0 || fragmentIndex >= len(fragmentStr)-1 {
		return ""
	}
	return fragmentStr[fragmentIndex+1:]
}

func sortedFragments(items map[grammar.FragmentStringPattern]struct{}) []grammar.FragmentStringPattern {
	result := make([]grammar.FragmentStringPattern, 0, len(items))
	for fragment := range items {
		result = append(result, fragment)
	}
	sort.Slice(result, func(i int, j int) bool {
		return string(result[i]) < string(result[j])
	})
	return result
}

func buildSharedMaskVisibilityExpr(dataAlias string, runtime *auth.SharedFragmentMaskRuntime, fragments []grammar.FragmentStringPattern) (exp.Expression, error) {
	if len(fragments) == 0 {
		return goqu.V(true), nil
	}

	seenAliases := make(map[string]struct{}, len(fragments))
	conditions := make([]exp.Expression, 0, len(fragments))
	for _, fragment := range fragments {
		alias, err := runtime.FlagAlias(fragment)
		if err != nil {
			return nil, err
		}
		if _, seen := seenAliases[alias]; seen {
			continue
		}
		seenAliases[alias] = struct{}{}
		conditions = append(conditions, goqu.I(dataAlias+"."+alias))
	}

	if len(conditions) == 0 {
		return goqu.V(true), nil
	}
	if len(conditions) == 1 {
		return conditions[0], nil
	}

	return goqu.And(conditions...), nil
}

func buildMaskedSMEValuePayloadExpr(rawValueAlias string) exp.Expression {
	return goqu.L("(COALESCE(?::jsonb, '{}'::jsonb) - 'value')", goqu.I(rawValueAlias))
}

func readSubmodelElementRowsByPath(ctx context.Context, db *sql.DB, submodelDatabaseID int64, idShortOrPath string, includeChildren bool) ([]loadedSMERow, error) {
	dialect := goqu.Dialect("postgres")
	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if collectorErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEBYPATH-BADCOLLECTOR " + collectorErr.Error())
	}
	maskRuntime, maskGroups, maskRuntimeErr := buildSMEMaskRuntime(ctx, collector)
	if maskRuntimeErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEBYPATH-MASKRUNTIME " + maskRuntimeErr.Error())
	}

	const dataAlias = "sme_path_data"
	idShortVisibleExpr, idShortVisibleErr := buildSharedMaskVisibilityExpr(dataAlias, maskRuntime, maskGroups.idShort)
	if idShortVisibleErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEBYPATH-IDSHORTMASK " + idShortVisibleErr.Error())
	}
	semanticVisibleExpr, semanticVisibleErr := buildSharedMaskVisibilityExpr(dataAlias, maskRuntime, maskGroups.semantic)
	if semanticVisibleErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEBYPATH-SEMMASK " + semanticVisibleErr.Error())
	}
	valueVisibleExpr, valueVisibleErr := buildSharedMaskVisibilityExpr(dataAlias, maskRuntime, maskGroups.value)
	if valueVisibleErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEBYPATH-VALUEMASK " + valueVisibleErr.Error())
	}

	valueExpr := getSMEValueExpressionForRead(dialect)
	innerQuery := dialect.
		From(goqu.T("submodel_element").As("sme")).
		InnerJoin(
			goqu.T("submodel_element").As("submodel_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("sme.id"))),
		).
		LeftJoin(
			goqu.T("submodel_element_payload").As("sme_p"),
			goqu.On(goqu.I("sme.id").Eq(goqu.I("sme_p.submodel_element_id"))),
		).
		LeftJoin(
			goqu.T("submodel_element_semantic_id_reference_payload").As("sme_sem_payload"),
			goqu.On(goqu.I("sme_sem_payload.reference_id").Eq(goqu.I("sme.id"))),
		).
		Select(append([]interface{}{
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
			valueExpr.As("raw_value_payload"),
			goqu.L("'[]'::jsonb").As("raw_semantic_id_referred_payload"),
			goqu.L("'[]'::jsonb").As("raw_supplemental_semantic_ids_referred_payload"),
			goqu.L("COALESCE(sme_p.qualifiers_payload, '[]'::jsonb)").As("raw_qualifiers_payload"),
			goqu.L("COALESCE(sme_sem_payload.parent_reference_payload, '{}'::jsonb)").As("raw_semantic_payload"),
		}, maskRuntime.Projections()...)...)

	if includeChildren {
		innerQuery = innerQuery.Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			goqu.Or(
				goqu.I("sme.idshort_path").Eq(idShortOrPath),
				goqu.I("sme.idshort_path").Like(goqu.L("? || '.%'", idShortOrPath)),
				goqu.I("sme.idshort_path").Like(goqu.L("? || '[%'", idShortOrPath)),
			),
		)
	} else {
		innerQuery = innerQuery.Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			goqu.Or(
				goqu.I("sme.idshort_path").Eq(idShortOrPath),
				goqu.I("sme.parent_sme_id").In(
					dialect.From(goqu.T("submodel_element").As("sme_parent")).
						Select(goqu.I("sme_parent.id")).
						Where(
							goqu.I("sme_parent.submodel_id").Eq(submodelDatabaseID),
							goqu.I("sme_parent.idshort_path").Eq(idShortOrPath),
						),
				),
			),
		)
	}

	query := dialect.From(innerQuery.As(dataAlias)).
		Select(
			goqu.I(dataAlias+".c_id"),
			goqu.I(dataAlias+".c_parent_sme_id"),
			goqu.I(dataAlias+".c_root_sme_id"),
			goqu.Case().When(idShortVisibleExpr, goqu.I(dataAlias+".c_id_short")).Else(nil),
			goqu.I(dataAlias+".c_idshort_path"),
			goqu.I(dataAlias+".c_category"),
			goqu.I(dataAlias+".c_model_type"),
			goqu.I(dataAlias+".c_position"),
			goqu.I(dataAlias+".raw_embedded_data_specification_payload"),
			goqu.I(dataAlias+".raw_supplemental_semantic_ids_payload"),
			goqu.I(dataAlias+".raw_extensions_payload"),
			goqu.I(dataAlias+".raw_displayname_payload"),
			goqu.I(dataAlias+".raw_description_payload"),
			goqu.Case().When(valueVisibleExpr, goqu.I(dataAlias+".raw_value_payload")).Else(buildMaskedSMEValuePayloadExpr(dataAlias+".raw_value_payload")),
			goqu.I(dataAlias+".raw_semantic_id_referred_payload"),
			goqu.I(dataAlias+".raw_supplemental_semantic_ids_referred_payload"),
			goqu.I(dataAlias+".raw_qualifiers_payload"),
			goqu.Case().When(semanticVisibleExpr, goqu.I(dataAlias+".raw_semantic_payload")).Else(nil),
			goqu.Case().When(semanticVisibleExpr, true).Else(false),
			goqu.Case().When(valueVisibleExpr, true).Else(false),
		).
		Order(goqu.I(dataAlias+".c_idshort_path").Asc(), goqu.I(dataAlias+".c_position").Asc())

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEBYPATH-BUILDQ " + toSQLErr.Error())
	}

	return executeLoadedSMERowQuery(db, sqlQuery, args, "SMREPO-GETSMEBYPATH")
}

func readSubmodelElementRowsByRootIDs(ctx context.Context, db *sql.DB, submodelDatabaseID int64, rootIDs []int64, includeChildren bool, isGetSubmodelElements bool) ([]loadedSMERow, error) {
	if len(rootIDs) == 0 {
		return []loadedSMERow{}, nil
	}

	dialect := goqu.Dialect("postgres")
	collector, collectorErr := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if collectorErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMES-BATCHREAD-BADCOLLECTOR " + collectorErr.Error())
	}
	maskRuntime, maskGroups, maskRuntimeErr := buildSMEMaskRuntime(ctx, collector)
	if maskRuntimeErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMES-BATCHREAD-MASKRUNTIME " + maskRuntimeErr.Error())
	}

	const dataAlias = "sme_batch_data"
	idShortVisibleExpr, idShortVisibleErr := buildSharedMaskVisibilityExpr(dataAlias, maskRuntime, maskGroups.idShort)
	if idShortVisibleErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMES-BATCHREAD-IDSHORTMASK " + idShortVisibleErr.Error())
	}
	semanticVisibleExpr, semanticVisibleErr := buildSharedMaskVisibilityExpr(dataAlias, maskRuntime, maskGroups.semantic)
	if semanticVisibleErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMES-BATCHREAD-SEMMASK " + semanticVisibleErr.Error())
	}
	valueVisibleExpr, valueVisibleErr := buildSharedMaskVisibilityExpr(dataAlias, maskRuntime, maskGroups.value)
	if valueVisibleErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMES-BATCHREAD-VALUEMASK " + valueVisibleErr.Error())
	}

	rootOrderExpr := goqu.Case().
		Value(goqu.L("COALESCE(sme.root_sme_id, sme.id)"))
	for index, rootID := range rootIDs {
		rootOrderExpr = rootOrderExpr.When(rootID, index)
	}
	rootOrderExpr = rootOrderExpr.Else(len(rootIDs))

	var rootFilter exp.Expression = goqu.I("sme.id").In(rootIDs)
	if includeChildren {
		rootFilter = goqu.COALESCE(goqu.I("sme.root_sme_id"), goqu.I("sme.id")).In(rootIDs)
	} else if !includeChildren && isGetSubmodelElements {
		// For GET /submodel-elements with level=core, return root elements and their direct children.
		rootFilter = goqu.Or(
			goqu.I("sme.id").In(rootIDs),
			goqu.I("sme.parent_sme_id").In(rootIDs),
		)
	}

	valueExpr := getSMEValueExpressionForRead(dialect)
	innerQuery := dialect.
		From(goqu.T("submodel_element").As("sme")).
		InnerJoin(
			goqu.T("submodel_element").As("submodel_element"),
			goqu.On(goqu.I("submodel_element.id").Eq(goqu.I("sme.id"))),
		).
		LeftJoin(
			goqu.T("submodel_element_payload").As("sme_p"),
			goqu.On(goqu.I("sme.id").Eq(goqu.I("sme_p.submodel_element_id"))),
		).
		LeftJoin(
			goqu.T("submodel_element_semantic_id_reference_payload").As("sme_sem_payload"),
			goqu.On(goqu.I("sme_sem_payload.reference_id").Eq(goqu.I("sme.id"))),
		).
		Select(append([]interface{}{
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
			valueExpr.As("raw_value_payload"),
			goqu.L("'[]'::jsonb").As("raw_semantic_id_referred_payload"),
			goqu.L("'[]'::jsonb").As("raw_supplemental_semantic_ids_referred_payload"),
			goqu.L("COALESCE(sme_p.qualifiers_payload, '[]'::jsonb)").As("raw_qualifiers_payload"),
			goqu.L("COALESCE(sme_sem_payload.parent_reference_payload, '{}'::jsonb)").As("raw_semantic_payload"),
			goqu.COALESCE(goqu.I("sme.root_sme_id"), goqu.I("sme.id")).As("sort_root_id"),
			rootOrderExpr.As("sort_root_order"),
			goqu.I("sme.id").As("sort_id"),
		}, maskRuntime.Projections()...)...).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			rootFilter,
		)

	query := dialect.From(innerQuery.As(dataAlias)).
		Select(
			goqu.I(dataAlias+".c_id"),
			goqu.I(dataAlias+".c_parent_sme_id"),
			goqu.I(dataAlias+".c_root_sme_id"),
			goqu.Case().When(idShortVisibleExpr, goqu.I(dataAlias+".c_id_short")).Else(nil),
			goqu.I(dataAlias+".c_idshort_path"),
			goqu.I(dataAlias+".c_category"),
			goqu.I(dataAlias+".c_model_type"),
			goqu.I(dataAlias+".c_position"),
			goqu.I(dataAlias+".raw_embedded_data_specification_payload"),
			goqu.I(dataAlias+".raw_supplemental_semantic_ids_payload"),
			goqu.I(dataAlias+".raw_extensions_payload"),
			goqu.I(dataAlias+".raw_displayname_payload"),
			goqu.I(dataAlias+".raw_description_payload"),
			goqu.Case().When(valueVisibleExpr, goqu.I(dataAlias+".raw_value_payload")).Else(buildMaskedSMEValuePayloadExpr(dataAlias+".raw_value_payload")),
			goqu.I(dataAlias+".raw_semantic_id_referred_payload"),
			goqu.I(dataAlias+".raw_supplemental_semantic_ids_referred_payload"),
			goqu.I(dataAlias+".raw_qualifiers_payload"),
			goqu.Case().When(semanticVisibleExpr, goqu.I(dataAlias+".raw_semantic_payload")).Else(nil),
			goqu.Case().When(semanticVisibleExpr, true).Else(false),
			goqu.Case().When(valueVisibleExpr, true).Else(false),
		).
		Order(
			goqu.I(dataAlias+".sort_root_order").Asc(),
			goqu.I(dataAlias+".c_position").Asc(),
			goqu.I(dataAlias+".c_idshort_path").Asc(),
			goqu.I(dataAlias+".sort_id").Asc(),
		)

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMES-BATCHREAD-BUILDQ " + toSQLErr.Error())
	}

	return executeLoadedSMERowQuery(db, sqlQuery, args, "SMREPO-GETSMES-BATCHREAD")
}

func executeLoadedSMERowQuery(db *sql.DB, sqlQuery string, args []interface{}, errorCodePrefix string) ([]loadedSMERow, error) {
	rows, queryErr := db.Query(sqlQuery, args...)
	if queryErr != nil {
		return nil, common.NewInternalServerError(errorCodePrefix + "-EXECQ " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	parsedRows := make([]loadedSMERow, 0, 32)
	for rows.Next() {
		var dbID sql.NullInt64
		var parentID sql.NullInt64
		var rootID sql.NullInt64
		var idShort sql.NullString
		var idShortPath string
		var category sql.NullString
		var modelType int64
		var position int
		var embeddedPayload []byte
		var supplementalPayload []byte
		var extensionsPayload []byte
		var displayNamePayload []byte
		var descriptionPayload []byte
		var valuePayload []byte
		var semanticIDReferredPayload []byte
		var supplementalSemanticIDsReferredPayload []byte
		var qualifiersPayload []byte
		var semanticPayload []byte
		var semanticVisible bool
		var valueVisible bool

		scanErr := rows.Scan(
			&dbID,
			&parentID,
			&rootID,
			&idShort,
			&idShortPath,
			&category,
			&modelType,
			&position,
			&embeddedPayload,
			&supplementalPayload,
			&extensionsPayload,
			&displayNamePayload,
			&descriptionPayload,
			&valuePayload,
			&semanticIDReferredPayload,
			&supplementalSemanticIDsReferredPayload,
			&qualifiersPayload,
			&semanticPayload,
			&semanticVisible,
			&valueVisible,
		)
		if scanErr != nil {
			return nil, common.NewInternalServerError(errorCodePrefix + "-SCANROW " + scanErr.Error())
		}

		row := model.SubmodelElementRow{
			DbID:                            dbID,
			ParentID:                        parentID,
			RootID:                          rootID,
			IDShort:                         idShort,
			IDShortPath:                     idShortPath,
			Category:                        category,
			ModelType:                       modelType,
			Position:                        position,
			EmbeddedDataSpecifications:      bytesToRawMessagePtr(embeddedPayload),
			SupplementalSemanticIDs:         bytesToRawMessagePtr(supplementalPayload),
			Extensions:                      bytesToRawMessagePtr(extensionsPayload),
			DisplayNames:                    bytesToRawMessagePtr(displayNamePayload),
			Descriptions:                    bytesToRawMessagePtr(descriptionPayload),
			Value:                           bytesToRawMessagePtr(valuePayload),
			SemanticID:                      nil,
			SemanticIDReferred:              bytesToRawMessagePtr(semanticIDReferredPayload),
			SupplementalSemanticIDsReferred: bytesToRawMessagePtr(supplementalSemanticIDsReferredPayload),
			Qualifiers:                      bytesToRawMessagePtr([]byte("[]")),
		}

		parsedRows = append(parsedRows, loadedSMERow{row: row, semanticPayload: semanticPayload, qualifiers: qualifiersPayload, semanticVisible: semanticVisible, valueVisible: valueVisible})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError(errorCodePrefix + "-ROWSERR " + rowsErr.Error())
	}

	return parsedRows, nil
}

type loadedSMENode struct {
	id           int64
	parentID     sql.NullInt64
	path         string
	position     int
	element      types.ISubmodelElement
	valueVisible bool
}

func buildSubmodelElementTreeFromRows(db *sql.DB, parsedRows []loadedSMERow, submodelID string, idShortOrPath string) (types.ISubmodelElement, error) {
	nodes, children, rootNodes, buildNodesErr := buildLoadedSubmodelElementNodes(db, parsedRows, "SMREPO-GETSMEBYPATH")
	if buildNodesErr != nil {
		return nil, buildNodesErr
	}

	attachLoadedSubmodelElementChildren(children, nodes)

	if len(rootNodes) == 0 {
		return nil, common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortOrPath + "' not found in submodel '" + submodelID + "'")
	}

	sort.SliceStable(rootNodes, func(i int, j int) bool {
		return rootNodes[i].path < rootNodes[j].path
	})

	attachEntityChildrenByPathFallback(rootNodes)

	return rootNodes[0].element, nil
}

func attachEntityChildrenByPathFallback(rootNodes []*loadedSMENode) {
	if len(rootNodes) <= 1 {
		return
	}

	root := rootNodes[0]
	if root.element.ModelType() != types.ModelTypeEntity {
		return
	}

	orphanStatements := make([]*loadedSMENode, 0, len(rootNodes)-1)
	for i := 1; i < len(rootNodes); i++ {
		candidate := rootNodes[i]
		if isDirectEntityStatementPath(root.path, candidate.path) {
			orphanStatements = append(orphanStatements, candidate)
		}
	}

	if len(orphanStatements) == 0 {
		return
	}

	sort.SliceStable(orphanStatements, func(i int, j int) bool {
		if orphanStatements[i].position == orphanStatements[j].position {
			return orphanStatements[i].path < orphanStatements[j].path
		}
		return orphanStatements[i].position < orphanStatements[j].position
	})

	setEntityChildren(root.element, orphanStatements)
}

func isDirectEntityStatementPath(entityPath string, candidatePath string) bool {
	if strings.HasPrefix(candidatePath, entityPath+".") {
		suffix := strings.TrimPrefix(candidatePath, entityPath+".")
		return suffix != "" && !strings.Contains(suffix, ".") && !strings.Contains(suffix, "[")
	}

	if strings.HasPrefix(candidatePath, entityPath+"[") {
		suffix := strings.TrimPrefix(candidatePath, entityPath)
		return strings.Count(suffix, "[") == 1 && !strings.Contains(suffix, ".")
	}

	return false
}

func buildSubmodelElementForestFromRows(db *sql.DB, parsedRows []loadedSMERow) (map[int64]types.ISubmodelElement, error) {
	nodes, children, rootNodes, buildNodesErr := buildLoadedSubmodelElementNodes(db, parsedRows, "SMREPO-GETSMES-BUILDFOREST")
	if buildNodesErr != nil {
		return nil, buildNodesErr
	}

	attachLoadedSubmodelElementChildren(children, nodes)

	result := make(map[int64]types.ISubmodelElement, len(rootNodes))
	for _, rootNode := range rootNodes {
		result[rootNode.id] = rootNode.element
	}

	return result, nil
}

func buildLoadedSubmodelElementNodes(db *sql.DB, parsedRows []loadedSMERow, errorCodePrefix string) (map[int64]*loadedSMENode, map[int64][]*loadedSMENode, []*loadedSMENode, error) {
	nodes := make(map[int64]*loadedSMENode, len(parsedRows))
	children := make(map[int64][]*loadedSMENode, len(parsedRows))
	rootNodes := make([]*loadedSMENode, 0, 1)
	elementsByID := make(map[int64]types.ISubmodelElement, len(parsedRows))
	missingSemanticReferenceIDs := make([]int64, 0, len(parsedRows))
	missingSemanticReferenceSet := make(map[int64]struct{}, len(parsedRows))

	for _, item := range parsedRows {
		if !item.row.DbID.Valid {
			return nil, nil, nil, common.NewInternalServerError(errorCodePrefix + "-NODBID Missing database id for submodel element")
		}

		element, _, buildErr := builders.BuildSubmodelElement(item.row, db)
		if buildErr != nil {
			return nil, nil, nil, common.NewInternalServerError(errorCodePrefix + "-BUILDELEM " + buildErr.Error())
		}

		if semanticID, parseSemanticErr := common.ParseReferenceJSON(item.semanticPayload); parseSemanticErr != nil {
			return nil, nil, nil, parseSemanticErr
		} else if semanticID != nil {
			element.SetSemanticID(semanticID)
		} else if item.semanticVisible {
			if _, exists := missingSemanticReferenceSet[item.row.DbID.Int64]; !exists {
				missingSemanticReferenceSet[item.row.DbID.Int64] = struct{}{}
				missingSemanticReferenceIDs = append(missingSemanticReferenceIDs, item.row.DbID.Int64)
			}
		}

		if qualifiers, parseQualifiersErr := common.ParseQualifiersJSON(item.qualifiers); parseQualifiersErr != nil {
			return nil, nil, nil, parseQualifiersErr
		} else if len(qualifiers) > 0 {
			element.SetQualifiers(qualifiers)
		}

		n := &loadedSMENode{
			id:           item.row.DbID.Int64,
			parentID:     item.row.ParentID,
			path:         item.row.IDShortPath,
			position:     item.row.Position,
			element:      element,
			valueVisible: item.valueVisible,
		}
		nodes[n.id] = n
		elementsByID[n.id] = element
	}

	if len(missingSemanticReferenceIDs) > 0 {
		fallbackSemanticIDs, fallbackErr := getReferencesFromKeyTables(
			db,
			"submodel_element_semantic_id_reference",
			"submodel_element_semantic_id_reference_key",
			missingSemanticReferenceIDs,
			errorCodePrefix+"-SEMKEYS",
		)
		if fallbackErr != nil {
			return nil, nil, nil, fallbackErr
		}

		for referenceID, semanticID := range fallbackSemanticIDs {
			element, exists := elementsByID[referenceID]
			if !exists || semanticID == nil {
				continue
			}
			element.SetSemanticID(semanticID)
		}
	}

	for _, item := range parsedRows {
		if !item.row.DbID.Valid {
			continue
		}

		n, exists := nodes[item.row.DbID.Int64]
		if !exists {
			continue
		}

		if n.parentID.Valid {
			if _, exists := nodes[n.parentID.Int64]; exists {
				children[n.parentID.Int64] = append(children[n.parentID.Int64], n)
				continue
			}
		}
		rootNodes = append(rootNodes, n)
	}

	return nodes, children, rootNodes, nil
}

func attachLoadedSubmodelElementChildren(children map[int64][]*loadedSMENode, nodes map[int64]*loadedSMENode) {
	for id, parent := range nodes {
		kids := children[id]
		if len(kids) == 0 {
			continue
		}

		sort.SliceStable(kids, func(i int, j int) bool {
			if kids[i].position == kids[j].position {
				return kids[i].path < kids[j].path
			}
			return kids[i].position < kids[j].position
		})

		switch parent.element.ModelType() {
		case types.ModelTypeSubmodelElementCollection:
			if parent.valueVisible {
				setCollectionChildren(parent.element, kids)
			}
		case types.ModelTypeSubmodelElementList:
			if parent.valueVisible {
				setListChildren(parent.element, kids)
			}
		case types.ModelTypeAnnotatedRelationshipElement:
			setAnnotatedRelationshipChildren(parent.element, kids)
		case types.ModelTypeEntity:
			setEntityChildren(parent.element, kids)
		}
	}
}

func setCollectionChildren(parent types.ISubmodelElement, kids []*loadedSMENode) {
	p, ok := parent.(types.ISubmodelElementCollection)
	if !ok {
		return
	}
	values := p.Value()
	for _, child := range kids {
		values = append(values, child.element)
	}
	p.SetValue(values)
}

func setListChildren(parent types.ISubmodelElement, kids []*loadedSMENode) {
	p, ok := parent.(types.ISubmodelElementList)
	if !ok {
		return
	}
	values := p.Value()
	for _, child := range kids {
		values = append(values, child.element)
	}
	p.SetValue(values)
}

func setAnnotatedRelationshipChildren(parent types.ISubmodelElement, kids []*loadedSMENode) {
	p, ok := parent.(types.IAnnotatedRelationshipElement)
	if !ok {
		return
	}
	annotations := p.Annotations()
	for _, child := range kids {
		annotations = append(annotations, child.element)
	}
	p.SetAnnotations(annotations)
}

func setEntityChildren(parent types.ISubmodelElement, kids []*loadedSMENode) {
	p, ok := parent.(types.IEntity)
	if !ok {
		return
	}
	statements := p.Statements()
	for _, child := range kids {
		statements = append(statements, child.element)
	}
	p.SetStatements(statements)
}

func getReferencesFromKeyTables(db *sql.DB, referenceTable string, referenceKeyTable string, referenceIDs []int64, errorCodePrefix string) (map[int64]types.IReference, error) {
	if len(referenceIDs) == 0 {
		return map[int64]types.IReference{}, nil
	}

	dialect := goqu.Dialect("postgres")

	typeQuery, typeArgs, typeToSQLErr := dialect.
		From(goqu.T(referenceTable)).
		Select(goqu.I("id"), goqu.I("type")).
		Where(goqu.I("id").In(referenceIDs)).
		ToSQL()
	if typeToSQLErr != nil {
		return nil, common.NewInternalServerError(errorCodePrefix + "-READTYPES-BUILDQ " + typeToSQLErr.Error())
	}

	typeRows, typeQueryErr := db.Query(typeQuery, typeArgs...)
	if typeQueryErr != nil {
		return nil, common.NewInternalServerError(errorCodePrefix + "-READTYPES-EXECQ " + typeQueryErr.Error())
	}
	defer func() { _ = typeRows.Close() }()

	referenceTypes := make(map[int64]int64, len(referenceIDs))
	for typeRows.Next() {
		var referenceID int64
		var referenceTypeInt int64
		if scanErr := typeRows.Scan(&referenceID, &referenceTypeInt); scanErr != nil {
			return nil, common.NewInternalServerError(errorCodePrefix + "-SCANTYPES " + scanErr.Error())
		}
		referenceTypes[referenceID] = referenceTypeInt
	}

	if typeRowsErr := typeRows.Err(); typeRowsErr != nil {
		return nil, common.NewInternalServerError(errorCodePrefix + "-ROWSTYPES " + typeRowsErr.Error())
	}

	if len(referenceTypes) == 0 {
		return map[int64]types.IReference{}, nil
	}

	keysQuery, keyArgs, keysToSQLErr := dialect.
		From(goqu.T(referenceKeyTable)).
		Select(goqu.I("reference_id"), goqu.I("type"), goqu.I("value")).
		Where(goqu.I("reference_id").In(referenceIDs)).
		Order(goqu.I("reference_id").Asc(), goqu.I("position").Asc()).
		ToSQL()
	if keysToSQLErr != nil {
		return nil, common.NewInternalServerError(errorCodePrefix + "-READKEYS-BUILDQ " + keysToSQLErr.Error())
	}

	rows, queryErr := db.Query(keysQuery, keyArgs...)
	if queryErr != nil {
		return nil, common.NewInternalServerError(errorCodePrefix + "-READKEYS-EXECQ " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	keysByReferenceID := make(map[int64][]types.IKey, len(referenceTypes))
	for rows.Next() {
		var referenceID int64
		var keyTypeInt int64
		var keyValue string
		if scanErr := rows.Scan(&referenceID, &keyTypeInt, &keyValue); scanErr != nil {
			return nil, common.NewInternalServerError(errorCodePrefix + "-SCANKEYS " + scanErr.Error())
		}

		key := types.Key{}
		keyType := types.KeyTypes(keyTypeInt)
		key.SetType(keyType)
		key.SetValue(keyValue)
		keysByReferenceID[referenceID] = append(keysByReferenceID[referenceID], &key)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError(errorCodePrefix + "-ROWSKEYS " + rowsErr.Error())
	}

	result := make(map[int64]types.IReference, len(referenceTypes))
	for referenceID, referenceTypeInt := range referenceTypes {
		keys := keysByReferenceID[referenceID]
		if len(keys) == 0 {
			continue
		}

		reference := types.Reference{}
		referenceType := types.ReferenceTypes(referenceTypeInt)
		reference.SetType(referenceType)
		reference.SetKeys(keys)
		result[referenceID] = &reference
	}

	return result, nil
}

func bytesToRawMessagePtr(data []byte) *json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	msg := json.RawMessage(data)
	return &msg
}
