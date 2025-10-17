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

	// Define datasets once (no alias needed here)
	descTbl := goqu.T(tblDescriptor)

	// Insert into descriptor and return ID
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
	d := goqu.Dialect(dialect)

	// Define datasets with aliases once
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

// Batch: lang_string_text_type
func GetLangStringTextTypesByIDs(
	db *sql.DB,
	textTypeIDs []int64,
) (map[int64][]model.LangStringTextType, error) {
	out := make(map[int64][]model.LangStringTextType, len(textTypeIDs))
	if len(textTypeIDs) == 0 {
		return out, nil
	}

	// dedupe
	seen := make(map[int64]struct{}, len(textTypeIDs))
	uniq := make([]int64, 0, len(textTypeIDs))
	for _, id := range textTypeIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}

	// build IN ($1,$2,...)
	inClause, args := makeInClause(uniq)

	q := `
SELECT lang_string_text_type_reference_id, text, language
FROM lang_string_text_type
WHERE lang_string_text_type_reference_id IN (` + inClause + `)`

	rows, err := db.Query(q, args...)
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

	// ensure keys exist for requested IDs (optional, but handy)
	for _, id := range uniq {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}

	return out, nil
}

// Batch: lang_string_name_type
func GetLangStringNameTypesByIDs(
	db *sql.DB,
	nameTypeIDs []int64,
) (map[int64][]model.LangStringNameType, error) {
	out := make(map[int64][]model.LangStringNameType, len(nameTypeIDs))
	if len(nameTypeIDs) == 0 {
		return out, nil
	}

	// dedupe
	seen := make(map[int64]struct{}, len(nameTypeIDs))
	uniq := make([]int64, 0, len(nameTypeIDs))
	for _, id := range nameTypeIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}

	// build IN ($1,$2,...)
	inClause, args := makeInClause(uniq)

	q := `
SELECT lang_string_name_type_reference_id, text, language
FROM lang_string_name_type
WHERE lang_string_name_type_reference_id IN (` + inClause + `)`

	rows, err := db.Query(q, args...)
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

	// ensure keys exist for requested IDs (optional)
	for _, id := range uniq {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}

	return out, nil
}

// makeInClause builds "[$1,$2,...]" and args for sql.DB.
// Example: ids=[10,20] -> " $1,$2 ", args=[int64(10), int64(20)]
func makeInClause(ids []int64) (string, []any) {
	args := make([]any, len(ids))
	ph := make([]byte, 0, len(ids)*4) // rough cap
	for i, id := range ids {
		if i > 0 {
			ph = append(ph, ',', ' ')
		}
		// $1, $2, ...
		ph = append(ph, '$')
		ph = append(ph, []byte(intToString(i+1))...)
		args[i] = id
	}
	return string(ph), args
}

// tiny itoa without strconv to keep it self-contained (use strconv.Itoa if preferred)
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + (n % 10))
		n /= 10
	}
	return string(buf[i:])
}

