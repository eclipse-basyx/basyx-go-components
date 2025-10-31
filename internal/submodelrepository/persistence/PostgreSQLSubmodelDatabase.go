/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// Author: Prajwala Prabhakar Adiga ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )
package persistence_postgresql

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	_ "github.com/lib/pq" // PostgreSQL Treiber

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/SubmodelElements"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

type PostgreSQLSubmodelDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

var failedPostgresTransactionSubmodelRepo = common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")
var beginTransactionErrorSubmodelRepo = common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")

var maxCacheSize = 1000

// InMemory Cache for submodels
var submodelCache map[string]gen.Submodel = make(map[string]gen.Submodel)

func NewPostgreSQLSubmodelBackend(dsn string, maxOpenConns, maxIdleConns int, connMaxLifetimeMinutes int, cacheEnabled bool, databaseSchema string) (*PostgreSQLSubmodelDatabase, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLSubmodelDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}

func (p *PostgreSQLSubmodelDatabase) GetDB() *sql.DB {
	return p.db
}

// GetAllSubmodels and a next cursor ("" if no more pages).
func (p *PostgreSQLSubmodelDatabase) GetAllSubmodels(limit int32, cursor string, idShort string) ([]gen.Submodel, string, error) {
	tx, err := p.db.Begin()
	if limit <= 0 {
		limit = 100
	}

	if err != nil {
		return nil, "", beginTransactionErrorSubmodelRepo
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	sm, err := submodelelements.GetSubmodelWithSubmodelElementsOrAll(p.db, tx)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		return nil, "", failedPostgresTransactionSubmodelRepo
	}

	return sm, "", nil
}

