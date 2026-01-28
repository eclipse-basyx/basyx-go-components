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

// Package persistencepostgresql provides PostgreSQL-based persistence implementation
// for the Eclipse BaSyx Discovery Service.
//
// This package implements the storage and retrieval of Asset Administration Shell (AAS)
// identifiers and their associated asset links in a PostgreSQL database. It supports
// operations for creating, retrieving, searching, and deleting AAS discovery information
// with cursor-based pagination for efficient querying of large datasets.
package persistencepostgresql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	// nolint:revive
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/descriptors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// PostgreSQLDiscoveryDatabase provides PostgreSQL-based persistence for the Discovery Service.
//
// It manages AAS identifiers and their associated specific asset IDs in a PostgreSQL database,
// using connection pooling for efficient database access. The database schema can be initialized
// on startup via the provided schema path.
type PostgreSQLDiscoveryDatabase struct {
	db *sql.DB
}

// NewPostgreSQLDiscoveryBackend creates and initializes a new PostgreSQL discovery database backend.
//
// This function establishes a connection pool to the PostgreSQL database using the provided DSN
// (Data Source Name), configures connection pool settings, and optionally initializes the database
// schema using the provided schema file path.
//
// Parameters:
//   - dsn: PostgreSQL connection string (e.g., "postgres://user:pass@localhost:5432/dbname")
//   - maxOpenConns: Maximum number of open connections in the pool
//   - maxIdleConns: Maximum number of idle connections in the pool
//   - connMaxLifetimeMinutes: Maximum connection lifetime in minutes
//   - databaseSchema: SQL schema file path for initialization (empty to skip)
//
// Returns:
//   - *PostgreSQLDiscoveryDatabase: Initialized database instance
//   - error: Configuration, connection, or schema initialization error
func NewPostgreSQLDiscoveryBackend(
	dsn string,
	maxOpenConns int32,
	maxIdleConns int,
	connMaxLifetimeMinutes int,
	databaseSchema string,
) (*PostgreSQLDiscoveryDatabase, error) {
	db, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return nil, err
	}
	if maxOpenConns > 0 {
		db.SetMaxOpenConns(int(maxOpenConns))
	}
	if maxIdleConns > 0 {
		db.SetMaxIdleConns(maxIdleConns)
	}
	if connMaxLifetimeMinutes > 0 {
		db.SetConnMaxLifetime(time.Duration(connMaxLifetimeMinutes) * time.Minute)
	}

	return &PostgreSQLDiscoveryDatabase{db: db}, nil
}

// GetAllAssetLinks retrieves all asset links associated with a specific AAS identifier.
//
// This method queries the database for all asset links (name-value pairs) that belong
// to the specified AAS. The asset links are returned in order by their database ID.
//
// Parameters:
//   - aasID: The AAS identifier (string) to retrieve asset links for
//
// Returns:
//   - []model.SpecificAssetID: Slice of asset links with name and value fields
//   - error: ErrNotFound if the AAS identifier doesn't exist, or InternalServerError on database failures
//
// The method operates within a transaction to ensure consistency, though it performs
// read-only operations. If the AAS identifier is not found in the database, an ErrNotFound
// error is returned.
func (p *PostgreSQLDiscoveryDatabase) GetAllAssetLinks(ctx context.Context, aasID string) ([]model.SpecificAssetID, error) {
	links, err := descriptors.ReadSpecificAssetIDsByAASIdentifier(ctx, p.db, aasID)
	if err != nil {
		switch {
		case common.IsErrNotFound(err):
			return nil, err
		default:
			_, _ = fmt.Println(err)
			return nil, common.NewInternalServerError("Failed to query specific asset IDs. See console for information.")
		}
	}
	return links, nil
}

// DeleteAllAssetLinks deletes an AAS identifier and all its associated asset links.
//
// This method removes the AAS identifier record from the database, which cascades to
// delete all associated asset links due to foreign key constraints in the database schema.
//
// Parameters:
//   - aasID: The AAS identifier (string) to delete
//
// Returns:
//   - error: ErrNotFound if the AAS identifier doesn't exist, or InternalServerError on database failures
//
// The deletion is performed atomically. If the AAS identifier is not found (no rows affected),
// an ErrNotFound error is returned.
func (p *PostgreSQLDiscoveryDatabase) DeleteAllAssetLinks(ctx context.Context, aasID string) error {
	d := goqu.Dialect("postgres")
	sqlStr, args, err := d.Delete("aas_identifier").
		Where(goqu.C("aasid").Eq(aasID)).
		ToSQL()
	if err != nil {
		_, _ = fmt.Println("DeleteAllAssetLinks: build error:", err)
		return common.NewInternalServerError("Failed to delete AAS identifier. See console for information.")
	}
	result, err := p.db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to delete AAS identifier. See console for information.")
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return common.NewErrNotFound(fmt.Sprintf("AAS identifier %s not found. See console for information.", aasID))
	}
	return nil
}

