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

package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

type superpathShellCase struct {
	name             string
	rules            []map[string]any
	superpathStatus  int
	directShellCheck int
}

func TestSuperpathAuthorizesShellAndSubmodelSeparately(t *testing.T) {
	provider := testenv.NewPasswordGrantTokenProvider(testKeycloakTokenURL, "basyx-ui", 10*time.Second)
	admin, err := provider.GetAccessToken(&testenv.TokenCredentials{User: "admin", Password: "pwd"})
	require.NoError(t, err)
	editor, err := provider.GetAccessToken(&testenv.TokenCredentials{User: "userx", Password: "pwd"})
	require.NoError(t, err)
	original := activePolicyVersionID(t, admin)
	t.Cleanup(func() {
		restored := clonePolicyVersion(t, original, admin)
		validatePolicyVersion(t, restored, admin)
		activatePolicyVersion(t, restored, admin)
	})
	prefix := fmt.Sprintf("urn:test:superpath:%d:", time.Now().UnixNano())
	id := prefix + "product"
	installEnvironmentRules(t, admin)
	requireEnvironmentUpload(t, admin, environmentFixture(id, "Product"), http.StatusOK)
	submodelRule := superpathFormulaRule("sm", map[string]any{"$regex": []any{map[string]string{"$field": "$sm#id"}, map[string]string{"$strVal": "^" + prefix}}}, "READ")
	openShells := environmentObjectRule("aas", "*", "READ")

	for _, test := range []superpathShellCase{
		{"open shell and Submodel formula", []map[string]any{openShells, submodelRule}, http.StatusOK, http.StatusOK},
		{"shell formula and Submodel formula", []map[string]any{superpathIDRule("aas", id, "READ"), submodelRule}, http.StatusOK, http.StatusOK},
		{"shell formula and open Submodels", []map[string]any{superpathIDRule("aas", id, "READ"), environmentObjectRule("sm", "*", "READ")}, http.StatusOK, http.StatusOK},
		{"shell hidden by formula", []map[string]any{superpathIDRule("aas", "urn:other", "READ"), submodelRule}, http.StatusNotFound, http.StatusNotFound},
		{"no shell rule", []map[string]any{submodelRule}, http.StatusForbidden, http.StatusForbidden},
		{"Submodel hidden by formula", []map[string]any{openShells, superpathIDRule("sm", "urn:other", "READ")}, http.StatusNotFound, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			installEnvironmentRules(t, admin, test.rules...)
			status, body := runSuperpathRequest(t, http.MethodGet, "/shells/"+common.EncodeString(id), "", "", editor)
			require.Equal(t, test.directShellCheck, status, body)
			for _, path := range []string{superpathPath(id), superpathPath(id) + "/submodel-elements/Public"} {
				status, body = runSuperpathRequest(t, http.MethodGet, path, "", "", editor)
				require.Equal(t, test.superpathStatus, status, "GET %s: %s", path, body)
			}
		})
	}

	t.Run("PUT and DELETE require UPDATE on the shell", func(t *testing.T) {
		writeSubmodels := superpathFormulaRule("sm", map[string]any{"$regex": []any{map[string]string{"$field": "$sm#id"}, map[string]string{"$strVal": "^" + prefix}}}, "READ", "UPDATE", "DELETE")
		body := fmt.Sprintf(`{"modelType":"Submodel","id":%q,"idShort":"Renamed"}`, id)

		installEnvironmentRules(t, admin, openShells, writeSubmodels)
		status, response := runSuperpathRequest(t, http.MethodPut, superpathPath(id), body, "", editor)
		require.Equal(t, http.StatusForbidden, status, response)
		status, response = runSuperpathRequest(t, http.MethodDelete, superpathPath(id), "", "", editor)
		require.Equal(t, http.StatusForbidden, status, response)

		installEnvironmentRules(t, admin, environmentObjectRule("aas", "*", "READ", "UPDATE"), writeSubmodels)
		status, response = runSuperpathRequest(t, http.MethodPut, superpathPath(id), body, "", editor)
		require.Equal(t, http.StatusNoContent, status, response)
		status, response = runSuperpathRequest(t, http.MethodDelete, superpathPath(id), "", "", editor)
		require.Equal(t, http.StatusNoContent, status, response)
		status, response = runSuperpathRequest(t, http.MethodGet, "/submodels/"+common.EncodeString(id), "", "", admin)
		require.Equal(t, http.StatusNotFound, status, response)
	})
}

func superpathPath(id string) string {
	return "/shells/" + common.EncodeString(id) + "/submodels/" + common.EncodeString(id)
}

func superpathIDRule(scope, id string, rights ...string) map[string]any {
	return superpathFormulaRule(scope, map[string]any{"$eq": []any{map[string]string{"$field": "$" + scope + "#id"}, map[string]string{"$strVal": id}}}, rights...)
}

func superpathFormulaRule(scope string, formula map[string]any, rights ...string) map[string]any {
	rule := environmentObjectRule(scope, "*", rights...)
	rule["FORMULA"] = formula
	return rule
}
