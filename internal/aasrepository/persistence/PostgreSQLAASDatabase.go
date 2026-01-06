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
// Package persistencepostgresql provides PostgreSQL-based persistence implementation
// for the Eclipse BaSyx AAS Service.
//
// This package implements the storage and retrieval of Asset Administration Shell (AAS)
// identifiers and their associated asset links in a PostgreSQL database. It supports
// operations for creating, retrieving, searching, and deleting AAS discovery information
// with cursor-based pagination for efficient querying of large datasets.

// Package persistencepostgresql provides PostgreSQL-based persistence for the AAS repository.
package persistencepostgresql

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"

	// Import for PostgreSQL driver
	_ "github.com/lib/pq"
)

// PostgreSQLAASDatabase is the DB handler used by the AAS Repository.
type PostgreSQLAASDatabase struct {
	DB *sql.DB
}

// NewPostgreSQLAASDatabaseBackend initializes the database and applies schema.
func NewPostgreSQLAASDatabaseBackend(
	dsn string,
	maxOpenConns int,
	maxIdleConns int,
	_ int, // connMaxLifetimeMinutes is unused for now
	databaseSchema string,
) (*PostgreSQLAASDatabase, error) {
	// common.InitializeDatabase executes the SQL schema file automatically.
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}

	// (Optional) configure SQL connection pooling
	if maxOpenConns > 0 {
		db.SetMaxOpenConns(maxOpenConns)
	}
	if maxIdleConns > 0 {
		db.SetMaxIdleConns(maxIdleConns)
	}

	return &PostgreSQLAASDatabase{DB: db}, nil
}

// GetAllAAS retrieves all Asset Administration Shells from the database.
func (p *PostgreSQLAASDatabase) GetAllAAS() ([]model.AssetAdministrationShell, error) {
	dialect := goqu.New("postgres", p.DB)

	query, _, err := dialect.
		From("aas").
		Select("id", "id_short", "category", "model_type", "administration_id", "asset_information_id", "derived_from_reference_id").
		Order(goqu.I("id").Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := p.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("failed to close rows: %v", err)
		}
	}()

	var result []model.AssetAdministrationShell

	for rows.Next() {
		var shell model.AssetAdministrationShell
		var adminID sql.NullInt64
		var assetInfoID sql.NullInt64
		var derivedID sql.NullInt64
		if err := rows.Scan(&shell.ID, &shell.IdShort, &shell.Category, &shell.ModelType, &adminID, &assetInfoID, &derivedID); err != nil {
			return nil, err
		}

		dn, err := p.fetchDisplayName(shell.ID)
		if err != nil {
			return nil, err
		}
		shell.DisplayName = dn

		desc, err := p.fetchDescription(shell.ID)
		if err != nil {
			return nil, err
		}
		shell.Description = desc

		// ----------------------
		// Fetch Administration
		// ----------------------
		if adminID.Valid {
			admin, err := p.fetchAdministration(adminID.Int64)
			if err != nil {
				return nil, err
			}
			shell.Administration = admin
		} else {
			shell.Administration = model.AdministrativeInformation{}
		}
		// ----------------------
		// Fetch AssetInformtaion
		// ----------------------
		if assetInfoID.Valid {
			ai, err := p.fetchAssetInformation(assetInfoID.Int64)
			if err != nil {
				return nil, err
			}
			shell.AssetInformation = &ai
		} else {
			shell.AssetInformation = &model.AssetInformation{}
		}
		// ----------------------
		// Fetch Derived from reference
		// ----------------------
		if derivedID.Valid {
			ref, err := p.fetchReference(derivedID.Int64)
			if err != nil {
				return nil, err
			}
			shell.DerivedFrom = ref
		} else {
			shell.DerivedFrom = nil
		}

		// Add placeholders
		// shell.DisplayName = []model.LangStringNameType{}
		// shell.Description = []model.LangStringTextType{}
		shell.Extensions = []model.Extension{}
		shell.EmbeddedDataSpecifications = []model.EmbeddedDataSpecification{}
		shell.Submodels = []model.Reference{}
		// shell.DerivedFrom = nil
		// shell.Administration = model.AdministrativeInformation{}
		// shell.AssetInformation = &model.AssetInformation{}

		result = append(result, shell)
	}

	return result, nil
}

