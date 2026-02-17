package submodelpersistence

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/Submodel/submodelElements"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

// NewSMHandler is responsible for creating submodels and all linked persistence records.
type NewSMHandler struct {
	database *sql.DB
}

// NewSubmodelHandler creates a handler for high-performance submodel create operations.
func NewSubmodelHandler(database *sql.DB) *NewSMHandler {
	return &NewSMHandler{database: database}
}

// GetSubmodelByID retrieves a submodel by its external identifier using the new handler.
func (h *NewSMHandler) GetSubmodelByID(submodelID string) (*types.Submodel, error) {
	if submodelID == "" {
		return nil, common.NewErrBadRequest("SMREPO-NEWSM-GETBYID-EMPTYID Submodel id must not be empty")
	}

	dialect := goqu.Dialect("postgres")
	query := dialect.
		From(goqu.T("submodel").As("s")).
		LeftJoin(
			goqu.T("submodel_payload").As("sp"),
			goqu.On(goqu.I("s.id").Eq(goqu.I("sp.submodel_id"))),
		).
		Select(
			goqu.I("s.id"),
			goqu.I("s.id_short"),
			goqu.I("s.category"),
			goqu.I("s.kind"),
			goqu.L("COALESCE(sp.description_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.displayname_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.administrative_information_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.embedded_data_specification_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.supplemental_semantic_ids_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.extensions_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.qualifiers_payload, '[]'::jsonb)"),
			goqu.L("COALESCE((SELECT parent_reference_payload FROM submodel_semantic_id_reference_payload WHERE reference_id = s.id LIMIT 1), '{}'::jsonb)"),
		).
		Where(goqu.I("s.submodel_identifier").Eq(submodelID)).
		Limit(1)

	sqlQuery, args, err := query.ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-NEWSM-GETBYID-BUILDQ " + err.Error())
	}

	var submodelDatabaseID int64
	var idShort sql.NullString
	var category sql.NullString
	var kind sql.NullInt64
	var descriptionPayload []byte
	var displayNamePayload []byte
	var administrationPayload []byte
	var edsPayload []byte
	var supplementalSemanticIDsPayload []byte
	var extensionsPayload []byte
	var qualifiersPayload []byte
	var semanticIDPayload []byte

	err = h.database.QueryRow(sqlQuery, args...).Scan(
		&submodelDatabaseID,
		&idShort,
		&category,
		&kind,
		&descriptionPayload,
		&displayNamePayload,
		&administrationPayload,
		&edsPayload,
		&supplementalSemanticIDsPayload,
		&extensionsPayload,
		&qualifiersPayload,
		&semanticIDPayload,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound(submodelID)
		}
		return nil, common.NewInternalServerError("SMREPO-NEWSM-GETBYID-EXECQ " + err.Error())
	}

	result := types.NewSubmodel(submodelID)
	if idShort.Valid {
		result.SetIDShort(&idShort.String)
	}
	if category.Valid {
		result.SetCategory(&category.String)
	}
	if kind.Valid {
		modellingKind := types.ModellingKind(kind.Int64)
		result.SetKind(&modellingKind)
	}

	if semanticID, parseSemanticErr := common.ParseReferenceJSON(semanticIDPayload); parseSemanticErr != nil {
		return nil, parseSemanticErr
	} else if semanticID != nil {
		result.SetSemanticID(semanticID)
	} else {
		fallbackSemanticID, fallbackErr := h.getReferenceFromKeyTables(
			"submodel_semantic_id_reference",
			"submodel_semantic_id_reference_key",
			submodelDatabaseID,
			"SMREPO-NEWSM-GETBYID-SEMKEYS",
		)
		if fallbackErr != nil {
			return nil, fallbackErr
		}
		if fallbackSemanticID != nil {
			result.SetSemanticID(fallbackSemanticID)
		}
	}

	if descriptions, parseDescriptionsErr := common.ParseLangStringTextTypesJSON(descriptionPayload); parseDescriptionsErr != nil {
		return nil, parseDescriptionsErr
	} else if len(descriptions) > 0 {
		result.SetDescription(descriptions)
	}

	if displayNames, parseDisplayNamesErr := common.ParseLangStringNameTypesJSON(displayNamePayload); parseDisplayNamesErr != nil {
		return nil, parseDisplayNamesErr
	} else if len(displayNames) > 0 {
		result.SetDisplayName(displayNames)
	}

	if administration, parseAdministrationErr := common.ParseAdministrationJSON(administrationPayload); parseAdministrationErr != nil {
		return nil, parseAdministrationErr
	} else if administration != nil {
		result.SetAdministration(administration)
	}

	if eds, parseEDSErr := common.ParseEmbeddedDataSpecificationsJSON(edsPayload); parseEDSErr != nil {
		return nil, parseEDSErr
	} else if len(eds) > 0 {
		result.SetEmbeddedDataSpecifications(eds)
	}

	if supplementalSemanticIDs, parseSupplementalErr := common.ParseReferencesJSONArray(supplementalSemanticIDsPayload); parseSupplementalErr != nil {
		return nil, parseSupplementalErr
	} else if len(supplementalSemanticIDs) > 0 {
		result.SetSupplementalSemanticIDs(supplementalSemanticIDs)
	}

	if extensions, parseExtensionsErr := common.ParseExtensionsJSON(extensionsPayload); parseExtensionsErr != nil {
		return nil, parseExtensionsErr
	} else if len(extensions) > 0 {
		result.SetExtensions(extensions)
	}

	if qualifiers, parseQualifiersErr := common.ParseQualifiersJSON(qualifiersPayload); parseQualifiersErr != nil {
		return nil, parseQualifiersErr
	} else if len(qualifiers) > 0 {
		result.SetQualifiers(qualifiers)
	}

	return result, nil
}

