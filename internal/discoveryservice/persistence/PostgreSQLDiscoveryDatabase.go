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
// for the Eclipse BaSyx Discovery Service.
//
// This package implements the storage and retrieval of Asset Administration Shell (AAS)
// identifiers and their associated asset links in a PostgreSQL database. It supports
// operations for creating, retrieving, searching, and deleting AAS discovery information
// with cursor-based pagination for efficient querying of large datasets.
package persistencepostgresql

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgreSQLDiscoveryDatabase provides PostgreSQL-based persistence for the Discovery Service.
//
// It manages AAS identifiers and their associated asset links in a PostgreSQL database,
// using connection pooling for efficient database access. The database schema is automatically
// initialized on startup from the discoveryschema.sql file.
type PostgreSQLDiscoveryDatabase struct {
	pool *pgxpool.Pool
}

// NewPostgreSQLDiscoveryBackend creates and initializes a new PostgreSQL discovery database backend.
//
// This function establishes a connection pool to the PostgreSQL database using the provided DSN
// (Data Source Name), configures connection pool settings, and initializes the database schema
// by executing the discoveryschema.sql file from the resources/sql directory.
//
// Parameters:
//   - dsn: PostgreSQL connection string (e.g., "postgres://user:pass@localhost:5432/dbname")
//   - maxConns: Maximum number of connections in the pool
//
// Returns:
//   - *PostgreSQLDiscoveryDatabase: Initialized database instance
//   - error: Configuration, connection, or schema initialization error
//
// The connection pool is configured with:
//   - MaxConns: Set to the provided maxConns parameter
//   - MaxConnLifetime: 5 minutes to ensure connection freshness
//
// The function reads and executes discoveryschema.sql from the current working directory's
// resources/sql subdirectory to set up the required database tables.
func NewPostgreSQLDiscoveryBackend(dsn string, maxConns int) (*PostgreSQLDiscoveryDatabase, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = int32(maxConns)
	cfg.MaxConnLifetime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	dir, _ := os.Getwd()
	schema, err := os.ReadFile(dir + "/resources/sql/discoveryschema.sql")
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(context.Background(), string(schema)); err != nil {
		return nil, err
	}

	return &PostgreSQLDiscoveryDatabase{pool: pool}, nil
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
func (p *PostgreSQLDiscoveryDatabase) GetAllAssetLinks(aasID string) ([]model.SpecificAssetID, error) {
	ctx := context.Background()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		fmt.Println(err)
		return nil, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var referenceID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM aas_identifier WHERE aasID = $1`, aasID).Scan(&referenceID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, common.NewErrNotFound("AAS identifier '" + aasID + "'")
		}
		fmt.Println(err)
		return nil, common.NewInternalServerError("Failed to fetch aas identifier. See console for information.")
	}

	rows, err := tx.Query(ctx, `SELECT name, value FROM asset_link WHERE aasRef = $1 ORDER BY id`, referenceID)
	if err != nil {
		fmt.Println(err)
		return nil, common.NewInternalServerError("Failed to query asset links. See console for information.")
	}
	defer rows.Close()

	var result []model.SpecificAssetID
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			fmt.Println(err)
			return nil, common.NewInternalServerError("Failed to scan asset link. See console for information.")
		}
		result = append(result, model.SpecificAssetID{
			Name:  name,
			Value: value,
		})
	}
	if rows.Err() != nil {
		fmt.Println(rows.Err())
		return nil, common.NewInternalServerError("Failed to iterate asset links. See console for information.")
	}

	if err := tx.Commit(ctx); err != nil {
		fmt.Println(err)
		return nil, common.NewInternalServerError("Failed to commit postgres transaction. See console for information.")
	}

	return result, nil
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
func (p *PostgreSQLDiscoveryDatabase) DeleteAllAssetLinks(aasID string) error {
	ctx := context.Background()

	tag, err := p.pool.Exec(ctx, `DELETE FROM aas_identifier WHERE aasID = $1`, aasID)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to delete AAS identifier. See console for information.")
	}
	if tag.RowsAffected() == 0 {
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
func (p *PostgreSQLDiscoveryDatabase) CreateAllAssetLinks(aasID string, specificAssetIDs []model.SpecificAssetID) error {
	ctx := context.Background()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var referenceID int64
	err = tx.QueryRow(ctx, "INSERT INTO aas_identifier (aasID) VALUES ($1) ON CONFLICT (aasID) DO UPDATE SET aasID = EXCLUDED.aasID RETURNING id", aasID).Scan(&referenceID)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to insert aas identifier. See console for information.")
	}

	if _, err := tx.Exec(ctx, `DELETE FROM asset_link WHERE aasRef = $1`, referenceID); err != nil {
		return common.NewInternalServerError("Failed to remove old asset links.")
	}

	rows := make([][]any, len(specificAssetIDs))
	for i, v := range specificAssetIDs {
		rows[i] = []any{v.Name, v.Value, referenceID}
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"asset_link"},
		[]string{"name", "value", "aasref"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to insert asset link. See console for information.")
	}

	if err := tx.Commit(ctx); err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to commit postgres transaction. See console for information.")
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
//     Uses a GROUP BY with HAVING COUNT to ensure all links are present
//
// The query uses a Common Table Expression (CTE) to efficiently match asset links when provided.
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

	args := []any{}
	argPos := 1
	whereCursor := fmt.Sprintf("( $%d = '' OR ai.aasID >= $%d )", argPos, argPos)
	args = append(args, cursor)
	argPos++

	var sqlStr string
	if len(links) == 0 {
		sqlStr = fmt.Sprintf(`
			SELECT ai.aasId
			FROM aas_identifier ai
			WHERE %s
			ORDER BY ai.aasID ASC
			LIMIT $%d
		`, whereCursor, argPos)
		args = append(args, peekLimit)
	} else {
		var valuesSQL strings.Builder
		for i, l := range links {
			if i > 0 {
				valuesSQL.WriteString(", ")
			}
			valuesSQL.WriteString(fmt.Sprintf("($%d, $%d)", argPos, argPos+1))
			args = append(args, l.Name, l.Value)
			argPos += 2
		}

		sqlStr = fmt.Sprintf(`
			WITH v(name, value) AS (VALUES %s)
			SELECT ai.aasId
			FROM aas_identifier ai
			JOIN asset_link al ON al.aasRef = ai.id
			JOIN v ON v.name = al.name AND v.value = al.value
			WHERE %s
			GROUP BY ai.aasId
			HAVING COUNT(DISTINCT (al.name, al.value)) = (SELECT COUNT(*) FROM v)
			ORDER BY ai.aasID ASC
			LIMIT $%d
		`, valuesSQL.String(), whereCursor, argPos)
		args = append(args, peekLimit)
	}

	rows, err := p.pool.Query(ctx, sqlStr, args...)
	if err != nil {
		fmt.Println("SearchAASIDsByAssetLinks: query error:", err)
		return nil, "", common.NewInternalServerError("Failed to query AAS IDs. See server logs for details.")
	}
	defer rows.Close()

	buf := make([]string, 0, peekLimit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			fmt.Println("SearchAASIDsByAssetLinks: scan error:", err)
			return nil, "", common.NewInternalServerError("Failed to scan AAS ID. See server logs for details.")
		}
		buf = append(buf, id)
	}
	if rows.Err() != nil {
		fmt.Println("SearchAASIDsByAssetLinks: rows error:", rows.Err())
		return nil, "", common.NewInternalServerError("Failed to iterate AAS IDs. See server logs for details.")
	}

	if len(buf) > int(limit) {
		result := buf[:limit]
		nextCursor := buf[limit]
		return result, nextCursor, nil
	}

	return buf, "", nil
}