// InsertAAS inserts a new Asset Administration Shell into the database.
func (p *PostgreSQLAASDatabase) InsertAAS(aas model.AssetAdministrationShell) error {
	exists, err := p.existsAAS(aas.ID)
	if err != nil {
		return err
	} else if exists {
		return common.NewErrConflict(
			fmt.Sprintf("AAS with id '%s' already exists", aas.ID),
		)
	}
	if err := p.insertBaseAAS(aas); err != nil {
		return err
	}
	if err := p.insertDisplayName(aas); err != nil {
		return err
	}
	if err := p.insertDescription(aas); err != nil {
		return err
	}
	if err := p.insertAdministration(aas); err != nil {
		return err
	}
	return p.insertAssetInformation(aas)
}

// GetAASByID retrieves an Asset Administration Shell by its ID.
func (p *PostgreSQLAASDatabase) GetAASByID(id string) (*model.AssetAdministrationShell, error) {
	dialect := goqu.New("postgres", p.DB)
	query, _, err := dialect.
		From("aas").
		Select("id", "id_short", "category", "model_type", "administration_id", "asset_information_id", "derived_from_reference_id").
		Where(goqu.Ex{"id": id}).
		Limit(1).
		ToSQL()
	if err != nil {
		return nil, err
	}
	row := p.DB.QueryRow(query)

	var shell model.AssetAdministrationShell
	var adminID sql.NullInt64
	var assetInfoID sql.NullInt64
	var derivedID sql.NullInt64
	if err := row.Scan(&shell.ID, &shell.IdShort, &shell.Category, &shell.ModelType, &adminID, &assetInfoID, &derivedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	// ----------------------
	// Fetch DisplayName
	// ----------------------
	dn, err := p.fetchDisplayName(id)
	if err != nil {
		return nil, err
	}
	shell.DisplayName = dn

	// ----------------------
	// Fetch Description
	// ----------------------
	desc, err := p.fetchDescription(id)
	if err != nil {
		return nil, err
	}
	shell.Description = desc

	// ----------------------
	// Fetch Administration (only if FK exists)
	// ----------------------
	if adminID.Valid {
		admin, err := p.fetchAdministration(adminID.Int64)
		if err != nil {
			return nil, err
		}
		shell.Administration = admin
	} else {
		shell.Administration = model.AdministrativeInformation{}
	}
	// ----------------------
	// Fetch Asset Information
	// ----------------------
	if assetInfoID.Valid {
		ai, err := p.fetchAssetInformation(assetInfoID.Int64)
		if err != nil {
			return nil, err
		}
		shell.AssetInformation = &ai
	} else {
		shell.AssetInformation = &model.AssetInformation{}
	}
	// ----------------------
	// Fetch Derived from reference
	// ----------------------
	if derivedID.Valid {
		ref, err := p.fetchReference(derivedID.Int64)
		if err != nil {
			return nil, err
		}
		shell.DerivedFrom = ref
	} else {
		shell.DerivedFrom = nil
	}
	// Placeholder values
	// shell.DisplayName = []model.LangStringNameType{}
	// shell.Description = []model.LangStringTextType{}
	shell.Extensions = []model.Extension{}
	shell.EmbeddedDataSpecifications = []model.EmbeddedDataSpecification{}
	shell.Submodels = []model.Reference{}
	// shell.DerivedFrom = nil
	// shell.Administration = model.AdministrativeInformation{}
	// shell.AssetInformation = &model.AssetInformation{}

	return &shell, nil
}

// DeleteAASByID deletes an Asset Administration Shell by its ID.
func (p *PostgreSQLAASDatabase) DeleteAASByID(id string) error {
	dialect := goqu.New("postgres", p.DB)

	query, _, err := dialect.
		Delete("aas").
		Where(goqu.Ex{"id": id}).
		ToSQL()
	if err != nil {
		return err
	}

	result, err := p.DB.Exec(query)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// ----------------------
// Helper Functions
// ----------------------
func (p *PostgreSQLAASDatabase) fetchDescription(aasID string) ([]model.LangStringTextType, error) {
	dialect := goqu.New("postgres", p.DB)

	query, _, err := dialect.
		From("aas_description_ref").
		Select("lang_string_text_type_reference_id").
		Where(goqu.Ex{"aas_id": aasID}).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := p.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("failed to close rows: %v", err)
		}
	}()

	var refIDs []int64
	for rows.Next() {
		var rid int64
		if err := rows.Scan(&rid); err != nil {
			return nil, err
		}
		refIDs = append(refIDs, rid)
	}

	if len(refIDs) == 0 {
		return []model.LangStringTextType{}, nil
	}

	var descriptions []model.LangStringTextType

	for _, rid := range refIDs {
		querySql, _, err := dialect.
			From("lang_string_text_type").
			Select("language", "text").
			Where(goqu.Ex{"lang_string_text_type_reference_id": rid}).
			ToSQL()
		if err != nil {
			return nil, err
		}

		textRows, err := p.DB.Query(querySql)
		if err != nil {
			return nil, err
		}

		for textRows.Next() {
			var d model.LangStringTextType
			if err := textRows.Scan(&d.Language, &d.Text); err != nil {
				if err := textRows.Close(); err != nil {
					log.Printf("error: %v", err)
				}
				return nil, err
			}
			descriptions = append(descriptions, d)
		}
		if err := textRows.Close(); err != nil {
			log.Printf("error: %v", err)
		}
	}

	return descriptions, nil
}

