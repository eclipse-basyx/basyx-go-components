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

package auth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestAddFilterQueriesFromContext_DeduplicatesEquivalentSignatures(t *testing.T) {
	expr := mustParseLogicalExpression(t, `{"$eq":[{"$field":"$aasdesc#idShort"},{"$strVal":"shell-short"}]}`)

	qf := &QueryFilter{
		Filters: FragmentFilters{
			"$aasdesc#endpoints[2]":  expr,
			"$aasdesc#endpoints[10]": expr,
		},
	}
	ctx := context.WithValue(context.Background(), filterKey, qf)

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).Select(goqu.V(1)).Prepared(true)

	fragments := []grammar.FragmentStringPattern{
		"$aasdesc#endpoints[2]",
		"$aasdesc#endpoints[10]",
	}
	filteredDS, err := AddFilterQueriesFromContext(ctx, ds, fragments, nil)
	if err != nil {
		t.Fatalf("AddFilterQueriesFromContext returned error: %v", err)
	}

	sqlStr, args, err := filteredDS.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if got := strings.Count(sqlStr, "\"aas_descriptor_endpoint\".\"position\""); got != 2 {
		t.Fatalf("expected endpoint position binding to appear exactly twice after signature dedupe, got %d SQL: %s", got, sqlStr)
	}

	// one predicate application: select arg + 2x (idShort + endpoint index)
	if len(args) != 5 {
		t.Fatalf("expected 5 SQL args after signature dedupe, got %d args: %#v SQL: %s", len(args), args, sqlStr)
	}
	if !containsIntArg(args, 2) || !containsIntArg(args, 10) {
		t.Fatalf("expected args to contain endpoint indexes 2 and 10, got %#v", args)
	}
}

func mustParseLogicalExpression(t *testing.T, raw string) grammar.LogicalExpression {
	t.Helper()
	var expr grammar.LogicalExpression
	if err := json.Unmarshal([]byte(raw), &expr); err != nil {
		t.Fatalf("failed to unmarshal logical expression: %v", err)
	}
	return expr
}

func containsIntArg(args []interface{}, want int) bool {
	for _, arg := range args {
		switch v := arg.(type) {
		case int:
			if v == want {
				return true
			}
		case int64:
			if v == int64(want) {
				return true
			}
		}
	}
	return false
}

