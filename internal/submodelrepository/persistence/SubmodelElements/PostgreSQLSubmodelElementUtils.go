package submodelelements

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
)

func GetSMEHandler(submodelElement gen.SubmodelElement, db *sql.DB) (PostgreSQLSMECrudInterface, error) {
	return GetSMEHandlerByModelType(string(submodelElement.GetModelType()), db)
}

func GetSMEHandlerByModelType(modelType string, db *sql.DB) (PostgreSQLSMECrudInterface, error) {
	var handler PostgreSQLSMECrudInterface

	switch modelType {
	case "AnnotatedRelationshipElement":
		areHandler, err := NewPostgreSQLAnnotatedRelationshipElementHandler(db)
		if err != nil {
			fmt.Println("Error creating AnnotatedRelationshipElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create AnnotatedRelationshipElement handler. See console for details.")
		}
		handler = areHandler
	case "BasicEventElement":
		beeHandler, err := NewPostgreSQLBasicEventElementHandler(db)
		if err != nil {
			fmt.Println("Error creating BasicEventElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create BasicEventElement handler. See console for details.")
		}
		handler = beeHandler
	case "Blob":
		blobHandler, err := NewPostgreSQLBlobHandler(db)
		if err != nil {
			fmt.Println("Error creating Blob handler:", err)
			return nil, common.NewInternalServerError("Failed to create Blob handler. See console for details.")
		}
		handler = blobHandler
	case "Capability":
		capHandler, err := NewPostgreSQLCapabilityHandler(db)
		if err != nil {
			fmt.Println("Error creating Capability handler:", err)
			return nil, common.NewInternalServerError("Failed to create Capability handler. See console for details.")
		}
		handler = capHandler
	case "DataElement":
		deHandler, err := NewPostgreSQLDataElementHandler(db)
		if err != nil {
			fmt.Println("Error creating DataElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create DataElement handler. See console for details.")
		}
		handler = deHandler
	case "Entity":
		entityHandler, err := NewPostgreSQLEntityHandler(db)
		if err != nil {
			fmt.Println("Error creating Entity handler:", err)
			return nil, common.NewInternalServerError("Failed to create Entity handler. See console for details.")
		}
		handler = entityHandler
	case "EventElement":
		eventElemHandler, err := NewPostgreSQLEventElementHandler(db)
		if err != nil {
			fmt.Println("Error creating EventElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create EventElement handler. See console for details.")
		}
		handler = eventElemHandler
	case "File":
		fileHandler, err := NewPostgreSQLFileHandler(db)
		if err != nil {
			fmt.Println("Error creating File handler:", err)
			return nil, common.NewInternalServerError("Failed to create File handler. See console for details.")
		}
		handler = fileHandler
	case "MultiLanguageProperty":
		mlpHandler, err := NewPostgreSQLMultiLanguagePropertyHandler(db)
		if err != nil {
			fmt.Println("Error creating MultiLanguageProperty handler:", err)
			return nil, common.NewInternalServerError("Failed to create MultiLanguageProperty handler. See console for details.")
		}
		handler = mlpHandler
	case "Operation":
		opHandler, err := NewPostgreSQLOperationHandler(db)
		if err != nil {
			fmt.Println("Error creating Operation handler:", err)
			return nil, common.NewInternalServerError("Failed to create Operation handler. See console for details.")
		}
		handler = opHandler
	case "Property":
		propHandler, err := NewPostgreSQLPropertyHandler(db)
		if err != nil {
			fmt.Println("Error creating Property handler:", err)
			return nil, common.NewInternalServerError("Failed to create Property handler. See console for details.")
		}
		handler = propHandler
	case "Range":
		rangeHandler, err := NewPostgreSQLRangeHandler(db)
		if err != nil {
			fmt.Println("Error creating Range handler:", err)
			return nil, common.NewInternalServerError("Failed to create Range handler. See console for details.")
		}
		handler = rangeHandler
	case "ReferenceElement":
		refElemHandler, err := NewPostgreSQLReferenceElementHandler(db)
		if err != nil {
			fmt.Println("Error creating ReferenceElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create ReferenceElement handler. See console for details.")
		}
		handler = refElemHandler
	case "RelationshipElement":
		relElemHandler, err := NewPostgreSQLRelationshipElementHandler(db)
		if err != nil {
			fmt.Println("Error creating RelationshipElement handler:", err)
			return nil, common.NewInternalServerError("Failed to create RelationshipElement handler. See console for details.")
		}
		handler = relElemHandler
	case "SubmodelElementCollection":
		smeColHandler, err := NewPostgreSQLSubmodelElementCollectionHandler(db)
		if err != nil {
			fmt.Println("Error creating SubmodelElementCollection handler:", err)
			return nil, common.NewInternalServerError("Failed to create SubmodelElementCollection handler. See console for details.")
		}
		handler = smeColHandler
	case "SubmodelElementList":
		smeListHandler, err := NewPostgreSQLSubmodelElementListHandler(db)
		if err != nil {
			fmt.Println("Error creating SubmodelElementList handler:", err)
			return nil, common.NewInternalServerError("Failed to create SubmodelElementList handler. See console for details.")
		}
		handler = smeListHandler
	default:
		return nil, errors.New("ModelType " + modelType + " unsupported.")
	}
	return handler, nil
}