func strOrEmpty(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

func (p *PostgreSQLAASRegistryDatabase) ListAssetAdministrationShellDescriptors(
	ctx context.Context,
	limit int32,
	cursor string,
	assetKind model.AssetKind,
	assetType string,
) ([]model.AssetAdministrationShellDescriptor, string, error) {

	if limit <= 0 {
		limit = 100
	}
	peekLimit := int(limit) + 1

	d := goqu.Dialect(dialect)
	aas := goqu.T(tblAASDescriptor).As("aas")
	desc := goqu.T(tblDescriptor).As("desc")

	// Build base dataset: include all columns we need to hydrate in one pass
	ds := d.
		From(aas).
		InnerJoin(desc, goqu.On(aas.Col(colDescriptorID).Eq(desc.Col(colID)))).
		Select(
			aas.Col(colDescriptorID),  // 0
			aas.Col(colAssetKind),     // 1
			aas.Col(colAssetType),     // 2
			aas.Col(colGlobalAssetID), // 3
			aas.Col(colIdShort),       // 4
			aas.Col(colAASID),         // 5
			aas.Col(colAdminInfoID),   // 6
			aas.Col(colDisplayNameID), // 7
			aas.Col(colDescriptionID), // 8
		)

	// Cursor semantics: >= to behave like previous version
	if cursor != "" {
		ds = ds.Where(aas.Col(colAASID).Gte(cursor))
	}

	// Optional filters
	if assetType != "" {
		ds = ds.Where(aas.Col(colAssetType).Eq(assetType))
	}
	if akStr := fmt.Sprint(assetKind); akStr != "" && akStr != fmt.Sprint(model.ASSETKIND_NOT_APPLICABLE) {
		ds = ds.Where(aas.Col(colAssetKind).Eq(akStr))
	}

	// Order + limit (peek one extra)
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

	// Handle next cursor by peeking one extra
	var nextCursor string
	if len(all) > int(limit) {
		nextCursor = all[limit].idStr
		all = all[:limit]
	}

	if len(all) == 0 {
		return []model.AssetAdministrationShellDescriptor{}, nextCursor, nil
	}

	// Collect unique IDs for batch hydration
	descIDs := make([]int64, 0, len(all))
	adminInfoIDs := make([]int64, 0, len(all))
	displayNameIDs := make([]int64, 0, len(all))
	descriptionIDs := make([]int64, 0, len(all))

	seenDesc := make(map[int64]struct{}, len(all))
	seenAI := map[int64]struct{}{}
	seenDN := map[int64]struct{}{}
	seenDE := map[int64]struct{}{}

	for _, r := range all {
		// descriptor ids (always present)
		if _, ok := seenDesc[r.descID]; !ok {
			seenDesc[r.descID] = struct{}{}
			descIDs = append(descIDs, r.descID)
		}
		// adminInfo (nullable)
		if r.adminInfoID.Valid {
			id := r.adminInfoID.Int64
			if _, ok := seenAI[id]; !ok {
				seenAI[id] = struct{}{}
				adminInfoIDs = append(adminInfoIDs, id)
			}
		}
		// displayName (nullable)
		if r.displayNameID.Valid {
			id := r.displayNameID.Int64
			if _, ok := seenDN[id]; !ok {
				seenDN[id] = struct{}{}
				displayNameIDs = append(displayNameIDs, id)
			}
		}
		// description (nullable)
		if r.descriptionID.Valid {
			id := r.descriptionID.Int64
			if _, ok := seenDE[id]; !ok {
				seenDE[id] = struct{}{}
				descriptionIDs = append(descriptionIDs, id)
			}
		}
	}

	// ---- Bulk hydration (parallel) ----
	admByID := map[int64]model.AdministrativeInformation{}
	dnByID := map[int64][]model.LangStringNameType{}
	descByID := map[int64][]model.LangStringTextType{}
	endpointsByDesc := map[int64][]model.Endpoint{}
	specificByDesc := map[int64][]model.SpecificAssetId{}
	extByDesc := map[int64][]model.Extension{}
	smdByDesc := map[int64][]model.SubmodelDescriptor{}

	g, gctx := errgroup.WithContext(ctx)

	// AdministrativeInformation
	if len(adminInfoIDs) > 0 {
		ids := append([]int64(nil), adminInfoIDs...) // copy to avoid accidental capture issues
		g.Go(func() error {
			m, err := readAdministrativeInformationByIDs(gctx, p.db, ids)
			if err != nil {
				return err
			}
			admByID = m
			return nil
		})
	}

	// DisplayName
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

	// Description
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

	// Hydrations keyed by descriptor IDs
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

	// Wait for all hydrations (fail fast if any returns error)
	if err := g.Wait(); err != nil {
		return nil, "", err
	}

	// ---- Assemble output in the same order as 'all' ----
	out := make([]model.AssetAdministrationShellDescriptor, 0, len(all))
	for _, r := range all {
		// AssetKind (nullable string -> enum, fallback to NOT_APPLICABLE)
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
			adminInfo = admByID[r.adminInfoID.Int64] // zero-value if missing
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

	return out, nextCursor, nil
}