func TestAddFilterQueryFromContext_ArrayEndedFragment_UsesInlinePredicate(t *testing.T) {
	expr := mustParseLogicalExpression(t, `{"$or":[{"$eq":[{"$strVal":"BPN_A"},{"$field":"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"}]},{"$eq":[{"$strVal":"PUBLIC_READABLE"},{"$field":"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"}]}]}`)
	match := true

	qf := &QueryFilter{
		Filters: FragmentFilters{
			"$aasdesc#specificAssetIds[]": expr,
		},
		FilterMatch: FragmentMatchModes{
			"$aasdesc#specificAssetIds[]": match,
		},
	}
	ctx := context.WithValue(context.Background(), filterKey, qf)

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAASDesc)
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T(common.TblDescriptor).As("descriptor")).
		InnerJoin(
			goqu.T(common.TblAASDescriptor).As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		LeftJoin(
			goqu.T(common.TblSpecificAssetID).As(common.AliasSpecificAssetID),
			goqu.On(goqu.I("specific_asset_id.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		LeftJoin(
			goqu.T("specific_asset_id_external_subject_id_reference").As(common.AliasExternalSubjectReference),
			goqu.On(goqu.I("external_subject_reference.id").Eq(goqu.I("specific_asset_id.id"))),
		).
		LeftJoin(
			goqu.T("specific_asset_id_external_subject_id_reference_key").As(common.AliasExternalSubjectReferenceKey),
			goqu.On(goqu.I("external_subject_reference_key.reference_id").Eq(goqu.I("external_subject_reference.id"))),
		).
		Select(goqu.V(1)).
		Prepared(true)

	filteredDS, err := AddFilterQueryFromContext(ctx, ds, "$aasdesc#specificAssetIds[]", collector)
	if err != nil {
		t.Fatalf("AddFilterQueryFromContext returned error: %v", err)
	}

	sqlStr, _, err := filteredDS.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if strings.Contains(sqlStr, "EXISTS (") {
		t.Fatalf("did not expect EXISTS for array-ended fragment filter, got SQL: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `"external_subject_reference_key"."value"`) {
		t.Fatalf("expected inline predicate on external_subject_reference_key.value, got SQL: %s", sqlStr)
	}
}

func TestAddFilterQueryFromContext_ArrayEndedFragment_DefaultBehavior_UsesExists(t *testing.T) {
	expr := mustParseLogicalExpression(t, `{"$or":[{"$eq":[{"$strVal":"BPN_A"},{"$field":"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"}]},{"$eq":[{"$strVal":"PUBLIC_READABLE"},{"$field":"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"}]}]}`)

	qf := &QueryFilter{
		Filters: FragmentFilters{
			"$aasdesc#specificAssetIds[]": expr,
		},
	}
	ctx := context.WithValue(context.Background(), filterKey, qf)

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAASDesc)
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T(common.TblDescriptor).As("descriptor")).
		InnerJoin(
			goqu.T(common.TblAASDescriptor).As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		Select(goqu.V(1)).
		Prepared(true)

	filteredDS, err := AddFilterQueryFromContext(ctx, ds, "$aasdesc#specificAssetIds[]", collector)
	if err != nil {
		t.Fatalf("AddFilterQueryFromContext returned error: %v", err)
	}

	sqlStr, _, err := filteredDS.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if !strings.Contains(sqlStr, "EXISTS (") {
		t.Fatalf("expected EXISTS for default fragment behavior, got SQL: %s", sqlStr)
	}
}

func TestAddFilterQueryFromContext_IndexedFragment_UsesExists(t *testing.T) {
	expr := mustParseLogicalExpression(t, `{"$eq":[{"$field":"$aasdesc#specificAssetIds[1].name"},{"$strVal":"Banane2"}]}`)
	match := true
	fragment := grammar.FragmentStringPattern("$aasdesc#specificAssetIds[0]")
	ctx := context.WithValue(context.Background(), filterKey, &QueryFilter{
		Filters: FragmentFilters{
			fragment: expr,
		},
		FilterMatch: FragmentMatchModes{
			fragment: match,
		},
	})

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAASDesc)
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}
	collector.AllowInlineAliases(common.AliasSpecificAssetID)

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T(common.TblDescriptor).As("descriptor")).
		InnerJoin(
			goqu.T(common.TblAASDescriptor).As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		LeftJoin(
			goqu.T(common.TblSpecificAssetID).As(common.AliasSpecificAssetID),
			goqu.On(goqu.I("specific_asset_id.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		Select(goqu.V(1)).
		Prepared(true)

	filteredDS, err := AddFilterQueryFromContext(ctx, ds, fragment, collector)
	if err != nil {
		t.Fatalf("AddFilterQueryFromContext returned error: %v", err)
	}
	sqlStr, _, err := filteredDS.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if !strings.Contains(sqlStr, "EXISTS (") {
		t.Fatalf("expected indexed fragment condition to use EXISTS: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `"specific_asset_id"."position"`) {
		t.Fatalf("expected indexed fragment condition to bind the indexed row: %s", sqlStr)
	}
}

func TestAddCorrelatedFilterQueryFromContext_MixedAliasesUseInlineAndExists(t *testing.T) {
	expr := mustParseLogicalExpression(t, `{"$and":[{"$eq":[{"$strVal":"PUBLIC_READABLE"},{"$field":"$aasdesc#specificAssetIds[].externalSubjectId.keys[].value"}]},{"$eq":[{"$strVal":"BPN_A"},{"$field":"$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value"}]}]}`)
	match := true
	fragment := grammar.FragmentStringPattern("$aasdesc#submodelDescriptors[]")
	ctx := context.WithValue(context.Background(), filterKey, &QueryFilter{
		Filters: FragmentFilters{
			fragment: expr,
		},
		FilterMatch: FragmentMatchModes{
			fragment: match,
		},
	})

	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSMDesc)
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}
	collector.AllowInlineAliases(
		"submodel_descriptor",
		"aasdesc_submodel_descriptor_supplemental_semantic_id_reference",
		"aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key",
	)

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("submodel_descriptor").As("submodel_descriptor")).
		LeftJoin(
			goqu.T("submodel_descriptor_supplemental_semantic_id_reference").
				As("aasdesc_submodel_descriptor_supplemental_semantic_id_reference"),
			goqu.On(goqu.I("aasdesc_submodel_descriptor_supplemental_semantic_id_reference.descriptor_id").
				Eq(goqu.I("submodel_descriptor.descriptor_id"))),
		).
		LeftJoin(
			goqu.T("submodel_descriptor_supplemental_semantic_id_reference_key").
				As("aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key"),
			goqu.On(goqu.I("aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key.reference_id").
				Eq(goqu.I("aasdesc_submodel_descriptor_supplemental_semantic_id_reference.id"))),
		).
		Select(goqu.V(1)).
		Prepared(true)

	filteredDS, err := AddCorrelatedFilterQueryFromContext(ctx, ds, fragment, collector)
	if err != nil {
		t.Fatalf("AddCorrelatedFilterQueryFromContext returned error: %v", err)
	}
	sqlStr, _, err := filteredDS.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}

	if !strings.Contains(sqlStr, `"aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key"."value"`) {
		t.Fatalf("expected supplemental semantic ID predicate to remain row-local: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `EXISTS (`) ||
		!strings.Contains(sqlStr, `"external_subject_reference_key__exists"."value"`) {
		t.Fatalf("expected specific asset route guard in correlated EXISTS: %s", sqlStr)
	}
}
