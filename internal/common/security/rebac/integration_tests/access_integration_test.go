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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebacintegration

import (
	"encoding/json"
	"net/http"
	neturl "net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDenialsWithoutGrantsLookExactlyLikeABAC(t *testing.T) {
	existing := createSubmodel(t, submodelURL, "alice", "denial-probe")
	existingResponse := call(t, "bob", http.MethodGet, submodelURL+"/submodels/"+enc(existing), nil, nil)
	missingResponse := call(t, "bob", http.MethodGet, submodelURL+"/submodels/"+enc(unique("missing")), nil, nil)
	expectStatus(t, http.StatusForbidden, existingResponse, "ungranted read")
	require.Equal(t, missingResponse.status, existingResponse.status, "existence must not leak")
	require.JSONEq(t, string(missingResponse.body), string(existingResponse.body), "denial bodies must not reveal existence")

	anonymous := call(t, "", http.MethodGet, submodelURL+"/submodels/"+enc(existing), nil, nil)
	expectStatus(t, http.StatusForbidden, anonymous, "anonymous callers are ABAC-only")

	noCreator := call(t, "eve", http.MethodPost, submodelURL+"/submodels", submodel(unique("sm"), "eve"), nil)
	expectStatus(t, http.StatusForbidden, noCreator, "create without creator relation or ABAC right")

	unmanaged := call(t, "bob", http.MethodGet, submodelAccess(submodelURL, existing), nil, nil)
	expectStatus(t, http.StatusNotFound, unmanaged, "$access without can_manage is hidden")
}

func TestCreatorOwnsAndSharesWithOptimisticConcurrency(t *testing.T) {
	identifier := createSubmodel(t, submodelURL, "alice", "shared", property("p", "1"))
	target := submodelURL + "/submodels/" + enc(identifier)
	accessURL := submodelAccess(submodelURL, identifier)

	expectStatus(t, http.StatusOK, call(t, "alice", http.MethodGet, target, nil, nil), "owner reads")
	grants, etag := readAccess(t, "alice", accessURL)
	require.Len(t, grants, 1)
	require.Equal(t, "owner", grants[0].Relation)
	require.Equal(t, subject(t, "alice"), grants[0].Subject)

	share := map[string]any{"grants": append(grants, userGrant(t, "viewer", "bob"))}
	expectStatus(t, http.StatusPreconditionRequired, call(t, "alice", http.MethodPut, accessURL+"/grants", share, nil), "missing If-Match")
	expectStatus(t, http.StatusPreconditionFailed, call(t, "alice", http.MethodPut, accessURL+"/grants", share, map[string]string{"If-Match": `"999"`}), "stale If-Match")
	shared := call(t, "alice", http.MethodPut, accessURL+"/grants", share, map[string]string{"If-Match": etag})
	expectStatus(t, http.StatusOK, shared, "share with bob")
	require.NotEqual(t, etag, shared.header.Get("ETag"), "grant changes must advance the ETag")

	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, target, nil, nil), "viewer reads")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodPatch, target, submodel(identifier, "renamed"), nil), "viewer cannot update")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodDelete, target, nil, nil), "viewer cannot delete")
	require.Contains(t, listSubmodelIDs(t, "bob", submodelURL), identifier)

	lastOwner := call(t, "alice", http.MethodPut, accessURL+"/grants", map[string]any{"grants": []grant{userGrant(t, "viewer", "bob")}},
		map[string]string{"If-Match": shared.header.Get("ETag")})
	expectStatus(t, http.StatusConflict, lastOwner, "removing the last owner")

	setGrants(t, "alice", accessURL, grants)
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, target, nil, nil), "revocation takes effect immediately")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, submodelURL+"/submodels", nil, nil),
		"without any visible Submodel the list keeps today's ABAC denial")
}

func TestGroupGrantsUseTokenMembershipAndIssuerIsolation(t *testing.T) {
	identifier := createSubmodel(t, submodelURL, "alice", "group-shared", property("p", "1"))
	target := submodelURL + "/submodels/" + enc(identifier)
	addGrants(t, "alice", submodelAccess(submodelURL, identifier),
		groupGrant("editor", "engineering"),
		grant{Relation: "viewer", SubjectType: "user", Issuer: "https://other-issuer.example", Subject: subject(t, "bob")},
	)
	expectStatus(t, http.StatusNoContent, call(t, "carol", http.MethodPatch, target, submodel(identifier, "patched-by-group"), nil), "group editor updates")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, target, nil, nil), "same subject of another issuer gets nothing")
	expectStatus(t, http.StatusForbidden, call(t, "eve", http.MethodGet, target, nil, nil), "non-member gets nothing")
}

