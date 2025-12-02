package descriptors

import (
	"context"
	"database/sql"
	"errors"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	"golang.org/x/sync/errgroup"
)

// InsertRegistryDescriptor creates a new RegistryDescriptor
// and all its related entities (display name, description,
// administration, and endpoints).
//
// The operation runs in its own database transaction. If any part of the write
// fails, the transaction is rolled back and no partial data is left behind.
//
// Parameters:
//   - ctx: request context used for cancellation/deadlines
//   - db:  open SQL database handle
//   - registryDescriptor: descriptor to persist
//
// Returns an error when SQL building/execution fails or when writing any of the
// dependent rows fails. Errors are wrapped into common errors where relevant.
func InsertRegistryDescriptor(ctx context.Context, db *sql.DB, registryDescriptor model.RegistryDescriptor) error {
	return WithTx(ctx, db, func(tx *sql.Tx) error {
		return InsertRegistryDescriptorTx(ctx, tx, registryDescriptor)
	})
}

// InsertRegistryDescriptorTx performs the same insert as
// InsertRegistryDescriptor but uses the provided transaction. This allows
// callers to compose multiple writes into a single atomic unit.
//
// The function inserts the base descriptor row first and then creates related
// entities (display name/description/admin info/endpoints). If any step fails,
// the error is returned and the caller is responsible for rolling back the transaction.
func InsertRegistryDescriptorTx(_ context.Context, tx *sql.Tx, aasd model.RegistryDescriptor) error {
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

	dnID, err := persistence_utils.CreateLangStringNameTypes(tx, aasd.DisplayName)
	if err != nil {
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}
	displayNameID = dnID
	var convertedDescription []model.LangStringText
	for _, desc := range aasd.Description {
		convertedDescription = append(convertedDescription, desc)
	}
	descID, err := persistence_utils.CreateLangStringTextTypes(tx, convertedDescription)
	if err != nil {
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}
	descriptionID = descID

	adminID, err := persistence_utils.CreateRegistryAdministrativeInformation(tx, aasd.Administration)
	if err != nil {
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}
	administrationID = adminID

	sqlStr, args, buildErr = d.
		Insert(tblRegistryDescriptor).
		Rows(goqu.Record{
			colDescriptorID:  descriptorID,
			colDescriptionID: descriptionID,
			colDisplayNameID: displayNameID,
			colAdminInfoID:   administrationID,
			colRegistryType:  aasd.RegistryType,
			colGlobalAssetID: aasd.GlobalAssetId,
			colIDShort:       aasd.IdShort,
			colAASID:         aasd.Id,
		}).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return err
	}

	if err = CreateEndpoints(tx, descriptorID, aasd.Endpoints); err != nil {
		return common.NewInternalServerError("Failed to create Endpoints - no changes applied - see console for details")
	}

	return nil
}

// GetRegistryDescriptorByID returns a fully materialized
// RegistryDescriptor by its Registry Id string.
//
// The function loads optional related entities (administration, display name,
// description, and endpoints) concurrently to minimize latency. If the
// Registry does not exist, a NotFound error is returned.
func GetRegistryDescriptorByID(
	ctx context.Context, db *sql.DB, registryIdentifier string,
) (model.RegistryDescriptor, error) {
	d := goqu.Dialect(dialect)

	reg := goqu.T(tblRegistryDescriptor).As("reg")

	sqlStr, args, buildErr := d.
		From(reg).
		Select(
			reg.Col(colDescriptorID),
			reg.Col(colRegistryType),
			reg.Col(colGlobalAssetID),
			reg.Col(colIDShort),
			reg.Col(colAASID),
			reg.Col(colAdminInfoID),
			reg.Col(colDisplayNameID),
			reg.Col(colDescriptionID),
		).
		Where(reg.Col(colAASID).Eq(registryIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return model.RegistryDescriptor{}, buildErr
	}

	var (
		descID                               int64
		registryType, globalAssetID, idShort sql.NullString
		idStr                                string
		adminInfoID                          sql.NullInt64
		displayNameID                        sql.NullInt64
		descriptionID                        sql.NullInt64
	)

	if err := db.QueryRowContext(ctx, sqlStr, args...).Scan(
		&descID,
		&registryType,
		&globalAssetID,
		&idShort,
		&idStr,
		&adminInfoID,
		&displayNameID,
		&descriptionID,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.RegistryDescriptor{}, common.NewErrNotFound("Registry Descriptor not found")
		}
		return model.RegistryDescriptor{}, err
	}

	g, ctx := errgroup.WithContext(ctx)

	var (
		adminInfo   *model.RegistryAdministrativeInformation
		displayName []model.LangStringNameType
		description []model.LangStringTextType
		endpoints   []model.Endpoint
	)

	g.Go(func() error {
		if adminInfoID.Valid {
			ai, err := ReadRegistryAdministrativeInformationByID(ctx, db, tblRegistryDescriptor, adminInfoID)
			if err != nil {
				return err
			}
			adminInfo = ai
		}
		return nil
	})
	GoAssign(g, func() ([]model.LangStringNameType, error) {
		return persistence_utils.GetLangStringNameTypes(db, displayNameID)
	}, &displayName)

	GoAssign(g, func() ([]model.LangStringTextType, error) {
		return persistence_utils.GetLangStringTextTypes(db, descriptionID)
	}, &description)

	GoAssign(g, func() ([]model.Endpoint, error) { return ReadEndpointsByDescriptorID(ctx, db, descID) }, &endpoints)

	if err := g.Wait(); err != nil {
		return model.RegistryDescriptor{}, err
	}

	return model.RegistryDescriptor{
		RegistryType:   registryType.String,
		GlobalAssetId:  globalAssetID.String,
		IdShort:        idShort.String,
		Id:             idStr,
		Administration: adminInfo,
		DisplayName:    displayName,
		Description:    description,
		Endpoints:      endpoints,
	}, nil
}

