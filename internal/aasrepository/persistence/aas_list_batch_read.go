/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package persistence

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/jackc/pgx/v5"
)

type aasListPage struct {
	databaseIDs    []int64
	nextDatabaseID int64
}

type batchedReferenceKey struct {
	kind int
	id   int64
}

type batchedReferenceScanRow struct {
	ownerID       sql.NullInt64
	referenceID   sql.NullInt64
	referenceType sql.NullInt64
	keyID         sql.NullInt64
	keyType       sql.NullInt64
	keyValue      sql.NullString
	parentPayload []byte
}

type batchedReferenceState struct {
	builders            map[int64]*builder.ReferenceBuilder
	references          map[int64]types.IReference
	referenceIDsByOwner map[int64][]int64
	seenByOwner         map[int64]map[int64]struct{}
}

type batchedSpecificAssetRow struct {
	ownerID           int64
	id                int64
	name              string
	value             string
	semanticIDPayload []byte
}

type batchedSpecificAssetScanRow struct {
	rowKind           int
	ownerID           sql.NullInt64
	specificID        sql.NullInt64
	specificPosition  sql.NullInt64
	name              sql.NullString
	value             sql.NullString
	semanticPayload   []byte
	referenceID       sql.NullInt64
	referencePosition sql.NullInt64
	referenceType     sql.NullInt64
	keyID             sql.NullInt64
	keyPosition       sql.NullInt64
	keyType           sql.NullInt64
	keyValue          sql.NullString
	parentPayload     []byte
}

type batchedSpecificAssetState struct {
	baseRows               map[int64]batchedSpecificAssetRow
	idsByOwner             map[int64][]int64
	referenceBuilders      map[batchedReferenceKey]*builder.ReferenceBuilder
	references             map[batchedReferenceKey]types.IReference
	referenceIDsBySpecific map[int64][]batchedReferenceKey
	seenReferences         map[int64]map[batchedReferenceKey]struct{}
}

func (s *AssetAdministrationShellDatabase) getAssetAdministrationShellsPostgreSQLBatch(
	ctx context.Context,
	db *sql.DB,
	limit int32,
	cursor string,
	idShort string,
	specificAssetIDs []types.ISpecificAssetID,
	createdFrom time.Time,
	updatedFrom time.Time,
) ([]types.IAssetAdministrationShell, string, error) {
	if limit < 0 {
		return nil, "", common.NewErrBadRequest("AASREPO-GETAASLIST-BADLIMIT Limit " + strconv.FormatInt(int64(limit), 10) + " too small")
	}

	var result []types.IAssetAdministrationShell
	var nextCursor string
	err := common.ExecutePostgreSQLReadTransaction(ctx, db, func(tx pgx.Tx) error {
		pageQuery, pageArgs, err := buildAASListPageQuery(ctx, limit, cursor, idShort, specificAssetIDs, createdFrom, updatedFrom)
		if err != nil {
			return err
		}
		page, err := readAASListPage(ctx, tx, pageQuery, pageArgs, limit)
		if err != nil {
			return err
		}
		if len(page.databaseIDs) == 0 {
			result = []types.IAssetAdministrationShell{}
			return nil
		}

		result, nextCursor, err = s.readAASListMaterializationBatch(ctx, tx, page)
		return err
	})
	return result, nextCursor, err
}

func buildAASListPageQuery(
	ctx context.Context,
	limit int32,
	cursor string,
	idShort string,
	specificAssetIDs []types.ISpecificAssetID,
	createdFrom time.Time,
	updatedFrom time.Time,
) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	dataset, err := buildGetAssetAdministrationShellsDataset(&dialect, limit, cursor, idShort, specificAssetIDs, createdFrom, updatedFrom)
	if err != nil {
		return "", nil, common.NewInternalServerError("AASREPO-GETAASLIST-BUILDPAGE " + err.Error())
	}
	collector, err := buildAASCollector()
	if err != nil {
		return "", nil, err
	}
	shouldEnforce, err := shouldEnforceFormula(ctx, "AASREPO-GETAASLIST-SHOULDENFORCE")
	if err != nil {
		return "", nil, err
	}
	if shouldEnforce {
		dataset, err = auth.AddFormulaQueryFromContext(ctx, dataset, collector)
		if err != nil {
			return "", nil, common.NewInternalServerError("AASREPO-GETAASLIST-ABACFORMULA " + err.Error())
		}
	}
	query, args, err := dataset.Prepared(true).ToSQL()
	if err != nil {
		return "", nil, common.NewInternalServerError("AASREPO-GETAASLIST-BUILDPAGESQL " + err.Error())
	}
	return query, args, nil
}

