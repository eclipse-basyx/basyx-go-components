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

	sqlStr, args, buildErr := d.
		Insert(tblDescriptor).
		Returning(tDescriptor.Col(colID)).
		ToSQL()
	if buildErr != nil {
		return buildErr
	}
	var descriptorId int64
	err = tx.QueryRow(sqlStr, args...).Scan(&descriptorId)
	if err != nil {
		return err
	}

	desc := aasd.Description
	fmt.Println(desc)

	var displayNameId, descriptionId, administrationId sql.NullInt64

	displayNameId, err = persistence_utils.CreateLangStringNameTypes(tx, aasd.DisplayName)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}

	descriptionId, err = persistence_utils.CreateLangStringTextTypes(tx, aasd.Description)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}

	administrationId, err = CreateAdministrativeInformation(tx, &aasd.Administration)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}

	fmt.Println(displayNameId)
	fmt.Println(descriptionId)
	fmt.Println(administrationId)

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

	if err = CreateSubModelDescriptors(tx, descriptorId, aasd.SubmodelDescriptors); err != nil {
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
	d := goqu.Dialect(dialect)

	sqlStr, args, buildErr := d.
		From(goqu.T(tblAASDescriptor).As("a")).
		InnerJoin(
			goqu.T(tblDescriptor).As("d"),
			goqu.On(goqu.I("a."+colDescriptorID).Eq(goqu.I("d."+colID))),
		).
		Select(
			goqu.I("a."+colDescriptorID),
			goqu.I("a."+colAssetKind),
			goqu.I("a."+colAssetType),
			goqu.I("a."+colGlobalAssetID),
			goqu.I("a."+colIdShort),
			goqu.I("a."+colAASID),
			goqu.I("a."+colAdminInfoID),
			goqu.I("a."+colDisplayNameID),
			goqu.I("a."+colDescriptionID),
		).
		Where(goqu.I("a." + colAASID).Eq(aasIdentifier)).
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

	adminInfo, err := readAdministrativeInformationByID(ctx, p.db, adminInfoID)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}

	displayName, err := persistence_utils.GetLangStringNameTypes(p.db, displayNameID)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}
	description, err := persistence_utils.GetLangStringTextTypes(p.db, descriptionID)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}

	endpoints, err := readEndpointsByDescriptorID(ctx, p.db, descID)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}

	specificAssetIds, err := readSpecificAssetIdsByDescriptorID(ctx, p.db, descID)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}

	extensions, err := readExtensionsByDescriptorID(ctx, p.db, descID)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}

	smds, err := readSubmodelDescriptorsByAASDescriptorID(ctx, p.db, descID)
	if err != nil {
		return model.AssetAdministrationShellDescriptor{}, err
	}

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
