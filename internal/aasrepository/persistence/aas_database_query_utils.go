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
	"fmt"
	"strconv"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	jsoniter "github.com/json-iterator/go"
)

const globalAssetIDAssetLinkName = "globalAssetId"

func buildPageLimitPlusOne(limit int32) (uint, error) {
	pageLimitPlusOneString := strconv.FormatInt(int64(limit)+1, 10)
	pageLimitPlusOne, err := strconv.ParseUint(pageLimitPlusOneString, 10, strconv.IntSize)
	if err != nil {
		return 0, fmt.Errorf("AASREPO-BUILDPAGELIMIT-PARSEUINT: %w", err)
	}

	maxUint := uint64(^uint(0))
	if pageLimitPlusOne > maxUint {
		return 0, fmt.Errorf("AASREPO-BUILDPAGELIMIT-CHECKMAXUINT: invalid limit %d", limit)
	}

	return uint(pageLimitPlusOne), nil
}

func buildAssetAdministrationShellQuery(dialect *goqu.DialectWrapper, aas types.IAssetAdministrationShell) (string, []any, error) {
	return dialect.Insert("aas").Rows(goqu.Record{
		"aas_id":   aas.ID(),
		"id_short": aas.IDShort(),
		"category": aas.Category(),
	}).Returning(goqu.I("id")).ToSQL()
}

func buildAssetAdministrationShellPayloadQuery(dialect *goqu.DialectWrapper, aasDBID int64, descriptionJsonString *string, displayNameJsonString *string, administrativeInformationJsonString *string, edsJsonString *string, extensionJsonString *string, derivedFromJsonString *string) (string, []any, error) {
	return dialect.Insert("aas_payload").Rows(goqu.Record{
		"aas_id":                              aasDBID,
		"description_payload":                 descriptionJsonString,
		"displayname_payload":                 displayNameJsonString,
		"administrative_information_payload":  administrativeInformationJsonString,
		"embedded_data_specification_payload": edsJsonString,
		"extensions_payload":                  extensionJsonString,
		"derived_from_payload":                derivedFromJsonString,
	}).ToSQL()
}

func buildAssetInformationQuery(dialect *goqu.DialectWrapper, aasDBID int64, asset_information types.IAssetInformation) (string, []any, error) {
	return dialect.Insert("asset_information").Rows(goqu.Record{
		"asset_information_id": aasDBID,
		"asset_kind":           asset_information.AssetKind(),
		"global_asset_id":      asset_information.GlobalAssetID(),
		"asset_type":           asset_information.AssetType(),
	}).ToSQL()
}

func buildAssetAdministrationShellSubmodelReferenceQuery(dialect *goqu.DialectWrapper, aasDBID int64, position int, submodelRef types.IReference) (string, []any, error) {
	return dialect.Insert("aas_submodel_reference").Rows(goqu.Record{
		"aas_id":   aasDBID,
		"position": position,
		"type":     int(submodelRef.Type()),
	}).Returning(goqu.I("id")).ToSQL()
}

func buildGetNextAssetAdministrationShellSubmodelReferencePositionQuery(dialect *goqu.DialectWrapper, aasDBID int64) (string, []any, error) {
	return dialect.
		From("aas_submodel_reference").
		Select(goqu.L("COALESCE(MAX(position), -1) + 1")).
		Where(goqu.I("aas_id").Eq(aasDBID)).
		ToSQL()
}

func buildAssetAdministrationShellSubmodelReferenceKeysQuery(dialect *goqu.DialectWrapper, aasSubmodelReferenceDBID int64, submodelRef types.IReference) (string, []any, error) {
	keyRows := make([]goqu.Record, 0, len(submodelRef.Keys()))
	for position, key := range submodelRef.Keys() {
		keyRows = append(keyRows, goqu.Record{
			"reference_id": aasSubmodelReferenceDBID,
			"position":     position,
			"type":         int(key.Type()),
			"value":        key.Value(),
		})
	}

	if len(keyRows) == 0 {
		return "", nil, fmt.Errorf("reference must contain at least one key")
	}

	return dialect.Insert("aas_submodel_reference_key").Rows(keyRows).ToSQL()
}

