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
// Author: Martin Stemmer ( Fraunhofer IESE )

package grammar

import (
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
)

func TestLogicalExpression_SMDesc_WithCollector_BuildsCTE(t *testing.T) {
	expr := LogicalExpression{
		And: []LogicalExpression{
			{
				Eq: ComparisonItems{
					field("$smdesc#idShort"),
					strVal("sub-short"),
				},
			},
			{
				Eq: ComparisonItems{
					field("$smdesc#semanticId.keys[0].value"),
					strVal("urn:sm"),
				},
			},
		},
	}

	collector, err := NewResolvedFieldPathCollectorForRoot(CollectorRootSMDesc)
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}

	whereExpr, _, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).
		InnerJoin(
			goqu.T("aas_descriptor").As("aas_descriptor"),
			goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))),
		).
		LeftJoin(
			goqu.T("submodel_descriptor").As("submodel_descriptor"),
			goqu.On(goqu.I("submodel_descriptor.aas_descriptor_id").Eq(goqu.I("aas_descriptor.descriptor_id"))),
		).
		Select(goqu.V(1)).
		Where(whereExpr)

	sql, _, err := ds.Prepared(true).ToSQL()

	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	t.Logf("SQL: %s", sql)

	if !strings.Contains(sql, "'sub-short'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'sub-short'", sql)
	}
	if !strings.Contains(sql, "'urn:sm'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'urn:sm'", sql)
	}
	if !strings.Contains(sql, "position\" = 0") {
		t.Fatalf("expected SQL to contain position binding 0, got: %s", sql)
	}
}