func readAASListPage(
	ctx context.Context,
	tx pgx.Tx,
	query string,
	args []any,
	limit int32,
) (aasListPage, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return aasListPage{}, common.NewInternalServerError("AASREPO-GETAASLIST-EXECPAGE " + err.Error())
	}
	defer rows.Close()

	ids := make([]int64, 0, max(int(limit)+1, 1))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return aasListPage{}, common.NewInternalServerError("AASREPO-GETAASLIST-SCANPAGE " + err.Error())
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return aasListPage{}, common.NewInternalServerError("AASREPO-GETAASLIST-ROWSPAGE " + err.Error())
	}

	page := aasListPage{databaseIDs: ids}
	if limit > 0 && len(ids) > int(limit) {
		page.nextDatabaseID = ids[len(ids)-1]
		page.databaseIDs = ids[:len(ids)-1]
	}
	return page, nil
}

func (s *AssetAdministrationShellDatabase) readAASListMaterializationBatch(
	ctx context.Context,
	tx pgx.Tx,
	page aasListPage,
) ([]types.IAssetAdministrationShell, string, error) {
	statements, err := buildAASListMaterializationStatements(ctx, page)
	if err != nil {
		return nil, "", err
	}
	batch := &pgx.Batch{}
	for _, statement := range statements {
		batch.Queue(statement.SQL, statement.Args...)
	}
	results := tx.SendBatch(ctx, batch)
	closed := false
	defer func() {
		if !closed {
			_ = results.Close()
		}
	}()

	coreRows, err := readBatchedAASCoreRows(results)
	if err != nil {
		return nil, "", err
	}
	submodelsByAASID, err := readBatchedAASSubmodelReferences(results, page.databaseIDs)
	if err != nil {
		return nil, "", err
	}
	specificByAASID, err := readBatchedSpecificAssetIDs(results, page.databaseIDs)
	if err != nil {
		return nil, "", err
	}
	nextCursor := ""
	if page.nextDatabaseID != 0 {
		rows, queryErr := results.Query()
		if queryErr != nil {
			return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-GETCURSOR " + queryErr.Error())
		}
		if !rows.Next() {
			rowsErr := rows.Err()
			rows.Close()
			if rowsErr != nil {
				return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-GETCURSOR " + rowsErr.Error())
			}
			return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-GETCURSOR " + sql.ErrNoRows.Error())
		}
		if scanErr := rows.Scan(&nextCursor); scanErr != nil {
			rows.Close()
			return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-GETCURSOR " + scanErr.Error())
		}
		rows.Close()
		if rowsErr := rows.Err(); rowsErr != nil {
			return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-GETCURSOR " + rowsErr.Error())
		}
	}
	if err := results.Close(); err != nil {
		return nil, "", common.NewInternalServerError("AASREPO-GETAASLIST-CLOSEBATCH " + err.Error())
	}
	closed = true

	assembled, err := assembleAASListModels(page.databaseIDs, coreRows, submodelsByAASID, specificByAASID)
	if err != nil {
		return nil, "", err
	}
	return assembled, nextCursor, nil
}

func (s *AssetAdministrationShellDatabase) readAASListMaterializationSequential(
	ctx context.Context,
	db aasDBQueryer,
	databaseIDs []int64,
) ([]types.IAssetAdministrationShell, error) {
	statements, err := buildAASListMaterializationStatements(ctx, aasListPage{databaseIDs: databaseIDs})
	if err != nil {
		return nil, err
	}
	coreSQLRows, err := db.QueryContext(ctx, statements[0].SQL, statements[0].Args...)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-EXECCORE " + err.Error())
	}
	coreRows, err := scanAASCoreRows(coreSQLRows)
	_ = coreSQLRows.Close()
	if err != nil {
		return nil, err
	}

	referenceSQLRows, err := db.QueryContext(ctx, statements[1].SQL, statements[1].Args...)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-EXECREFS " + err.Error())
	}
	submodelsByAASID, err := scanBatchedReferences(referenceSQLRows, databaseIDs, true, "AASREPO-MAPAASBATCH-REFS")
	_ = referenceSQLRows.Close()
	if err != nil {
		return nil, err
	}

	specificSQLRows, err := db.QueryContext(ctx, statements[2].SQL, statements[2].Args...)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-MAPAASBATCH-EXECSPECIFIC " + err.Error())
	}
	specificByAASID, err := scanBatchedSpecificAssetIDs(specificSQLRows, databaseIDs)
	_ = specificSQLRows.Close()
	if err != nil {
		return nil, err
	}
	return assembleAASListModels(databaseIDs, coreRows, submodelsByAASID, specificByAASID)
}

