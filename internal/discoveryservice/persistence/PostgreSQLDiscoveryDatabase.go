package persistence_postgresql

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

func (p *PostgreSQLDiscoveryDatabase) GetAllAssetLinks(aasID string) ([]model.SpecificAssetId, error) {
	ctx := context.Background()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		fmt.Println(err)
		return nil, common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer tx.Rollback(ctx)

	var referenceID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM aas_identifier WHERE aasId = $1`, aasID).Scan(&referenceID); err != nil {
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

	var result []model.SpecificAssetId
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			fmt.Println(err)
			return nil, common.NewInternalServerError("Failed to scan asset link. See console for information.")
		}
		result = append(result, model.SpecificAssetId{
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

func (p *PostgreSQLDiscoveryDatabase) DeleteAllAssetLinks(aasID string) error {
	ctx := context.Background()

	tag, err := p.pool.Exec(ctx, `DELETE FROM aas_identifier WHERE aasId = $1`, aasID)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to delete AAS identifier. See console for information.")
	}
	if tag.RowsAffected() == 0 {
		return common.NewErrNotFound(fmt.Sprintf("AAS identifier %s not found. See console for information.", aasID))
	}
	return nil
}

func (p *PostgreSQLDiscoveryDatabase) CreateAllAssetLinks(aas_id string, specific_asset_ids []model.SpecificAssetId) error {
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
	whereCursor := fmt.Sprintf("( $%d = '' OR ai.aasId >= $%d )", argPos, argPos)
	args = append(args, cursor)
	argPos++

	var sqlStr string
	if len(links) == 0 {
		sqlStr = fmt.Sprintf(`
			SELECT ai.aasId
			FROM aas_identifier ai
			WHERE %s
			ORDER BY ai.aasId ASC
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
			ORDER BY ai.aasId ASC
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