// get submodel metadata
func (p *PostgreSQLSubmodelDatabase) GetAllSubmodelsMetadata(
	limit int32,
	cursor string,
	idShort string,
	semanticId string,
) ([]gen.Submodel, string, error) {

	tx, err := p.db.Begin()
	if limit <= 0 {
		limit = 100
	}

	if err != nil {
		return nil, "", beginTransactionErrorSubmodelRepo
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	query := `
		SELECT 
			s.id, 
			s.id_short, 
			s.category, 
			s.kind, 
			s.model_type, 
			r.type AS semantic_reference_type,
			rk.type AS key_type,
   			rk.value AS key_value
		FROM submodel s
		LEFT JOIN reference r ON s.semantic_id = r.id
		LEFT JOIN reference_key rk ON r.id = rk.reference_id
		WHERE ($1 = '' OR s.id_short ILIKE '%' || $1 || '%')
		ORDER BY s.id
		LIMIT $2;
	`

	rows, err := p.db.Query(query, idShort, limit)
	if err != nil {
		tx.Rollback()
		fmt.Println("Error querying submodel metadata:", err)
		return nil, "", err
	}
	defer rows.Close()

	var submodels []gen.Submodel
	for rows.Next() {
		var sm gen.Submodel
		var refType, keyType, keyValue sql.NullString

		err := rows.Scan(
			&sm.Id,
			&sm.IdShort,
			&sm.Category,
			&sm.Kind,
			&sm.ModelType,
			&refType,
			&keyType,
			&keyValue,
		)
		if err != nil {
			fmt.Println("Error scanning metadata row:", err)
			return nil, "", err
		}
		if refType.Valid {
			ref := gen.Reference{
				Type: gen.ReferenceTypes(refType.String),
			}
			// Only add keys if both type and value are valid
			if keyType.Valid && keyValue.Valid {
				ref.Keys = []gen.Key{{
					Type:  gen.KeyTypes(keyType.String),
					Value: keyValue.String,
				}}
			}
			sm.SemanticId = &ref
		}
		submodels = append(submodels, sm)
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return nil, "", failedPostgresTransactionSubmodelRepo
	}

	return submodels, "", nil
}

// GetSubmodel returns one Submodel by id
func (p *PostgreSQLSubmodelDatabase) GetSubmodel(id string) (gen.Submodel, error) {
	// Check cache first
	if p.cacheEnabled {
		if sm, found := submodelCache[id]; found {
			return sm, nil
		}
	}

	// Not in cache, fetch from DB
	tx, err := p.db.Begin()

	if err != nil {
		fmt.Println(err)
		return gen.Submodel{}, beginTransactionErrorSubmodelRepo
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	sm, err := submodelelements.GetSubmodelWithSubmodelElements(p.db, tx, id)
	if err != nil {
		return gen.Submodel{}, err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return gen.Submodel{}, failedPostgresTransactionSubmodelRepo
	}

	// Store in cache
	if p.cacheEnabled {
		submodelCache[id] = *sm
	}
	return *sm, nil
}

// DeleteSubmodel deletes a Submodel by id
func (p *PostgreSQLSubmodelDatabase) DeleteSubmodel(id string) error {
	// Check cache first
	if p.cacheEnabled {
		delete(submodelCache, id)
	}

	tx, err := p.db.Begin()

	if err != nil {
		fmt.Println(err)
		return beginTransactionErrorSubmodelRepo
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// First, get all the foreign key IDs that will be orphaned after deletion
	var administrationId, semanticId, descriptionId, displaynameId sql.NullInt64
	err = tx.QueryRow(`
		SELECT administration_id, semantic_id, description_id, displayname_id 
		FROM submodel 
		WHERE id=$1
	`, id).Scan(&administrationId, &semanticId, &descriptionId, &displaynameId)
	if err != nil {
		if err == sql.ErrNoRows {
			return sql.ErrNoRows
		}
		return err
	}

	// Get qualifier IDs associated with this submodel
	qualifierRows, err := tx.Query(`
		SELECT qualifier_id FROM submodel_qualifier WHERE submodel_id=$1
	`, id)
	if err != nil {
		return err
	}
	var qualifierIds []int64
	for qualifierRows.Next() {
		var qid int64
		if err := qualifierRows.Scan(&qid); err != nil {
			qualifierRows.Close()
			return err
		}
		qualifierIds = append(qualifierIds, qid)
	}
	qualifierRows.Close()

	// Get extension IDs associated with this submodel
	extensionRows, err := tx.Query(`
		SELECT extension_id FROM submodel_extension WHERE submodel_id=$1
	`, id)
	if err != nil {
		return err
	}
	var extensionIds []int64
	for extensionRows.Next() {
		var eid int64
		if err := extensionRows.Scan(&eid); err != nil {
			extensionRows.Close()
			return err
		}
		extensionIds = append(extensionIds, eid)
	}
	extensionRows.Close()

	// Get embedded data specification IDs associated with this submodel
	edsRows, err := tx.Query(`
		SELECT embedded_data_specification_id FROM submodel_embedded_data_specification WHERE submodel_id=$1
	`, id)
	if err != nil {
		return err
	}
	var edsIds []int64
	for edsRows.Next() {
		var edsId int64
		if err := edsRows.Scan(&edsId); err != nil {
			edsRows.Close()
			return err
		}
		edsIds = append(edsIds, edsId)
	}
	edsRows.Close()

	// Get supplemental semantic ID references
	suppSemanticRows, err := tx.Query(`
		SELECT reference_id FROM submodel_supplemental_semantic_id WHERE submodel_id=$1
	`, id)
	if err != nil {
		return err
	}
	var suppSemanticIds []int64
	for suppSemanticRows.Next() {
		var refId int64
		if err := suppSemanticRows.Scan(&refId); err != nil {
			suppSemanticRows.Close()
			return err
		}
		suppSemanticIds = append(suppSemanticIds, refId)
	}
	suppSemanticRows.Close()

	// Delete the submodel row (this will cascade to join tables and submodel_element)
	const q = `DELETE FROM submodel WHERE id=$1`
	res, err := tx.Exec(q, id)
	if err != nil {
		return err
	}

	// Check if a row was actually deleted
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	// Now delete the orphaned data
	// Delete qualifiers and their references
	for _, qid := range qualifierIds {
		// Get reference IDs from qualifier
		var qualSemanticId, qualValueId sql.NullInt64
		err = tx.QueryRow(`SELECT semantic_id, value_id FROM qualifier WHERE id=$1`, qid).Scan(&qualSemanticId, &qualValueId)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		
		// Get supplemental semantic IDs for this qualifier
		qualSuppRows, err := tx.Query(`SELECT reference_id FROM qualifier_supplemental_semantic_id WHERE qualifier_id=$1`, qid)
		if err != nil {
			return err
		}
		var qualSuppIds []int64
		for qualSuppRows.Next() {
			var refId int64
			if err := qualSuppRows.Scan(&refId); err != nil {
				qualSuppRows.Close()
				return err
			}
			qualSuppIds = append(qualSuppIds, refId)
		}
		qualSuppRows.Close()
		
		// Delete the qualifier
		_, err = tx.Exec(`DELETE FROM qualifier WHERE id=$1`, qid)
		if err != nil {
			return err
		}
		
		// Delete qualifier's references
		if qualSemanticId.Valid {
			if err := deleteReferenceRecursively(tx, qualSemanticId.Int64, 0); err != nil {
				return err
			}
		}
		if qualValueId.Valid {
			if err := deleteReferenceRecursively(tx, qualValueId.Int64, 0); err != nil {
				return err
			}
		}
		for _, refId := range qualSuppIds {
			if err := deleteReferenceRecursively(tx, refId, 0); err != nil {
				return err
			}
		}
	}

	// Delete extensions and their references
	for _, eid := range extensionIds {
		// Get reference IDs from extension
		var extSemanticId sql.NullInt64
		err = tx.QueryRow(`SELECT semantic_id FROM extension WHERE id=$1`, eid).Scan(&extSemanticId)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		
		// Get supplemental semantic IDs for this extension
		extSuppRows, err := tx.Query(`SELECT reference_id FROM extension_supplemental_semantic_id WHERE extension_id=$1`, eid)
		if err != nil {
			return err
		}
		var extSuppIds []int64
		for extSuppRows.Next() {
			var refId int64
			if err := extSuppRows.Scan(&refId); err != nil {
				extSuppRows.Close()
				return err
			}
			extSuppIds = append(extSuppIds, refId)
		}
		extSuppRows.Close()
		
		// Get refers_to references for this extension
		extRefersRows, err := tx.Query(`SELECT reference_id FROM extension_refers_to WHERE extension_id=$1`, eid)
		if err != nil {
			return err
		}
		var extRefersIds []int64
		for extRefersRows.Next() {
			var refId int64
			if err := extRefersRows.Scan(&refId); err != nil {
				extRefersRows.Close()
				return err
			}
			extRefersIds = append(extRefersIds, refId)
		}
		extRefersRows.Close()
		
		// Delete the extension
		_, err = tx.Exec(`DELETE FROM extension WHERE id=$1`, eid)
		if err != nil {
			return err
		}
		
		// Delete extension's references
		if extSemanticId.Valid {
			if err := deleteReferenceRecursively(tx, extSemanticId.Int64, 0); err != nil {
				return err
			}
		}
		for _, refId := range extSuppIds {
			if err := deleteReferenceRecursively(tx, refId, 0); err != nil {
				return err
			}
		}
		for _, refId := range extRefersIds {
			if err := deleteReferenceRecursively(tx, refId, 0); err != nil {
				return err
			}
		}
	}

	// Delete embedded data specifications and their references
	for _, edsId := range edsIds {
		// Get the reference ID and content ID from data_specification
		var dsRefId, dsContentId sql.NullInt64
		err = tx.QueryRow(`SELECT data_specification, data_specification_content FROM data_specification WHERE id=$1`, edsId).Scan(&dsRefId, &dsContentId)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		
		// Get references from data_specification_iec61360 (if it exists)
		if dsContentId.Valid {
			var preferredNameId, shortNameId, unitId, definitionId, valueListId, levelTypeId sql.NullInt64
			err = tx.QueryRow(`
				SELECT preferred_name_id, short_name_id, unit_id, definition_id, value_list_id, level_type_id 
				FROM data_specification_iec61360 
				WHERE id=$1
			`, dsContentId.Int64).Scan(&preferredNameId, &shortNameId, &unitId, &definitionId, &valueListId, &levelTypeId)
			// It's okay if no row exists (not all data_specification_content are iec61360)
			if err == nil {
				// Get value_id references from value_list_value_reference_pair if value_list exists
				if valueListId.Valid {
					vlRows, err := tx.Query(`SELECT value_id FROM value_list_value_reference_pair WHERE value_list_id=$1`, valueListId.Int64)
					if err != nil {
						return err
					}
					defer vlRows.Close()
					
					var vlRefIds []int64
					for vlRows.Next() {
						var refId sql.NullInt64
						if err := vlRows.Scan(&refId); err != nil {
							return err
						}
						if refId.Valid {
							vlRefIds = append(vlRefIds, refId.Int64)
						}
					}
					
					// Delete value_id references
					for _, refId := range vlRefIds {
						if err := deleteReferenceRecursively(tx, refId, 0); err != nil {
							return err
						}
					}
				}
				
				// Delete unit_id reference
				if unitId.Valid {
					if err := deleteReferenceRecursively(tx, unitId.Int64, 0); err != nil {
						return err
					}
				}
				
				// Delete lang strings (will cascade to lang_string_text_type)
				if preferredNameId.Valid {
					_, err = tx.Exec(`DELETE FROM lang_string_text_type_reference WHERE id=$1`, preferredNameId.Int64)
					if err != nil {
						return err
					}
				}
				if shortNameId.Valid {
					_, err = tx.Exec(`DELETE FROM lang_string_text_type_reference WHERE id=$1`, shortNameId.Int64)
					if err != nil {
						return err
					}
				}
				if definitionId.Valid {
					_, err = tx.Exec(`DELETE FROM lang_string_text_type_reference WHERE id=$1`, definitionId.Int64)
					if err != nil {
						return err
					}
				}
			}
		}
		
		// Delete the data_specification (this will cascade to data_specification_content)
		_, err = tx.Exec(`DELETE FROM data_specification WHERE id=$1`, edsId)
		if err != nil {
			return err
		}
		
		// Delete the data_specification reference
		if dsRefId.Valid {
			if err := deleteReferenceRecursively(tx, dsRefId.Int64, 0); err != nil {
				return err
			}
		}
	}

	// Delete supplemental semantic ID references
	for _, refId := range suppSemanticIds {
		if err := deleteReferenceRecursively(tx, refId, 0); err != nil {
			return err
		}
	}

	// Delete administrative information and its references
	if administrationId.Valid {
		// Get the creator reference from administrative_information
		var creatorRefId sql.NullInt64
		err = tx.QueryRow(`SELECT creator FROM administrative_information WHERE id=$1`, administrationId.Int64).Scan(&creatorRefId)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		
		// Get embedded data specifications for this administrative_information
		adminEdsRows, err := tx.Query(`
			SELECT embedded_data_specification_id 
			FROM administrative_information_embedded_data_specification 
			WHERE administrative_information_id=$1
		`, administrationId.Int64)
		if err != nil {
			return err
		}
		var adminEdsIds []int64
		for adminEdsRows.Next() {
			var edsId int64
			if err := adminEdsRows.Scan(&edsId); err != nil {
				adminEdsRows.Close()
				return err
			}
			adminEdsIds = append(adminEdsIds, edsId)
		}
		adminEdsRows.Close()
		
		// Delete the administrative_information (this will cascade to its join table)
		_, err = tx.Exec(`DELETE FROM administrative_information WHERE id=$1`, administrationId.Int64)
		if err != nil {
			return err
		}
		
		// Delete creator reference
		if creatorRefId.Valid {
			if err := deleteReferenceRecursively(tx, creatorRefId.Int64, 0); err != nil {
				return err
			}
		}
		
		// Delete embedded data specifications from administrative_information
		for _, edsId := range adminEdsIds {
			var dsRefId sql.NullInt64
			err = tx.QueryRow(`SELECT data_specification FROM data_specification WHERE id=$1`, edsId).Scan(&dsRefId)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			
			_, err = tx.Exec(`DELETE FROM data_specification WHERE id=$1`, edsId)
			if err != nil {
				return err
			}
			
			if dsRefId.Valid {
				if err := deleteReferenceRecursively(tx, dsRefId.Int64, 0); err != nil {
					return err
				}
			}
		}
	}

	// Delete semantic_id reference (this will cascade to reference_key)
	if semanticId.Valid {
		if err := deleteReferenceRecursively(tx, semanticId.Int64, 0); err != nil {
			return err
		}
	}

	// Delete description (this will cascade to lang_string_text_type)
	if descriptionId.Valid {
		_, err = tx.Exec(`DELETE FROM lang_string_text_type_reference WHERE id=$1`, descriptionId.Int64)
		if err != nil {
			return err
		}
	}

	// Delete displayname (this will cascade to lang_string_name_type)
	if displaynameId.Valid {
		_, err = tx.Exec(`DELETE FROM lang_string_name_type_reference WHERE id=$1`, displaynameId.Int64)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return failedPostgresTransactionSubmodelRepo
	}
	return nil
}

// deleteReferenceRecursively deletes a reference and all its nested references
// depth parameter prevents infinite recursion (max depth is 100)
func deleteReferenceRecursively(tx *sql.Tx, referenceId int64, depth int) error {
	const maxDepth = 100
	if depth > maxDepth {
		return fmt.Errorf("reference hierarchy exceeds maximum depth of %d", maxDepth)
	}
	
	// Get all child references that have this reference as their parent or root
	rows, err := tx.Query(`
		SELECT id FROM reference WHERE parentReference=$1 OR rootReference=$1
	`, referenceId)
	if err != nil {
		return err
	}
	defer rows.Close()
	
	var childIds []int64
	for rows.Next() {
		var childId int64
		if err := rows.Scan(&childId); err != nil {
			return err
		}
		childIds = append(childIds, childId)
	}
	
	// Recursively delete children first
	for _, childId := range childIds {
		if err := deleteReferenceRecursively(tx, childId, depth+1); err != nil {
			return err
		}
	}
	
	// Finally delete this reference (cascade will delete reference_key)
	_, err = tx.Exec(`DELETE FROM reference WHERE id=$1`, referenceId)
	return err
}

// CreateSubmodel inserts a new Submodel
// If a Submodel with the same id already exists, it does nothing and returns nil
// we might want ON CONFLICT DO UPDATE for upserts, but spec-wise POST usually means create new
// model_type is hardcoded to "Submodel"
func (p *PostgreSQLSubmodelDatabase) CreateSubmodel(sm gen.Submodel) error {
	tx, err := p.db.Begin()

	if err != nil {
		fmt.Println(err)
		return beginTransactionErrorSubmodelRepo
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var semanticIdDbId, displayNameId, descriptionId, administrationId sql.NullInt64

	semanticIdDbId, err = persistence_utils.CreateReference(tx, sm.SemanticId, sql.NullInt64{}, sql.NullInt64{})
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create SemanticId - no changes applied - see console for details")
	}

	displayNameId, err = persistence_utils.CreateLangStringNameTypes(tx, sm.DisplayName)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}

	// Handle possibly nil Description
	var convertedDescription []gen.LangStringText
	for _, desc := range sm.Description {
		convertedDescription = append(convertedDescription, desc)
	}
	descriptionId, err = persistence_utils.CreateLangStringTextTypes(tx, convertedDescription)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}

	administrationId, err = persistence_utils.CreateAdministrativeInformation(tx, sm.Administration)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}

	const q = `
        INSERT INTO submodel (id, id_short, category, kind, model_type, semantic_id, displayname_id, description_id, administration_id)
        VALUES ($1, $2, $3, $4, 'Submodel', $5, $6, $7, $8)
        ON CONFLICT (id) DO NOTHING
    `

	_, err = tx.Exec(q, sm.Id, sm.IdShort, sm.Category, sm.Kind, semanticIdDbId, displayNameId, descriptionId, administrationId)
	if err != nil {
		return err
	}

	if sm.SupplementalSemanticIds != nil {
		err = persistence_utils.InsertSupplementalSemanticIdsSubmodel(tx, sm.Id, sm.SupplementalSemanticIds)
		if err != nil {
			return err
		}
	}

	if sm.EmbeddedDataSpecifications != nil {
		for _, eds := range sm.EmbeddedDataSpecifications {
			edsDbId, err := persistence_utils.CreateEmbeddedDataSpecification(tx, eds)
			if err != nil {
				return err
			}
			_, err = tx.Exec("INSERT INTO submodel_embedded_data_specification(submodel_id, embedded_data_specification_id) VALUES ($1, $2)", sm.Id, edsDbId)
			if err != nil {
				return err
			}
		}
	}

	if len(sm.SubmodelElements) > 0 {
		for _, element := range sm.SubmodelElements {
			err = p.AddSubmodelElementWithTransaction(tx, sm.Id, element)
			if err != nil {
				return err
			}
		}
	}

	if len(sm.Qualifier) > 0 {
		for _, qualifier := range sm.Qualifier {
			qualifierId, err := persistence_utils.CreateQualifier(tx, qualifier)
			if err != nil {
				return err
			}
			_, err = tx.Exec(`INSERT INTO submodel_qualifier(submodel_id, qualifier_id) VALUES($1, $2)`, sm.Id, qualifierId)
			if err != nil {
				fmt.Println(err)
				return common.NewInternalServerError("Failed to Create Qualifier for Submodel with ID '" + sm.Id + "'. See console for details.")
			}
		}
	}

	if len(sm.Extension) > 0 {
		for _, extension := range sm.Extension {
			qualifierId, err := persistence_utils.CreateExtension(tx, extension)
			if err != nil {
				return err
			}
			_, err = tx.Exec(`INSERT INTO submodel_extension(submodel_id, extension_id) VALUES($1, $2)`, sm.Id, qualifierId)
			if err != nil {
				fmt.Println(err)
				return common.NewInternalServerError("Failed to Create Extension for Submodel with ID '" + sm.Id + "'. See console for details.")
			}
		}
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return failedPostgresTransactionSubmodelRepo
	}
	// Store in cache if enough space
	if p.cacheEnabled {
		if len(submodelCache) < maxCacheSize {
			submodelCache[sm.Id] = sm
		}
	}
	return nil
}