func assembleAASListModels(
	databaseIDs []int64,
	coreRows map[int64]coreAssetAdministrationShellRow,
	submodelsByAASID map[int64][]types.IReference,
	specificByAASID map[int64][]types.ISpecificAssetID,
) ([]types.IAssetAdministrationShell, error) {
	result := make([]types.IAssetAdministrationShell, 0, len(databaseIDs))
	for _, databaseID := range databaseIDs {
		row, exists := coreRows[databaseID]
		if !exists {
			continue
		}
		aas, err := buildAssetAdministrationShellFromCoreRow(
			row,
			submodelsByAASID[databaseID],
			specificByAASID[databaseID],
			"AASREPO-MAPAASBATCH",
		)
		if err != nil {
			return nil, err
		}
		result = append(result, aas)
	}
	return result, nil
}

func buildAASListMaterializationStatements(
	ctx context.Context,
	page aasListPage,
) ([]common.PostgreSQLBatchStatement, error) {
	dialect := goqu.Dialect(common.Dialect)
	collector, err := buildAASCollector()
	if err != nil {
		return nil, err
	}
	coreExpressions, err := buildCoreAssetAdministrationShellSelectExpressions(ctx, collector, true)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-GETAASLIST-BUILDMASKS " + err.Error())
	}
	coreQuery, coreArgs, err := buildGetAssetAdministrationShellMapsByDBIDsQueryWithSelect(&dialect, page.databaseIDs, coreExpressions)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-GETAASLIST-BUILDCORE " + err.Error())
	}
	referenceQuery, referenceArgs, err := buildBatchedAASSubmodelReferenceQuery(ctx, dialect, page.databaseIDs)
	if err != nil {
		return nil, err
	}
	specificQuery, specificArgs, err := buildBatchedSpecificAssetIDQuery(ctx, dialect, page.databaseIDs)
	if err != nil {
		return nil, err
	}
	statements := []common.PostgreSQLBatchStatement{
		{SQL: coreQuery, Args: coreArgs},
		{SQL: referenceQuery, Args: referenceArgs},
		{SQL: specificQuery, Args: specificArgs},
	}
	if page.nextDatabaseID != 0 {
		cursorQuery, cursorArgs, cursorErr := buildGetAssetAdministrationShellCursorByDBIDQuery(&dialect, page.nextDatabaseID)
		if cursorErr != nil {
			return nil, common.NewInternalServerError("AASREPO-GETAASLIST-BUILDCURSOR " + cursorErr.Error())
		}
		statements = append(statements, common.PostgreSQLBatchStatement{SQL: cursorQuery, Args: cursorArgs})
	}
	return statements, nil
}

