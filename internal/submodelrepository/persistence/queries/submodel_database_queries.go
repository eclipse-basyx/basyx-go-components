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

// Package queries contains SQL builders for submodel repository persistence.
package queries

import (
	"strconv"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	jsoniter "github.com/json-iterator/go"
)

// BuildInsertSubmodelSQL builds the insert statement for a submodel row.
func BuildInsertSubmodelSQL(submodel types.ISubmodel) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.Insert("submodel").Rows(goqu.Record{
		"submodel_identifier": submodel.ID(),
		"id_short":            submodel.IDShort(),
		"category":            submodel.Category(),
		"kind":                submodel.Kind(),
	}).Returning(goqu.I("id")).ToSQL()
}

// BuildInsertSubmodelPayloadSQL builds the insert statement for submodel payload data.
func BuildInsertSubmodelPayloadSQL(submodelDBID int64, descriptionJsonString *string, displayNameJsonString *string, administrativeInformationJsonString *string, edsJsonString *string, suplSemIdJsonString *string, extensionJsonString *string, qualifiersJsonString *string) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
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

// BuildInsertSubmodelSemanticIDReferenceSQL builds the semantic ID reference insert statement.
func BuildInsertSubmodelSemanticIDReferenceSQL(submodelDBID int64, semanticID types.IReference) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.Insert("submodel_semantic_id_reference").Rows(goqu.Record{
		"id":   submodelDBID,
		"type": int(semanticID.Type()),
	}).ToSQL()
}

// BuildInsertSubmodelSemanticIDReferenceKeysSQL builds semantic ID reference key inserts.
func BuildInsertSubmodelSemanticIDReferenceKeysSQL(submodelDBID int64, semanticID types.IReference) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
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

// BuildInsertSubmodelSemanticIDReferencePayloadSQL builds the semantic ID payload insert.
func BuildInsertSubmodelSemanticIDReferencePayloadSQL(submodelDBID int64, semanticID types.IReference) (string, []any, error) {
	semanticIDJsonable, err := jsonization.ToJsonable(semanticID)
	if err != nil {
		return "", nil, err
	}

	jsonAPI := jsoniter.ConfigCompatibleWithStandardLibrary
	semanticIDJSONBytes, err := jsonAPI.Marshal(semanticIDJsonable)
	if err != nil {
		return "", nil, err
	}

	dialect := goqu.Dialect(common.Dialect)
	return dialect.Insert("submodel_semantic_id_reference_payload").Rows(goqu.Record{
		"reference_id":             submodelDBID,
		"parent_reference_payload": goqu.L("?::jsonb", string(semanticIDJSONBytes)),
	}).ToSQL()
}

