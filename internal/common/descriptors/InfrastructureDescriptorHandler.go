package descriptors

import (
	"context"
	"database/sql"
	"errors"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	"golang.org/x/sync/errgroup"
)

// InsertInfrastructureDescriptor creates a new InfrastructureDescriptor
// and all its related entities (display name, description,
// administration, and endpoints).
//
// The operation runs in its own database transaction. If any part of the write
// fails, the transaction is rolled back and no partial data is left behind.
//
// Parameters:
//   - ctx: request context used for cancellation/deadlines
//   - db:  open SQL database handle
//   - infrastructureDescriptor: descriptor to persist
//
// Returns an error when SQL building/execution fails or when writing any of the
// dependent rows fails. Errors are wrapped into common errors where relevant.
func InsertInfrastructureDescriptor(ctx context.Context, db *sql.DB, infrastructureDescriptor model.InfrastructureDescriptor) error {
	return WithTx(ctx, db, func(tx *sql.Tx) error {
		return InsertInfrastructureDescriptorTx(ctx, tx, infrastructureDescriptor)
	})
}

// InsertInfrastructureDescriptorTx performs the same insert as
// InsertInfrastructureDescriptor but uses the provided transaction. This allows
// callers to compose multiple writes into a single atomic unit.
//
// The function inserts the base descriptor row first and then creates related
// entities (display name/description/admin info/endpoints). If any step fails,
// the error is returned and the caller is responsible for rolling back the transaction.
func InsertInfrastructureDescriptorTx(_ context.Context, tx *sql.Tx, regdesc model.InfrastructureDescriptor) error {
	d := goqu.Dialect(dialect)

	descTbl := goqu.T(tblDescriptor)

	sqlStr, args, buildErr := d.
		Insert(tblDescriptor).
		Returning(descTbl.Col(colID)).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	var descriptorID int64
	if err := tx.QueryRow(sqlStr, args...).Scan(&descriptorID); err != nil {
		return err
	}

	var displayNameID, descriptionID, administrationID sql.NullInt64

	dnID, err := persistence_utils.CreateLangStringNameTypes(tx, regdesc.DisplayName)
	if err != nil {
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}
	displayNameID = dnID
	descID, err := persistence_utils.CreateLangStringTextTypes(tx, regdesc.Description)
	if err != nil {
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}
	descriptionID = descID

	adminID, err := persistence_utils.CreateAdministrativeInformation(tx, regdesc.Administration)
	if err != nil {
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}
	administrationID = adminID

	sqlStr, args, buildErr = d.
		Insert(tblInfrastructureDescriptor).
		Rows(goqu.Record{
			colDescriptorID:  descriptorID,
			colDescriptionID: descriptionID,
			colDisplayNameID: displayNameID,
			colAdminInfoID:   administrationID,
			colGlobalAssetID: regdesc.GlobalAssetId,
			colIDShort:       regdesc.IdShort,
			colRegDescID:     regdesc.Id,
			colCompany:       regdesc.Company,
		}).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return err
	}

	if err = CreateEndpoints(tx, descriptorID, regdesc.Endpoints); err != nil {
		return common.NewInternalServerError("Failed to create Endpoints - no changes applied - see console for details")
	}

	return nil
}

