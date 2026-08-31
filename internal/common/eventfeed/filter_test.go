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

package eventfeed

import "testing"

func TestParseFilterOK(t *testing.T) {
	f, err := parseFilterParam("rsql:event.type==io.admin-shell.aas.created.v1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Comparisons) != 1 || f.Comparisons[0].Operator != "==" {
		t.Fatalf("unexpected %+v", f)
	}
}

func TestParseFilterIn(t *testing.T) {
	f, err := parseFilterParam("rsql:event.type=in=(io.admin-shell.aas.created.v1,io.admin-shell.aas.updated.v1)")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Comparisons[0].Values) != 2 {
		t.Fatalf("values=%v", f.Comparisons[0].Values)
	}
}

func TestParseFilterRejectsDataField(t *testing.T) {
	_, err := parseFilterParam("rsql:data.semanticId==x")
	if !IsQueryError(err) {
		t.Fatalf("expected query error, got %v", err)
	}
}

func TestParseFilterRejectsUnknownPrefix(t *testing.T) {
	_, err := parseFilterParam("jq:event.type==x")
	if !IsQueryError(err) {
		t.Fatalf("expected query error, got %v", err)
	}
}

func TestParseFilterAnd(t *testing.T) {
	f, err := parseFilterParam("rsql:event.type==a and event.subject==b")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Comparisons) != 2 {
		t.Fatalf("comparisons=%d", len(f.Comparisons))
	}
}