func buildAssetAdministrationShellSubmodelReferencePayloadQuery(dialect *goqu.DialectWrapper, aasSubmodelReferenceDBID int64, submodelRef types.IReference) (string, []any, error) {
	submodelRefJsonable, err := jsonization.ToJsonable(submodelRef)
	if err != nil {
		return "", nil, err
	}

	jsonAPI := jsoniter.ConfigCompatibleWithStandardLibrary
	submodelRefJSONBytes, err := jsonAPI.Marshal(submodelRefJsonable)
	if err != nil {
		return "", nil, err
	}

	return dialect.Insert("aas_submodel_reference_payload").Rows(goqu.Record{
		"reference_id":             aasSubmodelReferenceDBID,
		"parent_reference_payload": goqu.L("?::jsonb", string(submodelRefJSONBytes)),
	}).ToSQL()
}

func buildCheckAssetAdministrationShellSubmodelReferenceExistsQuery(dialect *goqu.DialectWrapper, aasDBID int64, submodelIdentifier string) (string, []any, error) {
	return dialect.
		Select(goqu.L("1")).
		From(goqu.T("aas_submodel_reference").As("ref")).
		InnerJoin(
			goqu.T("aas_submodel_reference_key").As("key"),
			goqu.On(goqu.I("key.reference_id").Eq(goqu.I("ref.id"))),
		).
		Where(
			goqu.I("ref.aas_id").Eq(aasDBID),
			goqu.I("key.value").Eq(submodelIdentifier),
		).
		Limit(1).
		ToSQL()
}

func buildGetAssetAdministrationShellsDataset(dialect *goqu.DialectWrapper, limit int32, cursor string, idShort string, assetLinks []commonmodel.AssetLink) (*goqu.SelectDataset, error) {
	ds := dialect.
		From(goqu.T("aas").As("aas")).
		LeftJoin(goqu.T("asset_information").As("asset_information"), goqu.On(goqu.I("asset_information.asset_information_id").Eq(goqu.I("aas.id")))).
		Select(goqu.I("aas.id")).
		Order(goqu.I("aas.aas_id").Asc())

	if limit > 0 {
		pageLimitPlusOne, err := buildPageLimitPlusOne(limit)
		if err != nil {
			return nil, err
		}

		ds = ds.Limit(pageLimitPlusOne)
	}

	if cursor != "" {
		ds = ds.Where(goqu.I("aas.aas_id").Gte(cursor))
	}

	if idShort != "" {
		ds = ds.Where(goqu.I("aas.id_short").Eq(idShort))
	}

	for _, link := range uniqueAssetLinks(assetLinks) {
		ds = ds.Where(buildAssetLinkFilterExpression(dialect, link))
	}

	return ds, nil
}

func buildAssetLinkFilterExpression(dialect *goqu.DialectWrapper, link commonmodel.AssetLink) goqu.Expression {
	if link.Name == globalAssetIDAssetLinkName {
		return goqu.I("asset_information.global_asset_id").Eq(link.Value)
	}

	specificAssetID := goqu.T("specific_asset_id").As("specific_asset_id_filter")
	existsSub := dialect.From(specificAssetID).
		Select(goqu.V(1)).
		Where(goqu.And(
			goqu.I("specific_asset_id_filter.asset_information_id").Eq(goqu.I("asset_information.asset_information_id")),
			goqu.I("specific_asset_id_filter.name").Eq(link.Name),
			goqu.I("specific_asset_id_filter.value").Eq(link.Value),
		))
	return goqu.L("EXISTS ?", existsSub)
}

func uniqueAssetLinks(assetLinks []commonmodel.AssetLink) []commonmodel.AssetLink {
	seen := make(map[string]struct{}, len(assetLinks))
	out := make([]commonmodel.AssetLink, 0, len(assetLinks))
	for _, link := range assetLinks {
		key := link.Name + "\x1f" + link.Value
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, link)
	}
	return out
}