// GetInfrastructureDescriptorByID returns a fully materialized
// InfrastructureDescriptor by its Registry Id string.
//
// The function loads optional related entities (administration, display name,
// description, and endpoints) concurrently to minimize latency. If the
// Registry does not exist, a NotFound error is returned.
func GetInfrastructureDescriptorByID(
	ctx context.Context, db *sql.DB, infrastructureIdentifier string,
) (model.InfrastructureDescriptor, error) {
	d := goqu.Dialect(dialect)

	reg := goqu.T(tblInfrastructureDescriptor).As("reg")

	sqlStr, args, buildErr := d.
		From(reg).
		Select(
			reg.Col(colDescriptorID),
			reg.Col(colGlobalAssetID),
			reg.Col(colIDShort),
			reg.Col(colCompany),
			reg.Col(colRegDescID),
			reg.Col(colAdminInfoID),
			reg.Col(colDisplayNameID),
			reg.Col(colDescriptionID),
		).
		Where(reg.Col(colRegDescID).Eq(infrastructureIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return model.InfrastructureDescriptor{}, buildErr
	}

	var (
		descID                          int64
		globalAssetID, idShort, company sql.NullString
		idStr                           string
		adminInfoID                     sql.NullInt64
		displayNameID                   sql.NullInt64
		descriptionID                   sql.NullInt64
	)

	if err := db.QueryRowContext(ctx, sqlStr, args...).Scan(
		&descID,
		&globalAssetID,
		&idShort,
		&company,
		&idStr,
		&adminInfoID,
		&displayNameID,
		&descriptionID,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.InfrastructureDescriptor{}, common.NewErrNotFound("Infrastructure Descriptor not found")
		}
		return model.InfrastructureDescriptor{}, err
	}

	g, ctx := errgroup.WithContext(ctx)

	var (
		adminInfo   types.IAdministrativeInformation
		displayName []types.ILangStringNameType
		description []types.ILangStringTextType
		endpoints   []model.Endpoint
	)

	g.Go(func() error {
		if adminInfoID.Valid {
			ai, err := ReadAdministrativeInformationByID(ctx, db, tblInfrastructureDescriptor, adminInfoID)
			if err != nil {
				return err
			}
			adminInfo = ai
		}
		return nil
	})
	GoAssign(g, func() ([]types.ILangStringNameType, error) {
		return persistence_utils.GetLangStringNameTypes(db, displayNameID)
	}, &displayName)

	GoAssign(g, func() ([]types.ILangStringTextType, error) {
		return persistence_utils.GetLangStringTextTypes(db, descriptionID)
	}, &description)

	GoAssign(g, func() ([]model.Endpoint, error) {
		return ReadEndpointsByDescriptorID(ctx, db, descID, "registry")
	}, &endpoints)

	if err := g.Wait(); err != nil {
		return model.InfrastructureDescriptor{}, err
	}

	return model.InfrastructureDescriptor{
		GlobalAssetId:  globalAssetID.String,
		IdShort:        idShort.String,
		Company:        company.String,
		Id:             idStr,
		Administration: adminInfo,
		DisplayName:    displayName,
		Description:    description,
		Endpoints:      endpoints,
	}, nil
}

// DeleteInfrastructureDescriptorByID deletes the descriptor for the
// given Infrastructure Descriptor Id string. Deletion happens on the base descriptor row with ON
// DELETE CASCADE removing dependent rows.
// The delete runs in its own transaction.
func DeleteInfrastructureDescriptorByID(ctx context.Context, db *sql.DB, infrastructureIdentifier string) error {
	return WithTx(ctx, db, func(tx *sql.Tx) error {
		return DeleteInfrastructureDescriptorByIDTx(ctx, tx, infrastructureIdentifier)
	})
}

// DeleteInfrastructureDescriptorByIDTx deletes using the provided
// transaction. It resolves the internal descriptor id and removes the base
// descriptor row. Dependent rows are removed via ON DELETE CASCADE.
func DeleteInfrastructureDescriptorByIDTx(ctx context.Context, tx *sql.Tx, infrastructureIdentifier string) error {
	d := goqu.Dialect("postgres")
	reg := goqu.T(tblInfrastructureDescriptor).As("reg")

	sqlStr, args, buildErr := d.
		From(reg).
		Select(reg.Col(colDescriptorID)).
		Where(reg.Col(colRegDescID).Eq(infrastructureIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}

	var descID int64
	if scanErr := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return common.NewErrNotFound("Infrastructure Descriptor not found")
		}
		return scanErr
	}

	delStr, delArgs, buildDelErr := d.
		Delete(tblDescriptor).
		Where(goqu.C(colID).Eq(descID)).
		ToSQL()
	if buildDelErr != nil {
		return buildDelErr
	}
	if _, execErr := tx.Exec(delStr, delArgs...); execErr != nil {
		return execErr
	}
	return nil
}

