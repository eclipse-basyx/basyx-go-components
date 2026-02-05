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
