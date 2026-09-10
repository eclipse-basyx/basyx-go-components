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
	"encoding/json"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestResourceBoundDocumentValidation(t *testing.T) {
	valid := `{"ResourceBoundAccessRuleModels":[{"RESOURCE":{"IDENTIFIABLE":"$sm(\"bridge\")"},"rules":[]}]}`
	models, err := ParseResourceBoundDocument([]byte(valid))
	require.NoError(t, err)
	require.Len(t, models, 1)
	cases := []string{
		`{}`, `{"ResourceBoundAccessRuleModels":[]}`,
		`{"ResourceBoundAccessRuleModels":null}`,
		`{"AllAccessPermissionRules":{"rules":[]},"ResourceBoundAccessRuleModels":[{"RESOURCE":{"ROUTE":"/shells"},"rules":[]}]}`,
		`{"ResourceBoundAccessRuleModels":[{"RESOURCE":{"ROUTE":"/shells"}}]}`,
		`{"ResourceBoundAccessRuleModels":[{"RESOURCE":{"ROUTE":"/shells"},"rules":null}]}`,
		`{"ResourceBoundAccessRuleModels":[{"RESOURCE":{"ROUTE":"/shells"},"rules":[],"DEFACLS":null}]}`,
		`{"ResourceBoundAccessRuleModels":[{"RESOURCE":{"ROUTE":"/shells"},"rules":[],"DEFOBJECTS":[]}]}`,
		`{"ResourceBoundAccessRuleModels":[{"RESOURCE":{"ROUTE":"/shells"},"rules":[]},{"RESOURCE":{"ROUTE":"/shells"},"rules":[]}]}`,
		`{"ResourceBoundAccessRuleModels":[{"RESOURCE":{"IDENTIFIABLE":"$sm(\"*\")"},"rules":[]}]}`,
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) { _, err := ParseResourceBoundDocument([]byte(input)); require.Error(t, err) })
	}
	_, err = ParseResourceBoundDocument([]byte(valid + `{}`))
	require.ErrorContains(t, err, "COMMON-JSON-TRAILING")
}

func TestResourceBoundRulesRejectObjectBindingsAndMissingAlternatives(t *testing.T) {
	rule, err := boundPrincipalRule(common.AccessPrincipal{Issuer: "issuer", Subject: "subject"}, []grammar.RightsEnum{grammar.RightsEnumREAD})
	require.NoError(t, err)
	for _, field := range []string{"OBJECTS", "USEOBJECTS", "unexpected"} {
		var input map[string]any
		require.NoError(t, json.Unmarshal(rule, &input))
		input[field] = []any{}
		data, err := json.Marshal(input)
		require.NoError(t, err)
		_, err = compileBoundRule(data)
		require.Error(t, err)
	}
	for _, field := range []string{"ACL", "FORMULA"} {
		var input map[string]any
		require.NoError(t, json.Unmarshal(rule, &input))
		delete(input, field)
		data, err := json.Marshal(input)
		require.NoError(t, err)
		_, err = compileBoundRule(data)
		require.Error(t, err)
	}
}

func TestResourceBoundGrantBindsIssuerAndSubject(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/submodels/{submodelIdentifier}", func(http.ResponseWriter, *http.Request) {})
	rule, err := boundPrincipalRule(common.AccessPrincipal{Issuer: "issuer", Subject: "user"}, []grammar.RightsEnum{grammar.RightsEnumREAD})
	require.NoError(t, err)
	target := boundTarget{Kind: "submodel", Submodel: "bridge"}
	model, err := CompileResourceBoundPolicy(ResourceBoundPolicy{Resource: target.object(), Rules: []json.RawMessage{rule}}, router, "")
	require.NoError(t, err)
	for _, test := range []struct {
		issuer, subject string
		allowed         bool
	}{{"issuer", "user", true}, {"other", "user", false}, {"issuer", "other", false}} {
		allowed, _, _ := model.AuthorizeWithFilter(EvalInput{Method: "GET", Path: "/submodels/YnJpZGdl", Claims: Claims{"iss": test.issuer, "sub": test.subject}})
		require.Equal(t, test.allowed, allowed)
	}
}

