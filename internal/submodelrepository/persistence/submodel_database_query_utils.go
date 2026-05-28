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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"strconv"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	jsoniter "github.com/json-iterator/go"
)

func buildSubmodelQuery(dialect *goqu.DialectWrapper, submodel types.ISubmodel) (string, []any, error) {
	return dialect.Insert("submodel").Rows(goqu.Record{
		"submodel_identifier": submodel.ID(),
		"id_short":            submodel.IDShort(),
		"category":            submodel.Category(),
		"kind":                submodel.Kind(),
	}).Returning(goqu.I("id")).ToSQL()
}

func buildSubmodelPayloadQuery(dialect *goqu.DialectWrapper, submodelDBID int64, descriptionJsonString *string, displayNameJsonString *string, administrativeInformationJsonString *string, edsJsonString *string, suplSemIdJsonString *string, extensionJsonString *string, qualifiersJsonString *string) (string, []any, error) {
	return dialect.Insert("submodel_payload").Rows(goqu.Record{
		"submodel_id":                         submodelDBID,
		"description_payload":                 descriptionJsonString,
		"displayname_payload":                 displayNameJsonString,
		"administrative_information_payload":  administrativeInformationJsonString,
		"embedded_data_specification_payload": edsJsonString,
		"supplemental_semantic_ids_payload":   suplSemIdJsonString,
		"extensions_payload":                  extensionJsonString,
		"qualifiers_payload":                  qualifiersJsonString,
	}).ToSQL()
}

func buildSubmodelSemanticIDReferenceQuery(dialect *goqu.DialectWrapper, submodelDBID int64, semanticID types.IReference) (string, []any, error) {
	return dialect.Insert("submodel_semantic_id_reference").Rows(goqu.Record{
		"id":   submodelDBID,
		"type": int(semanticID.Type()),
	}).ToSQL()
}

func buildSubmodelSemanticIDReferenceKeysQuery(dialect *goqu.DialectWrapper, submodelDBID int64, semanticID types.IReference) (string, []any, error) {
	keyRows := make([]goqu.Record, 0, len(semanticID.Keys()))
	for position, key := range semanticID.Keys() {
		keyRows = append(keyRows, goqu.Record{
			"reference_id": submodelDBID,
			"position":     position,
			"type":         int(key.Type()),
			"value":        key.Value(),
		})
	}

	if len(keyRows) == 0 {
		return "", nil, nil
	}

	return dialect.Insert("submodel_semantic_id_reference_key").Rows(keyRows).ToSQL()
}

func buildSubmodelSemanticIDReferencePayloadQuery(dialect *goqu.DialectWrapper, submodelDBID int64, semanticID types.IReference) (string, []any, error) {
	semanticIDJsonable, err := jsonization.ToJsonable(semanticID)
	if err != nil {
		return "", nil, err
	}

	jsonAPI := jsoniter.ConfigCompatibleWithStandardLibrary
	semanticIDJSONBytes, err := jsonAPI.Marshal(semanticIDJsonable)
	if err != nil {
		return "", nil, err
	}

	return dialect.Insert("submodel_semantic_id_reference_payload").Rows(goqu.Record{
		"reference_id":             submodelDBID,
		"parent_reference_payload": goqu.L("?::jsonb", string(semanticIDJSONBytes)),
	}).ToSQL()
}