// GetAllSubmodels retrieves paginated submodels by submodel_identifier using schema-based payload parsing.
func (h *NewSMHandler) GetAllSubmodels(limit int64, cursor string) ([]*types.Submodel, string, error) {
	if limit <= 0 {
		limit = 100
	}

	dialect := goqu.Dialect("postgres")
	query := dialect.
		From(goqu.T("submodel").As("s")).
		LeftJoin(
			goqu.T("submodel_payload").As("sp"),
			goqu.On(goqu.I("s.id").Eq(goqu.I("sp.submodel_id"))),
		).
		Select(
			goqu.I("s.id"),
			goqu.I("s.submodel_identifier"),
			goqu.I("s.id_short"),
			goqu.I("s.category"),
			goqu.I("s.kind"),
			goqu.L("COALESCE(sp.description_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.displayname_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.administrative_information_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.embedded_data_specification_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.supplemental_semantic_ids_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.extensions_payload, '[]'::jsonb)"),
			goqu.L("COALESCE(sp.qualifiers_payload, '[]'::jsonb)"),
			goqu.L("COALESCE((SELECT parent_reference_payload FROM submodel_semantic_id_reference_payload WHERE reference_id = s.id LIMIT 1), '{}'::jsonb)"),
		).
		Order(goqu.I("s.submodel_identifier").Asc())

	if cursor != "" {
		query = query.Where(goqu.I("s.submodel_identifier").Gt(cursor))
	}

	sqlQuery, args, err := query.ToSQL()
	if err != nil {
		return nil, "", common.NewInternalServerError("SMREPO-NEWSM-GETALL-BUILDQ " + err.Error())
	}

	rows, queryErr := h.database.Query(sqlQuery, args...)
	if queryErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-NEWSM-GETALL-EXECQ " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	result := make([]*types.Submodel, 0, limit)
	nextCursor := ""

	for rows.Next() {
		var scanned submodelPayloadRow

		err = rows.Scan(
			&scanned.dbID,
			&scanned.submodelID,
			&scanned.idShort,
			&scanned.category,
			&scanned.kind,
			&scanned.descriptionPayload,
			&scanned.displayNamePayload,
			&scanned.administrationPayload,
			&scanned.edsPayload,
			&scanned.supplementalSemanticIDsPayload,
			&scanned.extensionsPayload,
			&scanned.qualifiersPayload,
			&scanned.semanticIDPayload,
		)
		if err != nil {
			return nil, "", common.NewInternalServerError("SMREPO-NEWSM-GETALL-SCANROW " + err.Error())
		}

		sm, buildErr := buildSubmodelFromPayloads(scanned)
		if buildErr != nil {
			return nil, "", buildErr
		}

		if sm.SemanticID() == nil {
			fallbackSemanticID, fallbackErr := h.getReferenceFromKeyTables(
				"submodel_semantic_id_reference",
				"submodel_semantic_id_reference_key",
				scanned.dbID,
				"SMREPO-NEWSM-GETALL-SEMKEYS",
			)
			if fallbackErr != nil {
				return nil, "", fallbackErr
			}
			if fallbackSemanticID != nil {
				sm.SetSemanticID(fallbackSemanticID)
			}
		}

		result = append(result, sm)
		if int64(len(result)) > limit {
			nextCursor = sm.ID()
			result = result[:len(result)-1]
			break
		}
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-NEWSM-GETALL-ROWSERR " + rowsErr.Error())
	}

	return result, nextCursor, nil
}