// CreateAllAssetLinks creates or updates an AAS identifier with its associated asset links.
//
// This method performs an "upsert" operation: if the AAS identifier already exists,
// it updates the record; otherwise, it creates a new one. All existing asset links for
// the AAS are deleted and replaced with the provided new set of asset links.
//
// Parameters:
//   - aas_id: The AAS identifier (string) to create or update
//   - specific_asset_ids: Slice of asset links (name-value pairs) to associate with the AAS
//
// Returns:
//   - error: InternalServerError on database operation failures
//
// The operation is performed within a transaction to ensure atomicity:
//  1. Insert or update the AAS identifier (using ON CONFLICT DO UPDATE)
//  2. Delete all existing asset links for this AAS
//  3. Bulk insert the new asset links using PostgreSQL's COPY FROM feature for efficiency
//
// The use of COPY FROM makes this method highly efficient even for large numbers of asset links.
func (p *PostgreSQLDiscoveryDatabase) CreateAllAssetLinks(ctx context.Context, aasID string, specificAssetIDs []model.SpecificAssetID) error {
	if err := descriptors.ReplaceSpecificAssetIDsByAASIdentifier(ctx, p.db, aasID, specificAssetIDs); err != nil {
		_, _ = fmt.Println(err)
		return common.NewInternalServerError("Failed to store specific asset IDs. See console for information.")
	}
	return nil
}

// SearchAASIDsByAssetLinks searches for AAS identifiers that match the specified asset links.
//
// This method performs a search for AAS identifiers based on asset link criteria, with support
// for cursor-based pagination. If no asset links are provided, it returns all AAS identifiers.
// If asset links are provided, it returns only AAS identifiers that have ALL the specified
// asset links (AND logic).
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - links: Slice of asset links to search for (name-value pairs). Empty slice returns all AAS IDs
//   - limit: Maximum number of results to return (default 100 if <= 0)
//   - cursor: Pagination cursor (AAS ID to start from). Empty string starts from the beginning
//
// Returns:
//   - []string: Slice of matching AAS identifiers, sorted alphabetically
//   - string: Next cursor for pagination (empty string if no more results)
//   - error: InternalServerError on database query failures
//
// Pagination:
// The method uses cursor-based pagination where the cursor is the AAS ID to start from.
// Results are sorted alphabetically by AAS ID. The method fetches limit+1 results to
// determine if there are more pages available. If more results exist, the last result
// is used as the next cursor.
//
// Search Logic:
//   - Empty links: Returns all AAS IDs (paginated)
//   - With links: Returns AAS IDs that have ALL specified asset links (exact name-value matches)
//     Uses EXISTS subqueries to enforce the AND semantics
func (p *PostgreSQLDiscoveryDatabase) SearchAASIDsByAssetLinks(
	ctx context.Context,
	links []model.AssetLink,
	limit int32,
	cursor string,
) ([]string, string, error) {
	if limit <= 0 {
		limit = 100
	}

	peekLimit := int(limit) + 1

	d := goqu.Dialect("postgres")
	ai := goqu.T("aas_identifier")
	sai := goqu.T("specific_asset_id").As("sai")

	ds := d.From(ai).
		Select(ai.Col("aasid")).
		Where(
			goqu.Or(
				goqu.V(cursor).Eq(""),
				ai.Col("aasid").Gte(cursor),
			),
		)

	for _, link := range links {
		sub := d.From(sai).
			Select(goqu.V(1)).
			Where(sai.Col("aasref").Eq(ai.Col("id"))).
			Where(sai.Col("name").Eq(link.Name)).
			Where(sai.Col("value").Eq(link.Value))
		ds = ds.Where(goqu.L("EXISTS ?", sub))
	}

	ds = ds.
		Order(ai.Col("aasid").Asc()).
		Limit(uint(peekLimit))

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootBD)
	if err != nil {
		_, _ = fmt.Println("SearchAASIDsByAssetLinks: collector error:", err)
		return nil, "", common.NewInternalServerError("Failed to build query filters. See server logs for details.")
	}
	ds, err = auth.AddFormulaQueryFromContext(ctx, ds, collector)
	if err != nil {
		_, _ = fmt.Println("SearchAASIDsByAssetLinks: filter error:", err)
		return nil, "", common.NewInternalServerError("Failed to build query filters. See server logs for details.")
	}
	ds, err = auth.ApplyResolvedFieldPathCTEs(ds, collector, nil)
	if err != nil {
		_, _ = fmt.Println("SearchAASIDsByAssetLinks: cte error:", err)
		return nil, "", common.NewInternalServerError("Failed to build query filters. See server logs for details.")
	}

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		_, _ = fmt.Println("SearchAASIDsByAssetLinks: sql build error:", err)
		return nil, "", common.NewInternalServerError("Failed to query AAS IDs. See server logs for details.")
	}

	rows, err := p.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		_, _ = fmt.Println("SearchAASIDsByAssetLinks: query error:", err)
		return nil, "", common.NewInternalServerError("Failed to query AAS IDs. See server logs for details.")
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_, _ = fmt.Println("SearchAASIDsByAssetLinks: rows close error:", closeErr)
		}
	}()

	buf := make([]string, 0, peekLimit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_, _ = fmt.Println("SearchAASIDsByAssetLinks: scan error:", err)
			return nil, "", common.NewInternalServerError("Failed to scan AAS ID. See server logs for details.")
		}
		buf = append(buf, id)
	}
	if rows.Err() != nil {
		_, _ = fmt.Println("SearchAASIDsByAssetLinks: rows error:", rows.Err())
		return nil, "", common.NewInternalServerError("Failed to iterate AAS IDs. See server logs for details.")
	}

	if len(buf) > int(limit) {
		result := buf[:limit]
		nextCursor := buf[limit]
		return result, nextCursor, nil
	}

	return buf, "", nil
}
