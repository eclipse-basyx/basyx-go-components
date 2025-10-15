package persistence_postgresql

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func readAdministrativeInformationByID(ctx context.Context, db *sql.DB, adminInfoID sql.NullInt64) (model.AdministrativeInformation, error) {
	if !adminInfoID.Valid {
		return model.AdministrativeInformation{}, nil
	}

	d := goqu.Dialect(dialect)

	ai := goqu.T(tblAdministrativeInformation).As("ai")

	sqlStr, args, err := d.
		From(ai).
		Select(
			ai.Col(colVersion),
			ai.Col(colRevision),
			ai.Col(colTemplateId),
			ai.Col(colCreator),
		).
		Where(ai.Col(colID).Eq(adminInfoID.Int64)).
		Limit(1).
		ToSQL()
	if err != nil {
		return model.AdministrativeInformation{}, err
	}

	var (
		version, revision, templateID sql.NullString
		creatorRefID                  sql.NullInt64
	)

	if err := db.QueryRowContext(ctx, sqlStr, args...).Scan(
		&version,
		&revision,
		&templateID,
		&creatorRefID,
	); err != nil {
		return model.AdministrativeInformation{}, err
	}

	var creatorRef *model.Reference
	if creatorRefID.Valid {
		if ref, err := persistence_utils.GetReferenceByReferenceDBID(db, creatorRefID); err == nil {
			creatorRef = ref
		}
	}

	return model.AdministrativeInformation{
		Version:    version.String,
		Revision:   revision.String,
		TemplateId: templateID.String,
		Creator:    creatorRef,
	}, nil
}
