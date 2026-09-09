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

func TestQueryNestedOperandsRespectFieldVisibility(t *testing.T) {
	provider := testenv.NewPasswordGrantTokenProvider(testKeycloakTokenURL, "basyx-ui", 10*time.Second)
	adminToken, err := provider.GetAccessToken(&testenv.TokenCredentials{User: "admin", Password: "pwd"})
	require.NoError(t, err)
	editorToken, err := provider.GetAccessToken(&testenv.TokenCredentials{User: "userx", Password: "pwd"})
	require.NoError(t, err)
	originalVersion := activePolicyVersionID(t, adminToken)
	t.Cleanup(func() {
		restoredVersion := clonePolicyVersion(t, originalVersion, adminToken)
		validatePolicyVersion(t, restoredVersion, adminToken)
		activatePolicyVersion(t, restoredVersion, adminToken)
	})

	aasID := fmt.Sprintf("urn:test:aas:hidden-date:%d", time.Now().UnixNano())
	createQueryOperandShell(t, aasID, "2026-01-01T00:00:00Z", adminToken)
	createQueryOperandShell(t, aasID+":invalid", "not-a-date", adminToken)
	createQueryOperandShell(t, aasID+":missing", "", adminToken)
	hiddenVersion := clonePolicyVersion(t, originalVersion, adminToken)
	hiddenRule := ruleCount(t, hiddenVersion, adminToken) + 1
	endpoint := fmt.Sprintf("%s/security/abac/policy-versions/%d/rules", testBaseURL, hiddenVersion)
	assertStatus(t, http.MethodPost, endpoint, `{"rule":{
		"ACL":{"ATTRIBUTES":[{"CLAIM":"role"}],"RIGHTS":["READ"],"ACCESS":"ALLOW"},
		"OBJECTS":[{"ROUTE":"/query/shells"}],
		"FORMULA":{"$eq":[{"$attribute":{"CLAIM":"role"}},{"$strVal":"editor"}]},
		"FILTER":{"FRAGMENT":"$aas#assetInformation.assetType","CONDITION":{"$boolean":false}}
	}}`, adminToken, http.StatusOK)
	validatePolicyVersion(t, hiddenVersion, adminToken)
	activatePolicyVersion(t, hiddenVersion, adminToken)
	assertQueryOperandProjection(t, editorToken, aasID)

	for _, hidden := range []bool{true, false} {
		if !hidden {
			visibleVersion := clonePolicyVersion(t, hiddenVersion, adminToken)
			setPolicyRuleEnabled(t, visibleVersion, hiddenRule, false, adminToken)
			createRoleRouteReadRule(t, visibleVersion, "editor", "/query/shells", adminToken)
			validatePolicyVersion(t, visibleVersion, adminToken)
			activatePolicyVersion(t, visibleVersion, adminToken)
		}
		t.Run(fmt.Sprintf("hidden=%t", hidden), func(t *testing.T) {
			checkQueryOperandConditions(t, editorToken, aasID, hidden)
			assertQueryOperandResponseFilter(t, editorToken, aasID, hidden)
		})
	}
}

func createQueryOperandShell(t *testing.T, aasID, assetType, token string) {
	t.Helper()
	assetInformation := map[string]any{"assetKind": "Instance"}
	if assetType != "" {
		assetInformation["assetType"] = assetType
	}
	body, err := json.Marshal(map[string]any{
		"id": aasID, "idShort": "QueryOperandSecurity", "modelType": "AssetAdministrationShell",
		"assetInformation": assetInformation,
	})
	require.NoError(t, err)
	assertStatus(t, http.MethodPost, testBaseURL+"/shells", string(body), token, http.StatusCreated)
	t.Cleanup(func() {
		assertStatus(t, http.MethodDelete, testBaseURL+"/shells/"+common.EncodeString(aasID), "", token, http.StatusNoContent)
	})
}

