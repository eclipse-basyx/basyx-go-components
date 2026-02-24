package submodelelements

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

// GetSubmodelElementByIDShortOrPath loads a submodel element by submodel ID and idShort path,
// including all nested children in original structure.
func GetSubmodelElementByIDShortOrPath(db *sql.DB, submodelID string, idShortOrPath string) (types.ISubmodelElement, error) {
	if submodelID == "" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMEBYPATH-EMPTYSMID Submodel id must not be empty")
	}
	if idShortOrPath == "" {
		return nil, common.NewErrBadRequest("SMREPO-GETSMEBYPATH-EMPTYPATH idShort or path must not be empty")
	}

	submodelDatabaseID, submodelIDErr := persistenceutils.GetSubmodelDatabaseIDFromDB(db, submodelID)
	if submodelIDErr != nil {
		if errors.Is(submodelIDErr, sql.ErrNoRows) {
			return nil, common.NewErrNotFound(submodelID)
		}
		return nil, common.NewInternalServerError("SMREPO-GETSMEBYPATH-GETSMDATABASEID " + submodelIDErr.Error())
	}

	return getSubmodelElementByIDShortOrPathWithSubmodelDBID(db, submodelID, int64(submodelDatabaseID), idShortOrPath)
}

func getSubmodelElementByIDShortOrPathWithSubmodelDBID(db *sql.DB, submodelID string, submodelDatabaseID int64, idShortOrPath string) (types.ISubmodelElement, error) {
	parsedRows, readRowsErr := readSubmodelElementRowsByPath(db, submodelDatabaseID, idShortOrPath)
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

// GetSubmodelElementsBySubmodelID loads top-level submodel elements and reconstructs
// each complete subtree in original hierarchy.
func GetSubmodelElementsBySubmodelID(db *sql.DB, submodelID string, limit *int, cursor string, level string) ([]types.ISubmodelElement, string, error) {
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

	rootElements, nextCursor, rootPathErr := getRootElementPage(db, int64(submodelDatabaseID), limit, cursor)
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
	parsedRows, readRowsErr := readSubmodelElementRowsByRootIDs(db, int64(submodelDatabaseID), rootIDs, includeChildren)
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
func GetSubmodelElementReferencesBySubmodelID(db *sql.DB, submodelID string, limit *int, cursor string) ([]types.IReference, string, error) {
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

	rootElements, nextCursor, rootPathErr := getRootElementPage(db, int64(submodelDatabaseID), limit, cursor)
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

func getRootElementPage(db *sql.DB, submodelDatabaseID int64, limit *int, cursor string) ([]rootElementCursorRow, string, error) {
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
}

func readSubmodelElementRowsByPath(db *sql.DB, submodelDatabaseID int64, idShortOrPath string) ([]loadedSMERow, error) {
	dialect := goqu.Dialect("postgres")

	valueExpr := getSMEValueExpressionForRead(dialect)
	query := dialect.
		From(goqu.T("submodel_element").As("sme")).
		LeftJoin(
			goqu.T("submodel_element_payload").As("sme_p"),
			goqu.On(goqu.I("sme.id").Eq(goqu.I("sme_p.submodel_element_id"))),
		).
		LeftJoin(
			goqu.T("submodel_element_semantic_id_reference_payload").As("sme_sem_payload"),
			goqu.On(goqu.I("sme_sem_payload.reference_id").Eq(goqu.I("sme.id"))),
		).
		Select(
			goqu.I("sme.id"),
			goqu.I("sme.parent_sme_id"),
			goqu.I("sme.root_sme_id"),
			goqu.I("sme.id_short"),
			goqu.I("sme.idshort_path"),
			goqu.I("sme.category"),
			goqu.I("sme.model_type"),
			goqu.COALESCE(goqu.I("sme.position"), 0),
			goqu.L("COALESCE(sme_p.embedded_data_specification_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sme_p.supplemental_semantic_ids_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sme_p.extensions_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sme_p.displayname_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sme_p.description_payload, '[]'::jsonb)"),
			valueExpr,
			goqu.L("'[]'::jsonb"),
			goqu.L("'[]'::jsonb"),
			goqu.L("COALESCE(sme_p.qualifiers_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sme_sem_payload.parent_reference_payload, '{}'::jsonb)"),
		).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			goqu.Or(
				goqu.I("sme.idshort_path").Eq(idShortOrPath),
				goqu.I("sme.idshort_path").Like(goqu.L("? || '.%'", idShortOrPath)),
				goqu.I("sme.idshort_path").Like(goqu.L("? || '[%'", idShortOrPath)),
			),
		).
		Order(goqu.I("sme.idshort_path").Asc(), goqu.I("sme.position").Asc())

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return nil, common.NewInternalServerError("SMREPO-GETSMEBYPATH-BUILDQ " + toSQLErr.Error())
	}

	return executeLoadedSMERowQuery(db, sqlQuery, args, "SMREPO-GETSMEBYPATH")
}

func readSubmodelElementRowsByRootIDs(db *sql.DB, submodelDatabaseID int64, rootIDs []int64, includeChildren bool) ([]loadedSMERow, error) {
	if len(rootIDs) == 0 {
		return []loadedSMERow{}, nil
	}

	dialect := goqu.Dialect("postgres")

	rootOrderExpr := goqu.Case().
		Value(goqu.L("COALESCE(sme.root_sme_id, sme.id)"))
	for index, rootID := range rootIDs {
		rootOrderExpr = rootOrderExpr.When(rootID, index)
	}
	rootOrderExpr = rootOrderExpr.Else(len(rootIDs))

	rootFilter := goqu.I("sme.id").In(rootIDs)
	if includeChildren {
		rootFilter = goqu.COALESCE(goqu.I("sme.root_sme_id"), goqu.I("sme.id")).In(rootIDs)
	}

	valueExpr := getSMEValueExpressionForRead(dialect)
	query := dialect.
		From(goqu.T("submodel_element").As("sme")).
		LeftJoin(
			goqu.T("submodel_element_payload").As("sme_p"),
			goqu.On(goqu.I("sme.id").Eq(goqu.I("sme_p.submodel_element_id"))),
		).
		LeftJoin(
			goqu.T("submodel_element_semantic_id_reference_payload").As("sme_sem_payload"),
			goqu.On(goqu.I("sme_sem_payload.reference_id").Eq(goqu.I("sme.id"))),
		).
		Select(
			goqu.I("sme.id"),
			goqu.I("sme.parent_sme_id"),
			goqu.I("sme.root_sme_id"),
			goqu.I("sme.id_short"),
			goqu.I("sme.idshort_path"),
			goqu.I("sme.category"),
			goqu.I("sme.model_type"),
			goqu.COALESCE(goqu.I("sme.position"), 0),
			goqu.L("COALESCE(sme_p.embedded_data_specification_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sme_p.supplemental_semantic_ids_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sme_p.extensions_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sme_p.displayname_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sme_p.description_payload, '[]'::jsonb)"),
			valueExpr,
			goqu.L("'[]'::jsonb"),
			goqu.L("'[]'::jsonb"),
			goqu.L("COALESCE(sme_p.qualifiers_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sme_sem_payload.parent_reference_payload, '{}'::jsonb)"),
		).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			rootFilter,
		).
		Order(
			rootOrderExpr.Asc(),
			goqu.COALESCE(goqu.I("sme.position"), 0).Asc(),
			goqu.I("sme.idshort_path").Asc(),
			goqu.I("sme.id").Asc(),
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

		parsedRows = append(parsedRows, loadedSMERow{row: row, semanticPayload: semanticPayload, qualifiers: qualifiersPayload})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError(errorCodePrefix + "-ROWSERR " + rowsErr.Error())
	}

	return parsedRows, nil
}

type loadedSMENode struct {
	id       int64
	parentID sql.NullInt64
	path     string
	position int
	element  types.ISubmodelElement
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
		} else {
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
			id:       item.row.DbID.Int64,
			parentID: item.row.ParentID,
			path:     item.row.IDShortPath,
			position: item.row.Position,
			element:  element,
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
			setCollectionChildren(parent.element, kids)
		case types.ModelTypeSubmodelElementList:
			setListChildren(parent.element, kids)
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
