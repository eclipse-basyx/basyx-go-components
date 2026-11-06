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

//nolint:all
package testenv

// IDShortPathEncodingCase is an idShortPath of IDShortPathEncodingSubmodel as
// a client sends it, with the idShort of the element it addresses.
type IDShortPathEncodingCase struct {
	Name    string
	Segment string
	IDShort string
}

// IDShortPathWithSlash addresses the element "a/b" of
// IDShortPathEncodingSubmodel, which stays unreachable through idShortPath.
const IDShortPathWithSlash = "Coll.a%2Fb"

// IDShortPathEncodingCases returns percent-encodings that Go does not produce
// itself, so the server receives them still encoded, next to an unencoded
// control.
func IDShortPathEncodingCases() []IDShortPathEncodingCase {
	return []IDShortPathEncodingCase{
		{Name: "unencoded semicolon", Segment: "Coll.a;b", IDShort: "a;b"},
		{Name: "encoded semicolon", Segment: "Coll.a%3Bb", IDShort: "a;b"},
		{Name: "encoded colon", Segment: "Coll.a%3Ab", IDShort: "a:b"},
		{Name: "over-encoded dot", Segment: "Coll%2Eplain", IDShort: "plain"},
		{Name: "encoded space next to encoded semicolon", Segment: "Coll.a%20b%3Bc", IDShort: "a b;c"},
	}
}

// IDShortPathEncodingSubmodel returns a Submodel whose collection Coll holds
// the elements addressed by IDShortPathEncodingCases.
func IDShortPathEncodingSubmodel(id string) map[string]any {
	children := make([]any, 0, 5)
	for _, idShort := range []string{"plain", "a;b", "a:b", "a/b", "a b;c"} {
		children = append(children, map[string]any{"modelType": "Property", "valueType": "xs:string", "idShort": idShort, "value": idShort})
	}
	return map[string]any{
		"modelType": "Submodel",
		"id":        id,
		"idShort":   "IDShortPathEncoding",
		"submodelElements": []any{
			map[string]any{"modelType": "SubmodelElementCollection", "idShort": "Coll", "value": children},
		},
	}
}
