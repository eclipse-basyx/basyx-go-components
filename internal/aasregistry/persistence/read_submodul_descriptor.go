package persistence_postgresql

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

func readSubmodelDescriptorsByAASDescriptorID(
	ctx context.Context,
	db *sql.DB,
	aasDescriptorID int64,
) ([]model.SubmodelDescriptor, error) {
	d := goqu.Dialect(dialect)

	smd := goqu.T(tblSubmodelDescriptor).As("smd")

	sqlStr, args, err := d.
		From(smd).
		Select(
			smd.Col(colDescriptorID),
			smd.Col(colIdShort),
			smd.Col(colAASID),
			smd.Col(colSemanticID),
			smd.Col(colAdminInfoID),
			smd.Col(colDescriptionID),
			smd.Col(colDisplayNameID),
		).
		Where(smd.Col(colAASDescriptorID).Eq(aasDescriptorID)).
		Order(smd.Col(colDescriptorID).Asc()).
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
		descID        int64
		idShort       sql.NullString
		id            sql.NullString
		semanticRefID sql.NullInt64
		adminInfoID   sql.NullInt64
		descriptionID sql.NullInt64
		displayNameID sql.NullInt64
	}

	var out []model.SubmodelDescriptor

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.descID,
			&r.idShort,
			&r.id,
			&r.semanticRefID,
			&r.adminInfoID,
			&r.descriptionID,
			&r.displayNameID,
		); err != nil {
			return nil, err
		}

		var semanticRef *model.Reference
		if r.semanticRefID.Valid {
			ref, err := persistence_utils.GetReferenceByReferenceDBID(db, r.semanticRefID)
			if err != nil {
				return nil, err
			}
			semanticRef = ref
		}

		adminInfo, err := readAdministrativeInformationByID(ctx, db, r.adminInfoID)
		if err != nil {
			return nil, err
		}

		displayName, err := persistence_utils.GetLangStringNameTypes(db, r.displayNameID)
		if err != nil {
			return nil, err
		}
		description, err := persistence_utils.GetLangStringTextTypes(db, r.descriptionID)
		if err != nil {
			return nil, err
		}

		endpoints, err := readEndpointsByDescriptorID(ctx, db, r.descID)
		if err != nil {
			return nil, err
		}

		extensions, err := readExtensionsByDescriptorID(ctx, db, r.descID)
		if err != nil {
			return nil, err
		}

		out = append(out, model.SubmodelDescriptor{
			IdShort:        r.idShort.String,
			Id:             r.id.String,
			SemanticId:     semanticRef,
			Administration: adminInfo,
			DisplayName:    displayName,
			Description:    description,
			Endpoints:      endpoints,
			Extensions:     extensions,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
