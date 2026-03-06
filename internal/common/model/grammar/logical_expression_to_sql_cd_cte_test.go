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

func TestLogicalExpression_CD_WithCollector_BuildsCTE(t *testing.T) {
	expr := LogicalExpression{
		And: []LogicalExpression{
			{
				Eq: ComparisonItems{
					field("$cd#idShort"),
					strVal("ViewerAllowed"),
				},
			},
			{
				Eq: ComparisonItems{
					field("$cd#id"),
					strVal("urn:example:cd:viewer:allowed"),
				},
			},
		},
	}

	collector := mustCollectorForRoot(t, "$cd")
	whereExpr, _, err := expr.EvaluateToExpression(collector)
	if err != nil {
		t.Fatalf("EvaluateToExpression returned error: %v", err)
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("concept_description").As("concept_description")).Select(goqu.V(1)).Where(whereExpr)
	sql, _, err := ds.ToSQL()
	if err != nil {
		t.Fatalf("ToSQL returned error: %v", err)
	}
	t.Logf("SQL: %s", sql)

	if !strings.Contains(sql, "EXISTS") {
		t.Fatalf("expected EXISTS in SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "concept_description__exists") {
		t.Fatalf("expected rewritten inner alias for collision handling, got: %s", sql)
	}
	if !strings.Contains(sql, "'ViewerAllowed'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'ViewerAllowed'", sql)
	}
	if !strings.Contains(sql, "'urn:example:cd:viewer:allowed'") {
		t.Fatalf("expected SQL to contain %q, got: %s", "'urn:example:cd:viewer:allowed'", sql)
	}
}
