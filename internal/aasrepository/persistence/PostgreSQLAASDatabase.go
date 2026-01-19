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
	"context"
	"database/sql"
	"log"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence/helpers"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
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
	//  Base query: AAS + FK IDs

	dialect := goqu.Dialect("postgres")

	query := dialect.
		From(goqu.T("aas").As("a")).
		Select(
			goqu.I("a.id"),
			goqu.I("a.id_short"),
			goqu.I("a.category"),
			goqu.I("a.model_type"),
			goqu.I("a.displayname_id"),
			goqu.I("a.description_id"),
			goqu.I("a.administrative_information_id"),
			goqu.I("a.asset_information_id"),
		).
		Order(goqu.I("a.id").Asc())

	sqlQuery, args, err := query.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := p.DB.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("failed to close rows: %v", err)
		}
	}()

	// scan base row and collect FK ids
	type aasBaseRow struct {
		shell         model.AssetAdministrationShell
		displayNameID sql.NullInt64
		descriptionID sql.NullInt64
		adminID       sql.NullInt64
		assetInfoID   sql.NullInt64
	}

	base := make([]aasBaseRow, 0, 64)

	// sets for dedupe
	dnSet := map[int64]struct{}{}
	descSet := map[int64]struct{}{}
	adminSet := map[int64]struct{}{}
	assetInfoSet := map[int64]struct{}{}
	// derivedSet := map[int64]struct{}{}

	for rows.Next() {
		var r aasBaseRow
		if err := rows.Scan(
			&r.shell.ID,
			&r.shell.IdShort,
			&r.shell.Category,
			&r.shell.ModelType,
			&r.displayNameID,
			&r.descriptionID,
			&r.adminID,
			&r.assetInfoID,
		); err != nil {
			return nil, err
		}

		if r.displayNameID.Valid {
			dnSet[r.displayNameID.Int64] = struct{}{}
		}
		if r.descriptionID.Valid {
			descSet[r.descriptionID.Int64] = struct{}{}
		}
		if r.adminID.Valid {
			adminSet[r.adminID.Int64] = struct{}{}
		}
		if r.assetInfoID.Valid {
			assetInfoSet[r.assetInfoID.Int64] = struct{}{}
		}
		base = append(base, r)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// convert set -> slice
	displayNameIDs := make([]int64, 0, len(dnSet))
	for id := range dnSet {
		displayNameIDs = append(displayNameIDs, id)
	}
	// convert set -> slice
	descriptionIDs := make([]int64, 0, len(descSet))
	for id := range descSet {
		descriptionIDs = append(descriptionIDs, id)
	}
	adminIDs := make([]int64, 0, len(descSet))
	for id := range adminSet {
		adminIDs = append(adminIDs, id)
	}
	assetInfoIDs := make([]int64, 0, len(assetInfoSet))
	for id := range assetInfoSet {
		assetInfoIDs = append(assetInfoIDs, id)
	}

	// --- DisplayNames: map[displayname_id][]LangStringNameType using helper function ---
	displayNamesByRefID, err := descriptors.GetLangStringNameTypesByIDs(p.DB, displayNameIDs)
	if err != nil {
		return nil, err
	}
	descriptionsByRefID, err := descriptors.GetLangStringTextTypesByIDs(p.DB, descriptionIDs)
	if err != nil {
		return nil, err
	}
	adminByID, err := descriptors.ReadAdministrativeInformationByIDs(
		context.Background(),
		p.DB,
		"aas",
		adminIDs,
	)
	if err != nil {
		return nil, err
	}
	assetInfoByID, err := helpers.ReadAasAssetInformationByIDs(
		p.DB,
		assetInfoIDs,
	)
	if err != nil {
		return nil, err
	}
	specificAssetIDsByAssetInfoID, err :=
		helpers.ReadSpecificAssetIDsByAssetInfoIDs(
			p.DB,
			assetInfoIDs,
		)
	if err != nil {
		return nil, err
	}

	//  Attach from maps and return
	result := make([]model.AssetAdministrationShell, 0, len(base))

	for _, r := range base {
		shell := r.shell

		// Attach display name
		shell.DisplayName = helpers.ResolveByID(
			r.displayNameID,
			displayNamesByRefID,
			[]model.LangStringNameType{},
		)
		// Attach description
		shell.Description = helpers.ResolveByID(
			r.descriptionID,
			descriptionsByRefID,
			[]model.LangStringTextType{},
		)
		// Attach administration
		if r.adminID.Valid {
			if admin, ok := adminByID[r.adminID.Int64]; ok && admin != nil {
				shell.Administration = *admin
			} else {
				shell.Administration = model.AdministrativeInformation{}
			}
		} else {
			shell.Administration = model.AdministrativeInformation{}
		}
		// Attach asset information
		if r.assetInfoID.Valid {
			if ai, ok := assetInfoByID[r.assetInfoID.Int64]; ok {
				if sids, ok := specificAssetIDsByAssetInfoID[r.assetInfoID.Int64]; ok {
					ai.SpecificAssetIds = sids
				} else {
					ai.SpecificAssetIds = []model.SpecificAssetID{}
				}

				shell.AssetInformation = ai
			} else {
				shell.AssetInformation = &model.AssetInformation{}
			}
		} else {
			shell.AssetInformation = &model.AssetInformation{}
		}

		// placeholders for unimplemented fields
		shell.DerivedFrom = nil
		shell.Extensions = []model.Extension{}
		shell.EmbeddedDataSpecifications = []model.EmbeddedDataSpecification{}
		shell.Submodels = []model.Reference{}

		result = append(result, shell)
	}
	return result, nil
}