func (p *PostgreSQLAASDatabase) fetchDisplayName(aasID string) ([]model.LangStringNameType, error) {
	dialect := goqu.New("postgres", p.DB)

	// 1) Find reference IDs linked to the AAS
	query, _, err := dialect.
		From("aas_display_name_ref").
		Select("lang_string_name_type_reference_id").
		Where(goqu.Ex{"aas_id": aasID}).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := p.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error: %v", err)
		}
	}()

	var refIDs []int64
	for rows.Next() {
		var refID int64
		if err := rows.Scan(&refID); err != nil {
			return nil, err
		}
		refIDs = append(refIDs, refID)
	}

	// No DisplayName for this AAS
	if len(refIDs) == 0 {
		return []model.LangStringNameType{}, nil
	}

	// 2) Fetch actual multilingual entries
	var displayNames []model.LangStringNameType

	for _, rid := range refIDs {
		querySql, _, err := dialect.
			From("lang_string_name_type").
			Select("language", "text").
			Where(goqu.Ex{"lang_string_name_type_reference_id": rid}).
			ToSQL()
		if err != nil {
			return nil, err
		}

		lnRows, err := p.DB.Query(querySql)
		if err != nil {
			return nil, err
		}

		for lnRows.Next() {
			var ln model.LangStringNameType
			if err := lnRows.Scan(&ln.Language, &ln.Text); err != nil {
				if err := lnRows.Close(); err != nil {
					log.Printf("failed to close rows: %v", err)
				}
				return nil, err
			}
			displayNames = append(displayNames, ln)
		}
		if err := lnRows.Close(); err != nil {
			log.Printf("failed to close rows: %v", err)
		}
	}

	return displayNames, nil
}

