package persistence_postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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

func NewPostgreSQLAASRegistryDatabase(dsn string, maxOpenConns, maxIdleConns int, connMaxLifetimeMinutes int, cacheEnabled bool) (*PostgreSQLAASRegistryDatabase, error) {
	db, err := sql.Open(dialect, dsn)
	db.SetMaxOpenConns(500)
	db.SetMaxIdleConns(500)
	db.SetConnMaxLifetime(time.Minute * 5)

	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	dir, osErr := os.Getwd()
	if osErr != nil {
		return nil, osErr
	}

	schemaPath := filepath.Join(dir, "resources", "sql", "aasregistryschema.sql")

	queryBytes, fileError := os.ReadFile(schemaPath)
	if fileError != nil {
		return nil, fileError
	}

	if _, dbError := db.Exec(string(queryBytes)); dbError != nil {
		return nil, dbError
	}

	return &PostgreSQLAASRegistryDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}
func (p *PostgreSQLAASRegistryDatabase) InsertAdministrationShellDescriptor(ctx context.Context, aasd model.AssetAdministrationShellDescriptor) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		fmt.Println(err)
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
	err = tx.QueryRow(sqlStr, args...).Scan(&descriptorId)
	if err != nil {
		return err
	}

	var displayNameId, descriptionId, administrationId sql.NullInt64

	displayNameId, err = persistence_utils.CreateLangStringNameTypes(tx, aasd.DisplayName)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}

	descriptionId, err = persistence_utils.CreateLangStringTextTypesN(tx, aasd.Description)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}

	administrationId, err = persistence_utils.CreateAdministrativeInformation(tx, &aasd.Administration)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}

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

	if err = tx.Commit(); err != nil {
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

	ak := model.ASSETKIND_NOT_APPLICABLE
	if assetKindStr.Valid && assetKindStr.String != "" {
		v, err := model.NewAssetKindFromValue(assetKindStr.String)
		if err != nil {
			return model.AssetAdministrationShellDescriptor{}, fmt.Errorf("invalid AssetKind %q", assetKindStr.String)
		}
		ak = v
	}
	g, ctx := errgroup.WithContext(ctx)

	var (
		adminInfo        model.AdministrativeInformation
		displayName      []model.LangStringNameType
		description      []model.LangStringTextType
		endpoints        []model.Endpoint
		specificAssetIds []model.SpecificAssetId
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
		AssetKind:           &ak,
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
func (p *PostgreSQLAASRegistryDatabase) DeleteAssetAdministrationShellDescriptorById(ctx context.Context, aasIdentifier string) error {
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
		err = scanErr
		return err
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
		err = execErr
		return err
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return commitErr
	}
	return nil
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

	admByID := map[int64]model.AdministrativeInformation{}
	dnByID := map[int64][]model.LangStringNameType{}
	descByID := map[int64][]model.LangStringTextType{}
	endpointsByDesc := map[int64][]model.Endpoint{}
	specificByDesc := map[int64][]model.SpecificAssetId{}
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

	out := make([]model.AssetAdministrationShellDescriptor, 0, len(all))
	for _, r := range all {
		ak := model.ASSETKIND_NOT_APPLICABLE
		if r.assetKindStr.Valid && r.assetKindStr.String != "" {
			v, convErr := model.NewAssetKindFromValue(r.assetKindStr.String)
			if convErr != nil {
				return nil, "", fmt.Errorf("invalid AssetKind %q for AAS %s", r.assetKindStr.String, r.idStr)
			}
			ak = v
		}

		var adminInfo model.AdministrativeInformation
		if r.adminInfoID.Valid {
			adminInfo = admByID[r.adminInfoID.Int64]
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
			AssetKind:           &ak,
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
