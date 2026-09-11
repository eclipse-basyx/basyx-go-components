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
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestAuthorizedQueryPreservesManyAllowRules(t *testing.T) {
	for _, count := range []int{128, 512, 1024} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			rules := make([]string, count)
			for i := range rules {
				rules[i] = fmt.Sprintf(`{"USEACL":"read","OBJECTS":[{"ROUTE":"/shells"}],"FORMULA":{"$eq":[{"$field":"$aas#idShort"},{"$strVal":"A%d"}]}}`, i)
			}
			raw := `{"AllAccessPermissionRules":{"DEFACLS":[{"name":"read","acl":{"ATTRIBUTES":[{"CLAIM":"role"}],"RIGHTS":["READ"],"ACCESS":"ALLOW"}}],"rules":[` + strings.Join(rules, ",") + `]}}`
			router := chi.NewRouter()
			router.Get("/shells", func(http.ResponseWriter, *http.Request) {})
			model, err := ParseAccessModel([]byte(raw), router, "")
			require.NoError(t, err)
			session := newAuthorizationSession(model, Claims{"role": "viewer"}, nil, grammar.DefaultSimplifyOptions())
			result := session.evaluate(http.MethodGet, "/shells")
			require.True(t, result.Allowed)
			require.NotNil(t, result.QueryFilter)

			combinedJSON, err := json.Marshal(result.QueryFilter.Formula)
			require.NoError(t, err)
			if count >= 512 {
				var external grammar.LogicalExpression
				require.ErrorContains(t, json.Unmarshal(combinedJSON, &external), "GRAMMAR-JSON-COMPLEXITY")
			}
			cloned, err := CloneQueryFilter(result.QueryFilter)
			require.NoError(t, err, "internally combined rules must not reapply external JSON limits")
			require.Equal(t, result.QueryFilter, cloned)
			view := accessViewFromEvaluation(SemanticResourceAAS, result)
			require.Equal(t, AccessViewRestricted, view.Decision())

			field := grammar.ModelStringPattern("$aas#idShort")
			literal := grammar.StandardString("A0")
			condition := grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &field}, {StrVal: &literal}}}
			ctx := WithQueryFilter(t.Context(), result.QueryFilter)
			ctx = context.WithValue(ctx, authorizationSessionContextKey{}, session.withOuterAccess(view))
			ctx, err = WithAuthorizedQuery(ctx, SemanticResourceAAS, grammar.Query{Condition: &condition})
			require.NoError(t, err)
			sql, args := buildAuthorizedAASSelectionSQLWithArgs(ctx, t)
			require.Contains(t, sql, `"aas"."id_short"`)
			require.NotContains(t, strings.ToUpper(sql), "WHERE FALSE")
			for i := range count {
				require.Contains(t, args, fmt.Sprintf("A%d", i), "rule grant must reach the backend query")
			}
		})
	}
}
