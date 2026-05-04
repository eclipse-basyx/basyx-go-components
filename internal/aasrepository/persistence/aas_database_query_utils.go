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

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/doug-martin/goqu/v9"
	jsoniter "github.com/json-iterator/go"
)

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

func buildGetAssetAdministrationShellsDataset(dialect *goqu.DialectWrapper, limit int32, cursor string, idShort string, assetIDs []string) (*goqu.SelectDataset, error) {
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

	if len(assetIDs) > 0 {
		ds = ds.Where(goqu.I("asset_information.global_asset_id").In(assetIDs))
	}

	return ds, nil
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

func buildDeleteSubmodelReferenceByIDQuery(dialect *goqu.DialectWrapper, submodelReferenceDBID int64) (string, []any, error) {
	return dialect.Delete("aas_submodel_reference").Where(goqu.I("id").Eq(submodelReferenceDBID)).ToSQL()
}

func buildGetAssetAdministrationShellMapByDBIDQuery(dialect *goqu.DialectWrapper, aasDBID int64) (string, []any, error) {
	return dialect.
		From(goqu.T("aas").As("a")).
		LeftJoin(goqu.T("aas_payload").As("ap"), goqu.On(goqu.I("ap.aas_id").Eq(goqu.I("a.id")))).
		LeftJoin(goqu.T("asset_information").As("ai"), goqu.On(goqu.I("ai.asset_information_id").Eq(goqu.I("a.id")))).
		LeftJoin(goqu.T("thumbnail_file_element").As("tfe"), goqu.On(goqu.I("tfe.id").Eq(goqu.I("a.id")))).
		Select(
			goqu.I("a.aas_id"),
			goqu.I("a.id_short"),
			goqu.I("a.category"),
			goqu.I("ap.displayname_payload"),
			goqu.I("ap.description_payload"),
			goqu.I("ap.administrative_information_payload"),
			goqu.I("ap.embedded_data_specification_payload"),
			goqu.I("ap.extensions_payload"),
			goqu.I("ap.derived_from_payload"),
			goqu.I("ai.asset_kind"),
			goqu.I("ai.global_asset_id"),
			goqu.I("ai.asset_type"),
			goqu.I("tfe.value"),
			goqu.I("tfe.content_type"),
		).
		Where(goqu.I("a.id").Eq(aasDBID)).
		ToSQL()
}

func buildGetAssetAdministrationShellMapsByDBIDsQuery(dialect *goqu.DialectWrapper, aasDBIDs []int64) (string, []any, error) {
	return dialect.
		From(goqu.T("aas").As("a")).
		LeftJoin(goqu.T("aas_payload").As("ap"), goqu.On(goqu.I("ap.aas_id").Eq(goqu.I("a.id")))).
		LeftJoin(goqu.T("asset_information").As("ai"), goqu.On(goqu.I("ai.asset_information_id").Eq(goqu.I("a.id")))).
		LeftJoin(goqu.T("thumbnail_file_element").As("tfe"), goqu.On(goqu.I("tfe.id").Eq(goqu.I("a.id")))).
		Select(
			goqu.I("a.id"),
			goqu.I("a.aas_id"),
			goqu.I("a.id_short"),
			goqu.I("a.category"),
			goqu.I("ap.displayname_payload"),
			goqu.I("ap.description_payload"),
			goqu.I("ap.administrative_information_payload"),
			goqu.I("ap.embedded_data_specification_payload"),
			goqu.I("ap.extensions_payload"),
			goqu.I("ap.derived_from_payload"),
			goqu.I("ai.asset_kind"),
			goqu.I("ai.global_asset_id"),
			goqu.I("ai.asset_type"),
			goqu.I("tfe.value"),
			goqu.I("tfe.content_type"),
		).
		Where(goqu.I("a.id").In(aasDBIDs)).
		ToSQL()
}

func buildGetSubmodelReferencePayloadsByAASIDQuery(dialect *goqu.DialectWrapper, aasDBID int64) (string, []any, error) {
	return dialect.
		From(goqu.T("aas_submodel_reference").As("r")).
		InnerJoin(goqu.T("aas_submodel_reference_payload").As("rp"), goqu.On(goqu.I("rp.reference_id").Eq(goqu.I("r.id")))).
		Select(goqu.I("rp.parent_reference_payload")).
		Where(goqu.I("r.aas_id").Eq(aasDBID)).
		Order(goqu.I("r.position").Asc(), goqu.I("r.id").Asc()).
		ToSQL()
}

func buildGetSubmodelReferencePayloadsByAASIDsQuery(dialect *goqu.DialectWrapper, aasDBIDs []int64) (string, []any, error) {
	return dialect.
		From(goqu.T("aas_submodel_reference").As("r")).
		InnerJoin(goqu.T("aas_submodel_reference_payload").As("rp"), goqu.On(goqu.I("rp.reference_id").Eq(goqu.I("r.id")))).
		Select(goqu.I("r.aas_id"), goqu.I("rp.parent_reference_payload")).
		Where(goqu.I("r.aas_id").In(aasDBIDs)).
		Order(goqu.I("r.aas_id").Asc(), goqu.I("r.position").Asc(), goqu.I("r.id").Asc()).
		ToSQL()
}

func buildReadSpecificAssetIDsByAssetInformationIDQuery(dialect *goqu.DialectWrapper, assetInformationID int64) (string, []any, error) {
	return dialect.
		From(goqu.T("specific_asset_id").As("sid")).
		LeftJoin(goqu.T("specific_asset_id_payload").As("sp"), goqu.On(goqu.I("sp.specific_asset_id").Eq(goqu.I("sid.id")))).
		Select(
			goqu.I("sid.id"),
			goqu.I("sid.name"),
			goqu.I("sid.value"),
			goqu.I("sp.semantic_id_payload"),
		).
		Where(goqu.I("sid.asset_information_id").Eq(assetInformationID)).
		Order(goqu.I("sid.position").Asc(), goqu.I("sid.id").Asc()).
		ToSQL()
}

func buildReadSpecificAssetIDsByAssetInformationIDsQuery(dialect *goqu.DialectWrapper, assetInformationIDs []int64) (string, []any, error) {
	return dialect.
		From(goqu.T("specific_asset_id").As("sid")).
		LeftJoin(goqu.T("specific_asset_id_payload").As("sp"), goqu.On(goqu.I("sp.specific_asset_id").Eq(goqu.I("sid.id")))).
		Select(
			goqu.I("sid.asset_information_id"),
			goqu.I("sid.id"),
			goqu.I("sid.name"),
			goqu.I("sid.value"),
			goqu.I("sp.semantic_id_payload"),
		).
		Where(goqu.I("sid.asset_information_id").In(assetInformationIDs)).
		Order(goqu.I("sid.asset_information_id").Asc(), goqu.I("sid.position").Asc(), goqu.I("sid.id").Asc()).
		ToSQL()
}