// GetSubmodelElementByIdShortOrPath retrieves one submodel element (including nested children) by idShort or full idShort path.
func (h *NewSMHandler) GetSubmodelElementByIdShortOrPath(submodelID string, idShortOrPath string) (types.ISubmodelElement, error) {
	if submodelID == "" {
		return nil, common.NewErrBadRequest("SMREPO-NEWSM-GETSMEBYPATH-EMPTYSMID Submodel id must not be empty")
	}
	if idShortOrPath == "" {
		return nil, common.NewErrBadRequest("SMREPO-NEWSM-GETSMEBYPATH-EMPTYPATH idShort or path must not be empty")
	}

	submodelDatabaseID, submodelIDErr := h.getSubmodelDatabaseIDByIdentifier(submodelID)
	if submodelIDErr != nil {
		return nil, submodelIDErr
	}

	parsedRows, readRowsErr := h.readSubmodelElementRows(submodelDatabaseID, idShortOrPath)
	if readRowsErr != nil {
		return nil, readRowsErr
	}
	if len(parsedRows) == 0 {
		return nil, common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortOrPath + "' not found in submodel '" + submodelID + "'")
	}

	rootElement, buildTreeErr := h.buildSubmodelElementTree(parsedRows, submodelID, idShortOrPath)
	if buildTreeErr != nil {
		return nil, buildTreeErr
	}

	return rootElement, nil
}

// GetSubmodelElements retrieves top-level submodel elements with optional cursor pagination.
// It builds each root element with all nested children using the new schema-based path resolver.
func (h *NewSMHandler) GetSubmodelElements(submodelID string, limit int, cursor string, valueOnly bool) ([]types.ISubmodelElement, string, error) {
	_ = valueOnly

	if submodelID == "" {
		return nil, "", common.NewErrBadRequest("SMREPO-NEWSM-GETSMES-EMPTYSMID Submodel id must not be empty")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < -1 {
		return nil, "", common.NewErrBadRequest("SMREPO-NEWSM-GETSMES-BADLIMIT limit must be >= -1")
	}

	submodelDatabaseID, submodelIDErr := h.getSubmodelDatabaseIDByIdentifier(submodelID)
	if submodelIDErr != nil {
		return nil, "", submodelIDErr
	}

	rootPaths, nextCursor, rootPathErr := h.getRootElementPaths(submodelDatabaseID, limit, cursor)
	if rootPathErr != nil {
		return nil, "", rootPathErr
	}

	result := make([]types.ISubmodelElement, 0, len(rootPaths))
	for _, rootPath := range rootPaths {
		element, elementErr := h.GetSubmodelElementByIdShortOrPath(submodelID, rootPath)
		if elementErr != nil {
			return nil, "", elementErr
		}
		result = append(result, element)
	}

	return result, nextCursor, nil
}

func (h *NewSMHandler) getRootElementPaths(submodelDatabaseID int64, limit int, cursor string) ([]string, string, error) {
	dialect := goqu.Dialect("postgres")

	query := dialect.
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.idshort_path")).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			goqu.I("sme.parent_sme_id").IsNull(),
		).
		Order(goqu.I("sme.idshort_path").Asc())

	if cursor != "" {
		query = query.Where(goqu.I("sme.idshort_path").Gt(cursor))
	}

	sqlQuery, args, toSQLErr := query.ToSQL()
	if toSQLErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-NEWSM-GETROOTPATHS-BUILDQ " + toSQLErr.Error())
	}

	rows, queryErr := h.database.Query(sqlQuery, args...)
	if queryErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-NEWSM-GETROOTPATHS-EXECQ " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	paths := make([]string, 0, 32)
	nextCursor := ""

	for rows.Next() {
		var path string
		if scanErr := rows.Scan(&path); scanErr != nil {
			return nil, "", common.NewInternalServerError("SMREPO-NEWSM-GETROOTPATHS-SCANROW " + scanErr.Error())
		}

		paths = append(paths, path)
		if limit > 0 && len(paths) > limit {
			nextCursor = path
			paths = paths[:len(paths)-1]
			break
		}
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, "", common.NewInternalServerError("SMREPO-NEWSM-GETROOTPATHS-ROWSERR " + rowsErr.Error())
	}

	return paths, nextCursor, nil
}

func (h *NewSMHandler) getSubmodelDatabaseIDByIdentifier(submodelID string) (int64, error) {
	dialect := goqu.Dialect("postgres")

	getSubmodelDBIDQuery := dialect.
		From("submodel").
		Select("id").
		Where(goqu.I("submodel_identifier").Eq(submodelID)).
		Limit(1)

	getSubmodelDBIDSQLQuery, getSubmodelDBIDArgs, getSubmodelDBIDToSQLErr := getSubmodelDBIDQuery.ToSQL()
	if getSubmodelDBIDToSQLErr != nil {
		return 0, common.NewInternalServerError("SMREPO-NEWSM-GETSMEBYPATH-BUILDSMDBIDQ " + getSubmodelDBIDToSQLErr.Error())
	}

	var submodelDatabaseID int64
	err := h.database.QueryRow(getSubmodelDBIDSQLQuery, getSubmodelDBIDArgs...).Scan(&submodelDatabaseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, common.NewErrNotFound(submodelID)
		}
		return 0, common.NewInternalServerError("SMREPO-NEWSM-GETSMEBYPATH-EXECSMDBIDQ " + err.Error())
	}

	return submodelDatabaseID, nil
}