func TestElementGrantsArePathScoped(t *testing.T) {
	identifier := createSubmodel(t, submodelURL, "alice", "elements",
		collection("a", collection("b", property("c", "deep"))),
		property("x", "sibling"),
	)
	elements := submodelURL + "/submodels/" + enc(identifier) + "/submodel-elements"
	addGrants(t, "alice", elementAccess(submodelURL, identifier, "a"), userGrant(t, "viewer", "bob"))

	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, submodelURL+"/submodels/"+enc(identifier), nil, nil), "element grant never exposes the Submodel")
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, elements+"/a", nil, nil), "granted element")
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, elements+"/a.b.c", nil, nil), "descendant of granted element")
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, elements+"/a.b.c/$value", nil, nil), "descendant value")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodGet, elements+"/x", nil, nil), "sibling stays hidden")
	expectStatus(t, http.StatusForbidden, call(t, "bob", http.MethodPatch, elements+"/a.b.c/$value", "changed", nil), "viewer cannot update elements")

	list := call(t, "bob", http.MethodGet, elements, nil, nil)
	expectStatus(t, http.StatusOK, list, "element-only list")
	require.ElementsMatch(t, []string{"a"}, resultIDShorts(t, list))

	listed := createSubmodel(t, submodelURL, "alice", "listed", map[string]any{
		"modelType": "SubmodelElementList", "idShort": "items", "typeValueListElement": "Property", "valueTypeListElement": "xs:string",
		"value": []any{map[string]any{"modelType": "Property", "valueType": "xs:string", "value": "first"}},
	})
	listElements := submodelURL + "/submodels/" + enc(listed) + "/submodel-elements"
	addGrants(t, "alice", elementAccess(submodelURL, listed, neturl.PathEscape("items[0]")), userGrant(t, "viewer", "bob"))
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, listElements+"/"+neturl.PathEscape("items[0]"), nil, nil), "escaped list item paths address the granted item")

	addGrants(t, "alice", elementAccess(submodelURL, identifier, "x"), userGrant(t, "editor", "carol"))
	expectStatus(t, http.StatusNoContent, call(t, "carol", http.MethodPatch, elements+"/x/$value", "edited", nil), "element editor updates")
	expectStatus(t, http.StatusForbidden, call(t, "carol", http.MethodPatch, elements+"/a.b.c/$value", "edited", nil), "element grant does not reach siblings")
}

func TestApprovedAASLinkCarriesAccessUntilReferenceRemoval(t *testing.T) {
	bootstrapCreators(t)
	submodelID := createSubmodel(t, environmentURL, "alice", "linked", property("p", "1"))
	shellID := unique("aas")
	expectStatus(t, http.StatusCreated, call(t, "alice", http.MethodPost, environmentURL+"/shells", shell(shellID, submodelID), nil), "create shell")
	addGrants(t, "alice", environmentURL+"/shells/"+enc(shellID)+"/$access", userGrant(t, "viewer", "eve"))

	target := environmentURL + "/submodels/" + enc(submodelID)
	expectStatus(t, http.StatusForbidden, call(t, "eve", http.MethodGet, target, nil, nil), "no inheritance without approved link")

	accessURL := submodelAccess(environmentURL, submodelID)
	_, etag := readAccess(t, "alice", accessURL)
	unreferenced := call(t, "alice", http.MethodPut, accessURL+"/inheritance", map[string]any{"aasIds": []string{unique("aas")}}, map[string]string{"If-Match": etag})
	expectStatus(t, http.StatusBadRequest, unreferenced, "links require an existing reference")
	approved := call(t, "alice", http.MethodPut, accessURL+"/inheritance", map[string]any{"aasIds": []string{shellID}}, map[string]string{"If-Match": etag})
	expectStatus(t, http.StatusOK, approved, "approve link")

	expectStatus(t, http.StatusOK, call(t, "eve", http.MethodGet, target, nil, nil), "AAS viewer reads linked Submodel")
	expectStatus(t, http.StatusOK, call(t, "eve", http.MethodGet, environmentURL+"/shells/"+enc(shellID)+"/submodels/"+enc(submodelID), nil, nil), "superpath read")
	expectStatus(t, http.StatusForbidden, call(t, "eve", http.MethodPatch, target, submodel(submodelID, "x"), nil), "link carries only the AAS relation")
	expectStatus(t, http.StatusNotFound, call(t, "eve", http.MethodGet, accessURL, nil, nil), "links never carry management")

	removed := call(t, "alice", http.MethodDelete, environmentURL+"/shells/"+enc(shellID)+"/submodel-refs/"+enc(submodelID), nil, nil)
	expectStatus(t, http.StatusNoContent, removed, "remove reference")
	expectStatus(t, http.StatusForbidden, call(t, "eve", http.MethodGet, target, nil, nil), "removing the reference removes the link")
}