func buildBatchedAASSubmodelReferenceQuery(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	aasDatabaseIDs []int64,
) (string, []any, error) {
	reference := goqu.T("aas_submodel_reference").As("aas_submodel_reference")
	key := goqu.T("aas_submodel_reference_key").As("aas_submodel_reference_key")
	payload := goqu.T("aas_submodel_reference_payload").As("aas_submodel_reference_payload")
	dataset := dialect.From(reference).
		LeftJoin(payload, goqu.On(payload.Col("reference_id").Eq(reference.Col("id")))).
		LeftJoin(key, goqu.On(key.Col("reference_id").Eq(reference.Col("id")))).
		Select(
			reference.Col("aas_id"),
			reference.Col("id"),
			reference.Col("type"),
			key.Col("id"),
			key.Col("type"),
			key.Col("value"),
			payload.Col("parent_reference_payload"),
		).
		Where(common.PostgreSQLBigIntArrayContains(reference.Col("aas_id"), aasDatabaseIDs)).
		Order(
			reference.Col("aas_id").Asc(),
			reference.Col("position").Asc(),
			reference.Col("id").Asc(),
			key.Col("position").Asc(),
			key.Col("id").Asc(),
		)
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAAS)
	if err != nil {
		return "", nil, common.NewInternalServerError("AASREPO-GETAASLIST-REFCOLLECTOR " + err.Error())
	}
	collector.AllowInlineAliases("aas_submodel_reference", "aas_submodel_reference_key")
	collector.SetRootJoinKey("aas_submodel_reference", "aas_id")
	for _, fragment := range []grammar.FragmentStringPattern{"$aas#submodels[]", "$aas#submodels[].keys[]"} {
		dataset, err = auth.AddCorrelatedFilterQueryFromContext(ctx, dataset, fragment, collector)
		if err != nil {
			return "", nil, common.NewInternalServerError("AASREPO-GETAASLIST-REFFILTER " + err.Error())
		}
	}
	query, args, err := dataset.Prepared(true).ToSQL()
	if err != nil {
		return "", nil, common.NewInternalServerError("AASREPO-GETAASLIST-BUILDREFS " + err.Error())
	}
	return query, args, nil
}

func buildBatchedSpecificAssetIDQuery(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	aasDatabaseIDs []int64,
) (string, []any, error) {
	base, external, supplemental, err := buildBatchedSpecificAssetIDBranches(ctx, dialect, aasDatabaseIDs)
	if err != nil {
		return "", nil, err
	}
	const dataAlias = "batched_specific_asset_id_data"
	data := goqu.T(dataAlias)
	query := dialect.From(base.UnionAll(external).UnionAll(supplemental).As(dataAlias)).
		Select(
			data.Col("row_kind"),
			data.Col("owner_id"),
			data.Col("specific_id"),
			data.Col("specific_position"),
			data.Col("name"),
			data.Col("value"),
			data.Col("semantic_payload"),
			data.Col("reference_id"),
			data.Col("reference_position"),
			data.Col("reference_type"),
			data.Col("key_id"),
			data.Col("key_position"),
			data.Col("key_type"),
			data.Col("key_value"),
			data.Col("parent_payload"),
		).
		Order(
			data.Col("owner_id").Asc(),
			data.Col("specific_position").Asc(),
			data.Col("specific_id").Asc(),
			data.Col("row_kind").Asc(),
			data.Col("reference_position").Asc(),
			data.Col("reference_id").Asc(),
			data.Col("key_position").Asc(),
			data.Col("key_id").Asc(),
		)
	sqlQuery, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return "", nil, common.NewInternalServerError("AASREPO-GETAASLIST-BUILDSPECIFIC " + err.Error())
	}
	return sqlQuery, args, nil
}

