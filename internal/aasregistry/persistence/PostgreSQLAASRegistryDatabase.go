package persistence_postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"golang.org/x/sync/errgroup"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	_ "github.com/lib/pq"
)

type PostgreSQLAASRegistryDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

func NewPostgreSQLAASRegistryDatabase(dsn string, maxOpenConns, maxIdleConns int, connMaxLifetimeMinutes int, cacheEnabled bool, databaseSchema string) (*PostgreSQLAASRegistryDatabase, error) {
	// Determine which schema to load: prefer provided path, otherwise default bundled schema
	schemaPath := databaseSchema
	if schemaPath == "" {
		// Fallback to default schema in resources
		if dir, osErr := os.Getwd(); osErr == nil {
			schemaPath = filepath.Join(dir, "resources", "sql", "aasregistryschema.sql")
		}
	}

	db, err := common.InitializeDatabase(dsn, schemaPath)
	if err != nil {
		return nil, err
	}

	return &PostgreSQLAASRegistryDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}

// WithTx runs the given function within a database transaction.
// It commits on success and rolls back on error or panic.
func (p *PostgreSQLAASRegistryDatabase) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if rec := recover(); rec != nil {
			_ = tx.Rollback()
			panic(rec)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// InsertAdministrationShellDescriptor performs the insert in its own transaction by default.
func (p *PostgreSQLAASRegistryDatabase) InsertAdministrationShellDescriptor(ctx context.Context, aasd model.AssetAdministrationShellDescriptor) error {
	return p.WithTx(ctx, func(tx *sql.Tx) error {
		return p.InsertAdministrationShellDescriptorTx(ctx, tx, aasd)
	})
}

// InsertAdministrationShellDescriptorTx performs the insert using the provided transaction.
func (p *PostgreSQLAASRegistryDatabase) InsertAdministrationShellDescriptorTx(ctx context.Context, tx *sql.Tx, aasd model.AssetAdministrationShellDescriptor) error {
	d := goqu.Dialect(dialect)

	descTbl := goqu.T(tblDescriptor)

	sqlStr, args, buildErr := d.
		Insert(tblDescriptor).
		Returning(descTbl.Col(colID)).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	var descriptorId int64
	if err := tx.QueryRow(sqlStr, args...).Scan(&descriptorId); err != nil {
		return err
	}

	var displayNameId, descriptionId, administrationId sql.NullInt64

	dnID, err := persistence_utils.CreateLangStringNameTypes(tx, aasd.DisplayName)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}
	displayNameId = dnID

	descID, err := persistence_utils.CreateLangStringTextTypesN(tx, aasd.Description)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}
	descriptionId = descID

	adminID, err := persistence_utils.CreateAdministrativeInformation(tx, aasd.Administration)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}
	administrationId = adminID

	sqlStr, args, buildErr = d.
		Insert(tblAASDescriptor).
		Rows(goqu.Record{
			colDescriptorID:  descriptorId,
			colDescriptionID: descriptionId,
			colDisplayNameID: displayNameId,
			colAdminInfoID:   administrationId,
			colAssetKind:     aasd.AssetKind,
			colAssetType:     aasd.AssetType,
			colGlobalAssetID: aasd.GlobalAssetId,
			colIdShort:       aasd.IdShort,
			colAASID:         aasd.Id,
		}).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	if _, err = tx.Exec(sqlStr, args...); err != nil {
		return err
	}

	if err = createEndpoints(tx, descriptorId, aasd.Endpoints); err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Endpoints - no changes applied - see console for details")
	}

	if err = createSpecificAssetId(tx, descriptorId, aasd.SpecificAssetIds); err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Specific Asset Ids - no changes applied - see console for details")
	}

	if err = createExtensions(tx, descriptorId, aasd.Extensions); err != nil {
		return err
	}

	if err = createSubModelDescriptors(tx, descriptorId, aasd.SubmodelDescriptors); err != nil {
		return err
	}

	return nil
}

