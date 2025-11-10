package aasregistrydatabase

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"golang.org/x/sync/errgroup"
	"github.com/lib/pq"
)

func readSubmodelDescriptorsByAASDescriptorID(
	ctx context.Context,
	db *sql.DB,
	aasDescriptorID int64,
) ([]model.SubmodelDescriptor, error) {

	v, err := readSubmodelDescriptorsByAASDescriptorIDs(ctx, db, []int64{aasDescriptorID})
	return v[aasDescriptorID], err
}

func readSubmodelDescriptorsByAASDescriptorIDs(
	ctx context.Context,
	db *sql.DB,
	aasDescriptorIDs []int64,
) (map[int64][]model.SubmodelDescriptor, error) {

	out := make(map[int64][]model.SubmodelDescriptor, len(aasDescriptorIDs))
	if len(aasDescriptorIDs) == 0 {
		return out, nil
	}
	uniqAASDesc := aasDescriptorIDs

    d := goqu.Dialect(dialect)
    smd := goqu.T(tblSubmodelDescriptor).As("smd")

    arr := pq.Array(uniqAASDesc)
    sqlStr, args, err := d.
        From(smd).
        Select(
            smd.Col(colAASDescriptorID),
            smd.Col(colDescriptorID),
            smd.Col(colIDShort),
            smd.Col(colAASID),
            smd.Col(colSemanticID),
            smd.Col(colAdminInfoID),
            smd.Col(colDescriptionID),
            smd.Col(colDisplayNameID),
        ).
        Where(goqu.L("smd.aas_descriptor_id = ANY(?::bigint[])", arr)).
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
	defer func() {
		_ = rows.Close()
	}()

	perAAS := make(map[int64][]rowData, len(uniqAASDesc))
	allSmdDescIDs := make([]int64, 0, 10000)
	semRefIDs := make([]int64, 0, 10000)
	adminInfoIDs := make([]int64, 0, 10000)
	descIDs := make([]int64, 0, 10000)
	displayNameIDs := make([]int64, 0, 10000)

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

	if len(allSmdDescIDs) == 0 {
		for _, id := range uniqAASDesc {
			if _, ok := out[id]; !ok {
				out[id] = nil
			}
		}
		return out, nil
	}

	uniqSmdDescIDs := allSmdDescIDs
	uniqSemRefIDs := semRefIDs
	uniqAdminInfoIDs := adminInfoIDs
	uniqDescIDs := descIDs
	uniqDisplayNameIDs := displayNameIDs

	semRefByID := map[int64]*model.Reference{}
	admByID := map[int64]*model.AdministrativeInformation{}
	nameByID := map[int64][]model.LangStringNameType{}
	descByID := map[int64][]model.LangStringTextType{}
	suppBySmdDesc := map[int64][]model.Reference{}
	endpointsByDesc := map[int64][]model.Endpoint{}
	extensionsByDesc := map[int64][]model.Extension{}

	g, gctx := errgroup.WithContext(ctx)

	if len(uniqSemRefIDs) > 0 {
		ids := uniqSemRefIDs
		g.Go(func() error {
			m, err := GetReferencesByIDsBatch(db, ids)
			if err != nil {
				return err
			}
			semRefByID = m
			return nil
		})
	}

	if len(uniqAdminInfoIDs) > 0 {
		ids := uniqAdminInfoIDs
		g.Go(func() error {
			m, err := readAdministrativeInformationByIDs(gctx, db, "submodel_descriptor", ids)
			if err != nil {
				return err
			}
			admByID = m
			return nil
		})
	}

	if len(uniqDisplayNameIDs) > 0 {
		ids := uniqDisplayNameIDs
		g.Go(func() error {
			m, err := GetLangStringNameTypesByIDs(db, ids)
			if err != nil {
				return err
			}
			nameByID = m
			return nil
		})
	}

	if len(uniqDescIDs) > 0 {
		ids := uniqDescIDs
		g.Go(func() error {
			m, err := GetLangStringTextTypesByIDs(db, ids)
			if err != nil {
				return err
			}
			descByID = m
			return nil
		})
	}

	if len(uniqSmdDescIDs) > 0 {
		smdIDs := uniqSmdDescIDs

		g.Go(func() error {
			m, err := readEntityReferences1ToMany(
				gctx, db, smdIDs,
				"submodel_descriptor_supplemental_semantic_id",
				"descriptor_id",
				"reference_id",
			)
			if err != nil {
				return err
			}
			suppBySmdDesc = m
			return nil
		})

		// Endpoints
		g.Go(func() error {
			m, err := readEndpointsByDescriptorIDs(gctx, db, smdIDs)
			if err != nil {
				return err
			}
			endpointsByDesc = m
			return nil
		})

		// Extensions
		g.Go(func() error {
			m, err := readExtensionsByDescriptorIDs(gctx, db, smdIDs)
			if err != nil {
				return err
			}
			extensionsByDesc = m
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Assemble
	for aasID, rowsForAAS := range perAAS {
		for _, r := range rowsForAAS {
			var semanticRef *model.Reference
			if r.semanticRefID.Valid {
				semanticRef = semRefByID[r.semanticRefID.Int64]
			}
			var adminInfo *model.AdministrativeInformation
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
				IdShort:                r.idShort.String,
				Id:                     r.id.String,
				SemanticId:             semanticRef,
				Administration:         adminInfo,
				DisplayName:            displayName,
				Description:            description,
				Endpoints:              endpointsByDesc[r.smdDescID],
				Extensions:             extensionsByDesc[r.smdDescID],
				SupplementalSemanticId: suppBySmdDesc[r.smdDescID],
			})
		}
	}

	for _, id := range uniqAASDesc {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	return out, nil
}