func TestLogicalExpression_SMDesc_SupplementalSemanticIdsWithCollector_BuildsCTE(t *testing.T) {
	testCases := []struct {
		name      string
		root      CollectorRoot
		shorthand string
		explicit  string
	}{
		{
			name:      "aasdesc wildcard shorthand",
			root:      CollectorRootAASDesc,
			shorthand: "$aasdesc#submodelDescriptors[].supplementalSemanticIds",
			explicit:  "$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[0].value",
		},
		{
			name:      "smdesc wildcard shorthand",
			root:      CollectorRootSMDesc,
			shorthand: "$smdesc#supplementalSemanticIds",
			explicit:  "$smdesc#supplementalSemanticIds[].keys[0].value",
		},
		{
			name:      "aasdesc indexed shorthand",
			root:      CollectorRootAASDesc,
			shorthand: "$aasdesc#submodelDescriptors[2].supplementalSemanticIds[1]",
			explicit:  "$aasdesc#submodelDescriptors[2].supplementalSemanticIds[1].keys[0].value",
		},
		{
			name:      "smdesc indexed shorthand",
			root:      CollectorRootSMDesc,
			shorthand: "$smdesc#supplementalSemanticIds[1]",
			explicit:  "$smdesc#supplementalSemanticIds[1].keys[0].value",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			shorthandSQL := supplementalSemanticIDSQL(t, testCase.root, testCase.shorthand)
			explicitSQL := supplementalSemanticIDSQL(t, testCase.root, testCase.explicit)
			if shorthandSQL != explicitSQL {
				t.Fatalf("expected shorthand and explicit paths to generate identical SQL:\nshorthand: %s\nexplicit:  %s", shorthandSQL, explicitSQL)
			}
		})
	}

	aasdescResolved := resolvedSupplementalSemanticIDPath(t, "$aasdesc#submodelDescriptors[].supplementalSemanticIds")
	smdescResolved := resolvedSupplementalSemanticIDPath(t, "$smdesc#supplementalSemanticIds")
	if !resolvedSlicesEqual([]ResolvedFieldPath{aasdescResolved}, []ResolvedFieldPath{smdescResolved}) {
		t.Fatalf("expected AAS descriptor and submodel descriptor shorthand to resolve equally:\naasdesc: %#v\nsmdesc:  %#v", aasdescResolved, smdescResolved)
	}

	sql := supplementalSemanticIDSQL(t, CollectorRootSMDesc, "$smdesc#supplementalSemanticIds[].keys[].value")

	if !strings.Contains(sql, "submodel_descriptor_supplemental_semantic_id_reference") {
		t.Fatalf("expected supplemental semantic ID reference join, got: %s", sql)
	}
	if !strings.Contains(sql, "submodel_descriptor_supplemental_semantic_id_reference_key") {
		t.Fatalf("expected supplemental semantic ID reference key join, got: %s", sql)
	}
	if !strings.Contains(sql, "'urn:supplemental'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'urn:supplemental'", sql)
	}
	if strings.Contains(sql, ".position\" =") {
		t.Fatalf("did not expect wildcard supplemental references to contain position bindings, got: %s", sql)
	}

	aliases := buildPostgresExistsAliases([]string{
		"aasdesc_submodel_descriptor_supplemental_semantic_id_reference",
		"aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key",
	})
	referenceAlias := aliases["aasdesc_submodel_descriptor_supplemental_semantic_id_reference"]
	keyAlias := aliases["aasdesc_submodel_descriptor_supplemental_semantic_id_reference_key"]
	if referenceAlias == keyAlias {
		t.Fatalf("expected distinct PostgreSQL aliases, got %q", referenceAlias)
	}
	if len(referenceAlias) > postgresIdentifierMaxBytes || len(keyAlias) > postgresIdentifierMaxBytes {
		t.Fatalf("expected aliases within PostgreSQL's identifier limit, got %q and %q", referenceAlias, keyAlias)
	}
	if !strings.Contains(sql, `AS "`+referenceAlias+`"`) || !strings.Contains(sql, `AS "`+keyAlias+`"`) {
		t.Fatalf("expected SQL to use collision-safe aliases %q and %q, got: %s", referenceAlias, keyAlias, sql)
	}

	aasdescSQL := supplementalSemanticIDSQL(t, CollectorRootAASDesc, "$aasdesc#submodelDescriptors[].supplementalSemanticIds[].keys[].value")
	if !strings.Contains(aasdescSQL, `FROM "submodel_descriptor" AS "submodel_descriptor"`) {
		t.Fatalf("expected AAS descriptor supplemental semantic ID SQL to use submodel_descriptor as EXISTS base, got: %s", aasdescSQL)
	}
	if strings.Contains(aasdescSQL, `FROM "aas_descriptor" AS "aas_descriptor"`) || strings.Contains(aasdescSQL, `JOIN "aas_descriptor" AS "aas_descriptor"`) {
		t.Fatalf("did not expect redundant aas_descriptor root join in supplemental semantic ID SQL, got: %s", aasdescSQL)
	}
}

func supplementalSemanticIDSQL(t *testing.T, root CollectorRoot, fieldPath string) string {
	t.Helper()

	expr := LogicalExpression{
		Eq: ComparisonItems{
			field(fieldPath),
			strVal("urn:supplemental"),
		},
	}
	collector, err := NewResolvedFieldPathCollectorForRoot(root)
	if err != nil {
		t.Fatalf("NewResolvedFieldPathCollectorForRoot returned error: %v", err)
	}
	whereExpr, _, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error for %q: %v", fieldPath, err)
	}

	d := goqu.Dialect("postgres")
	var ds *goqu.SelectDataset
	if root == CollectorRootAASDesc {
		ds = d.From(goqu.T("descriptor").As("descriptor"))
	} else {
		ds = d.From(goqu.T("submodel_descriptor"))
	}
	sql, _, err := ds.Select(goqu.V(1)).Where(whereExpr).ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error for %q: %v", fieldPath, err)
	}
	return sql
}

func resolvedSupplementalSemanticIDPath(t *testing.T, fieldPath string) ResolvedFieldPath {
	t.Helper()

	fieldIdentifier := ModelStringPattern(fieldPath)
	operand := Value{Field: &fieldIdentifier}
	normalizeSemanticShorthand(&operand)
	resolved, err := ResolveScalarFieldToSQL(operand.Field)
	if err != nil {
		t.Fatalf("ResolveScalarFieldToSQL returned error for %q: %v", fieldPath, err)
	}
	return resolved
}