// DeleteRegistryDescriptorByID deletes the descriptor for the
// given Registry Id string. Deletion happens on the base descriptor row with ON
// DELETE CASCADE removing dependent rows.
//
// The delete runs in its own transaction.
func DeleteRegistryDescriptorByID(ctx context.Context, db *sql.DB, registryIdentifier string) error {
	return WithTx(ctx, db, func(tx *sql.Tx) error {
		return DeleteRegistryDescriptorByIDTx(ctx, tx, registryIdentifier)
	})
}

// DeleteRegistryDescriptorByIDTx deletes using the provided
// transaction. It resolves the internal descriptor id and removes the base
// descriptor row. Dependent rows are removed via ON DELETE CASCADE.
func DeleteRegistryDescriptorByIDTx(ctx context.Context, tx *sql.Tx, registryIdentifier string) error {
	d := goqu.Dialect("postqres")
	reg := goqu.T("registry_descriptor").As("reg")

	sqlStr, args, buildErr := d.
		From(reg).
		Select(reg.Col(colDescriptorID)).
		Where(reg.Col(colAASID).Eq(registryIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}

	var descID int64
	if scanErr := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return common.NewErrNotFound("Registry Descriptor not found")
		}
		return scanErr
	}

	delStr, delArgs, buildDelErr := d.
		Delete("descriptor").
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

// ReplaceRegistryDescriptor atomically replaces the descriptor with the same
// Registry Id: if a descriptor exists it is deleted (base descriptor row), then
// the provided descriptor is inserted. Related rows are recreated from the input.
// The returned boolean indicates whether a descriptor existed before the replace.
func ReplaceRegistryDescriptor(ctx context.Context, db *sql.DB, registryDescriptor model.RegistryDescriptor) (bool, error) {
	existed := false
	err := WithTx(ctx, db, func(tx *sql.Tx) error {
		d := goqu.Dialect(dialect)
		reg := goqu.T(tblRegistryDescriptor).As("reg")

		sqlStr, args, buildErr := d.
			From(reg).
			Select(reg.Col(colDescriptorID)).
			Where(reg.Col(colAASID).Eq(registryDescriptor.Id)).
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

		return InsertRegistryDescriptorTx(ctx, tx, registryDescriptor)
	})
	return existed, err
}

