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

package common

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseRoutedIDShortPathDecodesOnceAndValidatesGrammar(t *testing.T) {
	t.Parallel()

	accepted := []struct{ target, routed, want string }{
		{target: "/elements/Coll.Prop", routed: "Coll.Prop", want: "Coll.Prop"},
		{target: "/elements/X", routed: "X", want: "X"},
		{target: "/elements/List%5B0%5D.Prop", routed: "List[0].Prop", want: "List[0].Prop"},
		{target: "/elements/List%5b0%5d.Prop", routed: "List%5b0%5d.Prop", want: "List[0].Prop"},
		{target: "/elements/Coll.%41", routed: "Coll.%41", want: "Coll.A"},
		{target: "/elements/Matrix[0][1]", routed: "Matrix[0][1]", want: "Matrix[0][1]"},
	}
	for _, testCase := range accepted {
		got, err := ParseRoutedIDShortPath(httptest.NewRequest(http.MethodGet, testCase.target, nil), testCase.routed)
		if err != nil || got != testCase.want {
			t.Fatalf("ParseRoutedIDShortPath(%q) = %q, %v; want %q", testCase.target, got, err, testCase.want)
		}
	}

	rejected := []struct{ target, routed string }{
		{target: "/elements/%2541", routed: "%41"},
		{target: "/elements/Coll.x%2Fattachment", routed: "Coll.x%2Fattachment"},
		{target: "/elements/Coll.x%252Fattachment", routed: "Coll.x%252Fattachment"},
		{target: "/elements/%25", routed: "%25"},
		{target: "/elements/Coll%20Prop", routed: "Coll Prop"},
		{target: "/elements/%20Coll", routed: " Coll"},
		{target: "/elements/1Coll", routed: "1Coll"},
		{target: "/elements/Coll-", routed: "Coll-"},
		{target: "/elements/Coll..Prop", routed: "Coll..Prop"},
		{target: "/elements/List%5B%5D", routed: "List[]"},
		{target: "/elements/List%5B01%5D", routed: "List[01]"},
		{target: "/elements/%C3%A4rger", routed: "\u00e4rger"},
	}
	for _, testCase := range rejected {
		if got, err := ParseRoutedIDShortPath(httptest.NewRequest(http.MethodGet, testCase.target, nil), testCase.routed); err == nil {
			t.Fatalf("ParseRoutedIDShortPath(%q) = %q, want an error", testCase.target, got)
		}
	}
}

func TestParseRoutedIDShortPathRejectsInvalidPercentEncoding(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/elements/Coll.%2541", nil)
	request.URL.RawPath = "/elements/Coll.%zz"
	if _, err := ParseRoutedIDShortPath(request, "Coll.%zz"); err == nil {
		t.Fatal("an invalid percent-encoding must be rejected")
	}
}