// SelectSubmodelDataset builds the base submodel select dataset.
func SelectSubmodelDataset(
	submodelIdentifier *string,
	idShort *string,
	limit *int32,
	cursor *string,
	createdFrom time.Time,
	updatedFrom time.Time,
	additionalProjections []interface{},
) (*goqu.SelectDataset, error) {
	dialect := goqu.Dialect(common.Dialect)
	semanticIDSelectExpression := buildSubmodelSemanticIDSelectExpression(&dialect)

	baseProjections := []interface{}{
		goqu.I("submodel.submodel_identifier").As("c0"),
		goqu.I("submodel.id_short").As("c1"),
		goqu.I("submodel.category").As("c2"),
		goqu.I("submodel.kind").As("c3"),
		goqu.I("submodel_payload.description_payload").As("raw_description_payload"),
		goqu.I("submodel_payload.displayname_payload").As("raw_displayname_payload"),
		goqu.I("submodel_payload.administrative_information_payload").As("raw_administrative_information_payload"),
		goqu.I("submodel_payload.embedded_data_specification_payload").As("raw_embedded_data_specification_payload"),
		goqu.I("submodel_payload.supplemental_semantic_ids_payload").As("raw_supplemental_semantic_ids_payload"),
		goqu.I("submodel_payload.extensions_payload").As("raw_extensions_payload"),
		goqu.I("submodel_payload.qualifiers_payload").As("raw_qualifiers_payload"),
		semanticIDSelectExpression,
		goqu.I("submodel.submodel_identifier").As("sort_submodel_identifier"),
	}

	selectDS := dialect.From("submodel").
		Join(goqu.T("submodel_payload"), goqu.On(goqu.Ex{"submodel.id": goqu.I("submodel_payload.submodel_id")})).
		Select(append(baseProjections, additionalProjections...)...).
		Order(goqu.I("submodel.submodel_identifier").Asc())

	if submodelIdentifier != nil {
		selectDS = selectDS.Where(goqu.Ex{"submodel.submodel_identifier": *submodelIdentifier}).Limit(1)
		return selectDS, nil
	}

	if idShort != nil && *idShort != "" {
		selectDS = selectDS.Where(goqu.Ex{"submodel.id_short": *idShort})
	}

	if cursor != nil && *cursor != "" {
		cursorExistsDS := dialect.From(goqu.T("submodel").As("s2")).
			Select(goqu.V(1)).
			Where(goqu.Ex{"s2.submodel_identifier": *cursor})

		selectDS = selectDS.
			Where(goqu.Func("EXISTS", cursorExistsDS)).
			Where(goqu.I("submodel.submodel_identifier").Gte(*cursor))
	}
	switch {
	case !createdFrom.IsZero() && !updatedFrom.IsZero():
		selectDS = selectDS.Where(goqu.Or(
			goqu.I("submodel.administration_created_at").Gte(createdFrom.UTC()),
			goqu.I("submodel.administration_updated_at").Gte(updatedFrom.UTC()),
		))
	case !createdFrom.IsZero():
		selectDS = selectDS.Where(goqu.I("submodel.administration_created_at").Gte(createdFrom.UTC()))
	case !updatedFrom.IsZero():
		selectDS = selectDS.Where(goqu.I("submodel.administration_updated_at").Gte(updatedFrom.UTC()))
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

// ApplySubmodelSemanticIDFilter adds a semantic ID existence filter to a submodel dataset.
func ApplySubmodelSemanticIDFilter(selectDS *goqu.SelectDataset, semanticID string) *goqu.SelectDataset {
	if semanticID == "" {
		return selectDS
	}

	dialect := goqu.Dialect(common.Dialect)
	semanticIDFilterDS := dialect.
		From(goqu.T("submodel_semantic_id_reference_key").As("ssrk_filter")).
		Select(goqu.V(1)).
		Where(goqu.I("ssrk_filter.reference_id").Eq(goqu.I("submodel.id"))).
		Where(goqu.I("ssrk_filter.value").Eq(semanticID))
	return selectDS.Where(goqu.Func("EXISTS", semanticIDFilterDS))
}

// BuildSubmodelListSQL builds the final SQL for a masked submodel list query.
func BuildSubmodelListSQL(selectDS *goqu.SelectDataset, dataAlias string, maskedExpressions []exp.Expression) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.From(selectDS.As(dataAlias)).
		Select(
			goqu.I(dataAlias+".c0"),
			maskedExpressions[0],
			goqu.I(dataAlias+".c2"),
			goqu.I(dataAlias+".c3"),
			goqu.I(dataAlias+".raw_description_payload"),
			goqu.I(dataAlias+".raw_displayname_payload"),
			goqu.I(dataAlias+".raw_administrative_information_payload"),
			goqu.I(dataAlias+".raw_embedded_data_specification_payload"),
			goqu.I(dataAlias+".raw_supplemental_semantic_ids_payload"),
			goqu.I(dataAlias+".raw_extensions_payload"),
			goqu.I(dataAlias+".raw_qualifiers_payload"),
			maskedExpressions[1],
		).
		Order(goqu.I(dataAlias + ".sort_submodel_identifier").Asc()).
		ToSQL()
}

// BuildSubmodelCursorExistsSQL builds the cursor existence query.
func BuildSubmodelCursorExistsSQL(cursor string) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.
		From(goqu.T("submodel").As("sm")).
		Select(goqu.V(1)).
		Where(goqu.I("sm.submodel_identifier").Eq(cursor)).
		Limit(1).
		ToSQL()
}

// SelectVisibleSubmodelDataset builds a submodel visibility check dataset.
func SelectVisibleSubmodelDataset(submodelID string) *goqu.SelectDataset {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.
		From("submodel").
		Select(goqu.C("id")).
		Where(goqu.C("submodel_identifier").Eq(submodelID)).
		Limit(1)
}

// SelectSubmodelElementByPathDataset builds a submodel element path lookup dataset.
func SelectSubmodelElementByPathDataset(submodelDatabaseID int, idShortPath string) *goqu.SelectDataset {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.
		From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.id")).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			goqu.I("sme.idshort_path").Eq(idShortPath),
		).
		Limit(1)
}

// BuildTopLevelSubmodelElementMaxPositionSQL builds the top-level element max-position query.
func BuildTopLevelSubmodelElementMaxPositionSQL(submodelDatabaseID int) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.From("submodel_element").
		Select(goqu.MAX("position")).
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("parent_sme_id").IsNull(),
		).
		ToSQL()
}

// BuildFileAttachmentExistsSQL builds the file attachment existence query.
func BuildFileAttachmentExistsSQL(submodelID string, idShortPath string) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	sm := goqu.T("submodel").As("sm")
	sme := goqu.T("submodel_element").As("sme")
	fe := goqu.T("file_element").As("fe")
	fd := goqu.T("file_data").As("fd")

	return dialect.From(sm).
		Join(sme, goqu.On(goqu.I("sme.submodel_id").Eq(goqu.I("sm.id")))).
		LeftJoin(fe, goqu.On(goqu.I("fe.id").Eq(goqu.I("sme.id")))).
		LeftJoin(fd, goqu.On(goqu.I("fd.id").Eq(goqu.I("sme.id")))).
		Select(
			goqu.I("fe.id").As("file_element_id"),
			goqu.I("fd.file_oid").As("file_oid"),
		).
		Where(
			goqu.I("sm.submodel_identifier").Eq(submodelID),
			goqu.I("sme.idshort_path").Eq(idShortPath),
		).
		Limit(1).
		ToSQL()
}