func buildBatchedSpecificAssetIDBranches(
	ctx context.Context,
	dialect goqu.DialectWrapper,
	aasDatabaseIDs []int64,
) (*goqu.SelectDataset, *goqu.SelectDataset, *goqu.SelectDataset, error) {
	specific := goqu.T("specific_asset_id").As(common.AliasSpecificAssetID)
	payload := goqu.T("specific_asset_id_payload").As("specific_asset_payload")
	collector, err := buildBatchedSpecificAssetIDCollector()
	if err != nil {
		return nil, nil, nil, err
	}
	base := dialect.From(specific).
		LeftJoin(payload, goqu.On(payload.Col("specific_asset_id").Eq(specific.Col("id")))).
		Select(batchedSpecificAssetBaseColumns(specific, payload)...).
		Where(common.PostgreSQLBigIntArrayContains(specific.Col("asset_information_id"), aasDatabaseIDs))
	base, err = auth.AddFilterQueryFromContext(ctx, base, "$aas#assetInformation.specificAssetIds[]", collector)
	if err != nil {
		return nil, nil, nil, common.NewInternalServerError("AASREPO-GETAASLIST-SPECIFICFILTER " + err.Error())
	}

	externalReference := goqu.T("specific_asset_id_external_subject_id_reference").As(common.AliasExternalSubjectReference)
	externalKey := goqu.T("specific_asset_id_external_subject_id_reference_key").As(common.AliasExternalSubjectReferenceKey)
	externalPayload := goqu.T("specific_asset_id_external_subject_id_reference_payload").As("external_subject_payload")
	external := dialect.From(specific).
		Join(externalReference, goqu.On(externalReference.Col("id").Eq(specific.Col("id")))).
		LeftJoin(externalKey, goqu.On(externalKey.Col("reference_id").Eq(externalReference.Col("id")))).
		LeftJoin(externalPayload, goqu.On(externalPayload.Col("reference_id").Eq(externalReference.Col("id")))).
		Select(batchedSpecificAssetReferenceColumns(1, specific, externalReference, externalKey, externalPayload, goqu.V(0))...).
		Where(common.PostgreSQLBigIntArrayContains(specific.Col("asset_information_id"), aasDatabaseIDs))
	for _, fragment := range []grammar.FragmentStringPattern{
		"$aas#assetInformation.specificAssetIds[]",
		"$aas#assetInformation.specificAssetIds[].externalSubjectId",
		"$aas#assetInformation.specificAssetIds[].externalSubjectId.keys[]",
	} {
		external, err = auth.AddFilterQueryFromContext(ctx, external, fragment, collector)
		if err != nil {
			return nil, nil, nil, common.NewInternalServerError("AASREPO-GETAASLIST-EXTERNALFILTER " + err.Error())
		}
	}

	supplementalReference := goqu.T("specific_asset_id_supplemental_semantic_id_reference").As("specific_asset_supplemental_semantic_id_reference")
	supplementalKey := goqu.T("specific_asset_id_supplemental_semantic_id_reference_key").As("specific_asset_supplemental_semantic_id_reference_key")
	supplementalPayload := goqu.T("specific_asset_id_supplemental_semantic_id_reference_payload").As("specific_asset_supplemental_semantic_id_reference_payload")
	supplemental := dialect.From(specific).
		Join(supplementalReference, goqu.On(supplementalReference.Col("specific_asset_id_id").Eq(specific.Col("id")))).
		LeftJoin(supplementalKey, goqu.On(supplementalKey.Col("reference_id").Eq(supplementalReference.Col("id")))).
		LeftJoin(supplementalPayload, goqu.On(supplementalPayload.Col("reference_id").Eq(supplementalReference.Col("id")))).
		Select(batchedSpecificAssetReferenceColumns(2, specific, supplementalReference, supplementalKey, supplementalPayload, supplementalReference.Col("position"))...).
		Where(common.PostgreSQLBigIntArrayContains(specific.Col("asset_information_id"), aasDatabaseIDs))
	return base, external, supplemental, nil
}

func buildBatchedSpecificAssetIDCollector() (*grammar.ResolvedFieldPathCollector, error) {
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAAS)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-GETAASLIST-SPECIFICCOLLECTOR " + err.Error())
	}
	collector.AllowInlineAliases(
		common.AliasSpecificAssetID,
		common.AliasExternalSubjectReference,
		common.AliasExternalSubjectReferenceKey,
		"specific_asset_supplemental_semantic_id_reference",
		"specific_asset_supplemental_semantic_id_reference_key",
	)
	collector.SetRootJoinKey(common.AliasSpecificAssetID, common.ColAssetInformationID)
	return collector, nil
}

func batchedSpecificAssetBaseColumns(specific, payload expColumner) []interface{} {
	return []interface{}{
		goqu.V(0).As("row_kind"),
		specific.Col("asset_information_id").As("owner_id"),
		specific.Col("id").As("specific_id"),
		specific.Col("position").As("specific_position"),
		specific.Col("name").As("name"),
		specific.Col("value").As("value"),
		payload.Col("semantic_id_payload").As("semantic_payload"),
		goqu.L("NULL::bigint").As("reference_id"),
		goqu.L("NULL::integer").As("reference_position"),
		goqu.L("NULL::integer").As("reference_type"),
		goqu.L("NULL::bigint").As("key_id"),
		goqu.L("NULL::integer").As("key_position"),
		goqu.L("NULL::integer").As("key_type"),
		goqu.L("NULL::text").As("key_value"),
		goqu.L("NULL::jsonb").As("parent_payload"),
	}
}

type expColumner interface {
	Col(interface{}) exp.IdentifierExpression
}