func (p *PostgreSQLAASDatabase) fetchAdministration(adminID int64) (model.AdministrativeInformation, error) {
	dialect := goqu.New("postgres", p.DB)

	query, _, err := dialect.
		From("administrative_information").
		Select("version", "revision", "templateid").
		Where(goqu.Ex{"id": adminID}).
		Limit(1).
		ToSQL()
	if err != nil {
		return model.AdministrativeInformation{}, err
	}

	row := p.DB.QueryRow(query)

	var ai model.AdministrativeInformation
	err = row.Scan(&ai.Version, &ai.Revision, &ai.TemplateID)
	if err != nil {
		return model.AdministrativeInformation{}, err
	}

	return ai, nil
}

func (p *PostgreSQLAASDatabase) fetchAssetInformation(id int64) (model.AssetInformation, error) {
	var ai model.AssetInformation

	// --- Load main AssetInformation row ---
	row := p.DB.QueryRow(`
		SELECT asset_kind, global_asset_id, asset_type
		FROM asset_information
		WHERE id = $1
	`, id)

	err := row.Scan(&ai.AssetKind, &ai.GlobalAssetID, &ai.AssetType)
	if err != nil {
		return ai, err
	}

	// --- Load SpecificAssetIds ---
	rows, err := p.DB.Query(`
		SELECT name, value
		FROM aas_specific_asset_id
		WHERE asset_information_id = $1
	`, id)
	if err != nil {
		return ai, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error: %v", err)
		}
	}()

	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return ai, err
		}
		ai.SpecificAssetIds = append(ai.SpecificAssetIds, model.SpecificAssetID{
			Name:  name,
			Value: value,
		})
	}
	if err := rows.Err(); err != nil {
		return ai, err
	}

	// --- Load Thumbnail ---
	var path sql.NullString
	var content sql.NullString

	row = p.DB.QueryRow(`
		SELECT r.path, r.content_type
		FROM asset_information_default_thumbnail dt
		JOIN aas_resource r ON r.id = dt.default_thumbnail_id
		WHERE dt.asset_information_id = $1
	`, id)

	err = row.Scan(&path, &content)
	if err == nil && path.Valid {
		ai.DefaultThumbnail = model.Resource{
			Path:        path.String,
			ContentType: content.String,
		}
	}

	return ai, nil
}

func (p *PostgreSQLAASDatabase) insertReference(ref *model.Reference) (int64, error) {
	dialect := goqu.New("postgres", p.DB)

	// 1) Insert reference row
	query, _, err := dialect.
		Insert("reference").
		Rows(goqu.Record{
			"type": ref.Type,
		}).
		Returning("id").
		ToSQL()
	if err != nil {
		return 0, err
	}

	var refID int64
	err = p.DB.QueryRow(query).Scan(&refID)
	if err != nil {
		return 0, err
	}

	// 2) Insert keys
	for i, key := range ref.Keys {
		query, _, err = dialect.
			Insert("reference_key").
			Rows(goqu.Record{
				"reference_id": refID,
				"position":     i,
				"type":         key.Type,
				"value":        key.Value,
			}).
			ToSQL()
		if err != nil {
			return 0, err
		}

		_, err = p.DB.Exec(query)
		if err != nil {
			return 0, err
		}
	}

	return refID, nil
}

func (p *PostgreSQLAASDatabase) fetchReference(id int64) (*model.Reference, error) {
	// Fetch reference
	row := p.DB.QueryRow(`
        SELECT type FROM reference WHERE id = $1
    `, id)

	var refType string
	if err := row.Scan(&refType); err != nil {
		return nil, err
	}

	ref := &model.Reference{
		Type: model.ReferenceTypes(refType),
		Keys: []model.Key{},
	}

	// Fetch keys
	rows, err := p.DB.Query(`
        SELECT position, type, value
        FROM reference_key
        WHERE reference_id = $1
        ORDER BY position ASC
    `, id)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("error: %v", err)
		}
	}()

	for rows.Next() {
		var pos int
		var keyType string
		var value string

		if err := rows.Scan(&pos, &keyType, &value); err != nil {
			return nil, err
		}

		ref.Keys = append(ref.Keys, model.Key{
			Type:  model.KeyTypes(keyType),
			Value: value,
		})
	}

	return ref, nil
}

