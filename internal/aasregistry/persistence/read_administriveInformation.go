package aasregistrydatabase

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/queries"
)

// readAdministrativeInformationByID fetches a single AdministrativeInformation by a nullable ID.
func readAdministrativeInformationByID(
	ctx context.Context,
	db Queryer,
	tableName string,
	adminInfoID sql.NullInt64,
) (*model.AdministrativeInformation, error) {
	if !adminInfoID.Valid {
		return &model.AdministrativeInformation{}, errors.New("administrative information ID is NULL/invalid")
	}

	m, err := readAdministrativeInformationByIDs(ctx, db, tableName, []int64{adminInfoID.Int64})
	if err != nil {
		return &model.AdministrativeInformation{}, err
	}
	v, ok := m[adminInfoID.Int64]
	if !ok {
		return &model.AdministrativeInformation{}, fmt.Errorf("administrative information with id %d not found", adminInfoID.Int64)
	}
	return v, nil
}

// readAdministrativeInformationByIDs fetches multiple AdministrativeInformation records keyed by ID.
func readAdministrativeInformationByIDs(
	ctx context.Context,
	db Queryer,
	tableName string,
	adminInfoIDs []int64,
) (map[int64]*model.AdministrativeInformation, error) {
	start := time.Now()
	out := make(map[int64]*model.AdministrativeInformation, len(adminInfoIDs))
	if len(adminInfoIDs) == 0 {
		duration := time.Since(start)
		fmt.Printf("1 admin info block took %v to complete\n", duration)
		return out, nil
	}

	// Deduplicate IDs
	seen := make(map[int64]struct{}, len(adminInfoIDs))
	uniq := make([]int64, 0, len(adminInfoIDs))
	for _, id := range adminInfoIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}

	if len(uniq) == 0 {

		duration := time.Since(start)
		fmt.Printf("2 admin info block took %v to complete\n", duration)
		return out, nil
	}

	d := goqu.Dialect(dialect)

	// Correlated subquery that returns JSON for the administration block.
	adminJSON := queries.GetAdministrationSubquery(d, "s.administrative_information_id")

	// SELECT only the requested IDs.
	ds := d.From(goqu.T(tableName).As("s")).
		Select(
			goqu.I("s.administrative_information_id").As("id"),
			goqu.L("COALESCE((?), '[]'::jsonb)", adminJSON),
		).
		Where(goqu.I("s.administrative_information_id").In(uniq))

	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("building SQL failed: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying administrative information failed: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	type row struct {
		ID             int64
		Administration json.RawMessage
	}

	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Administration); err != nil {
			return nil, fmt.Errorf("scanning administrative information row failed: %w", err)
		}

		if !common.IsArrayNotEmpty(r.Administration) {
			continue
		}

		adminRow, err := builders.ParseAdministrationRow(r.Administration)
		if err != nil {
			return nil, fmt.Errorf("parsing administration row (id %d) failed: %w", r.ID, err)
		}
		if adminRow == nil {
			continue
		}

		admin, err := builders.BuildAdministration(*adminRow)
		if err != nil {
			return nil, fmt.Errorf("building administration (id %d) failed: %w", r.ID, err)
		}

		out[r.ID] = &model.AdministrativeInformation{
			Version:                    admin.Version,
			Revision:                   admin.Revision,
			TemplateID:                 admin.TemplateID,
			Creator:                    admin.Creator,
			EmbeddedDataSpecifications: admin.EmbeddedDataSpecifications,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration failed: %w", err)
	}
	duration := time.Since(start)
	fmt.Printf("admin info block took %v to complete\n", duration)

	return out, nil
}
