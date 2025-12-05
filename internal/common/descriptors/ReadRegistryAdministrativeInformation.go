package descriptors

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	builders "github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/queries"
	"github.com/lib/pq"
)

// ReadRegistryAdministrativeInformationByID fetches a single RegistryAdministrativeInformation
// referenced by a nullable foreign key in the given table.
//
// Parameters:
//   - ctx: request context used for cancellation/deadlines
//   - db:  open SQL database handle
//   - tableName: name of the table that contains the administrative_information_id
//     column (registry_descriptor)
//   - adminInfoID: nullable FK value pointing to the administration block
//
// Returns a zero value when the FK is NULL/invalid and a NotFound‑style error
// when the FK is valid but the referenced administration block is missing.
func ReadRegistryAdministrativeInformationByID(
	ctx context.Context,
	db *sql.DB,
	tableName string,
	adminInfoID sql.NullInt64,
) (*model.RegistryAdministrativeInformation, error) {
	if !adminInfoID.Valid {
		return &model.RegistryAdministrativeInformation{}, errors.New("administrative information ID is NULL/invalid")
	}

	m, err := ReadRegistryAdministrativeInformationByIDs(ctx, db, tableName, []int64{adminInfoID.Int64})
	if err != nil {
		return &model.RegistryAdministrativeInformation{}, err
	}
	v, ok := m[adminInfoID.Int64]
	if !ok {
		return &model.RegistryAdministrativeInformation{}, fmt.Errorf("administrative information with id %d not found", adminInfoID.Int64)
	}
	return v, nil
}

// ReadRegistryAdministrativeInformationByIDs fetches multiple RegistryAdministrativeInformation
// records for the provided FK values from the given table and returns them
// keyed by the FK (administrative_information_id). Missing IDs are omitted from
// the map.
//
// The function issues a single SQL SELECT over the given table, using a
// correlated subquery to produce the JSON shape expected by the builder, and
// then parses/assembles the Administration objects. Duplicate IDs in the input
// are de‑duplicated before querying.
func ReadRegistryAdministrativeInformationByIDs(
	ctx context.Context,
	db *sql.DB,
	tableName string,
	adminInfoIDs []int64,
) (map[int64]*model.RegistryAdministrativeInformation, error) {
	out := make(map[int64]*model.RegistryAdministrativeInformation, len(adminInfoIDs))
	if len(adminInfoIDs) == 0 {
		return out, nil
	}

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
		return out, nil
	}

	d := goqu.Dialect(dialect)

	// Correlated subquery that returns JSON for the administration block.
	adminJSON := queries.GetRegistryAdministrationSubquery(d, fmt.Sprintf("s.%s", colAdminInfoID))

	// SELECT only the requested IDs.
	arr := pq.Array(uniq)
	ds := d.From(goqu.T(tableName).As("s")).
		Select(
			goqu.I(fmt.Sprintf("s.%s", colAdminInfoID)).As("id"),
			goqu.L("COALESCE((?), '[]'::jsonb)", adminJSON),
		).
		Where(goqu.L(fmt.Sprintf("s.%s = ANY(?::bigint[])", colAdminInfoID), arr))

	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("building SQL failed: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying registry administrative information failed: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var r row
		if err := rows.Scan(&r.ID, &r.Administration); err != nil {
			return nil, fmt.Errorf("scanning registry administrative information row failed: %w", err)
		}

		if !common.IsArrayNotEmpty(r.Administration) {
			continue
		}

		adminRow, err := builders.ParseRegistryAdministrationRow(r.Administration)
		if err != nil {
			return nil, fmt.Errorf("parsing registry administration row (id %d) failed: %w", r.ID, err)
		}
		if adminRow == nil {
			continue
		}

		admin, err := builders.BuildRegistryAdministration(*adminRow)
		if err != nil {
			return nil, fmt.Errorf("building registry administration (id %d) failed: %w", r.ID, err)
		}

		out[r.ID] = &model.RegistryAdministrativeInformation{
			Version:    admin.Version,
			Revision:   admin.Revision,
			TemplateID: admin.TemplateID,
			Creator:    admin.Creator,
			Company:    admin.Company,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration failed: %w", err)
	}

	return out, nil
}