// GetSubmodelElementsWithPath loads all SubmodelElements for a Submodel (optionally a subtree
// identified by idShortOrPath) in one query and reconstructs the hierarchy using parent_sme_id
// (O(n)), avoiding expensive string parsing of idshort_path. It also minimizes allocations
// by using integer IDs and on-the-fly child bucketing.
func GetSubmodelElementsWithPath(tx *sql.Tx, submodelId string, idShortOrPath string, limit int, cursor string) ([]gen.SubmodelElement, string, error) {
	if limit < 1 {
		limit = 100
	}
	//Check if Submodel exists
	sRows, err := tx.Query(`SELECT id FROM submodel WHERE id = $1`, submodelId)
	if err != nil {
		return nil, "", err
	}
	if !sRows.Next() {
		return nil, "", common.NewErrNotFound("Submodel not found")
	}
	sRows.Close()

	// Get OFFSET based on Cursor
	offset := 0
	if cursor != "" {
		query := `SELECT ROW_NUMBER() OVER (ORDER BY id_short) AS position, id_short
    FROM submodel_element
    WHERE submodel_id = $1 AND parent_sme_id IS NULL`
		cRows, err := tx.Query(query, submodelId)
		if err != nil {
			return nil, "", err
		}
		defer cRows.Close()
		found := false
		for cRows.Next() {
			var position int
			var idShort string
			if err := cRows.Scan(&position, &idShort); err != nil {
				return nil, "", err
			}
			if idShort == cursor {
				found = true
				offset = position
				// Continue consuming all rows to avoid leftovers
			}
		}
		if err := cRows.Err(); err != nil {
			return nil, "", err
		}
		if !found {
			return nil, "", common.NewErrBadRequest("Invalid cursor " + cursor)
		}
	}

	// --- Build the unified query with CTE ----------------------------------------------------------
	var cte string
	args := []any{submodelId}
	if idShortOrPath != "" {
		// Subtree: Fetch all elements in the subtree
		cte = `
            WITH subtree AS (
                SELECT * FROM submodel_element
                WHERE submodel_id = $1 AND (idshort_path = $2 OR idshort_path LIKE $2 || '.%' OR idshort_path LIKE $2 || '[%')
            )`
		args = append(args, idShortOrPath)
	} else {
		// Pagination: Fetch paginated roots, then their subtrees
		cte = `
			WITH paginated_roots AS (
				SELECT id_short FROM submodel_element
				WHERE submodel_id = $1 AND parent_sme_id IS NULL
				ORDER BY id_short OFFSET $2 LIMIT $3
			),
			subtree_elements AS (
				SELECT sme.* FROM submodel_element sme
				WHERE sme.submodel_id = $1 AND EXISTS (
					SELECT 1 FROM paginated_roots pr
					WHERE sme.idshort_path = pr.id_short
					   OR sme.idshort_path LIKE pr.id_short || '.%'
					   OR sme.idshort_path LIKE pr.id_short || '[%'
				)
			)`
		args = append(args, offset, limit)
	}

	baseQuery := cte + `
        SELECT 
            -- SME base
            sme.id, sme.id_short, sme.category, sme.model_type, sme.idshort_path, sme.position, sme.parent_sme_id,
            -- Property data
            prop.value_type as prop_value_type,
            COALESCE(prop.value_text, prop.value_num::text, prop.value_bool::text, prop.value_time::text, prop.value_datetime::text) as prop_value,
            -- Blob data
            blob.content_type as blob_content_type, blob.value as blob_value,
            -- File data
            file.content_type as file_content_type, file.value as file_value,
            -- Range data
            range_elem.value_type as range_value_type,
            COALESCE(range_elem.min_text, range_elem.min_num::text, range_elem.min_time::text, range_elem.min_datetime::text) as range_min,
            COALESCE(range_elem.max_text, range_elem.max_num::text, range_elem.max_time::text, range_elem.max_datetime::text) as range_max,
            -- SubmodelElementList data
            sme_list.type_value_list_element, sme_list.value_type_list_element, sme_list.order_relevant,
            -- MultiLanguageProperty data
            mlp.value_id as mlp_value_id,
            -- ReferenceElement data
            ref_elem.value_ref as ref_value_ref,
            -- RelationshipElement data
            rel_elem.first_ref as rel_first_ref, rel_elem.second_ref as rel_second_ref,
            -- Entity data
            entity.entity_type as entity_type, entity.global_asset_id as entity_global_asset_id,
            -- BasicEventElement data
            bee.observed_ref as bee_observed_ref, bee.direction as bee_direction, bee.state as bee_state, bee.message_topic as bee_message_topic, bee.message_broker_ref as bee_message_broker_ref, bee.last_update as bee_last_update, bee.min_interval as bee_min_interval, bee.max_interval as bee_max_interval
        FROM ` + (func() string {
		if idShortOrPath != "" {
			return "subtree sme"
		}
		return "subtree_elements sme"
	}()) + `
        LEFT JOIN property_element prop ON sme.id = prop.id
        LEFT JOIN blob_element blob ON sme.id = blob.id
        LEFT JOIN file_element file ON sme.id = file.id
        LEFT JOIN range_element range_elem ON sme.id = range_elem.id
        LEFT JOIN submodel_element_list sme_list ON sme.id = sme_list.id
        LEFT JOIN multilanguage_property mlp ON sme.id = mlp.id
        LEFT JOIN reference_element ref_elem ON sme.id = ref_elem.id
        LEFT JOIN relationship_element rel_elem ON sme.id = rel_elem.id
        LEFT JOIN entity_element entity ON sme.id = entity.id
        LEFT JOIN basic_event_element bee ON sme.id = bee.id
        ORDER BY sme.parent_sme_id NULLS FIRST, sme.idshort_path, sme.position`

	rows, err := tx.Query(baseQuery, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	// --- Working structs ------------------------------------------------------
	type node struct {
		id       int64
		parentID sql.NullInt64
		path     string
		position sql.NullInt32
		element  gen.SubmodelElement
	}

	// Pre-size conservatively to reduce re-allocations
	nodes := make(map[int64]*node, 256)
	children := make(map[int64][]*node, 256)
	roots := make([]*node, 0, 16)
	var target *node // element whose idshort_path == idShortOrPath (if provided)

	for rows.Next() {
		var (
			// SME base
			id                                        int64
			idShort, category, modelType, idShortPath string
			position                                  sql.NullInt32
			parentSmeID                               sql.NullInt64
			// Property
			propValueType, propValue sql.NullString
			// Blob
			blobContentType sql.NullString
			blobValue       []byte
			// File
			fileContentType, fileValue sql.NullString
			// Range
			rangeValueType, rangeMin, rangeMax sql.NullString
			// SubmodelElementList
			typeValueListElement, valueTypeListElement sql.NullString
			orderRelevant                              sql.NullBool
			// MultiLanguageProperty
			mlpValueId sql.NullInt64
			// ReferenceElement
			refValueRef sql.NullInt64
			// RelationshipElement
			relFirstRef, relSecondRef sql.NullInt64
			// Entity
			entityType          sql.NullString
			entityGlobalAssetId sql.NullString
			// BasicEventElement
			beeObservedRef      sql.NullInt64
			beeDirection        sql.NullString
			beeState            sql.NullString
			beeMessageTopic     sql.NullString
			beeMessageBrokerRef sql.NullInt64
			beeLastUpdate       sql.NullTime
			beeMinInterval      sql.NullString
			beeMaxInterval      sql.NullString
		)

		if err := rows.Scan(
			&id, &idShort, &category, &modelType, &idShortPath, &position, &parentSmeID,
			&propValueType, &propValue,
			&blobContentType, &blobValue,
			&fileContentType, &fileValue,
			&rangeValueType, &rangeMin, &rangeMax,
			&typeValueListElement, &valueTypeListElement, &orderRelevant,
			&mlpValueId,
			&refValueRef,
			&relFirstRef, &relSecondRef,
			&entityType, &entityGlobalAssetId,
			&beeObservedRef, &beeDirection, &beeState, &beeMessageTopic, &beeMessageBrokerRef, &beeLastUpdate, &beeMinInterval, &beeMaxInterval,
		); err != nil {
			return nil, "", err
		}

		// Materialize the concrete element based on modelType (no reflection)
		var el gen.SubmodelElement
		switch modelType {
		case "Property":
			prop := &gen.Property{IdShort: idShort, Category: category, ModelType: modelType}
			if propValueType.Valid {
				if vt, err := gen.NewDataTypeDefXsdFromValue(propValueType.String); err == nil {
					prop.ValueType = vt
				}
			}
			if propValue.Valid {
				prop.Value = propValue.String
			}
			el = prop

		case "MultiLanguageProperty":
			// Values are in a separate table; we only hydrate the shell here.
			mlp := &gen.MultiLanguageProperty{IdShort: idShort, Category: category, ModelType: modelType}
			el = mlp

		case "Blob":
			blob := &gen.Blob{IdShort: idShort, Category: category, ModelType: modelType}
			if blobContentType.Valid {
				blob.ContentType = blobContentType.String
			}
			if blobValue != nil {
				blob.Value = string(blobValue)
			}
			el = blob

		case "File":
			file := &gen.File{IdShort: idShort, Category: category, ModelType: modelType}
			if fileContentType.Valid {
				file.ContentType = fileContentType.String
			}
			if fileValue.Valid {
				file.Value = fileValue.String
			}
			el = file

		case "Range":
			rg := &gen.Range{IdShort: idShort, Category: category, ModelType: modelType}
			if rangeValueType.Valid {
				if vt, err := gen.NewDataTypeDefXsdFromValue(rangeValueType.String); err == nil {
					rg.ValueType = vt
				}
			}
			if rangeMin.Valid {
				rg.Min = rangeMin.String
			}
			if rangeMax.Valid {
				rg.Max = rangeMax.String
			}
			el = rg

		case "SubmodelElementCollection":
			coll := &gen.SubmodelElementCollection{IdShort: idShort, Category: category, ModelType: modelType, Value: make([]gen.SubmodelElement, 0, 4)}
			el = coll

		case "SubmodelElementList":
			lst := &gen.SubmodelElementList{IdShort: idShort, Category: category, ModelType: modelType, Value: make([]gen.SubmodelElement, 0, 4)}
			if typeValueListElement.Valid {
				if tv, err := gen.NewAasSubmodelElementsFromValue(typeValueListElement.String); err == nil {
					lst.TypeValueListElement = &tv
				}
			}
			if valueTypeListElement.Valid {
				if vt, err := gen.NewDataTypeDefXsdFromValue(valueTypeListElement.String); err == nil {
					lst.ValueTypeListElement = vt
				}
			}
			if orderRelevant.Valid {
				lst.OrderRelevant = orderRelevant.Bool
			}
			el = lst

		case "ReferenceElement":
			refElem := &gen.ReferenceElement{IdShort: idShort, Category: category, ModelType: modelType}
			el = refElem

		case "RelationshipElement":
			relElem := &gen.RelationshipElement{IdShort: idShort, Category: category, ModelType: modelType}
			el = relElem

		case "AnnotatedRelationshipElement":
			areElem := &gen.AnnotatedRelationshipElement{IdShort: idShort, Category: category, ModelType: modelType}
			el = areElem

		case "Entity":
			entity := &gen.Entity{IdShort: idShort, Category: category, ModelType: modelType}
			if entityType.Valid {
				if et, err := gen.NewEntityTypeFromValue(entityType.String); err == nil {
					entity.EntityType = et
				}
			}
			if entityGlobalAssetId.Valid {
				entity.GlobalAssetId = entityGlobalAssetId.String
			}
			el = entity

		case "Operation":
			op := &gen.Operation{IdShort: idShort, Category: category, ModelType: modelType}
			el = op

		case "BasicEventElement":
			bee := &gen.BasicEventElement{IdShort: idShort, Category: category, ModelType: modelType}
			if beeDirection.Valid {
				if d, err := gen.NewDirectionFromValue(beeDirection.String); err == nil {
					bee.Direction = d
				}
			}
			if beeState.Valid {
				if s, err := gen.NewStateOfEventFromValue(beeState.String); err == nil {
					bee.State = s
				}
			}
			if beeMessageTopic.Valid {
				bee.MessageTopic = beeMessageTopic.String
			}
			if beeLastUpdate.Valid {
				bee.LastUpdate = beeLastUpdate.Time.Format(time.RFC3339)
			}
			// Intervals not set for now
			el = bee

		case "Capability":
			cap := &gen.Capability{IdShort: idShort, Category: category, ModelType: modelType}
			el = cap

		default:
			// Unknown/unsupported type: skip eagerly.
			continue
		}

		n := &node{
			id:       id,
			parentID: parentSmeID,
			path:     idShortPath,
			position: position,
			element:  el,
		}
		nodes[id] = n

		if parentSmeID.Valid {
			pid := parentSmeID.Int64
			children[pid] = append(children[pid], n)
		} else {
			// For both subtree and pagination, roots are elements with no parent in the fetched data
			roots = append(roots, n)
		}

		if idShortOrPath != "" && idShortPath == idShortOrPath {
			target = n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	// --- Attach children (O(n)) ----------------------------------------------
	// We only attach to SMC/SML parents; other types cannot have children.
	for id, parent := range nodes {
		kids := children[id]
		if len(kids) == 0 {
			continue
		}

		// Stable order: by position (if set), otherwise by path as tie-breaker
		sort.SliceStable(kids, func(i, j int) bool {
			a, b := kids[i], kids[j]
			switch {
			case a.position.Valid && b.position.Valid:
				if a.position.Int32 != b.position.Int32 {
					return a.position.Int32 < b.position.Int32
				}
				return a.path < b.path
			case a.position.Valid:
				return true
			case b.position.Valid:
				return false
			default:
				return a.path < b.path
			}
		})

		switch p := parent.element.(type) {
		case *gen.SubmodelElementCollection:
			for _, ch := range kids {
				p.Value = append(p.Value, ch.element)
			}
		case *gen.SubmodelElementList:
			for _, ch := range kids {
				p.Value = append(p.Value, ch.element)
			}
		}
	}

	// --- Build result ---------------------------------------------------------
	if idShortOrPath != "" && target != nil {
		return []gen.SubmodelElement{target.element}, "", nil
	}

	res := make([]gen.SubmodelElement, 0, len(roots))
	for _, r := range roots {
		res = append(res, r.element)
	}

	if len(res) == 0 {
		return res, "", nil
	}
	// return idShort of last element in res as next cursor
	return res, res[len(res)-1].GetIdShort(), nil
}

// This method removes a SubmodelElement by its idShort or path and all its nested elements
// If the deleted Element is in a SubmodelElementList, the indices of the remaining elements are adjusted accordingly
func DeleteSubmodelElementByPath(tx *sql.Tx, submodelId string, idShortOrPath string) error {
	query := `DELETE FROM submodel_element WHERE submodel_id = $1 AND (idshort_path = $2 OR idshort_path LIKE $2 || '.%' OR idshort_path LIKE $2 || '[%')`
	result, err := tx.Exec(query, submodelId, idShortOrPath)
	if err != nil {
		return err
	}
	affectedRows, err := result.RowsAffected()
	//if idShortPath ends with ] it is part of a SubmodelElementList and we need to update the indices of the remaining elements
	if idShortOrPath[len(idShortOrPath)-1] == ']' {
		//extract the parent path and the index of the deleted element
		var parentPath string
		var deletedIndex int
		for i := len(idShortOrPath) - 1; i >= 0; i-- {
			if idShortOrPath[i] == '[' {
				parentPath = idShortOrPath[:i]
				indexStr := idShortOrPath[i+1 : len(idShortOrPath)-1]
				var err error
				deletedIndex, err = strconv.Atoi(indexStr)
				if err != nil {
					return err
				}
				break
			}
		}

		//get the id of the parent SubmodelElementList
		var parentId int
		err = tx.QueryRow(`SELECT id FROM submodel_element WHERE submodel_id = $1 AND idshort_path = $2`, submodelId, parentPath).Scan(&parentId)
		if err != nil {
			return err
		}

		//update the indices of the remaining elements in the SubmodelElementList
		updateQuery := `UPDATE submodel_element SET position = position - 1 WHERE parent_sme_id = $1 AND position > $2`
		_, err = tx.Exec(updateQuery, parentId, deletedIndex)
		if err != nil {
			return err
		}
		// update their idshort_path as well
		updatePathQuery := `UPDATE submodel_element SET idshort_path = regexp_replace(idshort_path, '\[' || (position + 1) || '\]', '[' || position || ']') WHERE parent_sme_id = $1 AND position >= $2`
		_, err = tx.Exec(updatePathQuery, parentId, deletedIndex)
		if err != nil {
			return err
		}
	}
	if affectedRows == 0 {
		return common.NewErrNotFound("Submodel-Element ID-Short: " + idShortOrPath)
	}
	return nil
}