// ListRegistryDescriptors lists Registry descriptors with optional
// filtering by RegistryType. Results are ordered by Registry Id
// ascending and support cursor‑based pagination where the cursor is the Registry Id
// of the first element to include (i.e. Id >= cursor).
//
// It returns the page of fully assembled descriptors and, when more results are
// available, a next cursor value (the Id immediately after the page). When
// limit <= 0, a conservative large default is applied.
func ListRegistryDescriptors(
	ctx context.Context,
	db *sql.DB,
	limit int32,
	cursor string,
	registryType string,
) ([]model.RegistryDescriptor, string, error) {

	if limit <= 0 {
		limit = 1000000
	}
	peekLimit := int(limit) + 1

	d := goqu.Dialect(dialect)
	reg := goqu.T(tblRegistryDescriptor).As("reg")

	ds := d.
		From(reg).
		Select(
			reg.Col(colDescriptorID),
			reg.Col(colRegistryType),
			reg.Col(colGlobalAssetID),
			reg.Col(colIDShort),
			reg.Col(colAASID),
			reg.Col(colAdminInfoID),
			reg.Col(colDisplayNameID),
			reg.Col(colDescriptionID),
		)
	if cursor != "" {
		ds = ds.Where(reg.Col(colAASID).Gte(cursor))
	}

	if registryType != "" {
		ds = ds.Where(reg.Col(colRegistryType).Eq(registryType))
	}

	ds = ds.
		Order(reg.Col(colAASID).Asc()).
		Limit(uint(peekLimit))

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		return nil, "", common.NewInternalServerError("Failed to build Registry descriptor query. See server logs for details.")
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("Failed to query Registry descriptors. See server logs for details.")
	}
	defer func() {
		_ = rows.Close()
	}()

	descRows := make([]model.RegistryDescriptorRow, 0, peekLimit)
	for rows.Next() {
		var r model.RegistryDescriptorRow
		if err := rows.Scan(
			&r.DescID,
			&r.RegistryType,
			&r.GlobalAssetID,
			&r.IDShort,
			&r.IDStr,
			&r.AdminInfoID,
			&r.DisplayNameID,
			&r.DescriptionID,
		); err != nil {
			return nil, "", common.NewInternalServerError("Failed to scan Registry descriptor row. See server logs for details.")
		}
		descRows = append(descRows, r)
	}
	if rows.Err() != nil {
		return nil, "", common.NewInternalServerError("Failed to iterate Registry descriptors. See server logs for details.")
	}

	var nextCursor string
	if len(descRows) > int(limit) {
		nextCursor = descRows[limit].IDStr
		descRows = descRows[:limit]
	}

	if len(descRows) == 0 {
		return []model.RegistryDescriptor{}, nextCursor, nil
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

	admByID := map[int64]*model.RegistryAdministrativeInformation{}
	dnByID := map[int64][]model.LangStringNameType{}
	descByID := map[int64][]model.LangStringTextType{}
	endpointsByDesc := map[int64][]model.Endpoint{}

	g, gctx := errgroup.WithContext(ctx)

	if len(adminInfoIDs) > 0 {
		ids := append([]int64(nil), adminInfoIDs...)
		GoAssign(g, func() (map[int64]*model.RegistryAdministrativeInformation, error) {
			return ReadRegistryAdministrativeInformationByIDs(gctx, db, tblRegistryDescriptor, ids)
		}, &admByID)
	}
	if len(displayNameIDs) > 0 {
		ids := append([]int64(nil), displayNameIDs...)
		GoAssign(g, func() (map[int64][]model.LangStringNameType, error) {
			return GetLangStringNameTypesByIDs(db, ids)
		}, &dnByID)
	}

	if len(descriptionIDs) > 0 {
		ids := append([]int64(nil), descriptionIDs...)
		GoAssign(g, func() (map[int64][]model.LangStringTextType, error) {
			return GetLangStringTextTypesByIDs(db, ids)
		}, &descByID)
	}

	if len(descIDs) > 0 {
		ids := append([]int64(nil), descIDs...)
		GoAssign(g, func() (map[int64][]model.Endpoint, error) {
			return ReadEndpointsByDescriptorIDs(gctx, db, ids)
		}, &endpointsByDesc)
	}

	if err := g.Wait(); err != nil {
		return nil, "", err
	}

	out := make([]model.RegistryDescriptor, 0, len(descRows))
	for _, r := range descRows {
		var adminInfo *model.RegistryAdministrativeInformation
		if r.AdminInfoID.Valid {
			if v, ok := admByID[r.AdminInfoID.Int64]; ok {
				tmp := v
				adminInfo = tmp
			}
		}

		var displayName []model.LangStringNameType
		if r.DisplayNameID.Valid {
			displayName = dnByID[r.DisplayNameID.Int64]
		}

		var description []model.LangStringTextType
		if r.DescriptionID.Valid {
			description = descByID[r.DescriptionID.Int64]
		}

		out = append(out, model.RegistryDescriptor{
			RegistryType:   r.RegistryType.String,
			GlobalAssetId:  r.GlobalAssetID.String,
			IdShort:        r.IDShort.String,
			Id:             r.IDStr,
			Administration: adminInfo,
			DisplayName:    displayName,
			Description:    description,
			Endpoints:      endpointsByDesc[r.DescID],
		})
	}

	return out, nextCursor, nil
}
