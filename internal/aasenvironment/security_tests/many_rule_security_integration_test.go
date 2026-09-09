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

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func TestQueryAuthorizationCombinesManyValidRules(t *testing.T) {
	const readRuleCount = 1024
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

	prefix := fmt.Sprintf("urn:test:aas:many-rules:%d", time.Now().UnixNano())
	firstID := prefix + ":0"
	lastID := fmt.Sprintf("%s:%d", prefix, readRuleCount-1)
	hiddenID := prefix + ":hidden"
	for _, aasID := range []string{firstID, lastID, hiddenID} {
		createQueryOperandShell(t, aasID, "", adminToken)
	}
	policy := manyShellReadRulesPolicy(prefix, readRuleCount)
	body, err := json.Marshal(map[string]any{
		"source_ref": "integration-test:many-valid-read-rules",
		"policy":     policy,
	})
	require.NoError(t, err)
	status, response := doAuthorizedRequest(t, http.MethodPost, testBaseURL+"/security/abac/policy-versions", string(body), adminToken)
	require.Equal(t, http.StatusCreated, status, response)
	var version struct {
		VersionID int64  `json:"version_id"`
		Status    string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(response), &version))
	require.Equal(t, "staged", version.Status)
	require.Equal(t, readRuleCount+1, ruleCount(t, version.VersionID, adminToken))
	validatePolicyVersion(t, version.VersionID, adminToken)
	activatePolicyVersion(t, version.VersionID, adminToken)

	condition := map[string]any{"$or": []any{
		equalsQueryCondition("$aas#id", firstID),
		equalsQueryCondition("$aas#id", lastID),
		equalsQueryCondition("$aas#id", hiddenID),
	}}
	assertAuthorizedAASQueryIDs(t, editorToken, condition, firstID, lastID)
	assertAuthorizedAASQueryIDs(t, editorToken, equalsQueryCondition("$aas#id", hiddenID))
}

func manyShellReadRulesPolicy(prefix string, count int) map[string]any {
	roleCondition := func(role string) map[string]any {
		return map[string]any{"$eq": []any{
			map[string]any{"$attribute": map[string]any{"CLAIM": "role"}},
			map[string]any{"$strVal": role},
		}}
	}
	rules := make([]any, 0, count+1)
	rules = append(rules, map[string]any{
		"USEACL":  "admin",
		"OBJECTS": []any{map[string]any{"ROUTE": "*"}},
		"FORMULA": roleCondition("admin"),
	})
	for index := 0; index < count; index++ {
		rules = append(rules, map[string]any{
			"USEACL":  "read",
			"OBJECTS": []any{map[string]any{"ROUTE": "/query/shells"}},
			"FORMULA": map[string]any{"$and": []any{
				roleCondition("editor"),
				equalsQueryCondition("$aas#id", fmt.Sprintf("%s:%d", prefix, index)),
			}},
		})
	}
	return map[string]any{"AllAccessPermissionRules": map[string]any{
		"DEFATTRIBUTES": []any{map[string]any{
			"name": "role", "attributes": []any{map[string]any{"CLAIM": "role"}},
		}},
		"DEFACLS": []any{
			map[string]any{"name": "admin", "acl": map[string]any{
				"USEATTRIBUTES": "role", "RIGHTS": []string{"ALL"}, "ACCESS": "ALLOW",
			}},
			map[string]any{"name": "read", "acl": map[string]any{
				"USEATTRIBUTES": "role", "RIGHTS": []string{"READ"}, "ACCESS": "ALLOW",
			}},
		},
		"rules": rules,
	}}
}