type newHandlerSMERow struct {
	row             model.SubmodelElementRow
	semanticPayload []byte
	qualifiers      []byte
}

func (h *NewSMHandler) readSubmodelElementRows(submodelDatabaseID int64, idShortOrPath string) ([]newHandlerSMERow, error) {
	dialect := goqu.Dialect("postgres")

	valueExpr := getSMEValueExpressionForNewHandler(dialect)
	query := dialect.
		From(goqu.T("submodel_element").As("sme")).
		LeftJoin(
			goqu.T("submodel_element_payload").As("sme_p"),
			goqu.On(goqu.I("sme.id").Eq(goqu.I("sme_p.submodel_element_id"))),
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
			goqu.L("COALESCE((SELECT parent_reference_payload FROM submodel_element_semantic_id_reference_payload WHERE reference_id = sme.id LIMIT 1), '{}'::jsonb)"),
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
		return nil, common.NewInternalServerError("SMREPO-NEWSM-GETSMEBYPATH-BUILDQ " + toSQLErr.Error())
	}

	rows, queryErr := h.database.Query(sqlQuery, args...)
	if queryErr != nil {
		return nil, common.NewInternalServerError("SMREPO-NEWSM-GETSMEBYPATH-EXECQ " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	parsedRows := make([]newHandlerSMERow, 0, 32)
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
			return nil, common.NewInternalServerError("SMREPO-NEWSM-GETSMEBYPATH-SCANROW " + scanErr.Error())
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

		parsedRows = append(parsedRows, newHandlerSMERow{row: row, semanticPayload: semanticPayload, qualifiers: qualifiersPayload})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError("SMREPO-NEWSM-GETSMEBYPATH-ROWSERR " + rowsErr.Error())
	}

	return parsedRows, nil
}

type newHandlerSMENode struct {
	id       int64
	parentID sql.NullInt64
	path     string
	position int
	element  types.ISubmodelElement
}

func (h *NewSMHandler) buildSubmodelElementTree(parsedRows []newHandlerSMERow, submodelID string, idShortOrPath string) (types.ISubmodelElement, error) {
	nodes := make(map[int64]*newHandlerSMENode, len(parsedRows))
	children := make(map[int64][]*newHandlerSMENode, len(parsedRows))
	rootNodes := make([]*newHandlerSMENode, 0, 1)

	for _, item := range parsedRows {
		element, _, buildErr := builders.BuildSubmodelElement(item.row, h.database)
		if buildErr != nil {
			return nil, common.NewInternalServerError("SMREPO-NEWSM-GETSMEBYPATH-BUILDELEM " + buildErr.Error())
		}

		if semanticID, parseSemanticErr := common.ParseReferenceJSON(item.semanticPayload); parseSemanticErr != nil {
			return nil, parseSemanticErr
		} else if semanticID != nil {
			element.SetSemanticID(semanticID)
		} else {
			fallbackSemanticID, fallbackErr := h.getReferenceFromKeyTables(
				"submodel_element_semantic_id_reference",
				"submodel_element_semantic_id_reference_key",
				item.row.DbID.Int64,
				"SMREPO-NEWSM-GETSMEBYPATH-SEMKEYS",
			)
			if fallbackErr != nil {
				return nil, fallbackErr
			}
			if fallbackSemanticID != nil {
				element.SetSemanticID(fallbackSemanticID)
			}
		}

		if qualifiers, parseQualifiersErr := common.ParseQualifiersJSON(item.qualifiers); parseQualifiersErr != nil {
			return nil, parseQualifiersErr
		} else if len(qualifiers) > 0 {
			element.SetQualifiers(qualifiers)
		}

		n := &newHandlerSMENode{
			id:       item.row.DbID.Int64,
			parentID: item.row.ParentID,
			path:     item.row.IDShortPath,
			position: item.row.Position,
			element:  element,
		}
		nodes[n.id] = n
	}

	for _, n := range nodes {
		if n.parentID.Valid {
			if _, exists := nodes[n.parentID.Int64]; exists {
				children[n.parentID.Int64] = append(children[n.parentID.Int64], n)
				continue
			}
		}
		rootNodes = append(rootNodes, n)
	}

	attachSubmodelElementChildren(children, nodes)

	if len(rootNodes) == 0 {
		return nil, common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortOrPath + "' not found in submodel '" + submodelID + "'")
	}

	sort.SliceStable(rootNodes, func(i int, j int) bool {
		return rootNodes[i].path < rootNodes[j].path
	})

	return rootNodes[0].element, nil
}