func (p *PostgreSQLAASRegistryDatabase) GetAssetAdministrationShellDescriptorById(
	ctx context.Context, aasIdentifier string,
) (model.AssetAdministrationShellDescriptor, error) {
	adda := time.Now()
	d := goqu.Dialect(dialect)

	aas := goqu.T(tblAASDescriptor).As("aas")
	desc := goqu.T(tblDescriptor).As("desc")

	sqlStr, args, buildErr := d.
		From(aas).
		InnerJoin(
			desc,
			goqu.On(aas.Col(colDescriptorID).Eq(desc.Col(colID))),
		).
		Select(
			aas.Col(colDescriptorID),
			aas.Col(colAssetKind),
			aas.Col(colAssetType),
			aas.Col(colGlobalAssetID),
			aas.Col(colIdShort),
			aas.Col(colAASID),
			aas.Col(colAdminInfoID),
			aas.Col(colDisplayNameID),
			aas.Col(colDescriptionID),
		).
		Where(aas.Col(colAASID).Eq(aasIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return model.AssetAdministrationShellDescriptor{}, buildErr
	}

	var (
		descID                            int64
		assetKindStr                      sql.NullString
		assetType, globalAssetID, idShort sql.NullString
		idStr                             string
		adminInfoID                       sql.NullInt64
		displayNameID                     sql.NullInt64
		descriptionID                     sql.NullInt64
	)

	if err := p.db.QueryRowContext(ctx, sqlStr, args...).Scan(
		&descID,
		&assetKindStr,
		&assetType,
		&globalAssetID,
		&idShort,
		&idStr,
		&adminInfoID,
		&displayNameID,
		&descriptionID,
	); err != nil {
		if err == sql.ErrNoRows {
			return model.AssetAdministrationShellDescriptor{}, common.NewErrNotFound("AAS Descriptor not found")
		}
		return model.AssetAdministrationShellDescriptor{}, err
	}

	var ak *model.AssetKind
	if assetKindStr.Valid && assetKindStr.String != "" {
		v, err := model.NewAssetKindFromValue(assetKindStr.String)
		if err != nil {
			return model.AssetAdministrationShellDescriptor{}, fmt.Errorf("invalid AssetKind %q", assetKindStr.String)
		}
		ak = &v
	}
	g, ctx := errgroup.WithContext(ctx)

	var (
		adminInfo        *model.AdministrativeInformation
		displayName      []model.LangStringNameType
		description      []model.LangStringTextType
		endpoints        []model.Endpoint
		specificAssetIds []model.SpecificAssetID
		extensions       []model.Extension
		smds             []model.SubmodelDescriptor
	)

	g.Go(func() error {
		if adminInfoID.Valid {
			ai, err := readAdministrativeInformationByID(ctx, p.db, "aas_descriptor", adminInfoID)
			if err != nil {
				return err
			}
			adminInfo = ai
		}
		return nil
	})
	start := time.Now()
	g.Go(func() error {
		dn, err := persistence_utils.GetLangStringNameTypes(p.db, displayNameID)
		if err != nil {
			return err
		}
		displayName = dn
		return nil
	})

	g.Go(func() error {
		desc, err := persistence_utils.GetLangStringTextTypes(p.db, descriptionID)
		if err != nil {
			return err
		}
		description = desc
		return nil
	})

	duration := time.Since(start)
	fmt.Printf("single langstring block took %v to complete\n", duration)
	g.Go(func() error {
		eps, err := readEndpointsByDescriptorID(ctx, p.db, descID)
		if err != nil {
			return err
		}
		endpoints = eps
		return nil
	})

	g.Go(func() error {
		ids, err := readSpecificAssetIdsByDescriptorID(ctx, p.db, descID)
		if err != nil {
			return err
		}
		specificAssetIds = ids
		return nil
	})

	g.Go(func() error {
		ext, err := readExtensionsByDescriptorID(ctx, p.db, descID)
		if err != nil {
			return err
		}
		extensions = ext
		return nil
	})

	g.Go(func() error {
		sm, err := readSubmodelDescriptorsByAASDescriptorID(ctx, p.db, descID)
		if err != nil {
			return err
		}
		smds = sm
		return nil
	})

	if err := g.Wait(); err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}
	ada := time.Since(adda)
	fmt.Printf("total block took %v to complete\n", ada)

	return model.AssetAdministrationShellDescriptor{
		AssetKind:           ak,
		AssetType:           assetType.String,
		GlobalAssetId:       globalAssetID.String,
		IdShort:             idShort.String,
		Id:                  idStr,
		Administration:      adminInfo,
		DisplayName:         displayName,
		Description:         description,
		Endpoints:           endpoints,
		SpecificAssetIds:    specificAssetIds,
		Extensions:          extensions,
		SubmodelDescriptors: smds,
	}, nil
}

