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

package grammar

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestQueryComplexityLimits(t *testing.T) {
	shallow := `{"$boolean":true}`
	deep := strings.Repeat(`{"$not":`, 65) + shallow + strings.Repeat(`}`, 65)
	wide := `{"$and":[` + strings.TrimSuffix(strings.Repeat(shallow+",", 2200), ",") + `]}`
	filters := `{"$filters":[` + strings.TrimSuffix(strings.Repeat(`{"$fragment":"$aas#idShort","$condition":`+shallow+`},`, 1000), ",") + `]}`
	for name, raw := range map[string]string{"deep": `{"$condition":` + deep + `}`, "wide": `{"$condition":` + wide + `}`, "filters": filters} {
		t.Run(name, func(t *testing.T) {
			var q Query
			if err := json.Unmarshal([]byte(raw), &q); err == nil || !strings.Contains(err.Error(), "GRAMMAR-JSON-COMPLEXITY") {
				t.Fatalf("expected complexity rejection, got %v", err)
			}
		})
	}
	var condition LogicalExpression
	if err := json.Unmarshal([]byte(deep), &condition); err == nil || !strings.Contains(err.Error(), "GRAMMAR-JSON-COMPLEXITY") {
		t.Fatalf("policy condition did not enforce limits: %v", err)
	}
	for _, depth := range []int{1, 32, 62} {
		raw := `{"$condition":` + strings.Repeat(`{"$not":`, depth) + shallow + strings.Repeat(`}`, depth) + `}`
		var q Query
		if err := json.Unmarshal([]byte(raw), &q); err != nil {
			t.Fatalf("bounded query depth %d rejected: %v", depth, err)
		}
	}
}

func TestJSONComplexityBoundariesAndStringContents(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		allowed bool
	}{
		{strings.Repeat("[", 64) + "0" + strings.Repeat("]", 64), true},
		{strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65), false},
		{"[" + strings.TrimSuffix(strings.Repeat("0,", 8190), ",") + "]", true},
		{"[" + strings.TrimSuffix(strings.Repeat("0,", 8191), ",") + "]", false},
		{`"` + strings.Repeat(`[]{}\"`, 100) + `"`, true},
	} {
		err := validateExpressionJSONComplexity([]byte(tc.raw))
		if (err == nil) != tc.allowed {
			t.Fatalf("unexpected boundary result: %v (allowed=%t)", err, tc.allowed)
		}
	}
}

func TestExternalLogicalExpressionStillRejectsSingletonAnd(t *testing.T) {
	var expression LogicalExpression
	err := json.Unmarshal([]byte(`{"$and":[{"$boolean":true}]}`), &expression)
	if err == nil || !strings.Contains(err.Error(), "field $and length") {
		t.Fatalf("external expression bypassed logical operator validation: %v", err)
	}
}
