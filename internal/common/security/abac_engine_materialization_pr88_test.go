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

import "testing"

func TestParseAccessModelCombinesInlineAndReferencedAttributesAndObjects(t *testing.T) {
	t.Parallel()

	const policy = `{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [
				{ "name": "role_attr", "attributes": [ { "CLAIM": "role" } ] },
				{ "name": "combined_attr", "attributes": [ { "CLAIM": "tenant" } ], "USEATTRIBUTES": [ "role_attr" ] }
			],
			"DEFOBJECTS": [
				{ "name": "all_descriptors", "objects": [ { "DESCRIPTOR": "$aasdesc(\"*\")" } ] },
				{ "name": "descriptor_route", "objects": [ { "ROUTE": "/shell-descriptors" } ] },
				{ "name": "combined_objects", "USEOBJECTS": [ "descriptor_route", "all_descriptors" ] }
			],
			"rules": [
				{
					"ACL": {
						"ATTRIBUTES": [ { "GLOBAL": "ANONYMOUS" } ],
						"USEATTRIBUTES": "combined_attr",
						"RIGHTS": [ "READ" ],
						"ACCESS": "ALLOW"
					},
					"OBJECTS": [ { "ROUTE": "/submodels" } ],
					"USEOBJECTS": [ "combined_objects" ],
					"FORMULA": { "$boolean": true }
				}
			]
		}
	}`

	model, err := ParseAccessModel([]byte(policy), nil, "")
	if err != nil {
		t.Fatalf("ParseAccessModel returned error: %v", err)
	}
	if len(model.rules) != 1 {
		t.Fatalf("expected one materialized rule, got %d", len(model.rules))
	}

	rule := model.rules[0]
	if len(rule.attrs) != 3 {
		t.Fatalf("expected inline and referenced attributes to be combined, got %d", len(rule.attrs))
	}
	if len(rule.objs) != 3 {
		t.Fatalf("expected inline and referenced objects to be combined, got %d", len(rule.objs))
	}
}
