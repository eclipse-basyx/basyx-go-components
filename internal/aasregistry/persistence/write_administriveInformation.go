package persistence_postgresql

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func CreateAdministrativeInformation(tx *sql.Tx, admin *model.AdministrativeInformation) (sql.NullInt64, error) {
	if admin == nil ||
		(admin.Version == "" && admin.Revision == "" && admin.TemplateId == "" && admin.Creator == nil) {
		return sql.NullInt64{}, nil
	}

	var creatorRefID sql.NullInt64
	if admin.Creator != nil {
		id, err := persistence_utils.CreateReference(tx, admin.Creator)
		if err != nil {
			return sql.NullInt64{}, err
		}
		creatorRefID = id
	}

	version := sql.NullString{String: admin.Version, Valid: admin.Version != ""}
	revision := sql.NullString{String: admin.Revision, Valid: admin.Revision != ""}
	templateID := sql.NullString{String: admin.TemplateId, Valid: admin.TemplateId != ""}

	d := goqu.Dialect(dialect)
	sqlStr, args, err := d.
		Insert(tblAdministrativeInformation).
		Rows(goqu.Record{
			colVersion:    version,
			colRevision:   revision,
			colTemplateId: templateID,
			colCreator:    creatorRefID,
		}).
		Returning(goqu.T(tblAdministrativeInformation).Col(colID)).
		ToSQL()
	if err != nil {
		return sql.NullInt64{}, err
	}

	var newID int64
	if err := tx.QueryRow(sqlStr, args...).Scan(&newID); err != nil {
		return sql.NullInt64{}, err
	}

	return sql.NullInt64{Int64: newID, Valid: true}, nil
}