func attachSubmodelElementChildren(children map[int64][]*newHandlerSMENode, nodes map[int64]*newHandlerSMENode) {
	for id, parent := range nodes {
		kids := children[id]
		if len(kids) == 0 {
			continue
		}

		sort.SliceStable(kids, func(i int, j int) bool {
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

func setCollectionChildren(parent types.ISubmodelElement, kids []*newHandlerSMENode) {
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

func setListChildren(parent types.ISubmodelElement, kids []*newHandlerSMENode) {
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

func setAnnotatedRelationshipChildren(parent types.ISubmodelElement, kids []*newHandlerSMENode) {
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

func setEntityChildren(parent types.ISubmodelElement, kids []*newHandlerSMENode) {
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

// CreateSubmodel inserts a Submodel with all persisted structures defined in the schema.
//
// Persisted data:
//   - Base Submodel data in submodel
//   - Submodel payload data in submodel_payload
//   - SemanticId linkage in submodel_semantic_id_reference* tables
//   - All SubmodelElements (all supported model types) via batched insertion
//
// The method is transaction-safe and can either use an existing transaction or create its own.
func (h *NewSMHandler) CreateSubmodel(submodelInterface types.ISubmodel, optionalTX *sql.Tx) (err error) {
	if submodelInterface == nil {
		return common.NewErrBadRequest("SMREPO-NEWSM-CREATESM-NILINPUT Submodel must not be nil")
	}

	submodel, ok := submodelInterface.(*types.Submodel)
	if !ok {
		return common.NewErrBadRequest("SMREPO-NEWSM-CREATESM-BADTYPE Submodel must be of type *types.Submodel")
	}

	var tx *sql.Tx
	startedTransaction := false
	if optionalTX != nil {
		tx = optionalTX
	} else {
		startedTX, cleanup, startTxErr := common.StartTransaction(h.database)
		if startTxErr != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATESM-STARTTX " + startTxErr.Error())
		}
		tx = startedTX
		startedTransaction = true
		defer cleanup(&err)
	}

	var submodelDatabaseID int64
	submodelDatabaseID, err = h.insertSubmodelBase(tx, submodel)
	if err != nil {
		return err
	}

	_, err = persistenceutils.CreateContextReferenceByOwnerID(tx, submodelDatabaseID, "submodel_semantic_id", submodel.SemanticID())
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATESM-CRSEMID " + err.Error())
	}

	var payloadRecord goqu.Record
	payloadRecord, err = h.buildSubmodelPayloadRecord(submodelDatabaseID, submodel)
	if err != nil {
		return err
	}

	insertPayload := goqu.
		Dialect("postgres").
		Insert("submodel_payload").
		Rows(payloadRecord)

	payloadSQLQuery, payloadArgs, err := insertPayload.ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATESM-BUILDPAYLOADQ " + err.Error())
	}

	_, err = tx.Exec(payloadSQLQuery, payloadArgs...)
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATESM-EXECPAYLOADQ " + err.Error())
	}

	if len(submodel.SubmodelElements()) > 0 {
		_, err = submodelelements.BatchInsert(h.database, submodel.ID(), submodel.SubmodelElements(), tx, nil)
		if err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATESM-BATCHINSERTSME " + err.Error())
		}
	}

	if startedTransaction {
		if commitErr := tx.Commit(); commitErr != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATESM-COMMITTX " + commitErr.Error())
		}
	}

	return nil
}

func (h *NewSMHandler) insertSubmodelBase(tx *sql.Tx, submodel *types.Submodel) (int64, error) {
	insertSubmodel := goqu.
		Dialect("postgres").
		Insert("submodel").
		Rows(goqu.Record{
			"submodel_identifier": submodel.ID(),
			"id_short":            submodel.IDShort(),
			"category":            submodel.Category(),
			"kind":                submodel.Kind(),
			"model_type":          types.ModelTypeSubmodel,
		}).
		OnConflict(goqu.DoNothing()).
		Returning(goqu.I("id"))

	sqlQuery, args, err := insertSubmodel.ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("SMREPO-NEWSM-INSBASE-BUILDQ " + err.Error())
	}

	var submodelDatabaseID int64
	err = tx.QueryRow(sqlQuery, args...).Scan(&submodelDatabaseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, common.NewErrConflict(fmt.Sprintf("SMREPO-NEWSM-INSBASE-CONFLICT Submodel with ID '%s' already exists", submodel.ID()))
		}
		return 0, common.NewInternalServerError("SMREPO-NEWSM-INSBASE-EXECQ " + err.Error())
	}

	return submodelDatabaseID, nil
}