func TestFullUnionLiftsABACFiltersOnlyForGrantedResources(t *testing.T) {
	bootstrapCreators(t)
	publicID := unique("public")
	expectStatus(t, http.StatusCreated, call(t, "admin", http.MethodPost, submodelURL+"/submodels",
		submodel(publicID, "public", property("open", "1"), property("secret", "hidden")), nil), "admin creates ABAC-visible submodel")
	privateID := createSubmodel(t, submodelURL, "alice", "private", property("open", "1"), property("secret", "shared"))

	public := call(t, "userx", http.MethodGet, submodelURL+"/submodels/"+enc(publicID), nil, nil)
	expectStatus(t, http.StatusOK, public, "ABAC conditional read")
	require.ElementsMatch(t, []string{"open"}, elementIDShorts(t, public), "ABAC filter hides secret elements")
	expectStatus(t, http.StatusNotFound, call(t, "userx", http.MethodGet, submodelURL+"/submodels/"+enc(privateID), nil, nil),
		"conditional ABAC without ReBAC grant keeps today's not-found result")

	addGrants(t, "alice", submodelAccess(submodelURL, privateID), userGrant(t, "viewer", "userx"))
	private := call(t, "userx", http.MethodGet, submodelURL+"/submodels/"+enc(privateID), nil, nil)
	expectStatus(t, http.StatusOK, private, "ReBAC grant")
	require.ElementsMatch(t, []string{"open", "secret"}, elementIDShorts(t, private), "ReBAC grant lifts ABAC filters of the granted resource")
	public = call(t, "userx", http.MethodGet, submodelURL+"/submodels/"+enc(publicID), nil, nil)
	require.ElementsMatch(t, []string{"open"}, elementIDShorts(t, public), "filters of other resources stay in force")

	visible := pagedSubmodelIDs(t, "userx", submodelURL, 1)
	require.Contains(t, visible, publicID)
	require.Contains(t, visible, privateID)
	require.Len(t, visible, len(uniqueStrings(visible)), "paging must not repeat or skip rows")
}

func TestRevocationKeepsSurvivingABACAccess(t *testing.T) {
	bootstrapCreators(t)
	publicID := unique("public")
	expectStatus(t, http.StatusCreated, call(t, "admin", http.MethodPost, submodelURL+"/submodels", submodel(publicID, "public"), nil), "create")
	recovery := submodelURL + "/security/rebac/admin/owners/submodel/" + enc(publicID)
	_, etag := readAccess(t, "dave", submodelAccess(submodelURL, publicID))
	expectStatus(t, http.StatusOK, call(t, "dave", http.MethodPut, recovery, map[string]any{"owners": []grant{userGrant(t, "owner", "alice")}},
		map[string]string{"If-Match": etag}), "administrator assigns an owner")
	addGrants(t, "alice", submodelAccess(submodelURL, publicID), userGrant(t, "viewer", "userx"))
	current, _ := readAccess(t, "alice", submodelAccess(submodelURL, publicID))
	remaining := make([]grant, 0, len(current))
	for _, existing := range current {
		if existing.Relation == "owner" {
			remaining = append(remaining, existing)
		}
	}
	setGrants(t, "alice", submodelAccess(submodelURL, publicID), remaining)
	expectStatus(t, http.StatusOK, call(t, "userx", http.MethodGet, submodelURL+"/submodels/"+enc(publicID), nil, nil), "ABAC access survives revocation")
}