func buildGetAssetAdministrationShellCursorByDBIDQuery(dialect *goqu.DialectWrapper, aasDBID int64) (string, []any, error) {
	return dialect.From("aas").Select("aas_id").Where(goqu.I("id").Eq(aasDBID)).ToSQL()
}

func buildGetAssetAdministrationShellDBIDByIdentifierQuery(dialect *goqu.DialectWrapper, aasIdentifier string) (string, []any, error) {
	ds := buildGetAssetAdministrationShellDBIDByIdentifierDataset(dialect, aasIdentifier)
	return ds.ToSQL()
}

func buildGetAssetAdministrationShellDBIDByIdentifierDataset(dialect *goqu.DialectWrapper, aasIdentifier string) *goqu.SelectDataset {
	return dialect.From(goqu.T("aas").As("aas")).
		Select(goqu.I("aas.id")).
		Where(goqu.I("aas.aas_id").Eq(aasIdentifier)).
		Limit(1)
}

func buildDeleteAssetAdministrationShellByDBIDQuery(dialect *goqu.DialectWrapper, aasDBID int64) (string, []any, error) {
	return dialect.Delete("aas").Where(goqu.I("id").Eq(aasDBID)).ToSQL()
}

func buildDeleteAssetAdministrationShellByIdentifierQuery(dialect *goqu.DialectWrapper, aasIdentifier string) (string, []any, error) {
	return dialect.Delete("aas").Where(goqu.I("aas_id").Eq(aasIdentifier)).ToSQL()
}

func buildGetAssetInformationCurrentStateQuery(dialect *goqu.DialectWrapper, aasDBID int64) (string, []any, error) {
	return dialect.From("asset_information").
		Select("asset_kind", "global_asset_id", "asset_type").
		Where(goqu.I("asset_information_id").Eq(aasDBID)).
		ToSQL()
}

func buildUpdateAssetInformationQuery(dialect *goqu.DialectWrapper, aasDBID int64, record goqu.Record) (string, []any, error) {
	return dialect.Update("asset_information").
		Set(record).
		Where(goqu.I("asset_information_id").Eq(aasDBID)).
		ToSQL()
}

func buildDeleteSpecificAssetIDsByAssetInformationIDQuery(dialect *goqu.DialectWrapper, aasDBID int64) (string, []any, error) {
	return dialect.Delete("specific_asset_id").Where(goqu.I("asset_information_id").Eq(aasDBID)).ToSQL()
}

func buildGetAllSubmodelReferencesByAASIDQuery(dialect *goqu.DialectWrapper, aasDBID int64, limit int32, cursorID int64) (string, []any, error) {
	ds := dialect.
		From(goqu.T("aas_submodel_reference").As("r")).
		InnerJoin(goqu.T("aas_submodel_reference_payload").As("rp"), goqu.On(goqu.I("rp.reference_id").Eq(goqu.I("r.id")))).
		Select(goqu.I("r.id"), goqu.I("rp.parent_reference_payload")).
		Where(goqu.I("r.aas_id").Eq(aasDBID)).
		Order(goqu.I("r.id").Asc())

	if limit > 0 {
		pageLimitPlusOne, err := buildPageLimitPlusOne(limit)
		if err != nil {
			return "", nil, err
		}

		ds = ds.Limit(pageLimitPlusOne)
	}

	if cursorID > 0 {
		ds = ds.Where(goqu.I("r.id").Gte(cursorID))
	}

	return ds.ToSQL()
}

func buildFindSubmodelReferenceIDByAASIDAndSubmodelIdentifierQuery(dialect *goqu.DialectWrapper, aasDBID int64, submodelIdentifier string) (string, []any, error) {
	return dialect.
		From(goqu.T("aas_submodel_reference").As("r")).
		InnerJoin(goqu.T("aas_submodel_reference_key").As("k"), goqu.On(goqu.I("k.reference_id").Eq(goqu.I("r.id")))).
		Select(goqu.I("r.id")).
		Where(
			goqu.I("r.aas_id").Eq(aasDBID),
			goqu.I("k.value").Eq(submodelIdentifier),
		).
		Limit(1).
		ToSQL()
}