// DeleteAssetAdministrationShellDescriptorById deletes the main descriptor row for a given AAS id.
// ON DELETE CASCADE in the schema will remove dependent rows.
// DeleteAssetAdministrationShellDescriptorById performs the delete in its own transaction by default.
func (p *PostgreSQLAASRegistryDatabase) DeleteAssetAdministrationShellDescriptorById(ctx context.Context, aasIdentifier string) error {
	return p.WithTx(ctx, func(tx *sql.Tx) error {
		return p.DeleteAssetAdministrationShellDescriptorByIdTx(ctx, tx, aasIdentifier)
	})
}

// DeleteAssetAdministrationShellDescriptorByIdTx deletes using the provided transaction.
func (p *PostgreSQLAASRegistryDatabase) DeleteAssetAdministrationShellDescriptorByIdTx(ctx context.Context, tx *sql.Tx, aasIdentifier string) error {
	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")

	// Lookup the root descriptor id for this AAS
	sqlStr, args, buildErr := d.
		From(aas).
		Select(aas.Col(colDescriptorID)).
		Where(aas.Col(colAASID).Eq(aasIdentifier)).
		Limit(1).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}

	var descID int64
	if scanErr := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return common.NewErrNotFound("AAS Descriptor not found")
		}
		return scanErr
	}

	// Delete the main descriptor; cascades handle related rows
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

// ReplaceAdministrationShellDescriptor deletes any existing descriptor with the same Id
// and inserts the provided descriptor in a single transaction. The returned boolean
// indicates whether a descriptor existed before the replace.
func (p *PostgreSQLAASRegistryDatabase) ReplaceAdministrationShellDescriptor(ctx context.Context, aasd model.AssetAdministrationShellDescriptor) (bool, error) {
	existed := false
	err := p.WithTx(ctx, func(tx *sql.Tx) error {
		d := goqu.Dialect(dialect)
		aas := goqu.T(tblAASDescriptor).As("aas")

		// Lookup existing descriptor id by AAS Id
		sqlStr, args, buildErr := d.
			From(aas).
			Select(aas.Col(colDescriptorID)).
			Where(aas.Col(colAASID).Eq(aasd.Id)).
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
			// delete existing row; cascades clear dependents
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

		// Insert replacement
		return p.InsertAdministrationShellDescriptorTx(ctx, tx, aasd)
	})
	return existed, err
}

