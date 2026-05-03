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

package submodelpath

import (
	"errors"
	"testing"
)

func TestParseIDShortPathSegments(t *testing.T) {
	segments, err := ParseIDShortPathSegments("MechanicalParts.statements[0].Motor")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(segments) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(segments))
	}

	if segments[0].Value != "MechanicalParts" || segments[0].IsIndex {
		t.Fatalf("unexpected first segment: %+v", segments[0])
	}
	if segments[1].Value != "statements" || segments[1].IsIndex {
		t.Fatalf("unexpected second segment: %+v", segments[1])
	}
	if segments[2].Value != "0" || !segments[2].IsIndex {
		t.Fatalf("unexpected third segment: %+v", segments[2])
	}
	if segments[3].Value != "Motor" || segments[3].IsIndex {
		t.Fatalf("unexpected fourth segment: %+v", segments[3])
	}
}

func TestParseIDShortPathSegmentsErrors(t *testing.T) {
	_, err := ParseIDShortPathSegments("")
	if !errors.Is(err, ErrEmptyPath) {
		t.Fatalf("expected ErrEmptyPath, got %v", err)
	}

	_, err = ParseIDShortPathSegments("Collection[]")
	if !errors.Is(err, ErrEmptyListIndex) {
		t.Fatalf("expected ErrEmptyListIndex, got %v", err)
	}

	_, err = ParseIDShortPathSegments("Collection[")
	if !errors.Is(err, ErrInvalidSyntax) {
		t.Fatalf("expected ErrInvalidSyntax, got %v", err)
	}
}

func TestBuildIDShortPathFromSegments(t *testing.T) {
	path := BuildIDShortPathFromSegments([]Segment{
		{Value: "MechanicalParts"},
		{Value: "statements"},
		{Value: "0", IsIndex: true},
		{Value: "Motor"},
	})

	if path != "MechanicalParts.statements[0].Motor" {
		t.Fatalf("unexpected path: %s", path)
	}
}
