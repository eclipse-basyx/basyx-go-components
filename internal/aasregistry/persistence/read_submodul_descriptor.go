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

// Bulk: AAS descriptor IDs -> []SubmodelDescriptor
func readSubmodelDescriptorsByAASDescriptorIDs(
	ctx context.Context,
	db *sql.DB,
	aasDescriptorIDs []int64,
) (map[int64][]model.SubmodelDescriptor, error) {
	out := make(map[int64][]model.SubmodelDescriptor, len(aasDescriptorIDs))
	if len(aasDescriptorIDs) == 0 {
		return out, nil
	}
	uniqAASDesc := dedupeInt64(aasDescriptorIDs)

	d := goqu.Dialect(dialect)
	smd := goqu.T(tblSubmodelDescriptor).As("smd")

	// Pull all SMD rows for the given AAS descriptor IDs in one pass.
	sqlStr, args, err := d.
		From(smd).
		Select(
			smd.Col(colAASDescriptorID), // 0: parent AAS descriptor id (grouping key)
			smd.Col(colDescriptorID),    // 1: this SMD's descriptor id (for endpoints/ext)
			smd.Col(colIdShort),         // 2
			smd.Col(colAASID),           // 3
			smd.Col(colSemanticID),      // 4 (nullable reference id)
			smd.Col(colAdminInfoID),     // 5 (nullable)
			smd.Col(colDescriptionID),   // 6 (nullable)
			smd.Col(colDisplayNameID),   // 7 (nullable)
		).
		Where(smd.Col(colAASDescriptorID).In(uniqAASDesc)).
		Order(
			smd.Col(colAASDescriptorID).Asc(),
			smd.Col(colDescriptorID).Asc(),
		).
		ToSQL()
	if err != nil {
		return nil, err
	}

	type rowData struct {
		aasDescID     int64
		smdDescID     int64
		idShort       sql.NullString
		id            sql.NullString
		semanticRefID sql.NullInt64
		adminInfoID   sql.NullInt64
		descriptionID sql.NullInt64
		displayNameID sql.NullInt64
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect for batch hydration
	perAAS := make(map[int64][]rowData, len(uniqAASDesc))
	allSmdDescIDs := make([]int64, 0, 256)
	semRefIDs := make([]int64, 0, 128)
	adminInfoIDs := make([]int64, 0, 128)
	descIDs := make([]int64, 0, 128)        // description ids
	displayNameIDs := make([]int64, 0, 128) // display name ids

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.aasDescID,
			&r.smdDescID,
			&r.idShort,
			&r.id,
			&r.semanticRefID,
			&r.adminInfoID,
			&r.descriptionID,
			&r.displayNameID,
		); err != nil {
			return nil, err
		}
		perAAS[r.aasDescID] = append(perAAS[r.aasDescID], r)
		allSmdDescIDs = append(allSmdDescIDs, r.smdDescID)
		if r.semanticRefID.Valid {
			semRefIDs = append(semRefIDs, r.semanticRefID.Int64)
		}
		if r.adminInfoID.Valid {
			adminInfoIDs = append(adminInfoIDs, r.adminInfoID.Int64)
		}
		if r.descriptionID.Valid {
			descIDs = append(descIDs, r.descriptionID.Int64)
		}
		if r.displayNameID.Valid {
			displayNameIDs = append(displayNameIDs, r.displayNameID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Nothing found — still ensure keys exist (optional)
	if len(allSmdDescIDs) == 0 {
		for _, id := range uniqAASDesc {
			if _, ok := out[id]; !ok {
				out[id] = nil
			}
		}
		return out, nil
	}

	uniqSmdDescIDs := dedupeInt64(allSmdDescIDs)
	uniqSemRefIDs := dedupeInt64(semRefIDs)
	uniqAdminInfoIDs := dedupeInt64(adminInfoIDs)
	uniqDescIDs := dedupeInt64(descIDs)
	uniqDisplayNameIDs := dedupeInt64(displayNameIDs)

	// ---- Bulk hydrate all dependencies ----

	// References for SemanticId
	semRefByID := map[int64]*model.Reference{}
	if len(uniqSemRefIDs) > 0 {

		//todo: references
	}

	// Administrative info
	admByID := map[int64]model.AdministrativeInformation{}
	if len(uniqAdminInfoIDs) > 0 {
		admByID, err = readAdministrativeInformationByIDs(ctx, db, uniqAdminInfoIDs)
		if err != nil {
			return nil, err
		}
	}

	// Lang strings
	nameByID := map[int64][]model.LangStringNameType{}
	if len(uniqDisplayNameIDs) > 0 {
		// If your type is gen.LangStringNameType, adapt accordingly.
		nameByID, err = GetLangStringNameTypesByIDs(db, uniqDisplayNameIDs)
		if err != nil {
			return nil, err
		}
	}
	descByID := map[int64][]model.LangStringTextType{}
	if len(uniqDescIDs) > 0 {
		descByID, err = GetLangStringTextTypesByIDs(db, uniqDescIDs)
		if err != nil {
			return nil, err
		}
	}

	// Endpoints + Extensions (using the bulk helpers you created)
	endpointsByDesc, err := readEndpointsByDescriptorIDs(ctx, db, uniqSmdDescIDs)
	if err != nil {
		return nil, err
	}
	extensionsByDesc, err := readExtensionsByDescriptorIDs(ctx, db, uniqSmdDescIDs)
	if err != nil {
		return nil, err
	}

	// ---- Assemble results in stable order ----
	for aasID, rowsForAAS := range perAAS {
		for _, r := range rowsForAAS {
			var semanticRef *model.Reference
			if r.semanticRefID.Valid {
				semanticRef = semRefByID[r.semanticRefID.Int64]
			}
			var adminInfo model.AdministrativeInformation
			if r.adminInfoID.Valid {
				adminInfo = admByID[r.adminInfoID.Int64]
			}

			var displayName []model.LangStringNameType
			if r.displayNameID.Valid {
				displayName = nameByID[r.displayNameID.Int64]
			}
			var description []model.LangStringTextType
			if r.descriptionID.Valid {
				description = descByID[r.descriptionID.Int64]
			}

			out[aasID] = append(out[aasID], model.SubmodelDescriptor{
				IdShort:        r.idShort.String,
				Id:             r.id.String,
				SemanticId:     semanticRef,
				Administration: adminInfo,
				DisplayName:    displayName,
				Description:    description,
				Endpoints:      endpointsByDesc[r.smdDescID],
				Extensions:     extensionsByDesc[r.smdDescID],
			})
		}
	}

	// Ensure keys exist for all requested parents (optional)
	for _, id := range uniqAASDesc {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	return out, nil
}