func (p *PostgreSQLSubmodelDatabase) GetSubmodelElement(submodelId string, idShortOrPath string, limit int, cursor string) (gen.SubmodelElement, error) {
	tx, err := p.db.Begin()
	if err != nil {
		fmt.Println(err)
		return nil, beginTransactionErrorSubmodelRepo
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	elements, _, err := submodelelements.GetSubmodelElementsWithPath(p.db, tx, submodelId, idShortOrPath, limit, cursor)
	if err != nil {
		return nil, err
	}

	if len(elements) == 0 {
		return nil, common.NewErrNotFound("SubmodelElement with idShort or path '" + idShortOrPath + "' not found in submodel '" + submodelId + "'")
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return nil, failedPostgresTransactionSubmodelRepo
	}

	return elements[0], nil
}

func (p *PostgreSQLSubmodelDatabase) GetSubmodelElements(submodelId string, limit int, cursor string) ([]gen.SubmodelElement, string, error) {
	tx, err := p.db.Begin()
	if err != nil {
		fmt.Println(err)
		return nil, "", beginTransactionErrorSubmodelRepo
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	elements, cursor, err := submodelelements.GetSubmodelElementsWithPath(p.db, tx, submodelId, "", limit, cursor)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return nil, "", failedPostgresTransactionSubmodelRepo
	}

	return elements, cursor, nil
}

func (p *PostgreSQLSubmodelDatabase) AddSubmodelElementWithPath(submodelId string, idShortPath string, submodelElement gen.SubmodelElement) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		delete(submodelCache, submodelId)
	}
	handler, err := submodelelements.GetSMEHandler(submodelElement, p.db)
	if err != nil {
		return err
	}

	crud, err := submodelelements.NewPostgreSQLSMECrudHandler(p.db)
	if err != nil {
		return err
	}

	tx, err := p.db.Begin()
	if err != nil {
		fmt.Println(err)
		return beginTransactionErrorSubmodelRepo
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	parentId, err := crud.GetDatabaseId(idShortPath)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to execute PostgreSQL Query - no changes applied - see console for details.")
	}
	nextPosition, err := crud.GetNextPosition(parentId)
	if err != nil {
		return err
	}

	modelType, err := crud.GetSubmodelElementType(idShortPath)
	if err != nil {
		return err
	}
	if modelType != "SubmodelElementCollection" && modelType != "SubmodelElementList" {
		return errors.New("cannot add nested element to non-collection/list element")
	}
	var newIdShortPath string
	if modelType == "SubmodelElementList" {
		newIdShortPath = idShortPath + "[" + strconv.Itoa(nextPosition) + "]"
	} else {
		newIdShortPath = idShortPath + "." + submodelElement.GetIdShort()
	}
	id, err := handler.CreateNested(tx, submodelId, parentId, newIdShortPath, submodelElement, nextPosition)
	if err != nil {
		return err
	}
	err = p.AddNestedSubmodelElementsIteratively(tx, submodelId, id, submodelElement, newIdShortPath)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return failedPostgresTransactionSubmodelRepo
	}

	return nil
}
func (p *PostgreSQLSubmodelDatabase) AddSubmodelElement(submodelId string, submodelElement gen.SubmodelElement) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		delete(submodelCache, submodelId)
	}
	tx, err := p.db.Begin()
	if err != nil {
		fmt.Println(err)
		return beginTransactionErrorSubmodelRepo
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	err = p.AddSubmodelElementWithTransaction(tx, submodelId, submodelElement)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		fmt.Println(err)
		return failedPostgresTransactionSubmodelRepo
	}

	return nil
}

