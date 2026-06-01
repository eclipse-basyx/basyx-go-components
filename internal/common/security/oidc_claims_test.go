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
*******************************************************************************/

package auth

import (
	"reflect"
	"testing"
)

func TestNormalizeVerifiedClaims_DefaultScopes(t *testing.T) {
	t.Parallel()

	claims := Claims{
		"scope": "read write",
		"scp":   []any{"write", "admin"},
	}
	if err := normalizeVerifiedClaims(claims, defaultScopeClaimPointers, nil); err != nil {
		t.Fatalf("normalizeVerifiedClaims() error = %v", err)
	}

	want := []string{"read", "write", "admin"}
	if got := claims[canonicalScopesClaim]; !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", canonicalScopesClaim, got, want)
	}
	if claims["scope"] != "read write" {
		t.Fatalf("raw scope claim changed: %#v", claims["scope"])
	}
}

func TestNormalizeVerifiedClaims_CustomScopeArray(t *testing.T) {
	t.Parallel()

	claims := Claims{"permissions": []any{"read", "write"}}
	if err := normalizeVerifiedClaims(claims, []string{"/permissions"}, nil); err != nil {
		t.Fatalf("normalizeVerifiedClaims() error = %v", err)
	}

	want := []string{"read", "write"}
	if got := claims[canonicalScopesClaim]; !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", canonicalScopesClaim, got, want)
	}
}

func TestNormalizeVerifiedClaims_MappingsUseJSONPointers(t *testing.T) {
	t.Parallel()

	mappings, err := normalizeClaimMappings([]OIDCClaimMappingSettings{
		{Target: "roles", Mode: "list", Sources: []string{"/roles", "/realm_access/roles"}},
		{Target: "clear", Mode: "scalar", Sources: []string{"/extension_clearance"}},
	})
	if err != nil {
		t.Fatalf("normalizeClaimMappings() error = %v", err)
	}

	claims := Claims{
		"roles":               []any{"viewer", "admin"},
		"realm_access":        map[string]any{"roles": []any{"admin", "editor"}},
		"extension_clearance": []any{"high"},
	}
	if err := normalizeVerifiedClaims(claims, defaultScopeClaimPointers, mappings); err != nil {
		t.Fatalf("normalizeVerifiedClaims() error = %v", err)
	}

	wantRoles := []string{"viewer", "admin", "editor"}
	if got := claims["basyx.roles"]; !reflect.DeepEqual(got, wantRoles) {
		t.Fatalf("basyx.roles = %#v, want %#v", got, wantRoles)
	}
	if got := claims["basyx.clear"]; got != "high" {
		t.Fatalf("basyx.clear = %#v, want high", got)
	}
}

func TestNormalizeVerifiedClaims_RejectsReservedNamespaceCollision(t *testing.T) {
	t.Parallel()

	claims := Claims{"basyx.roles": []any{"admin"}}
	if err := normalizeVerifiedClaims(claims, defaultScopeClaimPointers, nil); err == nil {
		t.Fatalf("expected reserved namespace collision error")
	}
}

func TestNormalizeVerifiedClaims_RejectsInvalidMappedShapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		mapping OIDCClaimMappingSettings
		claims  Claims
	}{
		{
			name:    "list contains non-string",
			mapping: OIDCClaimMappingSettings{Target: "roles", Mode: "list", Sources: []string{"/roles"}},
			claims:  Claims{"roles": []any{"admin", 2}},
		},
		{
			name:    "scalar contains multiple items",
			mapping: OIDCClaimMappingSettings{Target: "clear", Mode: "scalar", Sources: []string{"/clear"}},
			claims:  Claims{"clear": []any{"high", "low"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mappings, err := normalizeClaimMappings([]OIDCClaimMappingSettings{testCase.mapping})
			if err != nil {
				t.Fatalf("normalizeClaimMappings() error = %v", err)
			}
			if err := normalizeVerifiedClaims(testCase.claims, defaultScopeClaimPointers, mappings); err == nil {
				t.Fatalf("expected invalid mapped token shape error")
			}
		})
	}
}

func TestJSONPointerValue_DecodesNestedEscapes(t *testing.T) {
	t.Parallel()

	value, found, err := jsonPointerValue(Claims{
		"nested": map[string]any{"a/b~c": []any{"first", "second"}},
	}, "/nested/a~1b~0c/1")
	if err != nil {
		t.Fatalf("jsonPointerValue() error = %v", err)
	}
	if !found || value != "second" {
		t.Fatalf("jsonPointerValue() = %#v, %v, want second, true", value, found)
	}
}

func TestJSONPointerValue_AcceptsRootPointer(t *testing.T) {
	t.Parallel()

	claims := Claims{"scope": "read"}
	value, found, err := jsonPointerValue(claims, "")
	if err != nil {
		t.Fatalf("jsonPointerValue() error = %v", err)
	}
	if !found || !reflect.DeepEqual(value, claims) {
		t.Fatalf("jsonPointerValue() = %#v, %v, want original claims, true", value, found)
	}
}