func selectSubmodelGoquQuery(dialect *goqu.DialectWrapper, submodelIdentifier *string, limit *int32, cursor *string) (*goqu.SelectDataset, error) {
	semanticIDSelectExpression := buildSubmodelSemanticIDSelectExpression(dialect)

	selectDS := dialect.From("submodel").
		Join(goqu.T("submodel_payload"), goqu.On(goqu.Ex{"submodel.id": goqu.I("submodel_payload.submodel_id")})).
		Select(
			goqu.I("submodel.submodel_identifier"),
			goqu.I("submodel.id_short"),
			goqu.I("submodel.category"),
			goqu.I("submodel.kind"),
			goqu.I("submodel_payload.description_payload"),
			goqu.I("submodel_payload.displayname_payload"),
			goqu.I("submodel_payload.administrative_information_payload"),
			goqu.I("submodel_payload.embedded_data_specification_payload"),
			goqu.I("submodel_payload.supplemental_semantic_ids_payload"),
			goqu.I("submodel_payload.extensions_payload"),
			goqu.I("submodel_payload.qualifiers_payload"),
			semanticIDSelectExpression,
		).
		Order(goqu.I("submodel.submodel_identifier").Asc())

	if submodelIdentifier != nil {
		selectDS = selectDS.Where(goqu.Ex{"submodel.submodel_identifier": *submodelIdentifier}).Limit(1)
		return selectDS, nil
	}

	if cursor != nil && *cursor != "" {
		cursorExistsDS := dialect.From(goqu.T("submodel").As("s2")).
			Select(goqu.V(1)).
			Where(goqu.Ex{"s2.submodel_identifier": *cursor})

		selectDS = selectDS.
			Where(goqu.Func("EXISTS", cursorExistsDS)).
			Where(goqu.I("submodel.submodel_identifier").Gte(*cursor))
	}

	if limit != nil && *limit > 0 {
		pageLimitPlusOneString := strconv.FormatInt(int64(*limit)+1, 10)
		pageLimitPlusOne, err := strconv.ParseUint(pageLimitPlusOneString, 10, 64)
		if err != nil {
			return nil, err
		}
		selectDS = selectDS.Limit(uint(pageLimitPlusOne))
	}
	return selectDS, nil
}

func buildSubmodelSemanticIDSelectExpression(dialect *goqu.DialectWrapper) exp.AliasedExpression {
	referenceTypeSelectExpression := buildReferenceTypeStringSelectExpression(goqu.I("ssr.type"))
	keyTypeSelectExpression := buildKeyTypeStringSelectExpression(goqu.I("ssrk.type"))

	semanticIDPayloadSelectDS := dialect.
		From(goqu.T("submodel_semantic_id_reference_payload").As("ssrp")).
		Select(goqu.I("ssrp.parent_reference_payload")).
		Where(goqu.I("ssrp.reference_id").Eq(goqu.I("submodel.id"))).
		Limit(1)

	orderedKeyValuesSelectDS := dialect.
		From(goqu.T("submodel_semantic_id_reference_key").As("ssrk")).
		Select(
			keyTypeSelectExpression.As("type"),
			goqu.I("ssrk.value").As("value"),
		).
		Where(goqu.I("ssrk.reference_id").Eq(goqu.I("ssr.id"))).
		Order(goqu.I("ssrk.position").Asc())

	aggregatedKeyValuesSelectDS := dialect.
		From(orderedKeyValuesSelectDS.As("ordered_key_values")).
		Select(
			goqu.COALESCE(
				goqu.Func(
					"jsonb_agg",
					goqu.Func(
						"jsonb_build_object",
						goqu.V("type"), goqu.I("ordered_key_values.type"),
						goqu.V("value"), goqu.I("ordered_key_values.value"),
					),
				),
				goqu.L("'[]'::jsonb"),
			),
		)

	semanticIDSelectDS := dialect.
		From(goqu.T("submodel_semantic_id_reference").As("ssr")).
		Select(
			goqu.Func(
				"jsonb_build_object",
				goqu.V("type"), referenceTypeSelectExpression,
				goqu.V("keys"), aggregatedKeyValuesSelectDS,
			),
		).
		Where(goqu.I("ssr.id").Eq(goqu.I("submodel.id"))).
		Limit(1)

	return goqu.COALESCE(semanticIDPayloadSelectDS, semanticIDSelectDS, goqu.L("'{}'::jsonb")).As("semantic_id_payload")
}

func buildReferenceTypeStringSelectExpression(typeColumn exp.Expression) exp.CaseExpression {
	caseExpression := goqu.Case().
		Value(typeColumn)

	for _, referenceType := range types.LiteralsOfReferenceTypes {
		caseExpression = caseExpression.
			When(int(referenceType), stringification.MustReferenceTypesToString(referenceType))
	}

	return caseExpression.Else(nil)
}

func buildKeyTypeStringSelectExpression(typeColumn exp.Expression) exp.CaseExpression {
	caseExpression := goqu.Case().
		Value(typeColumn)

	for _, keyType := range types.LiteralsOfKeyTypes {
		caseExpression = caseExpression.
			When(int(keyType), stringification.MustKeyTypesToString(keyType))
	}

	return caseExpression.Else(nil)
}