func GetLangStringTextTypesByIDs(
	db *sql.DB,
	textTypeIDs []int64,
) (map[int64][]model.LangStringTextType, error) {
	start := time.Now()
	out := make(map[int64][]model.LangStringTextType, len(textTypeIDs))
	if len(textTypeIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect("postgres")

	ds := dialect.
		From("lang_string_text_type").
		Select("lang_string_text_type_reference_id", "text", "language").
		Where(goqu.C("lang_string_text_type_reference_id").In(textTypeIDs))

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var refID int64
		var text, language string
		if err := rows.Scan(&refID, &text, &language); err != nil {
			return nil, err
		}
		out[refID] = append(out[refID], model.LangStringTextType{
			Text:     text,
			Language: language,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	duration := time.Since(start)
	fmt.Printf("name types block took %v to complete\n", duration)
	return out, nil
}

func GetLangStringNameTypesByIDs(
	db *sql.DB,
	nameTypeIDs []int64,
) (map[int64][]model.LangStringNameType, error) {

	start := time.Now()
	out := make(map[int64][]model.LangStringNameType, len(nameTypeIDs))
	if len(nameTypeIDs) == 0 {
		return out, nil
	}

	dialect := goqu.Dialect("postgres")

	// Build query
	ds := dialect.
		From("lang_string_name_type").
		Select("lang_string_name_type_reference_id", "text", "language").
		Where(goqu.C("lang_string_name_type_reference_id").In(nameTypeIDs))

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var refID int64
		var text, language string
		if err := rows.Scan(&refID, &text, &language); err != nil {
			return nil, err
		}
		out[refID] = append(out[refID], model.LangStringNameType{
			Text:     text,
			Language: language,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	duration := time.Since(start)
	fmt.Printf("name types block took %v to complete\n", duration)
	return out, nil
}

func (p *PostgreSQLAASRegistryDatabase) ListAssetAdministrationShellDescriptors(
	ctx context.Context,
	limit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
) ([]model.AssetAdministrationShellDescriptor, string, error) {

	adda := time.Now()
	if limit <= 0 {
		limit = 10000000
	}
	peekLimit := int(limit) + 1

	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")
	desc := goqu.T(tblDescriptor).As("desc")

	ds := d.
		From(aas).
		InnerJoin(desc, goqu.On(aas.Col(colDescriptorID).Eq(desc.Col(colID)))).
		Select(
			aas.Col(colDescriptorID),
			aas.Col(colAssetKind),
			aas.Col(colAssetType),
			aas.Col(colGlobalAssetID),
			aas.Col(colIdShort),
			aas.Col(colAASID),
			aas.Col(colAdminInfoID),
			aas.Col(colDisplayNameID),
			aas.Col(colDescriptionID),
		)
	if cursor != "" {
		ds = ds.Where(aas.Col(colAASID).Gte(cursor))
	}

	if assetType != "" {
		ds = ds.Where(aas.Col(colAssetType).Eq(assetType))
	}
	if akStr := fmt.Sprint(assetKind); akStr != "" && akStr != fmt.Sprint(model.ASSETKIND_NOT_APPLICABLE) {
		ds = ds.Where(aas.Col(colAssetKind).Eq(akStr))
	}

	ds = ds.
		Order(aas.Col(colAASID).Asc()).
		Limit(uint(peekLimit))

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		fmt.Println("ListAssetAdministrationShellDescriptors: build error:", buildErr)
		return nil, "", common.NewInternalServerError("Failed to build AAS descriptor query. See server logs for details.")
	}

	rows, err := p.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		fmt.Println("ListAssetAdministrationShellDescriptors: query error:", err)
		return nil, "", common.NewInternalServerError("Failed to query AAS descriptors. See server logs for details.")
	}
	defer rows.Close()

	type rowData struct {
		descID        int64
		assetKindStr  sql.NullString
		assetType     sql.NullString
		globalAssetID sql.NullString
		idShort       sql.NullString
		idStr         string
		adminInfoID   sql.NullInt64
		displayNameID sql.NullInt64
		descriptionID sql.NullInt64
	}

	all := make([]rowData, 0, peekLimit)
	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.descID,
			&r.assetKindStr,
			&r.assetType,
			&r.globalAssetID,
			&r.idShort,
			&r.idStr,
			&r.adminInfoID,
			&r.displayNameID,
			&r.descriptionID,
		); err != nil {
			fmt.Println("ListAssetAdministrationShellDescriptors: scan error:", err)
			return nil, "", common.NewInternalServerError("Failed to scan AAS descriptor row. See server logs for details.")
		}
		all = append(all, r)
	}
	if rows.Err() != nil {
		fmt.Println("ListAssetAdministrationShellDescriptors: rows error:", rows.Err())
		return nil, "", common.NewInternalServerError("Failed to iterate AAS descriptors. See server logs for details.")
	}

	var nextCursor string
	if len(all) > int(limit) {
		nextCursor = all[limit].idStr
		all = all[:limit]
	}

	if len(all) == 0 {
		return []model.AssetAdministrationShellDescriptor{}, nextCursor, nil
	}

	descIDs := make([]int64, 0, len(all))
	adminInfoIDs := make([]int64, 0, len(all))
	displayNameIDs := make([]int64, 0, len(all))
	descriptionIDs := make([]int64, 0, len(all))

	seenDesc := make(map[int64]struct{}, len(all))
	seenAI := map[int64]struct{}{}
	seenDN := map[int64]struct{}{}
	seenDE := map[int64]struct{}{}

	for _, r := range all {

		if _, ok := seenDesc[r.descID]; !ok {
			seenDesc[r.descID] = struct{}{}
			descIDs = append(descIDs, r.descID)
		}

		if r.adminInfoID.Valid {
			id := r.adminInfoID.Int64
			if _, ok := seenAI[id]; !ok {
				seenAI[id] = struct{}{}
				adminInfoIDs = append(adminInfoIDs, id)
			}
		}

		if r.displayNameID.Valid {
			id := r.displayNameID.Int64
			if _, ok := seenDN[id]; !ok {
				seenDN[id] = struct{}{}
				displayNameIDs = append(displayNameIDs, id)
			}
		}

		if r.descriptionID.Valid {
			id := r.descriptionID.Int64
			if _, ok := seenDE[id]; !ok {
				seenDE[id] = struct{}{}
				descriptionIDs = append(descriptionIDs, id)
			}
		}
	}

	admByID := map[int64]*model.AdministrativeInformation{}
	dnByID := map[int64][]model.LangStringNameType{}
	descByID := map[int64][]model.LangStringTextType{}
	endpointsByDesc := map[int64][]model.Endpoint{}
	specificByDesc := map[int64][]model.SpecificAssetID{}
	extByDesc := map[int64][]model.Extension{}
	smdByDesc := map[int64][]model.SubmodelDescriptor{}

	g, gctx := errgroup.WithContext(ctx)

	if len(adminInfoIDs) > 0 {
		ids := append([]int64(nil), adminInfoIDs...)
		g.Go(func() error {
			m, err := readAdministrativeInformationByIDs(gctx, p.db, "aas_descriptor", ids)
			if err != nil {
				return err
			}
			admByID = m
			return nil
		})
	}

	if len(displayNameIDs) > 0 {
		ids := append([]int64(nil), displayNameIDs...)
		g.Go(func() error {
			m, err := GetLangStringNameTypesByIDs(p.db, ids)
			if err != nil {
				return err
			}
			dnByID = m
			return nil
		})
	}

	if len(descriptionIDs) > 0 {
		ids := append([]int64(nil), descriptionIDs...)
		g.Go(func() error {
			m, err := GetLangStringTextTypesByIDs(p.db, ids)
			if err != nil {
				return err
			}
			descByID = m
			return nil
		})
	}

	if len(descIDs) > 0 {
		ids := append([]int64(nil), descIDs...)
		g.Go(func() error {
			m, err := readEndpointsByDescriptorIDs(gctx, p.db, ids)
			if err != nil {
				return err
			}
			endpointsByDesc = m
			return nil
		})
		g.Go(func() error {
			m, err := readSpecificAssetIdsByDescriptorIDs(gctx, p.db, ids)
			if err != nil {
				return err
			}
			specificByDesc = m
			return nil
		})
		g.Go(func() error {
			m, err := readExtensionsByDescriptorIDs(gctx, p.db, ids)
			if err != nil {
				return err
			}
			extByDesc = m
			return nil
		})
		g.Go(func() error {
			m, err := readSubmodelDescriptorsByAASDescriptorIDs(gctx, p.db, ids)
			if err != nil {
				return err
			}
			smdByDesc = m
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, "", err
	}
	lel := time.Since(adda)
	fmt.Printf("total block took %v to complete all requests\n", lel)

	out := make([]model.AssetAdministrationShellDescriptor, 0, len(all))
	for _, r := range all {
		var ak *model.AssetKind
		if r.assetKindStr.Valid && r.assetKindStr.String != "" {
			v, convErr := model.NewAssetKindFromValue(r.assetKindStr.String)
			if convErr != nil {
				return nil, "", fmt.Errorf("invalid AssetKind %q for AAS %s", r.assetKindStr.String, r.idStr)
			}
			ak = &v
		}

		var adminInfo *model.AdministrativeInformation
		if r.adminInfoID.Valid {
			if v, ok := admByID[r.adminInfoID.Int64]; ok {
				tmp := v
				adminInfo = tmp
			}
		}

		var displayName []model.LangStringNameType
		if r.displayNameID.Valid {
			displayName = dnByID[r.displayNameID.Int64]
		}

		var description []model.LangStringTextType
		if r.descriptionID.Valid {
			description = descByID[r.descriptionID.Int64]
		}

		out = append(out, model.AssetAdministrationShellDescriptor{
			AssetKind:           ak,
			AssetType:           r.assetType.String,
			GlobalAssetId:       r.globalAssetID.String,
			IdShort:             r.idShort.String,
			Id:                  r.idStr,
			Administration:      adminInfo,
			DisplayName:         displayName,
			Description:         description,
			Endpoints:           endpointsByDesc[r.descID],
			SpecificAssetIds:    specificByDesc[r.descID],
			Extensions:          extByDesc[r.descID],
			SubmodelDescriptors: smdByDesc[r.descID],
		})
	}

	ada := time.Since(adda)
	fmt.Printf("total block took %v to complete\n", ada)
	return out, nextCursor, nil
}

// ListSubmodelDescriptorsForAAS returns the list of SubmodelDescriptors that belong to
// a single AAS (identified by its AAS Id string). Results are ordered by Submodel Id ascending,
// support cursor-based pagination (cursor is the Submodel Id), and return a nextCursor when available.
//
// Cursor semantics:
//   - If cursor != "", only submodels with Id >= cursor are returned (ascending order).
//   - nextCursor, when non-empty, is the Id of the first element after the returned page.
//
// NOTE: This uses readSubmodelDescriptorsByAASDescriptorIDs to materialize descriptors for the
//
//	target AAS descriptor id, then applies ordering + pagination in-memory. This avoids
//	duplicating the submodel-join logic here and keeps the function compact. If the number
//	of submodels per AAS can become very large and you need DB-level pagination, you can
//	push the ORDER/LIMIT/GTE predicate down into SQL against your submodel tables.
func (p *PostgreSQLAASRegistryDatabase) ListSubmodelDescriptorsForAAS(
	ctx context.Context,
	aasID string,
	limit int32,
	cursor string,
) ([]model.SubmodelDescriptor, string, error) {

	start := time.Now()
	if limit <= 0 {
		limit = 10000000
	}
	peekLimit := int(limit) + 1

	// 1) Resolve the single AAS descriptor id for the provided AAS Id string.
	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")

	ds := d.
		From(aas).
		Select(aas.Col(colDescriptorID)).
		Where(aas.Col(colAASID).Eq(aasID)).
		Limit(1)

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		fmt.Println("ListSubmodelDescriptorsForAAS: build error:", buildErr)
		return nil, "", common.NewInternalServerError("Failed to build AAS lookup query. See server logs for details.")
	}

	var descID int64
	if err := p.db.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No such AAS => empty page
			return []model.SubmodelDescriptor{}, "", nil
		}
		fmt.Println("ListSubmodelDescriptorsForAAS: query error:", err)
		return nil, "", common.NewInternalServerError("Failed to query AAS descriptor id. See server logs for details.")
	}

	// 2) Fetch all submodel descriptors for that descriptor id using your existing helper.
	//    (Keeps logic consistent with your other readers.)
	m, err := readSubmodelDescriptorsByAASDescriptorIDs(ctx, p.db, []int64{descID})
	if err != nil {
		fmt.Println("ListSubmodelDescriptorsForAAS: readSubmodelDescriptorsByAASDescriptorIDs error:", err)
		return nil, "", err
	}
	list := append([]model.SubmodelDescriptor{}, m[descID]...)

	// 3) Sort by Id ascending (stable).
	sort.Slice(list, func(i, j int) bool {
		return list[i].Id < list[j].Id
	})

	// 4) Apply cursor (Id >= cursor).
	if cursor != "" {
		// Binary search lower bound for cursor.
		lo, hi := 0, len(list)
		for lo < hi {
			mid := (lo + hi) / 2
			if list[mid].Id < cursor {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		list = list[lo:]
	}

	// 5) Peek & trim to page size, compute nextCursor.
	var nextCursor string
	if len(list) > peekLimit {
		nextCursor = list[peekLimit-1].Id
		list = list[:peekLimit-1]
	} else if len(list) == peekLimit {
		nextCursor = list[limit].Id
		list = list[:limit]
	} else if len(list) > int(limit) {
		nextCursor = list[limit].Id
		list = list[:limit]
	}

	fmt.Printf("ListSubmodelDescriptorsForAAS: total block took %v to complete\n", time.Since(start))
	return list, nextCursor, nil
}

// InsertSubmodelDescriptorForAAS inserts a single SubmodelDescriptor under the AAS
// identified by aasID (the AAS's Id string). Returns NotFound if the AAS does not exist.
func (p *PostgreSQLAASRegistryDatabase) InsertSubmodelDescriptorForAAS(
	ctx context.Context,
	aasID string,
	submodel model.SubmodelDescriptor,
) error {

	// Lookup AAS descriptor id by AAS Id string
	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")

	ds := d.
		From(aas).
		Select(aas.Col(colDescriptorID)).
		Where(aas.Col(colAASID).Eq(aasID)).
		Limit(1)

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		fmt.Println("InsertSubmodelDescriptorForAAS: build error:", buildErr)
		return common.NewInternalServerError("Failed to build AAS lookup query. See server logs for details.")
	}

	var aasDescID int64
	if err := p.db.QueryRowContext(ctx, sqlStr, args...).Scan(&aasDescID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return common.NewErrNotFound("AAS Descriptor not found")
		}
		fmt.Println("InsertSubmodelDescriptorForAAS: query error:", err)
		return common.NewInternalServerError("Failed to query AAS descriptor id. See server logs for details.")
	}

	// Begin tx and insert using existing helper
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		} else if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = createSubModelDescriptors(tx, aasDescID, []model.SubmodelDescriptor{submodel}); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// ReplaceSubmodelDescriptorForAAS deletes any existing submodel descriptor with the same Id
// under the given AAS and inserts the provided descriptor in a single transaction.
// The returned boolean indicates whether a descriptor existed before the replace.
func (p *PostgreSQLAASRegistryDatabase) ReplaceSubmodelDescriptorForAAS(
	ctx context.Context,
	aasID string,
	submodel model.SubmodelDescriptor,
) (bool, error) {
	existed := false
	err := p.WithTx(ctx, func(tx *sql.Tx) error {
		d := goqu.Dialect(dialect)
		aas := goqu.T(tblAASDescriptor).As("aas")
		smd := goqu.T(tblSubmodelDescriptor).As("smd")

		// Resolve AAS descriptor id from AAS Id
		sqlStr, args, buildErr := d.
			From(aas).
			Select(aas.Col(colDescriptorID)).
			Where(aas.Col(colAASID).Eq(aasID)).
			Limit(1).
			ToSQL()
		if buildErr != nil {
			return buildErr
		}
		var aasDescID int64
		if err := tx.QueryRowContext(ctx, sqlStr, args...).Scan(&aasDescID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return common.NewErrNotFound("AAS Descriptor not found")
			}
			return common.NewInternalServerError("Failed to query AAS descriptor id. See server logs for details.")
		}

		// Lookup existing submodel descriptor globally by submodel Id (may belong to any AAS)
		sqlStr2, args2, buildErr2 := d.
			From(smd).
			Select(smd.Col(colDescriptorID)).
			Where(smd.Col(colAASID).Eq(submodel.Id)).
			Limit(1).
			ToSQL()
		if buildErr2 != nil {
			return buildErr2
		}
		var subDescID int64
		scanErr := tx.QueryRowContext(ctx, sqlStr2, args2...).Scan(&subDescID)
		if scanErr == nil {
			existed = true
			delSQL, delArgs, delErr := d.Delete(tblDescriptor).Where(goqu.C(colID).Eq(subDescID)).ToSQL()
			if delErr != nil {
				return delErr
			}
			if _, execErr := tx.Exec(delSQL, delArgs...); execErr != nil {
				return execErr
			}
		} else if !errors.Is(scanErr, sql.ErrNoRows) {
			return scanErr
		}

		// Insert replacement submodel descriptor
		if err := createSubModelDescriptors(tx, aasDescID, []model.SubmodelDescriptor{submodel}); err != nil {
			return err
		}
		return nil
	})
	return existed, err
}

