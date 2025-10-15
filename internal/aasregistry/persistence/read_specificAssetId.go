package persistence_postgresql

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func readSpecificAssetIdsByDescriptorID(
	ctx context.Context,
	db *sql.DB,
	descriptorID int64,
) ([]model.SpecificAssetId, error) {
	d := goqu.Dialect(dialect)

	sqlStr, args, err := d.
		From(goqu.T(tblSpecificAssetID).As("sai")).
		Select(
			goqu.I("sai."+colID),
			goqu.I("sai."+colName),
			goqu.I("sai."+colValue),
			goqu.I("sai."+colSemanticID),
			goqu.I("sai."+colExternalSubjectRef),
		).
		Where(goqu.I("sai." + colDescriptorID).Eq(descriptorID)).
		Order(goqu.I("sai." + colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rowData struct {
		id                      int64
		name, value             sql.NullString
		semanticRefID           sql.NullInt64
		externalSubjectRefRefID sql.NullInt64
	}

	var out []model.SpecificAssetId

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.id,
			&r.name,
			&r.value,
			&r.semanticRefID,
			&r.externalSubjectRefRefID,
		); err != nil {
			return nil, err
		}

		var semanticRef *model.Reference
		if r.semanticRefID.Valid {
			if ref, err := persistence_utils.GetReferenceByReferenceDBID(db, r.semanticRefID); err == nil {
				semanticRef = ref
			} else {
				return nil, err
			}
		}

		var externalSubjectRef *model.Reference
		if r.externalSubjectRefRefID.Valid {
			if ref, err := persistence_utils.GetReferenceByReferenceDBID(db, r.externalSubjectRefRefID); err == nil {
				externalSubjectRef = ref
			} else {
				return nil, err
			}
		}

		supplemental, err := readSpecificAssetIdSupplementalSemantic(ctx, db, r.id)
		if err != nil {
			return nil, err
		}

		out = append(out, model.SpecificAssetId{
			Name:                    r.name.String,
			Value:                   r.value.String,
			SemanticId:              semanticRef,
			ExternalSubjectId:       externalSubjectRef,
			SupplementalSemanticIds: supplemental,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func readSpecificAssetIdSupplementalSemantic(
	ctx context.Context,
	db *sql.DB,
	specificAssetID int64,
) ([]model.Reference, error) {
	d := goqu.Dialect(dialect)

	sqlStr, args, err := d.
		From(goqu.T(tblSpecificAssetIDSuppSemantic).As("ss")).
		Select(
			goqu.I("ss." + colReferenceID),
		).
		Where(goqu.I("ss." + colSpecificAssetIDID).Eq(specificAssetID)).
		Order(goqu.I("ss." + colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Reference
	for rows.Next() {
		var refID sql.NullInt64
		if err := rows.Scan(&refID); err != nil {
			return nil, err
		}
		if refID.Valid {
			ref, err := persistence_utils.GetReferenceByReferenceDBID(db, refID)
			if err != nil {
				return nil, err
			}
			if ref != nil {
				out = append(out, *ref)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