func (p *PostgreSQLSubmodelDatabase) AddSubmodelElementWithTransaction(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		delete(submodelCache, submodelId)
	}
	handler, err := submodelelements.GetSMEHandler(submodelElement, p.db)
	if err != nil {
		return err
	}
	parentId, err := handler.Create(tx, submodelId, submodelElement)
	if err != nil {
		return err
	}

	err = p.AddNestedSubmodelElementsIteratively(tx, submodelId, parentId, submodelElement, "")
	if err != nil {
		return err
	}
	return nil
}

type ElementToProcess struct {
	element                   gen.SubmodelElement
	parentId                  int
	currentIdShortPath        string
	isFromSubmodelElementList bool // Indicates if the current element is from a SubmodelElementList
	position                  int  // Position/index within the parent collection or list
}

func (p *PostgreSQLSubmodelDatabase) AddNestedSubmodelElementsIteratively(tx *sql.Tx, submodelId string, topLevelParentId int, topLevelElement gen.SubmodelElement, startPath string) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		delete(submodelCache, submodelId)
	}
	stack := []ElementToProcess{}

	switch string(topLevelElement.GetModelType()) {
	case "SubmodelElementCollection":
		submodelElementCollection, ok := topLevelElement.(*gen.SubmodelElementCollection)
		if !ok {
			return common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementCollection' is not of type SubmodelElementCollection")
		}
		for index, nestedElement := range submodelElementCollection.Value {
			var currentPath string
			if startPath == "" {
				currentPath = submodelElementCollection.IdShort
			} else {
				currentPath = startPath
			}
			stack = append(stack, ElementToProcess{
				element:                   nestedElement,
				parentId:                  topLevelParentId,
				currentIdShortPath:        currentPath,
				isFromSubmodelElementList: false,
				position:                  index,
			})
		}
	case "SubmodelElementList":
		submodelElementList, ok := topLevelElement.(*gen.SubmodelElementList)
		if !ok {
			return common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementList' is not of type SubmodelElementList")
		}
		// Add nested elements to stack with index-based paths
		for index, nestedElement := range submodelElementList.Value {
			var idShortPath string
			if startPath == "" {
				idShortPath = submodelElementList.IdShort + "[" + strconv.Itoa(index) + "]"
			} else {
				idShortPath = startPath
			}
			stack = append(stack, ElementToProcess{
				element:                   nestedElement,
				parentId:                  topLevelParentId,
				currentIdShortPath:        idShortPath,
				isFromSubmodelElementList: true,
				position:                  index,
			})
		}
	}

	for len(stack) > 0 {
		// LIFO Stack
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		handler, err := submodelelements.GetSMEHandler(current.element, p.db)
		if err != nil {
			return err
		}

		// Build the idShortPath for current element
		idShortPath := buildCurrentIdShortPath(current)

		newParentId, err := handler.CreateNested(tx, submodelId, current.parentId, idShortPath, current.element, current.position)
		if err != nil {
			return err
		}

		switch string(current.element.GetModelType()) {
		case "SubmodelElementCollection":
			submodelElementCollection, ok := current.element.(*gen.SubmodelElementCollection)
			if !ok {
				return common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementCollection' is not of type SubmodelElementCollection")
			}
			for i := len(submodelElementCollection.Value) - 1; i >= 0; i-- {
				stack = addNestedElementToStackWithNormalPath(submodelElementCollection, i, stack, newParentId, idShortPath)
			}
		case "SubmodelElementList":
			submodelElementList, ok := current.element.(*gen.SubmodelElementList)
			if !ok {
				return common.NewInternalServerError("SubmodelElement with modelType 'SubmodelElementList' is not of type SubmodelElementList")
			}
			for index := len(submodelElementList.Value) - 1; index >= 0; index-- {
				stack = addNestedElementToStackWithIndexPath(submodelElementList, index, idShortPath, stack, newParentId)
			}
		}
	}

	return nil
}