func (h *NewSMHandler) buildSubmodelPayloadRecord(submodelDatabaseID int64, submodel *types.Submodel) (goqu.Record, error) {
	descriptionPayload, err := serializeIClassSliceToJSON(toIClassSlice(submodel.Description()), "SMREPO-NEWSM-PAYLOAD-DESC")
	if err != nil {
		return nil, err
	}

	displayNamePayload, err := serializeIClassSliceToJSON(toIClassSlice(submodel.DisplayName()), "SMREPO-NEWSM-PAYLOAD-DISPNAME")
	if err != nil {
		return nil, err
	}

	administrativeInformationPayload, err := serializeIClassToSingleEntryArrayJSON(submodel.Administration(), "SMREPO-NEWSM-PAYLOAD-ADMIN")
	if err != nil {
		return nil, err
	}

	embeddedDataSpecificationPayload, err := serializeIClassSliceToJSON(toIClassSlice(submodel.EmbeddedDataSpecifications()), "SMREPO-NEWSM-PAYLOAD-EDS")
	if err != nil {
		return nil, err
	}

	supplementalSemanticIDsPayload, err := serializeIClassSliceToJSON(toIClassSlice(submodel.SupplementalSemanticIDs()), "SMREPO-NEWSM-PAYLOAD-SUPPLSEM")
	if err != nil {
		return nil, err
	}

	extensionsPayload, err := serializeIClassSliceToJSON(toIClassSlice(submodel.Extensions()), "SMREPO-NEWSM-PAYLOAD-EXT")
	if err != nil {
		return nil, err
	}

	qualifiersPayload, err := serializeIClassSliceToJSON(toIClassSlice(submodel.Qualifiers()), "SMREPO-NEWSM-PAYLOAD-QUAL")
	if err != nil {
		return nil, err
	}

	return goqu.Record{
		"submodel_id":                         submodelDatabaseID,
		"description_payload":                 goqu.L("?::jsonb", descriptionPayload),
		"displayname_payload":                 goqu.L("?::jsonb", displayNamePayload),
		"administrative_information_payload":  goqu.L("?::jsonb", administrativeInformationPayload),
		"embedded_data_specification_payload": goqu.L("?::jsonb", embeddedDataSpecificationPayload),
		"supplemental_semantic_ids_payload":   goqu.L("?::jsonb", supplementalSemanticIDsPayload),
		"extensions_payload":                  goqu.L("?::jsonb", extensionsPayload),
		"qualifiers_payload":                  goqu.L("?::jsonb", qualifiersPayload),
	}, nil
}

func toIClassSlice[T types.IClass](items []T) []types.IClass {
	result := make([]types.IClass, len(items))
	for i, item := range items {
		result[i] = item
	}
	return result
}

func serializeIClassSliceToJSON(items []types.IClass, errCode string) (string, error) {
	if len(items) == 0 {
		return "[]", nil
	}

	toJSON := make([]map[string]any, 0, len(items))
	for _, item := range items {
		jsonObj, err := jsonization.ToJsonable(item)
		if err != nil {
			return "", common.NewErrBadRequest("Failed to convert object to jsonable - no changes applied - " + errCode)
		}
		toJSON = append(toJSON, jsonObj)
	}

	resBytes, err := json.Marshal(toJSON)
	if err != nil {
		return "", common.NewInternalServerError("SMREPO-NEWSM-SERIALIZE-JSONARRAY " + err.Error())
	}

	return string(resBytes), nil
}

func serializeIClassToSingleEntryArrayJSON(item types.IClass, errCode string) (string, error) {
	if item == nil {
		return "[]", nil
	}

	jsonObj, err := jsonization.ToJsonable(item)
	if err != nil {
		return "", common.NewErrBadRequest("Failed to convert object to jsonable - no changes applied - " + errCode)
	}

	resBytes, err := json.Marshal([]map[string]any{jsonObj})
	if err != nil {
		return "", common.NewInternalServerError("SMREPO-NEWSM-SERIALIZE-SINGLEARRAY " + err.Error())
	}

	return string(resBytes), nil
}

func (h *NewSMHandler) getReferenceFromKeyTables(referenceTable string, referenceKeyTable string, referenceID int64, errorCodePrefix string) (types.IReference, error) {
	typeQuery := fmt.Sprintf("SELECT type FROM %s WHERE id = $1", referenceTable)

	var referenceTypeInt int64
	err := h.database.QueryRow(typeQuery, referenceID).Scan(&referenceTypeInt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, common.NewInternalServerError(errorCodePrefix + "-READTYPE " + err.Error())
	}

	keysQuery := fmt.Sprintf("SELECT type, value FROM %s WHERE reference_id = $1 ORDER BY position ASC", referenceKeyTable)
	rows, queryErr := h.database.Query(keysQuery, referenceID)
	if queryErr != nil {
		return nil, common.NewInternalServerError(errorCodePrefix + "-READKEYS " + queryErr.Error())
	}
	defer func() { _ = rows.Close() }()

	keys := make([]types.IKey, 0, 4)
	for rows.Next() {
		var keyTypeInt int64
		var keyValue string
		if scanErr := rows.Scan(&keyTypeInt, &keyValue); scanErr != nil {
			return nil, common.NewInternalServerError(errorCodePrefix + "-SCANKEYS " + scanErr.Error())
		}

		key := types.Key{}
		keyType := types.KeyTypes(keyTypeInt)
		key.SetType(keyType)
		key.SetValue(keyValue)
		keys = append(keys, &key)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, common.NewInternalServerError(errorCodePrefix + "-ROWSKEYS " + rowsErr.Error())
	}

	if len(keys) == 0 {
		return nil, nil
	}

	reference := types.Reference{}
	referenceType := types.ReferenceTypes(referenceTypeInt)
	reference.SetType(referenceType)
	reference.SetKeys(keys)

	return &reference, nil
}

