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
	"encoding/json"
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
	admin, editor := superpathTestTokens(t)
	prefix := fmt.Sprintf("urn:test:superpath:%d:", time.Now().UnixNano())
	id := prefix + "product"
	installEnvironmentRules(t, admin)
	requireEnvironmentUpload(t, admin, environmentFixture(id, "Product"), http.StatusOK)
	submodelRule := superpathFormulaRule("sm", map[string]any{"$regex": []any{map[string]string{"$field": "$sm#id"}, map[string]string{"$strVal": "^" + prefix}}}, "READ")
	openShells := environmentObjectRule("aas", "*", "READ")
	shellRoute := superpathRouteRule("/shells/*", map[string]any{"$regex": []any{map[string]string{"$field": "$aas#id"}, map[string]string{"$strVal": "^" + prefix}}})
	mixedRoute := superpathRouteRule("/shells/*", map[string]any{"$or": []any{superpathIDFormula("aas", id), superpathIDFormula("sm", id)}})

	for _, test := range []superpathShellCase{
		{"open shell and Submodel formula", []map[string]any{openShells, submodelRule}, http.StatusOK, http.StatusOK},
		{"shell formula and Submodel formula", []map[string]any{superpathIDRule("aas", id, "READ"), submodelRule}, http.StatusOK, http.StatusOK},
		{"shell formula and open Submodels", []map[string]any{superpathIDRule("aas", id, "READ"), environmentObjectRule("sm", "*", "READ")}, http.StatusOK, http.StatusOK},
		{"shell hidden by formula", []map[string]any{superpathIDRule("aas", "urn:other", "READ"), submodelRule}, http.StatusNotFound, http.StatusNotFound},
		{"no shell rule", []map[string]any{submodelRule}, http.StatusForbidden, http.StatusForbidden},
		{"Submodel hidden by formula", []map[string]any{openShells, superpathIDRule("sm", "urn:other", "READ")}, http.StatusNotFound, http.StatusOK},
		{"shell ROUTE formula and Submodel formula", []map[string]any{openShells, shellRoute, submodelRule}, http.StatusOK, http.StatusOK},
		{"shell ROUTE supplies the shell grant", []map[string]any{shellRoute, submodelRule}, http.StatusOK, http.StatusOK},
		{"shell ROUTE does not grant Submodel access", []map[string]any{openShells, shellRoute}, http.StatusForbidden, http.StatusOK},
		{"shell ROUTE does not widen hidden Submodel", []map[string]any{openShells, shellRoute, superpathIDRule("sm", "urn:other", "READ")}, http.StatusNotFound, http.StatusOK},
		{"shell excluded by ROUTE formula", []map[string]any{superpathRouteRule("/shells/*", superpathIDFormula("aas", "urn:other")), submodelRule}, http.StatusNotFound, http.StatusNotFound},
		{"mixed-resource route is no Submodel grant", []map[string]any{openShells, mixedRoute}, http.StatusForbidden, http.StatusOK},
		{"mixed-resource route preserves independent grants", []map[string]any{openShells, mixedRoute, submodelRule}, http.StatusOK, http.StatusOK},
		{"explicit Superpath ROUTE grant", []map[string]any{openShells, superpathRouteRule(superpathPath(id)+"*", map[string]bool{"$boolean": true})}, http.StatusOK, http.StatusOK},
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

	t.Run("shell ROUTE formula does not widen element masks", func(t *testing.T) {
		maskedSubmodels := superpathIDRule("sm", id, "READ")
		maskedSubmodels["FILTER"] = map[string]any{"FRAGMENT": "$sme.Secret#value", "CONDITION": map[string]bool{"$boolean": false}}
		installEnvironmentRules(t, admin, openShells, shellRoute, maskedSubmodels)
		status, body := runSuperpathRequest(t, http.MethodGet, superpathPath(id), "", "", editor)
		require.Equal(t, http.StatusOK, status, body)
		require.Contains(t, body, "public-value")
		require.NotContains(t, body, "secret-value")
	})

	for _, scope := range []string{"aas", "sm"} {
		t.Run("neutral ROUTE preserves "+scope+" mask", func(t *testing.T) {
			route := superpathRouteRule("/shells/*", map[string]bool{"$boolean": true})
			route["FILTER"] = map[string]any{"FRAGMENT": "$" + scope + "#idShort", "CONDITION": superpathIDFormula(scope, "urn:other")}
			installEnvironmentRules(t, admin, route)
			shellStatus, shellBody := runSuperpathRequest(t, http.MethodGet, "/shells/"+common.EncodeString(id), "", "", editor)
			require.Equal(t, http.StatusOK, shellStatus, shellBody)
			status, body := runSuperpathRequest(t, http.MethodGet, superpathPath(id), "", "", editor)
			require.Equal(t, http.StatusOK, status, body)
			if scope == "aas" {
				require.NotContains(t, shellBody, "Product")
				require.Contains(t, body, "Product")
			} else {
				require.Contains(t, shellBody, "Product")
				require.NotContains(t, body, "Product")
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

func TestDeleteIgnoresResponseMasks(t *testing.T) {
	admin, editor := superpathTestTokens(t)
	for _, test := range []struct {
		name, target, mask, denial string
	}{
		{name: "unmasked control", target: "superpath"},
		{name: "cross-resource mask", target: "superpath", mask: "foreign"},
		{name: "false mask", target: "superpath", mask: "false"},
		{name: "multiple masks", target: "superpath", mask: "list"},
		{name: "direct Submodel", target: "submodel", mask: "foreign"},
		{name: "direct element", target: "element", mask: "list"},
		{name: "superpath element", target: "superpath-element", mask: "foreign"},
		{name: "direct shell", target: "shell", mask: "foreign"},
		{name: "Submodel reference", target: "reference", mask: "list"},
		{name: "formula still required", target: "superpath", mask: "foreign", denial: "formula"},
		{name: "DELETE still required", target: "superpath", mask: "foreign", denial: "delete"},
		{name: "shell UPDATE still required", target: "superpath", mask: "foreign", denial: "shell"},
	} {
		t.Run(test.name, func(t *testing.T) {
			id := fmt.Sprintf("urn:test:delete-mask:%d", time.Now().UnixNano())
			installEnvironmentRules(t, admin)
			requireEnvironmentUpload(t, admin, environmentFixture(id, "Product"), http.StatusOK)
			installEnvironmentRules(t, admin, deleteResponseMaskRules(id, test.mask, test.denial)...)
			if test.mask == "false" {
				status, body := runSuperpathRequest(t, http.MethodGet, superpathPath(id), "", "", editor)
				require.Equal(t, http.StatusOK, status, body)
				require.NotContains(t, body, "Product")
			}
			status, body := runSuperpathRequest(t, http.MethodDelete, deleteResponseMaskPath(id, test.target), "", "", editor)
			if test.denial != "" {
				require.Equal(t, http.StatusForbidden, status, body)
				status, body = runSuperpathRequest(t, http.MethodGet, "/submodels/"+common.EncodeString(id), "", "", admin)
				require.Equal(t, http.StatusOK, status, body)
				return
			}
			require.Equal(t, http.StatusNoContent, status, body)
			require.Empty(t, body)
			assertResponseMaskDeletion(t, id, test.target, admin)
		})
	}
}

func TestSuperpathValuePatchIgnoresResponseMasks(t *testing.T) {
	admin, editor := superpathTestTokens(t)
	for _, test := range []struct {
		name         string
		maskShell    bool
		maskSubmodel bool
		denyUpdate   bool
	}{
		{name: "unmasked control"},
		{name: "cross-resource Submodel mask", maskSubmodel: true},
		{name: "cross-resource shell mask", maskShell: true},
		{name: "UPDATE still required", maskSubmodel: true, denyUpdate: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			id := fmt.Sprintf("urn:test:patch-mask:%d", time.Now().UnixNano())
			installEnvironmentRules(t, admin)
			requireEnvironmentUpload(t, admin, environmentFixture(id, "Product"), http.StatusOK)

			shell := superpathIDRule("aas", id, "READ")
			rights := []string{"READ", "UPDATE"}
			if test.denyUpdate {
				rights = []string{"READ"}
			}
			submodel := superpathIDRule("sm", id, rights...)
			if test.maskShell {
				addResponseMask(shell, "aas", "sm", id, "foreign")
			}
			if test.maskSubmodel {
				addResponseMask(submodel, "sm", "aas", id, "foreign")
			}
			installEnvironmentRules(t, admin, shell, submodel)

			status, body := runSuperpathRequest(t, http.MethodPatch, superpathPath(id)+"/$value", `{"Public":"updated-value"}`, "", editor)
			if test.denyUpdate {
				require.Equal(t, http.StatusForbidden, status, body)
			} else {
				require.Equal(t, http.StatusNoContent, status, body)
				require.Empty(t, body)
			}

			status, body = runSuperpathRequest(t, http.MethodGet, "/submodels/"+common.EncodeString(id)+"/$value", "", "", admin)
			require.Equal(t, http.StatusOK, status, body)
			if test.denyUpdate {
				require.Contains(t, body, "public-value")
			} else {
				require.Contains(t, body, "updated-value")
			}
		})
	}
}

func superpathTestTokens(t *testing.T) (string, string) {
	t.Helper()
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
	return admin, editor
}

func deleteResponseMaskRules(id, mask, denial string) []map[string]any {
	shellRights := []string{"READ", "UPDATE", "DELETE"}
	submodelRights := []string{"READ", "DELETE"}
	if denial == "shell" {
		shellRights = []string{"READ"}
	}
	if denial == "delete" {
		submodelRights = []string{"READ"}
	}
	shell := superpathIDRule("aas", id, shellRights...)
	submodel := superpathIDRule("sm", id, submodelRights...)
	if denial == "formula" {
		submodel["FORMULA"] = map[string]bool{"$boolean": false}
	}
	addResponseMask(shell, "aas", "sm", id, mask)
	addResponseMask(submodel, "sm", "aas", id, mask)
	return []map[string]any{shell, submodel}
}

func addResponseMask(rule map[string]any, scope, other, id, mask string) {
	filter := map[string]any{"FRAGMENT": "$" + scope + "#idShort", "CONDITION": superpathIDFormula(other, id)}
	switch mask {
	case "foreign":
		rule["FILTER"] = filter
	case "false":
		filter["CONDITION"] = map[string]bool{"$boolean": false}
		rule["FILTER"] = filter
	case "list":
		rule["FILTERLIST"] = []any{filter, map[string]any{
			"FRAGMENT": "$sme.Public#value", "CONDITION": map[string]bool{"$boolean": false},
		}}
	}
}

func deleteResponseMaskPath(id, target string) string {
	encoded := common.EncodeString(id)
	switch target {
	case "superpath":
		return superpathPath(id)
	case "superpath-element":
		return superpathPath(id) + "/submodel-elements/Public"
	case "element":
		return "/submodels/" + encoded + "/submodel-elements/Public"
	case "shell":
		return "/shells/" + encoded
	case "reference":
		return "/shells/" + encoded + "/submodel-refs/" + encoded
	default:
		return "/submodels/" + encoded
	}
}

func assertResponseMaskDeletion(t *testing.T, id, target, admin string) {
	t.Helper()
	if target == "reference" {
		status, body := runSuperpathRequest(t, http.MethodGet, "/shells/"+common.EncodeString(id), "", "", admin)
		require.Equal(t, http.StatusOK, status, body)
		var shell map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &shell))
		require.Empty(t, shell["submodels"])
		return
	}
	if target == "superpath" {
		target = "submodel"
	}
	status, body := runSuperpathRequest(t, http.MethodGet, deleteResponseMaskPath(id, target), "", "", admin)
	require.Equal(t, http.StatusNotFound, status, body)
}

func superpathPath(id string) string {
	return "/shells/" + common.EncodeString(id) + "/submodels/" + common.EncodeString(id)
}

func superpathIDRule(scope, id string, rights ...string) map[string]any {
	return superpathFormulaRule(scope, superpathIDFormula(scope, id), rights...)
}

func superpathIDFormula(scope, id string) map[string]any {
	return map[string]any{"$eq": []any{map[string]string{"$field": "$" + scope + "#id"}, map[string]string{"$strVal": id}}}
}

func superpathRouteRule(route string, formula any) map[string]any {
	rule := environmentObjectRule("aas", "*", "READ")
	rule["OBJECTS"] = []any{map[string]string{"ROUTE": route}}
	rule["FORMULA"] = formula
	return rule
}

func superpathFormulaRule(scope string, formula map[string]any, rights ...string) map[string]any {
	rule := environmentObjectRule(scope, "*", rights...)
	rule["FORMULA"] = formula
	return rule
}