func TestResourceBoundAliasesShareBinding(t *testing.T) {
	direct, err := parseBoundTarget("/api/submodels/c20/submodel-elements/a.b[0]/$access/grants", "/api")
	require.NoError(t, err)
	nested, err := parseBoundTarget("/api/shells/YWFz/submodels/c20/submodel-elements/a.b[0]/$access/grants", "/api")
	require.NoError(t, err)
	require.True(t, boundObjectsEqual(direct.object(), nested.object()))
	require.Equal(t, "aas", nested.AAS)
	require.Equal(t, "grants", nested.Suffix)
}

func TestResourceBoundDefinitionsAreLocal(t *testing.T) {
	model := `{"RESOURCE":{"ROUTE":"/submodels"},"DEFATTRIBUTES":[{"name":"identity","attributes":[{"CLAIM":"sub"}]}],"DEFACLS":[{"name":"reader","acl":{"USEATTRIBUTES":"identity","RIGHTS":["READ"],"ACCESS":"ALLOW"}}],"DEFFORMULAS":[{"name":"active","formula":{"$boolean":true}}],"rules":[{"USEACL":"reader","USEFORMULA":"active"}]}`
	document := `{"ResourceBoundAccessRuleModels":[` + model + `]}`
	_, err := ParseResourceBoundDocument([]byte(document))
	require.NoError(t, err)
	leaking := `{"RESOURCE":{"ROUTE":"/shells"},"rules":[{"USEACL":"reader","USEFORMULA":"active"}]}`
	_, err = ParseResourceBoundDocument([]byte(`{"ResourceBoundAccessRuleModels":[` + model + `,` + leaking + `]}`))
	require.Error(t, err)
	_, err = ParseAccessModel([]byte(document), nil, "")
	require.Error(t, err)
}

func TestResourceBoundRouteAndIdentifiableDuplicates(t *testing.T) {
	document := `{"ResourceBoundAccessRuleModels":[{"RESOURCE":{"ROUTE":"/submodels/c20"},"rules":[]},{"RESOURCE":{"IDENTIFIABLE":"$sm(\"sm\")"},"rules":[]}]}`
	_, err := ParseResourceBoundDocument([]byte(document))
	require.ErrorContains(t, err, "DUPLICATE")
}

func TestResourceBoundReferenceInputUsesSubmodelObjectsAndOriginalRoute(t *testing.T) {
	state := &boundRequest{
		repo:  &resourceBoundRepository{basePath: "/api"},
		input: EvalInput{Path: "/api/shells", RoutePath: "/api/shells"},
	}
	input := resourceBoundReferenceInput(state)
	require.Equal(t, "/api/submodels", input.Path)
	require.Equal(t, "/api/shells", input.RoutePath)

	state.input.RoutePath = ""
	input = resourceBoundReferenceInput(state)
	require.Equal(t, "/api/shells", input.RoutePath)
}

func TestResourceBoundTargetsCoverRegistriesDiscoveryAndConceptDescriptions(t *testing.T) {
	tests := []struct {
		path, kind, aas, submodel, suffix string
	}{
		{"/shell-descriptors", "shell-descriptors", "", "", ""},
		{"/shell-descriptors/YWFz", "aas_descriptor", "aas", "", ""},
		{"/shell-descriptors/YWFz/submodel-descriptors/c20", "submodel_descriptor", "aas", "sm", ""},
		{"/shell-descriptors/YWFz/submodel-descriptors", "aas_descriptor", "aas", "", "submodel-descriptors"},
		{"/submodel-descriptors/c20", "submodel_descriptor", "", "sm", ""},
		{"/concept-descriptions/Y2Q", "concept_description", "", "cd", ""},
		{"/lookup/shells/YWFz", "discovery", "aas", "", ""},
		{"/lookup/shellsByAssetLink", "lookup/shells", "", "", ""},
		{"/query/shell-descriptors", "shell-descriptors", "", "", ""},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			target, err := parseBoundTarget(test.path, "")
			require.NoError(t, err)
			require.Equal(t, test.kind, target.Kind)
			require.Equal(t, test.aas, target.AAS)
			require.Equal(t, test.submodel, target.Submodel)
			require.Equal(t, test.suffix, target.Suffix)
		})
	}
}

func TestResourceBoundDiscoveryRouteHasStableNonRecursiveKey(t *testing.T) {
	object := grammar.ObjectItem{Kind: grammar.Route, Route: &grammar.RouteValue{Route: "/lookup/shells/YWFz"}}
	key, err := ResourceBoundKey(object)
	require.NoError(t, err)
	require.Equal(t, "discovery:aas", key)
}