func buildListAASIdentifiersBySubmodelIdentifierQuery(dialect *goqu.DialectWrapper, submodelIdentifier string) (string, []any, error) {
	return dialect.
		From(goqu.T("aas_submodel_reference").As("r")).
		InnerJoin(goqu.T("aas_submodel_reference_key").As("k"), goqu.On(goqu.I("k.reference_id").Eq(goqu.I("r.id")))).
		InnerJoin(goqu.T("aas").As("a"), goqu.On(goqu.I("a.id").Eq(goqu.I("r.aas_id")))).
		SelectDistinct(goqu.I("a.aas_id")).
		Where(goqu.I("k.value").Eq(submodelIdentifier)).
		Order(goqu.I("a.aas_id").Asc()).
		ToSQL()
}

func buildDeleteSubmodelReferenceByIDQuery(dialect *goqu.DialectWrapper, submodelReferenceDBID int64) (string, []any, error) {
	return dialect.Delete("aas_submodel_reference").Where(goqu.I("id").Eq(submodelReferenceDBID)).ToSQL()
}

func unmaskedCoreAssetAdministrationShellSelectExpressions(includeDatabaseID bool) []interface{} {
	expressions := make([]interface{}, 0, 15)
	if includeDatabaseID {
		expressions = append(expressions, goqu.I("aas.id"))
	}
	return append(expressions,
		goqu.I("aas.aas_id"),
		goqu.I("aas.id_short"),
		goqu.I("aas.category"),
		goqu.I("ap.displayname_payload"),
		goqu.I("ap.description_payload"),
		goqu.I("ap.administrative_information_payload"),
		goqu.I("ap.embedded_data_specification_payload"),
		goqu.I("ap.extensions_payload"),
		goqu.I("ap.derived_from_payload"),
		goqu.I("asset_information.asset_kind"),
		goqu.I("asset_information.global_asset_id"),
		goqu.I("asset_information.asset_type"),
		goqu.I("tfe.value"),
		goqu.I("tfe.content_type"),
	)
}

func buildGetAssetAdministrationShellMapByDBIDQueryWithSelect(dialect *goqu.DialectWrapper, aasDBID int64, selectExpressions []interface{}) (string, []any, error) {
	return dialect.
		From(goqu.T("aas").As("aas")).
		LeftJoin(goqu.T("aas_payload").As("ap"), goqu.On(goqu.I("ap.aas_id").Eq(goqu.I("aas.id")))).
		LeftJoin(goqu.T("asset_information").As("asset_information"), goqu.On(goqu.I("asset_information.asset_information_id").Eq(goqu.I("aas.id")))).
		LeftJoin(goqu.T("thumbnail_file_element").As("tfe"), goqu.On(goqu.I("tfe.id").Eq(goqu.I("aas.id")))).
		Select(selectExpressions...).
		Where(goqu.I("aas.id").Eq(aasDBID)).
		ToSQL()
}

func buildGetAssetAdministrationShellMapsByDBIDsQueryWithSelect(dialect *goqu.DialectWrapper, aasDBIDs []int64, selectExpressions []interface{}) (string, []any, error) {
	return dialect.
		From(goqu.T("aas").As("aas")).
		LeftJoin(goqu.T("aas_payload").As("ap"), goqu.On(goqu.I("ap.aas_id").Eq(goqu.I("aas.id")))).
		LeftJoin(goqu.T("asset_information").As("asset_information"), goqu.On(goqu.I("asset_information.asset_information_id").Eq(goqu.I("aas.id")))).
		LeftJoin(goqu.T("thumbnail_file_element").As("tfe"), goqu.On(goqu.I("tfe.id").Eq(goqu.I("aas.id")))).
		Select(selectExpressions...).
		Where(goqu.I("aas.id").In(aasDBIDs)).
		ToSQL()
}

