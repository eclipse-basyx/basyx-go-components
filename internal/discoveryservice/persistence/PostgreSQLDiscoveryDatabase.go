package persistence_postgresql

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi/go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgreSQLDiscoveryDatabase struct {
	pool *pgxpool.Pool
}

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

// In persistence_postgresql/postgresql_discovery.go

func (p *PostgreSQLDiscoveryDatabase) GetAllAssetLinks(aasID string) ([]openapi.SpecificAssetId, error) {
	ctx := context.Background()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		fmt.Println(err)
		return nil, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer tx.Rollback(ctx)

	var referenceID int64
	// 1) Find the AAS identifier row
	if err := tx.QueryRow(ctx, `SELECT id FROM aas_identifier WHERE aasId = $1`, aasID).Scan(&referenceID); err != nil {
		if err == pgx.ErrNoRows {
			// AAS does not exist -> 404
			return nil, common.NewErrNotFound("AAS identifier '" + aasID + "'")
		}
		fmt.Println(err)
		return nil, common.NewInternalServerError("Failed to fetch aas identifier. See console for information.")
	}

	// 2) Load all asset links for this AAS (could be zero)
	rows, err := tx.Query(ctx, `SELECT name, value FROM asset_link WHERE aasRef = $1 ORDER BY id`, referenceID)
	if err != nil {
		fmt.Println(err)
		return nil, common.NewInternalServerError("Failed to query asset links. See console for information.")
	}
	defer rows.Close()

	var result []openapi.SpecificAssetId
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			fmt.Println(err)
			return nil, common.NewInternalServerError("Failed to scan asset link. See console for information.")
		}
		result = append(result, openapi.SpecificAssetId{
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

func (p *PostgreSQLDiscoveryDatabase) DeleteAllAssetLinks(aas_id string) error {
	ctx := context.Background()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer tx.Rollback(ctx)

	var referenceID int64
	// Find the existing aas_identifier row
	err = tx.QueryRow(ctx, "SELECT id FROM aas_identifier WHERE aasId = $1", aas_id).Scan(&referenceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No such AAS identifier exists — nothing to delete
			return common.NewErrBadRequest(fmt.Sprintf("AAS identifier %s not found. See console for information.", aas_id))
		}
		fmt.Println(err)
		return common.NewInternalServerError("Failed to fetch aas identifier. See console for information.")
	}

	// Delete the aas_identifier itself
	if _, err := tx.Exec(ctx, `DELETE FROM aas_identifier WHERE id = $1`, referenceID); err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to remove aas identifier. See console for information.")
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to commit postgres transaction. See console for information.")
	}

	return nil
}

func (p *PostgreSQLDiscoveryDatabase) CreateAllAssetLinks(aas_id string, specific_asset_ids []openapi.SpecificAssetId) error {
	ctx := context.Background()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer tx.Rollback(ctx)

	var referenceID int64
	err = tx.QueryRow(ctx, "INSERT INTO aas_identifier (aasId) VALUES ($1) ON CONFLICT (aasId) DO UPDATE SET aasId = EXCLUDED.aasId RETURNING id", aas_id).Scan(&referenceID)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to insert aas identifier. See console for information.")
	}

	if _, err := tx.Exec(ctx, `DELETE FROM asset_link WHERE aasRef = $1`, referenceID); err != nil {
		return common.NewInternalServerError("Failed to remove old asset links.")
	}

	rows := make([][]any, len(specific_asset_ids))
	for i, v := range specific_asset_ids {
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

func (p *PostgreSQLDiscoveryDatabase) SearchAASIDsByAssetLinks(
	ctx context.Context,
	links []openapi.AssetLink,
	limit int32,
	cursor string,
) ([]string, string, error) {
	if limit <= 0 {
		limit = 100
	}

	args := []any{}
	argPos := 1

	// Keyset filter by aasId (string). If cursor is empty, start from the beginning.
	whereCursor := fmt.Sprintf("( $%d = '' OR ai.aasId > $%d )", argPos, argPos)
	args = append(args, cursor)
	argPos++

	var sql string

	if len(links) == 0 {
		// No name/value filters -> just paginate over all AAS IDs
		// Note: ORDER BY aasId ensures stable, lexicographic keyset pagination.
		sql = fmt.Sprintf(`
			SELECT ai.aasId
			FROM aas_identifier ai
			WHERE %s
			ORDER BY ai.aasId ASC
			LIMIT $%d
		`, whereCursor, argPos)
		args = append(args, int(limit))
	} else {
		// Build VALUES table for (name,value) pairs
		valuesSQL := ""
		for i, l := range links {
			if i > 0 {
				valuesSQL += ", "
			}
			// ($argPos, $argPos+1)
			valuesSQL += fmt.Sprintf("($%d, $%d)", argPos, argPos+1)
			args = append(args, l.Name, l.Value)
			argPos += 2
		}

		// Compose query: match AAS that contain *all* provided (name,value) pairs.
		// We count DISTINCT pairs to avoid duplicates in asset_link skewing the count.
		sql = fmt.Sprintf(`
			WITH v(name, value) AS (VALUES %s)
			SELECT ai.aasId
			FROM aas_identifier ai
			JOIN asset_link al ON al.aasRef = ai.id
			JOIN v ON v.name = al.name AND v.value = al.value
			WHERE %s
			GROUP BY ai.aasId
			HAVING COUNT(DISTINCT (al.name, al.value)) = (SELECT COUNT(*) FROM v)
			ORDER BY ai.aasId ASC
			LIMIT $%d
		`, valuesSQL, whereCursor, argPos)

		args = append(args, int(limit))
	}

	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		fmt.Println(err)
		return nil, "", common.NewInternalServerError("Failed to query AAS IDs. See console for information.")
	}
	defer rows.Close()

	var aasIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			fmt.Println(err)
			return nil, "", common.NewInternalServerError("Failed to scan AAS ID. See console for information.")
		}
		aasIDs = append(aasIDs, id)
	}
	if rows.Err() != nil {
		fmt.Println(rows.Err())
		return nil, "", common.NewInternalServerError("Failed to iterate AAS IDs. See console for information.")
	}

	// Next cursor is the last aasId on this page (or empty if no results).
	var nextCursor string
	if n := len(aasIDs); n > 0 {
		nextCursor = aasIDs[n-1]
	}

	return aasIDs, nextCursor, nil
}