type submodelPayloadRow struct {
	dbID                           int64
	submodelID                     string
	idShort                        sql.NullString
	category                       sql.NullString
	kind                           sql.NullInt64
	descriptionPayload             []byte
	displayNamePayload             []byte
	administrationPayload          []byte
	edsPayload                     []byte
	supplementalSemanticIDsPayload []byte
	extensionsPayload              []byte
	qualifiersPayload              []byte
	semanticIDPayload              []byte
}

func buildSubmodelFromPayloads(payload submodelPayloadRow) (*types.Submodel, error) {
	result := types.NewSubmodel(payload.submodelID)
	if payload.idShort.Valid {
		result.SetIDShort(&payload.idShort.String)
	}
	if payload.category.Valid {
		result.SetCategory(&payload.category.String)
	}
	if payload.kind.Valid {
		modellingKind := types.ModellingKind(payload.kind.Int64)
		result.SetKind(&modellingKind)
	}

	if semanticID, parseSemanticErr := common.ParseReferenceJSON(payload.semanticIDPayload); parseSemanticErr != nil {
		return nil, parseSemanticErr
	} else if semanticID != nil {
		result.SetSemanticID(semanticID)
	}

	if descriptions, parseDescriptionsErr := common.ParseLangStringTextTypesJSON(payload.descriptionPayload); parseDescriptionsErr != nil {
		return nil, parseDescriptionsErr
	} else if len(descriptions) > 0 {
		result.SetDescription(descriptions)
	}

	if displayNames, parseDisplayNamesErr := common.ParseLangStringNameTypesJSON(payload.displayNamePayload); parseDisplayNamesErr != nil {
		return nil, parseDisplayNamesErr
	} else if len(displayNames) > 0 {
		result.SetDisplayName(displayNames)
	}

	if administration, parseAdministrationErr := common.ParseAdministrationJSON(payload.administrationPayload); parseAdministrationErr != nil {
		return nil, parseAdministrationErr
	} else if administration != nil {
		result.SetAdministration(administration)
	}

	if eds, parseEDSErr := common.ParseEmbeddedDataSpecificationsJSON(payload.edsPayload); parseEDSErr != nil {
		return nil, parseEDSErr
	} else if len(eds) > 0 {
		result.SetEmbeddedDataSpecifications(eds)
	}

	if supplementalSemanticIDs, parseSupplementalErr := common.ParseReferencesJSONArray(payload.supplementalSemanticIDsPayload); parseSupplementalErr != nil {
		return nil, parseSupplementalErr
	} else if len(supplementalSemanticIDs) > 0 {
		result.SetSupplementalSemanticIDs(supplementalSemanticIDs)
	}

	if extensions, parseExtensionsErr := common.ParseExtensionsJSON(payload.extensionsPayload); parseExtensionsErr != nil {
		return nil, parseExtensionsErr
	} else if len(extensions) > 0 {
		result.SetExtensions(extensions)
	}

	if qualifiers, parseQualifiersErr := common.ParseQualifiersJSON(payload.qualifiersPayload); parseQualifiersErr != nil {
		return nil, parseQualifiersErr
	} else if len(qualifiers) > 0 {
		result.SetQualifiers(qualifiers)
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

func getSMEValueExpressionForNewHandler(dialect goqu.DialectWrapper) exp.CaseExpression {
	return goqu.Case().
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeAnnotatedRelationshipElement),
			dialect.From(goqu.T("annotated_relationship_element").As("are")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("first"), goqu.I("are.first"),
					goqu.V("second"), goqu.I("are.second"),
				)).
				Where(goqu.I("are.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeBasicEventElement),
			dialect.From(goqu.T("basic_event_element").As("bee")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("direction"), goqu.I("bee.direction"),
					goqu.V("state"), goqu.I("bee.state"),
					goqu.V("message_topic"), goqu.I("bee.message_topic"),
					goqu.V("last_update"), goqu.I("bee.last_update"),
					goqu.V("min_interval"), goqu.I("bee.min_interval"),
					goqu.V("max_interval"), goqu.I("bee.max_interval"),
					goqu.V("observed"), goqu.I("bee.observed"),
					goqu.V("message_broker"), goqu.I("bee.message_broker"),
				)).
				Where(goqu.I("bee.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeBlob),
			dialect.From(goqu.T("blob_element").As("be")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("content_type"), goqu.I("be.content_type"),
					goqu.V("value"), goqu.I("be.value"),
				)).
				Where(goqu.I("be.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeEntity),
			dialect.From(goqu.T("entity_element").As("ee")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("entity_type"), goqu.I("ee.entity_type"),
					goqu.V("global_asset_id"), goqu.I("ee.global_asset_id"),
					goqu.V("specific_asset_ids"), goqu.I("ee.specific_asset_ids"),
				)).
				Where(goqu.I("ee.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeFile),
			dialect.From(goqu.T("file_element").As("fe")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("value"), goqu.I("fe.value"),
					goqu.V("content_type"), goqu.I("fe.content_type"),
				)).
				Where(goqu.I("fe.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeSubmodelElementList),
			dialect.From(goqu.T("submodel_element_list").As("sel")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("order_relevant"), goqu.I("sel.order_relevant"),
					goqu.V("type_value_list_element"), goqu.I("sel.type_value_list_element"),
					goqu.V("value_type_list_element"), goqu.I("sel.value_type_list_element"),
					goqu.V("semantic_id_list_element"), goqu.I("sel.semantic_id_list_element"),
				)).
				Where(goqu.I("sel.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeMultiLanguageProperty),
			dialect.From(goqu.T("multilanguage_property").As("mlp")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("value_id"), goqu.COALESCE(goqu.I("mlp.value_id_payload"), goqu.L("'[]'::jsonb")),
					goqu.V("value_id_referred"), goqu.L("'[]'::jsonb"),
					goqu.V("value"),
					dialect.From(goqu.T("multilanguage_property_value").As("mlpv")).
						Select(goqu.Func("jsonb_agg", goqu.Func("jsonb_build_object",
							goqu.V("language"), goqu.I("mlpv.language"),
							goqu.V("text"), goqu.I("mlpv.text"),
							goqu.V("id"), goqu.I("mlpv.id"),
						))).
						Where(goqu.I("mlpv.mlp_id").Eq(goqu.I("sme.id"))),
				)).
				Where(goqu.I("mlp.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeOperation),
			dialect.From(goqu.T("operation_element").As("oe")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("input_variables"), goqu.I("oe.input_variables"),
					goqu.V("output_variables"), goqu.I("oe.output_variables"),
					goqu.V("inoutput_variables"), goqu.I("oe.inoutput_variables"),
				)).
				Where(goqu.I("oe.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeProperty),
			dialect.From(goqu.T("property_element").As("pe")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("value"), goqu.COALESCE(
						goqu.I("pe.value_text"),
						goqu.L("?::text", goqu.I("pe.value_num")),
						goqu.L("?::text", goqu.I("pe.value_bool")),
						goqu.L("?::text", goqu.I("pe.value_time")),
						goqu.L("?::text", goqu.I("pe.value_date")),
						goqu.L("?::text", goqu.I("pe.value_datetime")),
					),
					goqu.V("value_type"), goqu.I("pe.value_type"),
					goqu.V("value_id"), goqu.COALESCE(
						dialect.From(goqu.T("property_element_payload").As("pep")).
							Select(goqu.I("pep.value_id_payload")).
							Where(goqu.I("pep.property_element_id").Eq(goqu.I("sme.id"))).
							Limit(1),
						goqu.L("'[]'::jsonb"),
					),
					goqu.V("value_id_referred"), goqu.L("'[]'::jsonb"),
				)).
				Where(goqu.I("pe.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeRange),
			dialect.From(goqu.T("range_element").As("re")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("value_type"), goqu.I("re.value_type"),
					goqu.V("min"), goqu.COALESCE(
						goqu.I("re.min_text"),
						goqu.L("?::text", goqu.I("re.min_num")),
						goqu.L("?::text", goqu.I("re.min_time")),
						goqu.L("?::text", goqu.I("re.min_date")),
						goqu.L("?::text", goqu.I("re.min_datetime")),
					),
					goqu.V("max"), goqu.COALESCE(
						goqu.I("re.max_text"),
						goqu.L("?::text", goqu.I("re.max_num")),
						goqu.L("?::text", goqu.I("re.max_time")),
						goqu.L("?::text", goqu.I("re.max_date")),
						goqu.L("?::text", goqu.I("re.max_datetime")),
					),
				)).
				Where(goqu.I("re.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeReferenceElement),
			dialect.From(goqu.T("reference_element").As("refe")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("value"), goqu.I("refe.value"),
				)).
				Where(goqu.I("refe.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		When(
			goqu.I("sme.model_type").Eq(types.ModelTypeRelationshipElement),
			dialect.From(goqu.T("relationship_element").As("rle")).
				Select(goqu.Func("jsonb_build_object",
					goqu.V("first"), goqu.I("rle.first"),
					goqu.V("second"), goqu.I("rle.second"),
				)).
				Where(goqu.I("rle.id").Eq(goqu.I("sme.id"))).
				Limit(1),
		).
		Else(goqu.V(nil))
}