// ExistsAASByID performs a lightweight existence check for an AAS by its Id string.
func (p *PostgreSQLAASRegistryDatabase) ExistsAASByID(ctx context.Context, aasID string) (bool, error) {
	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")

	ds := d.From(aas).Select(goqu.L("1")).Where(aas.Col(colAASID).Eq(aasID)).Limit(1)
	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return false, err
	}

	var one int
	if scanErr := p.db.QueryRowContext(ctx, sqlStr, args...).Scan(&one); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return false, nil
		}
		return false, scanErr
	}
	return true, nil
}

// ExistsSubmodelForAAS performs a lightweight existence check for a submodel under a given AAS.
func (p *PostgreSQLAASRegistryDatabase) ExistsSubmodelForAAS(ctx context.Context, aasID, submodelID string) (bool, error) {
	d := goqu.Dialect(dialect)
	smd := goqu.T(tblSubmodelDescriptor).As("smd")
	aas := goqu.T(tblAASDescriptor).As("aas")

	ds := d.
		From(smd).
		InnerJoin(aas, goqu.On(smd.Col(colAASDescriptorID).Eq(aas.Col(colDescriptorID)))).
		Select(goqu.L("1")).
		Where(
			goqu.And(
				aas.Col(colAASID).Eq(aasID),
				smd.Col(colAASID).Eq(submodelID),
			),
		).
		Limit(1)

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return false, err
	}
	var one int
	if scanErr := p.db.QueryRowContext(ctx, sqlStr, args...).Scan(&one); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return false, nil
		}
		return false, scanErr
	}
	return true, nil
}

