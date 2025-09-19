package submodelelements

import (
	"database/sql"
	"errors"
	"sort"
	"strconv"

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
			return nil, err
		}
		handler = areHandler
	case "BasicEventElement":
		beeHandler, err := NewPostgreSQLBasicEventElementHandler(db)
		if err != nil {
			return nil, err
		}
		handler = beeHandler
	case "Blob":
		blobHandler, err := NewPostgreSQLBlobHandler(db)
		if err != nil {
			return nil, err
		}
		handler = blobHandler
	case "Capability":
		capHandler, err := NewPostgreSQLCapabilityHandler(db)
		if err != nil {
			return nil, err
		}
		handler = capHandler
	case "DataElement":
		deHandler, err := NewPostgreSQLDataElementHandler(db)
		if err != nil {
			return nil, err
		}
		handler = deHandler
	case "Entity":
		entityHandler, err := NewPostgreSQLEntityHandler(db)
		if err != nil {
			return nil, err
		}
		handler = entityHandler
	case "EventElement":
		eventElemHandler, err := NewPostgreSQLEventElementHandler(db)
		if err != nil {
			return nil, err
		}
		handler = eventElemHandler
	case "File":
		fileHandler, err := NewPostgreSQLFileHandler(db)
		if err != nil {
			return nil, err
		}
		handler = fileHandler
	case "MultiLanguageProperty":
		mlpHandler, err := NewPostgreSQLMultiLanguagePropertyHandler(db)
		if err != nil {
			return nil, err
		}
		handler = mlpHandler
	case "Operation":
		opHandler, err := NewPostgreSQLOperationHandler(db)
		if err != nil {
			return nil, err
		}
		handler = opHandler
	case "Property":
		propHandler, err := NewPostgreSQLPropertyHandler(db)
		if err != nil {
			return nil, err
		}
		handler = propHandler
	case "Range":
		rangeHandler, err := NewPostgreSQLRangeHandler(db)
		if err != nil {
			return nil, err
		}
		handler = rangeHandler
	case "ReferenceElement":
		refElemHandler, err := NewPostgreSQLReferenceElementHandler(db)
		if err != nil {
			return nil, err
		}
		handler = refElemHandler
	case "RelationshipElement":
		relElemHandler, err := NewPostgreSQLRelationshipElementHandler(db)
		if err != nil {
			return nil, err
		}
		handler = relElemHandler
	case "SubmodelElementCollection":
		smeColHandler, err := NewPostgreSQLSubmodelElementCollectionHandler(db)
		if err != nil {
			return nil, err
		}
		handler = smeColHandler
	case "SubmodelElementList":
		smeListHandler, err := NewPostgreSQLSubmodelElementListHandler(db)
		if err != nil {
			return nil, err
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
func GetSubmodelElementsWithPath(tx *sql.Tx, submodelId string, idShortOrPath string) ([]gen.SubmodelElement, error) {

	//Check if Submodel exists
	sRows, err := tx.Query(`SELECT id FROM submodel WHERE id = $1`, submodelId)
	if err != nil {
		return nil, err
	}
	if !sRows.Next() {
		return nil, common.NewErrNotFound("Submodel not found")
	}
	sRows.Close()

	// --- Build query ----------------------------------------------------------
	// NOTE: we keep a single query, but order by parent first for better locality
	// and easy root detection. We also drop joins we don't use (e.g. MultiLanguageProperty row id).
	baseQuery := `
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
			sme_list.type_value_list_element, sme_list.value_type_list_element, sme_list.order_relevant
		FROM submodel_element sme
		LEFT JOIN property_element prop ON sme.id = prop.id
		LEFT JOIN blob_element blob ON sme.id = blob.id
		LEFT JOIN file_element file ON sme.id = file.id
		LEFT JOIN range_element range_elem ON sme.id = range_elem.id
		LEFT JOIN submodel_element_list sme_list ON sme.id = sme_list.id
		WHERE sme.submodel_id = $1`

	args := []any{submodelId}
	if idShortOrPath != "" {
		baseQuery += ` AND (sme.idshort_path = $2 OR sme.idshort_path LIKE $2 || '.%' OR sme.idshort_path LIKE $2 || '[%')`
		args = append(args, idShortOrPath)
	}

	baseQuery += ` ORDER BY sme.parent_sme_id NULLS FIRST, sme.idshort_path, sme.position`

	rows, err := tx.Query(baseQuery, args...)
	if err != nil {
		return nil, err
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
		)

		if err := rows.Scan(
			&id, &idShort, &category, &modelType, &idShortPath, &position, &parentSmeID,
			&propValueType, &propValue,
			&blobContentType, &blobValue,
			&fileContentType, &fileValue,
			&rangeValueType, &rangeMin, &rangeMax,
			&typeValueListElement, &valueTypeListElement, &orderRelevant,
		); err != nil {
			return nil, err
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
				//TODO: Ignore for now
				// blob.ContentType = gen.BlobAllOfContentType(blobContentType.String)
			}
			if blobValue != nil {
				blob.Value = string(blobValue)
			}
			el = blob

		case "File":
			file := &gen.File{IdShort: idShort, Category: category, ModelType: modelType}
			if fileContentType.Valid {
				//TODO: Ignore for now
				// file.ContentType = gen.FileAllOfContentType(fileContentType.String)
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
			roots = append(roots, n)
		}

		if idShortOrPath != "" && idShortPath == idShortOrPath {
			target = n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
		return []gen.SubmodelElement{target.element}, nil
	}

	res := make([]gen.SubmodelElement, 0, len(roots))
	for _, r := range roots {
		res = append(res, r.element)
	}
	return res, nil
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
