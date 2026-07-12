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

import (
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestABACPolicyManagementValidateRequiresUpdateRight(t *testing.T) {
	t.Parallel()

	rights, ok := rightsForMappedRoute(http.MethodPost, "/security/abac/policy-versions/{versionID}/validate")
	if !ok {
		t.Fatal("expected validate management route to have an ABAC rights mapping")
	}
	if len(rights) != 1 || len(rights[0]) != 1 || rights[0][0] != grammar.RightsEnumUPDATE {
		t.Fatalf("expected validate management route to require UPDATE, got %v", rights)
	}
}

func TestVerificationEndpointRequiresExecuteRight(t *testing.T) {
	t.Parallel()

	rights, ok := rightsForMappedRoute(http.MethodPost, "/verify")
	if !ok {
		t.Fatal("expected verification endpoint to have an ABAC rights mapping")
	}
	if len(rights) != 1 || len(rights[0]) != 1 || rights[0][0] != grammar.RightsEnumEXECUTE {
		t.Fatalf("expected verification endpoint to require EXECUTE, got %v", rights)
	}
}

func rightsForMappedRoute(method string, pattern string) ([][]grammar.RightsEnum, bool) {
	var matches [][]grammar.RightsEnum
	for _, mapping := range mapMethodAndPatternToRightsData {
		if mapping.Method == method && mapping.Pattern == pattern {
			matches = append(matches, mapping.Rights)
		}
	}
	return matches, len(matches) > 0
}
