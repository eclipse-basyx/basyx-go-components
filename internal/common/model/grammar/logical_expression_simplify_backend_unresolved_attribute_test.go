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

import "testing"

func TestSimplifyForBackendFilter_UnresolvedAttributeStringComparison_FailsClosed(t *testing.T) {
	role := map[string]any{"CLAIM": "role"}
	viewRole := StandardString("view_digital_twin")
	le := LogicalExpression{
		Contains: []StringValue{
			{Attribute: role},
			{StrVal: &viewRole},
		},
	}

	simplified, decision := le.SimplifyForBackendFilter(func(AttributeValue) any { return nil })
	if decision != SimplifyFalse {
		t.Fatalf("expected SimplifyFalse, got %v", decision)
	}
	if simplified.Boolean == nil || *simplified.Boolean {
		t.Fatalf("expected boolean false, got %#v", simplified.Boolean)
	}
}

func TestSimplifyForBackendFilter_UnresolvedAttributeNumCast_FailsClosed(t *testing.T) {
	role := map[string]any{"CLAIM": "clear"}
	minClearance := float64(1)
	le := LogicalExpression{
		Ge: []Value{
			{NumCast: &Value{Attribute: role}},
			{NumVal: &minClearance},
		},
	}

	simplified, decision := le.SimplifyForBackendFilter(func(AttributeValue) any { return nil })
	if decision != SimplifyFalse {
		t.Fatalf("expected SimplifyFalse, got %v", decision)
	}
	if simplified.Boolean == nil || *simplified.Boolean {
		t.Fatalf("expected boolean false, got %#v", simplified.Boolean)
	}
}
