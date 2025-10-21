package persistence_postgresql

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func readAdministrativeInformationByID(ctx context.Context, db *sql.DB, adminInfoID int64) (model.AdministrativeInformation, error) {
	v, err := readAdministrativeInformationByIDs(ctx, db, []int64{adminInfoID})
	return v[adminInfoID], err
}
func readAdministrativeInformationByIDs(
	ctx context.Context,
	db *sql.DB,
	adminInfoIDs []int64,
) (map[int64]model.AdministrativeInformation, error) {
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
	ai := goqu.T(tblAdministrativeInformation).As("ai")

	sqlStr, args, err := d.
		From(ai).
		Select(
			ai.Col(colID),
			ai.Col(colVersion),
			ai.Col(colRevision),
			ai.Col(colTemplateId),
			ai.Col(colCreator),
		).
		Where(ai.Col(colID).In(uniq)).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Gather creator reference IDs to resolve in one batch
	creatorIDs := make([]int64, 0, len(uniq))
	creatorByAdmin := make(map[int64]int64, len(uniq)) // adminInfoID -> creatorRefID
	seenCreator := make(map[int64]struct{}, len(uniq))

	for rows.Next() {
		var (
			id                            int64
			version, revision, templateID sql.NullString
			creatorRefID                  sql.NullInt64
		)
		if err := rows.Scan(&id, &version, &revision, &templateID, &creatorRefID); err != nil {
			return nil, err
		}

		if creatorRefID.Valid {
			creatorByAdmin[id] = creatorRefID.Int64
			if _, ok := seenCreator[creatorRefID.Int64]; !ok {
				seenCreator[creatorRefID.Int64] = struct{}{}
				creatorIDs = append(creatorIDs, creatorRefID.Int64)
			}
		}

		out[id] = model.AdministrativeInformation{
			Version:    version.String,
			Revision:   revision.String,
			TemplateId: templateID.String,
			Creator:    nil, // fill after batch fetch
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Resolve all creators in one go
	if len(creatorIDs) > 0 {
		if refsByID, err := GetReferencesByIdsBatch(db, creatorIDs); err == nil {
			for adminID, refID := range creatorByAdmin {
				ai := out[adminID]
				ai.Creator = refsByID[refID] // may be nil if missing, which is fine
				out[adminID] = ai
			}
		}
	}

	return out, nil
}