func assertQueryOperandProjection(t *testing.T, token, aasID string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"$condition": equalsQueryCondition("$aas#id", aasID)})
	require.NoError(t, err)
	status, response := doAuthorizedRequest(t, http.MethodPost, testBaseURL+"/query/shells", string(body), token)
	require.Equal(t, http.StatusOK, status, response)
	var page struct {
		Result []struct {
			AssetInformation map[string]any `json:"assetInformation"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(response), &page))
	require.Len(t, page.Result, 1)
	require.Equal(t, "Instance", page.Result[0].AssetInformation["assetKind"])
	require.NotContains(t, page.Result[0].AssetInformation, "assetType")
}

func checkQueryOperandConditions(t *testing.T, token, aasID string, hidden bool) {
	t.Helper()
	for _, test := range []struct {
		name      string
		condition string
		matches   bool
	}{
		{"direct", `{"$eq":[{"$field":"$aas#assetInformation.assetType"},{"$strVal":"2026-01-01T00:00:00Z"}]}`, true},
		{"year matches", `{"$eq":[{"$numCast":{"$year":{"$dateTimeCast":{"$field":"$aas#assetInformation.assetType"}}}},{"$numVal":2026}]}`, true},
		{"year with existing join", `{"$and":[{"$eq":[{"$field":"$aas#assetInformation.assetKind"},{"$strVal":"Instance"}]},{"$eq":[{"$numCast":{"$year":{"$dateTimeCast":{"$field":"$aas#assetInformation.assetType"}}}},{"$numVal":2026}]}]}`, true},
		{"year differs", `{"$eq":[{"$numCast":{"$year":{"$dateTimeCast":{"$field":"$aas#assetInformation.assetType"}}}},{"$numVal":2025}]}`, false},
		{"reversed month", `{"$eq":[{"$numVal":1},{"$numCast":{"$month":{"$dateTimeCast":{"$field":"$aas#assetInformation.assetType"}}}}]}`, true},
		{"day", `{"$eq":[{"$strCast":{"$dayOfMonth":{"$dateTimeCast":{"$field":"$aas#assetInformation.assetType"}}}},{"$strVal":"1"}]}`, true},
		{"weekday", `{"$eq":[{"$numCast":{"$dayOfWeek":{"$dateTimeCast":{"$field":"$aas#assetInformation.assetType"}}}},{"$numVal":4}]}`, true},
		{"contains year", `{"$contains":[{"$strCast":{"$year":{"$dateTimeCast":{"$field":"$aas#assetInformation.assetType"}}}},{"$strVal":"2026"}]}`, true},
		{"not equal", `{"$ne":[{"$numCast":{"$year":{"$dateTimeCast":{"$field":"$aas#assetInformation.assetType"}}}},{"$numVal":2025}]}`, true},
		{"negated differs", `{"$not":{"$eq":[{"$numCast":{"$year":{"$dateTimeCast":{"$field":"$aas#assetInformation.assetType"}}}},{"$numVal":2025}]}}`, true},
		{"negated matches", `{"$not":{"$eq":[{"$numCast":{"$year":{"$dateTimeCast":{"$field":"$aas#assetInformation.assetType"}}}},{"$numVal":2026}]}}`, false},
		{"nested casts", `{"$eq":[{"$numCast":{"$strCast":{"$year":{"$dateTimeCast":{"$strCast":{"$field":"$aas#assetInformation.assetType"}}}}}},{"$numVal":2026}]}`, true},
		{"failed inner cast", `{"$eq":[{"$strCast":{"$numCast":{"$field":"$aas#assetInformation.assetType"}}},{"$strVal":"2026-01-01T00:00:00Z"}]}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var condition map[string]any
			require.NoError(t, json.Unmarshal([]byte(test.condition), &condition))
			_, antiExistence := condition["$not"]
			for _, id := range []string{aasID, aasID + ":invalid", aasID + ":missing"} {
				var expected []string
				matches := test.matches && id == aasID || antiExistence && id != aasID
				if !hidden && matches {
					expected = []string{id}
				}
				assertAuthorizedAASQueryIDs(t, token, map[string]any{"$and": []any{
					equalsQueryCondition("$aas#id", id), condition,
				}}, expected...)
			}
		})
	}
}

func assertQueryOperandResponseFilter(t *testing.T, token, aasID string, hidden bool) {
	t.Helper()
	var condition map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"$eq":[
		{"$numCast":{"$year":{"$dateTimeCast":{"$field":"$aas#assetInformation.assetType"}}}},
		{"$numVal":2026}
	]}`), &condition))
	body, err := json.Marshal(map[string]any{
		"$condition": equalsQueryCondition("$aas#id", aasID),
		"$filters": []any{map[string]any{
			"$fragment": "$aas#idShort", "$condition": condition,
		}},
	})
	require.NoError(t, err)
	status, response := doAuthorizedRequest(t, http.MethodPost, testBaseURL+"/query/shells", string(body), token)
	require.Equal(t, http.StatusOK, status, response)
	var page struct {
		Result []map[string]any `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(response), &page))
	require.Len(t, page.Result, 1)
	if hidden {
		require.NotContains(t, page.Result[0], "idShort")
	} else {
		require.Equal(t, "QueryOperandSecurity", page.Result[0]["idShort"])
	}
}