// ReplaceInfrastructureDescriptor atomically replaces the descriptor with the same
// Registry Id: if a descriptor exists it is deleted (base descriptor row), then
// the provided descriptor is inserted. Related rows are recreated from the input.
// The returned boolean indicates whether a descriptor existed before the replace.
func ReplaceInfrastructureDescriptor(ctx context.Context, db *sql.DB, infrastructureDescriptor model.InfrastructureDescriptor) (bool, error) {
	existed := false
	err := WithTx(ctx, db, func(tx *sql.Tx) error {
		d := goqu.Dialect(dialect)
		reg := goqu.T(tblInfrastructureDescriptor).As("reg")

		sqlStr, args, buildErr := d.
			From(reg).
			Select(reg.Col(colDescriptorID)).
			Where(reg.Col(colRegDescID).Eq(infrastructureDescriptor.Id)).
			Limit(1).
			ToSQL()
		if buildErr != nil {
			return buildErr
		}
		var descID int64
		scanErr := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&descID)
		if scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
			return scanErr
		}
		if scanErr == nil {
			existed = true
			delStr, delArgs, buildDelErr := d.
				Delete(tblDescriptor).
				Where(goqu.C(colID).Eq(descID)).
				ToSQL()
			if buildDelErr != nil {
				return buildDelErr
			}
			if _, execErr := tx.Exec(delStr, delArgs...); execErr != nil {
				return execErr
			}
		}

		return InsertInfrastructureDescriptorTx(ctx, tx, infrastructureDescriptor)
	})
	return existed, err
}