// This method removes a SubmodelElement by its idShort or path and all its nested elements
// If the deleted Element is in a SubmodelElementList, the indices of the remaining elements are adjusted accordingly
func (p *PostgreSQLSubmodelDatabase) DeleteSubmodelElementByPath(submodelId string, idShortOrPath string) error {
	// Invalidate Submodel cache if enabled
	if p.cacheEnabled {
		delete(submodelCache, submodelId)
	}
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	err = submodelelements.DeleteSubmodelElementByPath(tx, submodelId, idShortOrPath)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func buildCurrentIdShortPath(current ElementToProcess) string {
	var idShortPath string
	if current.currentIdShortPath == "" {
		idShortPath = current.element.GetIdShort()
	} else {
		// If element comes from a SubmodelElementList, use the path as-is (includes [index])
		if current.isFromSubmodelElementList {
			idShortPath = current.currentIdShortPath
		} else {
			// For SubmodelElementCollection, append element's idShort with dot notation
			idShortPath = current.currentIdShortPath + "." + current.element.GetIdShort()
		}
	}
	return idShortPath
}

func addNestedElementToStackWithNormalPath(submodelElementCollection *gen.SubmodelElementCollection, i int, stack []ElementToProcess, newParentId int, idShortPath string) []ElementToProcess {
	nestedElement := submodelElementCollection.Value[i]
	stack = append(stack, ElementToProcess{
		element:                   nestedElement,
		parentId:                  newParentId,
		currentIdShortPath:        idShortPath,
		isFromSubmodelElementList: false, // Children of collection are not from list
		position:                  i,
	})
	return stack
}

func addNestedElementToStackWithIndexPath(submodelElementList *gen.SubmodelElementList, index int, idShortPath string, stack []ElementToProcess, newParentId int) []ElementToProcess {
	nestedElement := submodelElementList.Value[index]
	nestedIdShortPath := idShortPath + "[" + strconv.Itoa(index) + "]"
	stack = append(stack, ElementToProcess{
		element:                   nestedElement,
		parentId:                  newParentId,
		currentIdShortPath:        nestedIdShortPath,
		isFromSubmodelElementList: true,  // Children of list are from list
		position:                  index, // For lists, position is the actual index
	})
	return stack
}