// GetSubmodelDescriptorForAASByID returns a single SubmodelDescriptor for a given
// AAS (by AAS Id string) and Submodel Id. Returns NotFound if either the AAS or the
// Submodel under that AAS does not exist.
func (p *PostgreSQLAASRegistryDatabase) GetSubmodelDescriptorForAASByID(
	ctx context.Context,
	aasID string,
	submodelID string,
) (model.SubmodelDescriptor, error) {
	// Resolve AAS descriptor id
	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")

	ds := d.
		From(aas).
		Select(aas.Col(colDescriptorID)).
		Where(aas.Col(colAASID).Eq(aasID)).
		Limit(1)

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		return model.SubmodelDescriptor{}, common.NewInternalServerError("Failed to build AAS lookup query. See server logs for details.")
	}
	var descID int64
	if err := p.db.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.SubmodelDescriptor{}, common.NewErrNotFound("AAS Descriptor not found")
		}
		return model.SubmodelDescriptor{}, common.NewInternalServerError("Failed to query AAS descriptor id. See server logs for details.")
	}

	// Fetch all submodels for that AAS and find the one with matching Id
	m, err := readSubmodelDescriptorsByAASDescriptorIDs(ctx, p.db, []int64{descID})
	if err != nil {
		return model.SubmodelDescriptor{}, err
	}
	for _, smd := range m[descID] {
		if smd.Id == submodelID {
			return smd, nil
		}
	}
	return model.SubmodelDescriptor{}, common.NewErrNotFound("Submodel Descriptor not found")
}