// ListInfrastructureDescriptors lists Infrastructure descriptors with optional
// filtering by company and endpoint interface. Results are ordered by Registry Id
// ascending and support cursor‑based pagination where the cursor is the Registry Id
// of the first element to include (i.e. Id >= cursor).
//
// It returns the page of fully assembled descriptors and, when more results are
// available, a next cursor value (the Id immediately after the page). When
// limit <= 0, a conservative large default is applied.
//
// nolint:revive // complexity is 31 which is +1 above the allowed threshold of 30
func ListInfrastructureDescriptors(
	ctx context.Context,
	db *sql.DB,
	limit int32,
	cursor string,
	company string,
	endpointInterface string,
) ([]model.InfrastructureDescriptor, string, error) {
	if limit <= 0 {
		limit = 100
	}
	peekLimit := int(limit) + 1

	d := goqu.Dialect(dialect)
	reg := goqu.T(tblInfrastructureDescriptor).As("reg")
	aasdescendp := goqu.T(tblAASDescriptorEndpoint).As("aasdescendp")

	ds := d.
		From(reg).
		Select(
			reg.Col(colDescriptorID),
			reg.Col(colGlobalAssetID),
			reg.Col(colIDShort),
			reg.Col(colCompany),
			reg.Col(colRegDescID),
			reg.Col(colAdminInfoID),
			reg.Col(colDisplayNameID),
			reg.Col(colDescriptionID),
		)

	if cursor != "" {
		ds = ds.Where(reg.Col(colRegDescID).Gte(cursor))
	}

	if company != "" {
		ds = ds.Where(reg.Col(colCompany).Eq(company))
	}

	if endpointInterface != "" {
		ds = ds.
			LeftJoin(
				aasdescendp,
				goqu.On(
					reg.Col(colDescriptorID).Eq(aasdescendp.Col(colDescriptorID)),
				),
			).
			Where(aasdescendp.Col(colInterface).Eq(endpointInterface))
	}

	if peekLimit < 0 {
		return nil, "", common.NewErrBadRequest("Limit is too high.")
	}

	ds = ds.
		Order(reg.Col(colRegDescID).Asc()).
		Limit(uint(peekLimit))

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		return nil, "", common.NewInternalServerError("Failed to build Infrastructure descriptor query. See server logs for details.")
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("Failed to query Infrastructure descriptors. See server logs for details.")
	}
	defer func() {
		_ = rows.Close()
	}()

	descRows := make([]model.InfrastructureDescriptorRow, 0, peekLimit)
	for rows.Next() {
		var r model.InfrastructureDescriptorRow
		if err := rows.Scan(
			&r.DescID,
			&r.GlobalAssetID,
			&r.IDShort,
			&r.Company,
			&r.IDStr,
			&r.AdminInfoID,
			&r.DisplayNameID,
			&r.DescriptionID,
		); err != nil {
			return nil, "", common.NewInternalServerError("Failed to scan RegInfrastructureistry descriptor row. See server logs for details.")
		}
		descRows = append(descRows, r)
	}
	if rows.Err() != nil {
		return nil, "", common.NewInternalServerError("Failed to iterate Infrastructure descriptors. See server logs for details.")
	}

	var nextCursor string
	if len(descRows) > int(limit) {
		nextCursor = descRows[limit].IDStr
		descRows = descRows[:limit]
	}

	if len(descRows) == 0 {
		return []model.InfrastructureDescriptor{}, nextCursor, nil
	}

	descIDs := make([]int64, 0, len(descRows))
	adminInfoIDs := make([]int64, 0, len(descRows))
	displayNameIDs := make([]int64, 0, len(descRows))
	descriptionIDs := make([]int64, 0, len(descRows))

	seenDesc := make(map[int64]struct{}, len(descRows))
	seenAI := map[int64]struct{}{}
	seenDN := map[int64]struct{}{}
	seenDE := map[int64]struct{}{}

	for _, r := range descRows {
		if _, ok := seenDesc[r.DescID]; !ok {
			seenDesc[r.DescID] = struct{}{}
			descIDs = append(descIDs, r.DescID)
		}

		if r.AdminInfoID.Valid {
			id := r.AdminInfoID.Int64
			if _, ok := seenAI[id]; !ok {
				seenAI[id] = struct{}{}
				adminInfoIDs = append(adminInfoIDs, id)
			}
		}
		if r.DisplayNameID.Valid {
			id := r.DisplayNameID.Int64
			if _, ok := seenDN[id]; !ok {
				seenDN[id] = struct{}{}
				displayNameIDs = append(displayNameIDs, id)
			}
		}

		if r.DescriptionID.Valid {
			id := r.DescriptionID.Int64
			if _, ok := seenDE[id]; !ok {
				seenDE[id] = struct{}{}
				descriptionIDs = append(descriptionIDs, id)
			}
		}
	}

	admByID := map[int64]types.IAdministrativeInformation{}
	dnByID := map[int64][]types.ILangStringNameType{}
	descByID := map[int64][]types.ILangStringTextType{}
	endpointsByDesc := map[int64][]model.Endpoint{}

	g, gctx := errgroup.WithContext(ctx)

	if len(adminInfoIDs) > 0 {
		ids := append([]int64(nil), adminInfoIDs...)
		GoAssign(g, func() (map[int64]types.IAdministrativeInformation, error) {
			return ReadAdministrativeInformationByIDs(gctx, db, tblInfrastructureDescriptor, ids)
		}, &admByID)
	}
	if len(displayNameIDs) > 0 {
		ids := append([]int64(nil), displayNameIDs...)
		GoAssign(g, func() (map[int64][]types.ILangStringNameType, error) {
			return GetLangStringNameTypesByIDs(db, ids)
		}, &dnByID)
	}

	if len(descriptionIDs) > 0 {
		ids := append([]int64(nil), descriptionIDs...)
		GoAssign(g, func() (map[int64][]types.ILangStringTextType, error) {
			return GetLangStringTextTypesByIDs(db, ids)
		}, &descByID)
	}

	if len(descIDs) > 0 {
		ids := append([]int64(nil), descIDs...)
		GoAssign(g, func() (map[int64][]model.Endpoint, error) {
			return ReadEndpointsByDescriptorIDs(gctx, db, ids, "registry")
		}, &endpointsByDesc)
	}

	if err := g.Wait(); err != nil {
		return nil, "", err
	}

	out := make([]model.InfrastructureDescriptor, 0, len(descRows))
	for _, r := range descRows {
		var adminInfo types.IAdministrativeInformation
		if r.AdminInfoID.Valid {
			if v, ok := admByID[r.AdminInfoID.Int64]; ok {
				tmp := v
				adminInfo = tmp
			}
		}

		var displayName []types.ILangStringNameType
		if r.DisplayNameID.Valid {
			displayName = dnByID[r.DisplayNameID.Int64]
		}

		var description []types.ILangStringTextType
		if r.DescriptionID.Valid {
			description = descByID[r.DescriptionID.Int64]
		}

		out = append(out, model.InfrastructureDescriptor{
			GlobalAssetId:  r.GlobalAssetID.String,
			IdShort:        r.IDShort.String,
			Company:        r.Company.String,
			Id:             r.IDStr,
			Administration: adminInfo,
			DisplayName:    displayName,
			Description:    description,
			Endpoints:      endpointsByDesc[r.DescID],
		})
	}

	return out, nextCursor, nil
}