// insertBaseAAS inserts the base AAS information into the database.
func (p *PostgreSQLAASDatabase) insertBaseAAS(aas model.AssetAdministrationShell) error {
	dialect := goqu.New("postgres", p.DB)
	query, _, err := dialect.
		Insert("aas").
		Rows(goqu.Record{
			"id":         aas.ID,
			"id_short":   aas.IdShort,
			"category":   aas.Category,
			"model_type": aas.ModelType,
		}).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = p.DB.Exec(query)
	return err
}

// insertDisplayName inserts the DisplayName entries for the AAS.
func (p *PostgreSQLAASDatabase) insertDisplayName(aas model.AssetAdministrationShell) error {
	if len(aas.DisplayName) == 0 {
		return nil
	}

	dialect := goqu.New("postgres", p.DB)

	// Insert reference
	query, _, err := dialect.
		Insert("lang_string_name_type_reference").
		Rows(goqu.Record{}).
		Returning("id").
		ToSQL()
	if err != nil {
		return err
	}

	var refID int64
	if err = p.DB.QueryRow(query).Scan(&refID); err != nil {
		return err
	}

	// Insert language entries
	for _, dn := range aas.DisplayName {
		query, _, err = dialect.
			Insert("lang_string_name_type").
			Rows(goqu.Record{
				"lang_string_name_type_reference_id": refID,
				"language":                           dn.Language,
				"text":                               dn.Text,
			}).
			ToSQL()
		if err != nil {
			return err
		}

		if _, err = p.DB.Exec(query); err != nil {
			return err
		}
	}

	// Link AAS and reference
	query, _, err = dialect.
		Insert("aas_display_name_ref").
		Rows(goqu.Record{
			"aas_id":                             aas.ID,
			"lang_string_name_type_reference_id": refID,
		}).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = p.DB.Exec(query)
	return err
}

// insertDescription inserts the Description entries for the AAS.
func (p *PostgreSQLAASDatabase) insertDescription(aas model.AssetAdministrationShell) error {
	if len(aas.Description) == 0 {
		return nil
	}

	dialect := goqu.New("postgres", p.DB)

	query, _, err := dialect.
		Insert("lang_string_text_type_reference").
		Rows(goqu.Record{}).
		Returning("id").
		ToSQL()
	if err != nil {
		return err
	}

	var descRefID int64
	if err = p.DB.QueryRow(query).Scan(&descRefID); err != nil {
		return err
	}

	for _, d := range aas.Description {
		query, _, err = dialect.
			Insert("lang_string_text_type").
			Rows(goqu.Record{
				"lang_string_text_type_reference_id": descRefID,
				"language":                           d.Language,
				"text":                               d.Text,
			}).
			ToSQL()
		if err != nil {
			return err
		}
		if _, err = p.DB.Exec(query); err != nil {
			return err
		}
	}

	query, _, err = dialect.
		Insert("aas_description_ref").
		Rows(goqu.Record{
			"aas_id":                             aas.ID,
			"lang_string_text_type_reference_id": descRefID,
		}).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = p.DB.Exec(query)
	return err
}

// insertAdministration inserts the AdministrativeInformation for the AAS.
func (p *PostgreSQLAASDatabase) insertAdministration(aas model.AssetAdministrationShell) error {
	if aas.Administration.Version == "" && aas.Administration.Revision == "" && aas.Administration.TemplateID == "" {
		return nil
	}

	dialect := goqu.New("postgres", p.DB)
	query, _, err := dialect.
		Insert("administrative_information").
		Rows(goqu.Record{
			"version":    aas.Administration.Version,
			"revision":   aas.Administration.Revision,
			"templateid": aas.Administration.TemplateID,
		}).
		Returning("id").
		ToSQL()
	if err != nil {
		return err
	}

	var adminID int64
	if err = p.DB.QueryRow(query).Scan(&adminID); err != nil {
		return err
	}

	upd, _, err := dialect.
		Update("aas").
		Set(goqu.Record{"administration_id": adminID}).
		Where(goqu.Ex{"id": aas.ID}).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = p.DB.Exec(upd)
	return err
}

