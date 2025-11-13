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
// for the Eclipse BaSyx RegistryOfRegistries Service.
//
// This package implements the storage and retrieval of Asset Administration Shell (AAS)
// identifiers and their associated asset links in a PostgreSQL database. It supports
// operations for creating, retrieving, searching, and deleting AAS RegistryOfRegistries information
// with cursor-based pagination for efficient querying of large datasets.
package registryofregistriespostgresql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

// PostgreSQLRegistryOfRegistriesDatabase provides PostgreSQL-based persistence for the RegistryOfRegistries Service.
//
// It manages AAS identifiers and their associated asset links in a PostgreSQL database,
// using connection pooling for efficient database access. The database schema is automatically
// initialized on startup from the RegistryOfRegistriesschema.sql file.
type PostgreSQLRegistryOfRegistriesDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

// NewPostgreSQLRegistryOfRegistriesBackend creates and initializes a new PostgreSQL RegistryOfRegistries database backend.
//
// This function establishes a connection pool to the PostgreSQL database using the provided DSN
// (Data Source Name), configures connection pool settings, and initializes the database schema
// by executing the RegistryOfRegistriesschema.sql file from the resources/sql directory.
//
// Parameters:
//   - dsn: PostgreSQL connection string (e.g., "postgres://user:pass@localhost:5432/dbname")
//   - maxConns: Maximum number of connections in the pool
//
// Returns:
//   - *PostgreSQLRegistryOfRegistriesDatabase: Initialized database instance
//   - error: Configuration, connection, or schema initialization error
//
// The connection pool is configured with:
//   - MaxConns: Set to the provided maxConns parameter
//   - MaxConnLifetime: 5 minutes to ensure connection freshness
//
// The function reads and executes RegistryOfRegistriesschema.sql from the current working directory's
// resources/sql subdirectory to set up the required database tables.
func NewPostgreSQLRegistryOfRegistriesBackend(dsn string, _ /* maxOpenConns */, _ /* maxIdleConns */ int, _ /* connMaxLifetimeMinutes */ int, cacheEnabled bool, databaseSchema string) (*PostgreSQLRegistryOfRegistriesDatabase, error) {
	fmt.Println(dsn)
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}

	return &PostgreSQLRegistryOfRegistriesDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}

func (p *PostgreSQLRegistryOfRegistriesDatabase) GetRegistryDescriptorById(ctx context.Context, registryIdentifier string) (model.RegistryDescriptor, error) {

	d := goqu.Dialect("postgres")

	reg := goqu.T("registry_descriptor").As("reg")

	sqlStr, args, buildErr := d.
		From(reg).
		Select(
			reg.Col("descriptor_id"),
			reg.Col("registry_type"),
			reg.Col("global_asset_id"),
			reg.Col("id_short"),
			reg.Col("id"),
			reg.Col("administrative_information_id"),
			reg.Col("displayname_id"),
			reg.Col("description_id"),
		).
		Where(reg.Col("id").Eq(registryIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return model.RegistryDescriptor{}, buildErr
	}

	var (
		descID                 int64
		registryType           sql.NullString
		globalAssetID, idShort sql.NullString
		idStr                  string
		adminInfoID            sql.NullInt64
		displayNameID          sql.NullInt64
		descriptionID          sql.NullInt64
	)

	if err := p.db.QueryRowContext(ctx, sqlStr, args...).Scan(
		&descID,
		&registryType,
		&globalAssetID,
		&idShort,
		&idStr,
		&adminInfoID,
		&displayNameID,
		&descriptionID,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.RegistryDescriptor{}, common.NewErrNotFound("Registry Descriptor not found")
		}
		return model.RegistryDescriptor{}, err
	}

	var (
		displayName []model.LangStringNameType
		description []model.LangStringTextType
		endpoints   []model.Endpoint
	)

	displayName, err := persistence_utils.GetLangStringNameTypes(p.db, displayNameID)
	if err != nil {
		return model.RegistryDescriptor{}, err
	}
	description, err = persistence_utils.GetLangStringTextTypes(p.db, descriptionID)
	if err != nil {
		return model.RegistryDescriptor{}, err
	}
	endpoints, err = descriptors.ReadEndpointsByDescriptorID(ctx, p.db, descID)
	if err != nil {
		return model.RegistryDescriptor{}, err
	}

	return model.RegistryDescriptor{
		RegistryType:  registryType.String,
		GlobalAssetId: globalAssetID.String,
		IdShort:       idShort.String,
		Id:            idStr,
		DisplayName:   displayName,
		Description:   description,
		Endpoints:     endpoints,
	}, nil
}

func (p *PostgreSQLRegistryOfRegistriesDatabase) PostRegistryDescriptor(ctx context.Context, registryDescriptor model.RegistryDescriptor) error {
	d := goqu.Dialect("postgres")
	tx, err := p.db.Begin()
	if err != nil {
		return err
	}
	descTbl := goqu.T("")

	sqlStr, args, buildErr := d.
		Insert("descriptor").
		Returning(descTbl.Col("id")).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	var descriptorID int64
	if err := tx.QueryRow(sqlStr, args...).Scan(&descriptorID); err != nil {
		return err
	}

	var displayNameID, descriptionID sql.NullInt64

	dnID, err := persistence_utils.CreateLangStringNameTypes(tx, registryDescriptor.DisplayName)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}
	displayNameID = dnID

	var convertedDescription []model.LangStringText
	for _, desc := range registryDescriptor.Description {
		convertedDescription = append(convertedDescription, desc)
	}
	descID, err := persistence_utils.CreateLangStringTextTypes(tx, convertedDescription)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}
	descriptionID = descID

	sqlStr, args, buildErr = d.
		Insert("registry_descriptor").
		Rows(goqu.Record{
			"descriptor_id":                 descriptorID,
			"description_id":                descriptionID,
			"displayname_id":                displayNameID,
			"administrative_information_id": nil,
			"registry_type":                 registryDescriptor.RegistryType,
			"global_asset_id":               registryDescriptor.GlobalAssetId,
			"id_short":                      registryDescriptor.IdShort,
			"id":                            registryDescriptor.Id,
		}).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return err
	}

	if err = descriptors.CreateEndpoints(tx, descriptorID, registryDescriptor.Endpoints); err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Endpoints - no changes applied - see console for details")
	}

	return tx.Commit()
}