func buildGetSubmodelReferencePayloadsByAASIDsDataset(dialect *goqu.DialectWrapper, aasDBIDs []int64) *goqu.SelectDataset {
	return dialect.
		From(goqu.T("aas_submodel_reference").As("aas_submodel_reference")).
		InnerJoin(
			goqu.T("aas_submodel_reference_payload").As("rp"),
			goqu.On(goqu.I("rp.reference_id").Eq(goqu.I("aas_submodel_reference.id"))),
		).
		LeftJoin(
			goqu.T("aas_submodel_reference_key").As("aas_submodel_reference_key"),
			goqu.On(goqu.I("aas_submodel_reference_key.reference_id").Eq(goqu.I("aas_submodel_reference.id"))),
		).
		Select(
			goqu.I("aas_submodel_reference.aas_id"),
			goqu.I("rp.parent_reference_payload"),
		).
		Where(goqu.I("aas_submodel_reference.aas_id").In(aasDBIDs)).
		GroupBy(
			goqu.I("aas_submodel_reference.id"),
			goqu.I("aas_submodel_reference.aas_id"),
			goqu.I("aas_submodel_reference.position"),
			goqu.I("rp.parent_reference_payload"),
		).
		Order(
			goqu.I("aas_submodel_reference.aas_id").Asc(),
			goqu.I("aas_submodel_reference.position").Asc(),
			goqu.I("aas_submodel_reference.id").Asc(),
		)
}

func buildReadSpecificAssetIDsByAssetInformationIDDataset(dialect *goqu.DialectWrapper, assetInformationID int64) *goqu.SelectDataset {
	return buildSpecificAssetIDReadDataset(dialect).
		Select(
			goqu.I("specific_asset_id.id"),
			goqu.I("specific_asset_id.name"),
			goqu.I("specific_asset_id.value"),
			goqu.I("sp.semantic_id_payload"),
		).
		Where(goqu.I("specific_asset_id.asset_information_id").Eq(assetInformationID)).
		GroupBy(
			goqu.I("specific_asset_id.id"),
			goqu.I("specific_asset_id.name"),
			goqu.I("specific_asset_id.value"),
			goqu.I("specific_asset_id.position"),
			goqu.I("sp.semantic_id_payload"),
		).
		Order(goqu.I("specific_asset_id.position").Asc(), goqu.I("specific_asset_id.id").Asc())
}

func buildReadSpecificAssetIDsByAssetInformationIDsDataset(dialect *goqu.DialectWrapper, assetInformationIDs []int64) *goqu.SelectDataset {
	return buildSpecificAssetIDReadDataset(dialect).
		Select(
			goqu.I("specific_asset_id.asset_information_id"),
			goqu.I("specific_asset_id.id"),
			goqu.I("specific_asset_id.name"),
			goqu.I("specific_asset_id.value"),
			goqu.I("sp.semantic_id_payload"),
		).
		Where(goqu.I("specific_asset_id.asset_information_id").In(assetInformationIDs)).
		GroupBy(
			goqu.I("specific_asset_id.asset_information_id"),
			goqu.I("specific_asset_id.id"),
			goqu.I("specific_asset_id.name"),
			goqu.I("specific_asset_id.value"),
			goqu.I("specific_asset_id.position"),
			goqu.I("sp.semantic_id_payload"),
		).
		Order(goqu.I("specific_asset_id.asset_information_id").Asc(), goqu.I("specific_asset_id.position").Asc(), goqu.I("specific_asset_id.id").Asc())
}

func buildSpecificAssetIDReadDataset(dialect *goqu.DialectWrapper) *goqu.SelectDataset {
	return dialect.
		From(goqu.T("specific_asset_id").As("specific_asset_id")).
		LeftJoin(goqu.T("specific_asset_id_payload").As("sp"), goqu.On(goqu.I("sp.specific_asset_id").Eq(goqu.I("specific_asset_id.id")))).
		LeftJoin(
			goqu.T("specific_asset_id_external_subject_id_reference").As("external_subject_reference"),
			goqu.On(goqu.I("external_subject_reference.id").Eq(goqu.I("specific_asset_id.id"))),
		).
		LeftJoin(
			goqu.T("specific_asset_id_external_subject_id_reference_key").As("external_subject_reference_key"),
			goqu.On(goqu.I("external_subject_reference_key.reference_id").Eq(goqu.I("external_subject_reference.id"))),
		)
}