func TestQueryOperandsOnlySeeVisibleRows(t *testing.T) {
	visibleID := createSubmodel(t, submodelURL, "alice", "query-visible")
	hiddenID := createSubmodel(t, submodelURL, "alice", "query-hidden")
	addGrants(t, "alice", submodelAccess(submodelURL, visibleID), userGrant(t, "viewer", "bob"))
	for _, test := range []struct {
		idShort  string
		expected []string
	}{
		{"query-visible", []string{visibleID}},
		{"query-hidden", nil},
	} {
		query := map[string]any{"$condition": map[string]any{
			"$eq": []any{map[string]any{"$field": "$sm#idShort"}, map[string]any{"$strVal": test.idShort}},
		}}
		result := call(t, "bob", http.MethodPost, submodelURL+"/query/submodels", query, nil)
		expectStatus(t, http.StatusOK, result, "query")
		require.ElementsMatch(t, test.expected, resultIDs(t, result), "query operands must not reach hidden rows (%s)", hiddenID)
	}
}

func TestExcludedRoutesStayABACOnly(t *testing.T) {
	identifier := createSubmodel(t, submodelURL, "alice", "excluded")
	addGrants(t, "alice", submodelAccess(submodelURL, identifier), userGrant(t, "owner", "bob"))
	expectStatus(t, http.StatusOK, call(t, "bob", http.MethodGet, submodelURL+"/submodels/"+enc(identifier), nil, nil), "covered route")
	for _, excluded := range []string{
		submodelURL + "/submodels/" + enc(identifier) + "/$history",
		submodelURL + "/submodels/$recent-changes",
		submodelURL + "/security/abac/active-policy",
	} {
		result := call(t, "bob", http.MethodGet, excluded, nil, nil)
		require.Containsf(t, []int{http.StatusForbidden, http.StatusNotFound}, result.status, "%s must stay ABAC-only: %s", excluded, string(result.body))
	}
}

func listSubmodelIDs(t *testing.T, user string, base string) []string {
	t.Helper()
	result := call(t, user, http.MethodGet, base+"/submodels?limit=1000", nil, nil)
	expectStatus(t, http.StatusOK, result, "list submodels")
	return resultIDs(t, result)
}

func pagedSubmodelIDs(t *testing.T, user string, base string, limit int) []string {
	t.Helper()
	var ids []string
	cursor := ""
	for range 1000 {
		url := base + "/submodels?limit=" + strconv.Itoa(limit)
		if cursor != "" {
			url += "&cursor=" + neturl.QueryEscape(cursor)
		}
		result := call(t, user, http.MethodGet, url, nil, nil)
		expectStatus(t, http.StatusOK, result, "paged list")
		ids = append(ids, resultIDs(t, result)...)
		var page struct {
			PagingMetadata struct {
				Cursor string `json:"cursor"`
			} `json:"paging_metadata"`
		}
		require.NoError(t, json.Unmarshal(result.body, &page))
		if page.PagingMetadata.Cursor == "" {
			return ids
		}
		cursor = page.PagingMetadata.Cursor
	}
	t.Fatal("pagination did not terminate")
	return nil
}

func resultIDs(t *testing.T, result response) []string {
	t.Helper()
	var page struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(result.body, &page), string(result.body))
	ids := make([]string, 0, len(page.Result))
	for _, item := range page.Result {
		ids = append(ids, item.ID)
	}
	return ids
}

func resultIDShorts(t *testing.T, result response) []string {
	t.Helper()
	var page struct {
		Result []struct {
			IDShort string `json:"idShort"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(result.body, &page), string(result.body))
	idShorts := make([]string, 0, len(page.Result))
	for _, item := range page.Result {
		idShorts = append(idShorts, item.IDShort)
	}
	return idShorts
}

func elementIDShorts(t *testing.T, result response) []string {
	t.Helper()
	var document struct {
		Elements []struct {
			IDShort string `json:"idShort"`
		} `json:"submodelElements"`
	}
	require.NoError(t, json.Unmarshal(result.body, &document), string(result.body))
	idShorts := make([]string, 0, len(document.Elements))
	for _, element := range document.Elements {
		idShorts = append(idShorts, element.IDShort)
	}
	return idShorts
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var unique []string
	for _, value := range values {
		if _, ok := seen[value]; !ok {
			seen[value] = struct{}{}
			unique = append(unique, value)
		}
	}
	return unique
}