func batchedSpecificAssetReferenceColumns(
	kind int,
	specific expColumner,
	reference expColumner,
	key expColumner,
	payload expColumner,
	referencePosition interface{},
) []interface{} {
	return []interface{}{
		goqu.V(kind).As("row_kind"),
		specific.Col("asset_information_id").As("owner_id"),
		specific.Col("id").As("specific_id"),
		specific.Col("position").As("specific_position"),
		goqu.L("NULL::text").As("name"),
		goqu.L("NULL::text").As("value"),
		goqu.L("NULL::jsonb").As("semantic_payload"),
		reference.Col("id").As("reference_id"),
		goqu.L("?", referencePosition).As("reference_position"),
		reference.Col("type").As("reference_type"),
		key.Col("id").As("key_id"),
		key.Col("position").As("key_position"),
		key.Col("type").As("key_type"),
		key.Col("value").As("key_value"),
		payload.Col("parent_reference_payload").As("parent_payload"),
	}
}

func readBatchedAASCoreRows(results pgx.BatchResults) (map[int64]coreAssetAdministrationShellRow, error) {
	rows, err := results.Query()
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-GETAASLIST-BATCHCORE " + err.Error())
	}
	defer rows.Close()
	return scanAASCoreRows(rows)
}

func scanAASCoreRows(rows pgxRows) (map[int64]coreAssetAdministrationShellRow, error) {
	coreRows := make(map[int64]coreAssetAdministrationShellRow)
	for rows.Next() {
		var databaseID int64
		var row coreAssetAdministrationShellRow
		if err := rows.Scan(
			&databaseID,
			&row.aasID,
			&row.idShort,
			&row.category,
			&row.displayNamePayload,
			&row.descriptionPayload,
			&row.administrationPayload,
			&row.edsPayload,
			&row.extensionsPayload,
			&row.derivedFromPayload,
			&row.assetKind,
			&row.globalAssetID,
			&row.assetType,
			&row.thumbnailPath,
			&row.thumbnailContentType,
		); err != nil {
			return nil, common.NewInternalServerError("AASREPO-GETAASLIST-SCANCORE " + err.Error())
		}
		coreRows[databaseID] = row
	}
	if err := rows.Err(); err != nil {
		return nil, common.NewInternalServerError("AASREPO-GETAASLIST-ROWSCORE " + err.Error())
	}
	return coreRows, nil
}

func readBatchedAASSubmodelReferences(
	results pgx.BatchResults,
	ownerIDs []int64,
) (map[int64][]types.IReference, error) {
	rows, err := results.Query()
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-GETAASLIST-BATCHREFS " + err.Error())
	}
	defer rows.Close()
	return scanBatchedReferences(rows, ownerIDs, true, "AASREPO-GETAASLIST-REFS")
}

type pgxRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanBatchedReferences(
	rows pgxRows,
	ownerIDs []int64,
	payloadContainsFullReference bool,
	errorPrefix string,
) (map[int64][]types.IReference, error) {
	state := newBatchedReferenceState()
	for rows.Next() {
		row, err := scanBatchedReferenceRow(rows)
		if err != nil {
			return nil, common.NewInternalServerError(errorPrefix + "-SCANROW " + err.Error())
		}
		if err := state.add(row, payloadContainsFullReference, errorPrefix); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, common.NewInternalServerError(errorPrefix + "-ROWSERR " + err.Error())
	}
	return state.build(ownerIDs), nil
}

func newBatchedReferenceState() *batchedReferenceState {
	return &batchedReferenceState{
		builders:            make(map[int64]*builder.ReferenceBuilder),
		references:          make(map[int64]types.IReference),
		referenceIDsByOwner: make(map[int64][]int64),
		seenByOwner:         make(map[int64]map[int64]struct{}),
	}
}

func scanBatchedReferenceRow(rows pgxRows) (batchedReferenceScanRow, error) {
	var row batchedReferenceScanRow
	err := rows.Scan(
		&row.ownerID,
		&row.referenceID,
		&row.referenceType,
		&row.keyID,
		&row.keyType,
		&row.keyValue,
		&row.parentPayload,
	)
	return row, err
}

func (state *batchedReferenceState) add(
	row batchedReferenceScanRow,
	payloadContainsFullReference bool,
	errorPrefix string,
) error {
	if !row.ownerID.Valid || !row.referenceID.Valid || !row.referenceType.Valid {
		return nil
	}
	if err := state.ensureReference(row, payloadContainsFullReference, errorPrefix); err != nil {
		return err
	}
	state.addOwnerReference(row.ownerID.Int64, row.referenceID.Int64)
	if row.keyID.Valid && row.keyType.Valid && row.keyValue.Valid {
		state.builders[row.referenceID.Int64].CreateKey(row.keyID.Int64, types.KeyTypes(row.keyType.Int64), row.keyValue.String)
	}
	return nil
}

