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

func TestResourceBoundGroupGrantRequiresIssuerAndMembership(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/submodels/{submodelIdentifier}", func(http.ResponseWriter, *http.Request) {})
	principal := common.AccessPrincipal{Type: common.AccessPrincipalGroup, Issuer: "issuer", Subject: "/inspectors"}
	rule, err := boundPrincipalRule(principal, []grammar.RightsEnum{grammar.RightsEnumREAD})
	require.NoError(t, err)
	target := boundTarget{Kind: "submodel", Submodel: "bridge"}
	model, err := CompileResourceBoundPolicy(ResourceBoundPolicy{Resource: target.object(), Rules: []json.RawMessage{rule}}, router, "")
	require.NoError(t, err)

	repo := &resourceBoundRepository{groupsClaim: "groups"}
	for _, test := range []struct {
		name    string
		claims  Claims
		allowed bool
	}{
		{name: "matching group", claims: Claims{"iss": "issuer", "sub": "member", "groups": []any{"/inspectors"}}, allowed: true},
		{name: "different group", claims: Claims{"iss": "issuer", "sub": "member", "groups": []string{"/other"}}},
		{name: "different issuer", claims: Claims{"iss": "other", "sub": "member", "groups": []string{"/inspectors"}}},
		{name: "missing groups", claims: Claims{"iss": "issuer", "sub": "member"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			principals := repo.boundPrincipals(test.claims)
			input := EvalInput{Method: http.MethodGet, Path: "/submodels/YnJpZGdl", Claims: withBoundGroupClaims(test.claims, principals)}
			allowed, _, _ := model.AuthorizeWithFilter(input)
			require.Equal(t, test.allowed, allowed)
		})
	}
}

func TestResourceBoundPrincipalsDefaultToUserAndDeduplicateGroups(t *testing.T) {
	repo := &resourceBoundRepository{groupsClaim: "memberOf"}
	principals := repo.boundPrincipals(Claims{
		"iss":      "issuer",
		"sub":      "subject",
		"memberOf": []any{"/team", "/team", 17, " "},
	})
	require.Equal(t, []common.AccessPrincipal{
		{Type: common.AccessPrincipalUser, Issuer: "issuer", Subject: "subject"},
		{Type: common.AccessPrincipalGroup, Issuer: "issuer", Subject: "/team"},
	}, principals)
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
	model := `{"RESOURCE":{"IDENTIFIABLE":"$sm(\"one\")"},"DEFATTRIBUTES":[{"name":"identity","attributes":[{"CLAIM":"sub"}]}],"DEFACLS":[{"name":"reader","acl":{"USEATTRIBUTES":"identity","RIGHTS":["READ"],"ACCESS":"ALLOW"}}],"DEFFORMULAS":[{"name":"active","formula":{"$boolean":true}}],"rules":[{"USEACL":"reader","USEFORMULA":"active"}]}`
	document := `{"ResourceBoundAccessRuleModels":[` + model + `]}`
	_, err := ParseResourceBoundDocument([]byte(document))
	require.NoError(t, err)
	leaking := `{"RESOURCE":{"IDENTIFIABLE":"$aas(\"two\")"},"rules":[{"USEACL":"reader","USEFORMULA":"active"}]}`
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

func TestResourceBoundCollectionBindingsAreRejected(t *testing.T) {
	for _, route := range []string{"/shells", "/submodels", "/shell-descriptors", "/submodel-descriptors", "/concept-descriptions", "/lookup/shells"} {
		_, err := ResourceBoundKey(grammar.ObjectItem{Kind: grammar.Route, Route: &grammar.RouteValue{Route: route}})
		require.ErrorContains(t, err, "ABAC-only")
	}
}

func TestABACCollectionAdmissionIsSeparatedFromResourceVisibility(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/shells", func(http.ResponseWriter, *http.Request) {})
	model, err := ParseAccessModel([]byte(`{"AllAccessPermissionRules":{"rules":[
		{"ACL":{"ATTRIBUTES":[{"CLAIM":"sub"}],"RIGHTS":["READ"],"ACCESS":"ALLOW"},"FORMULA":{"$boolean":true},"OBJECTS":[{"ROUTE":"/shells"}]},
		{"ACL":{"ATTRIBUTES":[{"CLAIM":"sub"}],"RIGHTS":["READ"],"ACCESS":"ALLOW"},"FORMULA":{"$boolean":true},"OBJECTS":[{"IDENTIFIABLE":"$aas(\"*\")"}]}
	]}}`), router, "")
	require.NoError(t, err)
	input := EvalInput{Method: http.MethodGet, Path: "/shells", RoutePath: "/shells", Claims: Claims{"sub": "user"}}
	admission, resources := authorizeABACCollection(model, input, grammar.DefaultSimplifyOptions())
	require.True(t, admission.Allowed)
	require.True(t, resources.Allowed)

	routeOnly, err := ParseAccessModel([]byte(`{"AllAccessPermissionRules":{"rules":[
		{"ACL":{"ATTRIBUTES":[{"CLAIM":"sub"}],"RIGHTS":["READ"],"ACCESS":"ALLOW"},"FORMULA":{"$boolean":true},"OBJECTS":[{"ROUTE":"/shells"}]}
	]}}`), router, "")
	require.NoError(t, err)
	admission, resources = authorizeABACCollection(routeOnly, input, grammar.DefaultSimplifyOptions())
	require.True(t, admission.Allowed)
	require.False(t, resources.Allowed)
}

func TestResourceBoundFallbackPreservesABACEvaluation(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/submodels/{submodelIdentifier}", func(http.ResponseWriter, *http.Request) {})
	router.Put("/submodels/{submodelIdentifier}", func(http.ResponseWriter, *http.Request) {})
	model, err := ParseAccessModel([]byte(`{"AllAccessPermissionRules":{"rules":[
		{"ACL":{"ATTRIBUTES":[{"CLAIM":"sub"}],"RIGHTS":["READ","UPDATE"],"ACCESS":"ALLOW"},"FORMULA":{"$eq":[{"$attribute":{"CLAIM":"sub"}},{"$strVal":"reader"}]},"OBJECTS":[{"IDENTIFIABLE":"$sm(\"*\")"}]}
	]}}`), router, "")
	require.NoError(t, err)

	tests := []struct {
		name   string
		method string
		user   string
	}{
		{name: "read allow", method: http.MethodGet, user: "reader"},
		{name: "read no match", method: http.MethodGet, user: "other"},
		{name: "update allow", method: http.MethodPut, user: "reader"},
		{name: "update no match", method: http.MethodPut, user: "other"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := EvalInput{Method: test.method, Path: "/submodels/c20", RoutePath: "/submodels/{submodelIdentifier}", Claims: Claims{"sub": test.user}}
			legacy := model.AuthorizeWithFilterWithOptions(input, grammar.DefaultSimplifyOptions())
			fallback := authorizeABACFallback(model, input, grammar.DefaultSimplifyOptions())
			require.Equal(t, legacy, fallback)
		})
	}
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
