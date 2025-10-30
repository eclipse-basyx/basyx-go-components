package persistence_postgresql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	submodel_query "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils/SubmodelQuery"
)

func readAdministrativeInformationByID(ctx context.Context, db *sql.DB, table_name string, adminInfoID sql.NullInt64) (model.AdministrativeInformation, error) {

	v, err := readAdministrativeInformationByIDs(ctx, db, table_name, []int64{adminInfoID.Int64})
	return v[adminInfoID.Int64], err
}
func readAdministrativeInformationByIDs(
	ctx context.Context,
	db *sql.DB,
	table_name string,
	adminInfoIDs []int64,
) (map[int64]model.AdministrativeInformation, error) {
	start := time.Now()

	out := make(map[int64]model.AdministrativeInformation, len(adminInfoIDs))
	if len(adminInfoIDs) == 0 {
		return out, nil
	}

	// Deduplicate adminInfoIDs
	seen := make(map[int64]struct{}, len(adminInfoIDs))
	uniq := make([]int64, 0, len(adminInfoIDs))
	for _, id := range adminInfoIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}

	d := goqu.Dialect(dialect)

	adminJSON := submodel_query.GetAdministrationSubquery(d, "s.administrative_information_id")

	q, args, err2 := d.From(goqu.T(table_name).As("s")).
		Select(
			goqu.I("s.administrative_information_id").As("id"),
			goqu.L("COALESCE((?), '[]'::jsonb)", adminJSON), // correlated subquery
		).ToSQL()

	rows2, err2 := db.QueryContext(ctx, q, args...)
	if err2 != nil {
		return nil, err2
	}

	defer rows2.Close()

	type Row struct {
		Id             int64
		Administration json.RawMessage
	}
	for rows2.Next() {
		var row Row
		if err := rows2.Scan(
			&row.Id,
			&row.Administration,
		); err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}

		if isArrayNotEmpty(row.Administration) {
			adminRow, err := builders.ParseAdministrationRow(row.Administration)
			if err != nil {
				fmt.Println(err)
				return nil, err
			}
			if adminRow != nil {

				admin, err := builders.BuildAdministration(*adminRow)
				if err != nil {
					fmt.Println(err)
					return nil, err
				}

				out[row.Id] = model.AdministrativeInformation{
					Version:                    admin.Version,
					Revision:                   admin.Revision,
					TemplateId:                 admin.TemplateId,
					Creator:                    admin.Creator,
					EmbeddedDataSpecifications: admin.EmbeddedDataSpecifications,
				}
			} else {
				fmt.Println("Administration row is nil")
			}
		}
	}

	duration := time.Since(start)
	fmt.Printf("admin info block took %v to complete\n", duration)
	return out, nil
}
func isArrayNotEmpty(data json.RawMessage) bool {
	return len(data) > 0 && string(data) != "null"
}