func (state *batchedReferenceState) ensureReference(
	row batchedReferenceScanRow,
	payloadContainsFullReference bool,
	errorPrefix string,
) error {
	if _, exists := state.builders[row.referenceID.Int64]; exists {
		return nil
	}
	reference, referenceBuilder := builder.NewReferenceBuilder(types.ReferenceTypes(row.referenceType.Int64), row.referenceID.Int64)
	parentReference, err := parseReferencePayload(row.parentPayload, errorPrefix+"-PARSEPARENT")
	if err != nil {
		return err
	}
	if payloadContainsFullReference && parentReference != nil {
		parentReference = parentReference.ReferredSemanticID()
	}
	reference.SetReferredSemanticID(parentReference)
	state.builders[row.referenceID.Int64] = referenceBuilder
	state.references[row.referenceID.Int64] = reference
	return nil
}

func (state *batchedReferenceState) addOwnerReference(ownerID int64, referenceID int64) {
	if state.seenByOwner[ownerID] == nil {
		state.seenByOwner[ownerID] = make(map[int64]struct{})
	}
	if _, exists := state.seenByOwner[ownerID][referenceID]; exists {
		return
	}
	state.seenByOwner[ownerID][referenceID] = struct{}{}
	state.referenceIDsByOwner[ownerID] = append(state.referenceIDsByOwner[ownerID], referenceID)
}

func (state *batchedReferenceState) build(ownerIDs []int64) map[int64][]types.IReference {
	for _, referenceBuilder := range state.builders {
		referenceBuilder.BuildNestedStructure()
	}
	out := make(map[int64][]types.IReference, len(ownerIDs))
	for ownerID, referenceIDs := range state.referenceIDsByOwner {
		for _, referenceID := range referenceIDs {
			out[ownerID] = append(out[ownerID], state.references[referenceID])
		}
	}
	for _, ownerID := range ownerIDs {
		if _, exists := out[ownerID]; !exists {
			out[ownerID] = nil
		}
	}
	return out
}

func readBatchedSpecificAssetIDs(
	results pgx.BatchResults,
	ownerIDs []int64,
) (map[int64][]types.ISpecificAssetID, error) {
	rows, err := results.Query()
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-GETAASLIST-BATCHSPECIFIC " + err.Error())
	}
	defer rows.Close()
	return scanBatchedSpecificAssetIDs(rows, ownerIDs)
}

func scanBatchedSpecificAssetIDs(rows pgxRows, ownerIDs []int64) (map[int64][]types.ISpecificAssetID, error) {
	state := newBatchedSpecificAssetState()
	for rows.Next() {
		row, err := scanBatchedSpecificAssetRow(rows)
		if err != nil {
			return nil, common.NewInternalServerError("AASREPO-GETAASLIST-SCANSPECIFIC " + err.Error())
		}
		if err := state.add(row); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, common.NewInternalServerError("AASREPO-GETAASLIST-ROWSSPECIFIC " + err.Error())
	}
	return state.build(ownerIDs)
}

func newBatchedSpecificAssetState() *batchedSpecificAssetState {
	return &batchedSpecificAssetState{
		baseRows:               make(map[int64]batchedSpecificAssetRow),
		idsByOwner:             make(map[int64][]int64),
		referenceBuilders:      make(map[batchedReferenceKey]*builder.ReferenceBuilder),
		references:             make(map[batchedReferenceKey]types.IReference),
		referenceIDsBySpecific: make(map[int64][]batchedReferenceKey),
		seenReferences:         make(map[int64]map[batchedReferenceKey]struct{}),
	}
}

func scanBatchedSpecificAssetRow(rows pgxRows) (batchedSpecificAssetScanRow, error) {
	var row batchedSpecificAssetScanRow
	err := rows.Scan(
		&row.rowKind,
		&row.ownerID,
		&row.specificID,
		&row.specificPosition,
		&row.name,
		&row.value,
		&row.semanticPayload,
		&row.referenceID,
		&row.referencePosition,
		&row.referenceType,
		&row.keyID,
		&row.keyPosition,
		&row.keyType,
		&row.keyValue,
		&row.parentPayload,
	)
	return row, err
}