// insertAssetInformation inserts the AssetInformation for the AAS.
func (p *PostgreSQLAASDatabase) insertAssetInformation(aas model.AssetAdministrationShell) error {
	if aas.AssetInformation == nil {
		return nil
	}

	dialect := goqu.New("postgres", p.DB)

	// 1) Insert asset_information
	query, _, err := dialect.
		Insert("asset_information").
		Rows(goqu.Record{
			"asset_kind":      aas.AssetInformation.AssetKind,
			"global_asset_id": aas.AssetInformation.GlobalAssetID,
			"asset_type":      aas.AssetInformation.AssetType,
		}).
		Returning("id").
		ToSQL()
	if err != nil {
		return err
	}

	var assetInfoID int64
	if err = p.DB.QueryRow(query).Scan(&assetInfoID); err != nil {
		return err
	}

	// 2) Link AAS -> asset_information
	upd, _, err := dialect.
		Update("aas").
		Set(goqu.Record{"asset_information_id": assetInfoID}).
		Where(goqu.Ex{"id": aas.ID}).
		ToSQL()
	if err != nil {
		return err
	}
	if _, err = p.DB.Exec(upd); err != nil {
		return err
	}

	// 3) Specific Asset IDs
	for _, sid := range aas.AssetInformation.SpecificAssetIds {
		query, _, err = dialect.
			Insert("aas_specific_asset_id").
			Rows(goqu.Record{
				"asset_information_id": assetInfoID,
				"name":                 sid.Name,
				"value":                sid.Value,
				"semantic_id":          sid.SemanticID,
				"external_subject_id":  sid.ExternalSubjectID,
			}).
			ToSQL()
		if err != nil {
			return err
		}
		if _, err = p.DB.Exec(query); err != nil {
			return err
		}
	}

	// 4) Default Thumbnail
	if aas.AssetInformation.DefaultThumbnail.Path != "" {
		query, _, err = dialect.
			Insert("aas_resource").
			Rows(goqu.Record{
				"path":         aas.AssetInformation.DefaultThumbnail.Path,
				"content_type": aas.AssetInformation.DefaultThumbnail.ContentType,
			}).
			Returning("id").
			ToSQL()
		if err != nil {
			return err
		}

		var resourceID int64
		if err = p.DB.QueryRow(query).Scan(&resourceID); err != nil {
			return err
		}

		query, _, err = dialect.
			Insert("asset_information_default_thumbnail").
			Rows(goqu.Record{
				"asset_information_id": assetInfoID,
				"default_thumbnail_id": resourceID,
			}).
			ToSQL()
		if err != nil {
			return err
		}
		if _, err = p.DB.Exec(query); err != nil {
			return err
		}
	}

	// 5) DerivedFrom Reference
	if aas.DerivedFrom != nil {
		refID, err := p.insertReference(aas.DerivedFrom)
		if err != nil {
			return err
		}
		upd, _, err := dialect.
			Update("aas").
			Set(goqu.Record{"derived_from_reference_id": refID}).
			Where(goqu.Ex{"id": aas.ID}).
			ToSQL()
		if err != nil {
			return err
		}
		if _, err = p.DB.Exec(upd); err != nil {
			return err
		}
	}

	return nil
}

// checks if an AAS with the given ID exists in the database.
func (p *PostgreSQLAASDatabase) existsAAS(id string) (bool, error) {
	var exists bool
	err := p.DB.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM aas WHERE id = $1)`,
		id,
	).Scan(&exists)
	return exists, err
}