// BuildUpdateSubmodelMetadataSQL builds the submodel metadata update statement.
func BuildUpdateSubmodelMetadataSQL(submodelDatabaseID int, submodel types.ISubmodel) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.
		Update("submodel").
		Set(goqu.Record{
			"id_short": submodel.IDShort(),
			"category": submodel.Category(),
			"kind":     submodel.Kind(),
		}).
		Where(goqu.I("id").Eq(submodelDatabaseID)).
		ToSQL()
}

// BuildUpsertSubmodelPayloadSQL builds the submodel payload upsert statement.
func BuildUpsertSubmodelPayloadSQL(submodelDatabaseID int, descriptionJsonString *string, displayNameJsonString *string, administrativeInformationJsonString *string, edsJsonString *string, supplementalSemanticIDsJsonString *string, extensionJsonString *string, qualifiersJsonString *string) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	payloadRecord := goqu.Record{
		"submodel_id":                         submodelDatabaseID,
		"description_payload":                 descriptionJsonString,
		"displayname_payload":                 displayNameJsonString,
		"administrative_information_payload":  administrativeInformationJsonString,
		"embedded_data_specification_payload": edsJsonString,
		"supplemental_semantic_ids_payload":   supplementalSemanticIDsJsonString,
		"extensions_payload":                  extensionJsonString,
		"qualifiers_payload":                  qualifiersJsonString,
	}

	return dialect.
		Insert("submodel_payload").
		Rows(payloadRecord).
		OnConflict(goqu.DoUpdate("submodel_id", payloadRecord)).
		ToSQL()
}

// BuildDeleteSubmodelSemanticIDSQL builds the semantic ID delete statement for a submodel.
func BuildDeleteSubmodelSemanticIDSQL(submodelDatabaseID int) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.
		Delete("submodel_semantic_id_reference").
		Where(goqu.I("id").Eq(submodelDatabaseID)).
		ToSQL()
}

// BuildSiblingIDShortCollisionSQL builds the sibling idShort collision query.
func BuildSiblingIDShortCollisionSQL(submodelDatabaseID int, parentElementID *int, idShort string) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	query := dialect.From("submodel_element").
		Select(goqu.COUNT("*"))

	whereExpressions := []goqu.Expression{
		goqu.C("submodel_id").Eq(submodelDatabaseID),
		goqu.C("id_short").Eq(idShort),
	}

	if parentElementID == nil {
		whereExpressions = append(whereExpressions, goqu.C("parent_sme_id").IsNull())
	} else {
		whereExpressions = append(whereExpressions, goqu.C("parent_sme_id").Eq(*parentElementID))
	}

	return query.Where(whereExpressions...).ToSQL()
}

// BuildSubmodelElementModelTypeByPathSQL builds the model type lookup query for an element path.
func BuildSubmodelElementModelTypeByPathSQL(submodelDatabaseID int, idShortOrPath string) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.From("submodel_element").
		Select("model_type").
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("idshort_path").Eq(idShortOrPath),
		).
		ToSQL()
}

// BuildSubmodelElementPathExistsSQL builds the element path existence query.
func BuildSubmodelElementPathExistsSQL(submodelDatabaseID int, idShortPath string) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.From("submodel_element").
		Select(goqu.C("id")).
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("idshort_path").Eq(idShortPath),
		).
		Limit(1).
		ToSQL()
}

// BuildCleanupSubmodelLargeObjectsSQL builds the large object cleanup query for a submodel.
func BuildCleanupSubmodelLargeObjectsSQL(submodelDatabaseID int64) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	unlinkSubquery := dialect.From(goqu.T("submodel_element").As("sme")).
		Prepared(true).
		Join(goqu.T("file_data").As("fd"), goqu.On(goqu.I("fd.id").Eq(goqu.I("sme.id")))).
		Select(goqu.Func("lo_unlink", goqu.I("fd.file_oid")).As("unlink_result")).
		Where(
			goqu.I("sme.submodel_id").Eq(submodelDatabaseID),
			goqu.I("fd.file_oid").IsNotNull(),
		)

	return dialect.From(unlinkSubquery.As("unlink_results")).
		Prepared(true).
		Select(goqu.COUNT("*")).
		ToSQL()
}

// BuildDeleteSubmodelByDatabaseIDSQL builds the submodel delete statement.
func BuildDeleteSubmodelByDatabaseIDSQL(submodelDatabaseID int64) (string, []any, error) {
	dialect := goqu.Dialect(common.Dialect)
	return dialect.Delete("submodel").Where(goqu.I("id").Eq(submodelDatabaseID)).ToSQL()
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

	return goqu.COALESCE(semanticIDPayloadSelectDS, semanticIDSelectDS, goqu.L("'{}'::jsonb")).As("raw_semantic_id_payload")
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