func (state *batchedSpecificAssetState) add(row batchedSpecificAssetScanRow) error {
	if !row.ownerID.Valid || !row.specificID.Valid {
		return nil
	}
	if row.rowKind == 0 {
		state.addBaseRow(row)
		return nil
	}
	if !row.referenceID.Valid || !row.referenceType.Valid {
		return nil
	}
	return state.addReferenceRow(row)
}

func (state *batchedSpecificAssetState) addBaseRow(row batchedSpecificAssetScanRow) {
	state.baseRows[row.specificID.Int64] = batchedSpecificAssetRow{
		ownerID:           row.ownerID.Int64,
		id:                row.specificID.Int64,
		name:              row.name.String,
		value:             row.value.String,
		semanticIDPayload: row.semanticPayload,
	}
	state.idsByOwner[row.ownerID.Int64] = append(state.idsByOwner[row.ownerID.Int64], row.specificID.Int64)
}

func (state *batchedSpecificAssetState) addReferenceRow(row batchedSpecificAssetScanRow) error {
	referenceKey := batchedReferenceKey{kind: row.rowKind, id: row.referenceID.Int64}
	if _, exists := state.referenceBuilders[referenceKey]; !exists {
		reference, referenceBuilder := builder.NewReferenceBuilder(types.ReferenceTypes(row.referenceType.Int64), row.referenceID.Int64)
		parentReference, err := parseReferencePayload(row.parentPayload, "AASREPO-GETAASLIST-PARSESPECIFICREF")
		if err != nil {
			return err
		}
		reference.SetReferredSemanticID(parentReference)
		state.referenceBuilders[referenceKey] = referenceBuilder
		state.references[referenceKey] = reference
	}
	state.addSpecificAssetReference(row.specificID.Int64, referenceKey)
	if row.keyID.Valid && row.keyType.Valid && row.keyValue.Valid {
		state.referenceBuilders[referenceKey].CreateKey(row.keyID.Int64, types.KeyTypes(row.keyType.Int64), row.keyValue.String)
	}
	return nil
}

func (state *batchedSpecificAssetState) addSpecificAssetReference(specificID int64, referenceKey batchedReferenceKey) {
	if state.seenReferences[specificID] == nil {
		state.seenReferences[specificID] = make(map[batchedReferenceKey]struct{})
	}
	if _, exists := state.seenReferences[specificID][referenceKey]; exists {
		return
	}
	state.seenReferences[specificID][referenceKey] = struct{}{}
	state.referenceIDsBySpecific[specificID] = append(state.referenceIDsBySpecific[specificID], referenceKey)
}

func (state *batchedSpecificAssetState) build(ownerIDs []int64) (map[int64][]types.ISpecificAssetID, error) {
	for _, referenceBuilder := range state.referenceBuilders {
		referenceBuilder.BuildNestedStructure()
	}

	out := make(map[int64][]types.ISpecificAssetID, len(ownerIDs))
	for ownerID, specificIDs := range state.idsByOwner {
		for _, specificID := range specificIDs {
			row := state.baseRows[specificID]
			specificAssetID := types.NewSpecificAssetID(row.name, row.value)
			semanticID, hasSemanticID, err := parseSpecificAssetIDSemanticIDPayload(row.semanticIDPayload)
			if err != nil {
				return nil, common.NewInternalServerError("AASREPO-GETAASLIST-PARSESEMANTIC " + err.Error())
			}
			if hasSemanticID {
				specificAssetID.SetSemanticID(semanticID)
			}
			for _, referenceKey := range state.referenceIDsBySpecific[specificID] {
				if referenceKey.kind == 1 {
					specificAssetID.SetExternalSubjectID(state.references[referenceKey])
				} else {
					specificAssetID.SetSupplementalSemanticIDs(append(specificAssetID.SupplementalSemanticIDs(), state.references[referenceKey]))
				}
			}
			out[ownerID] = append(out[ownerID], specificAssetID)
		}
	}
	for _, ownerID := range ownerIDs {
		if _, exists := out[ownerID]; !exists {
			out[ownerID] = nil
		}
	}
	return out, nil
}