// DeleteSubmodelDescriptorForAASByID deletes the submodel descriptor under the given AAS.
// It deletes from the base descriptor table (cascade will clean up related rows).
func (p *PostgreSQLAASRegistryDatabase) DeleteSubmodelDescriptorForAASByID(
	ctx context.Context,
	aasID string,
	submodelID string,
) error {
	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")
	smd := goqu.T(tblSubmodelDescriptor).As("smd")

	// Join to find the underlying descriptor id to delete
	ds := d.
		From(smd).
		InnerJoin(aas, goqu.On(smd.Col(colAASDescriptorID).Eq(aas.Col(colDescriptorID)))).
		Select(smd.Col(colDescriptorID)).
		Where(
			goqu.And(
				aas.Col(colAASID).Eq(aasID),
				smd.Col(colAASID).Eq(submodelID),
			),
		).
		Limit(1)

	sqlStr, args, buildErr := ds.ToSQL()
	if buildErr != nil {
		return common.NewInternalServerError("Failed to build submodel lookup query. See server logs for details.")
	}
	var descID int64
	if err := p.db.QueryRowContext(ctx, sqlStr, args...).Scan(&descID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return common.NewErrNotFound("Submodel Descriptor not found")
		}
		return common.NewInternalServerError("Failed to query submodel descriptor id. See server logs for details.")
	}

	// Delete from descriptor (cascades remove all sub-rows)
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		} else if err != nil {
			_ = tx.Rollback()
		}
	}()

	delSQL, delArgs, delErr := d.Delete(tblDescriptor).Where(goqu.C(colID).Eq(descID)).ToSQL()
	if delErr != nil {
		return delErr
	}
	if _, err = tx.Exec(delSQL, delArgs...); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}
