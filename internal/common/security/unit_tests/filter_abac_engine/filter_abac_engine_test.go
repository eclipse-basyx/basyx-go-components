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

//nolint:all
package abacenginetest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	apis "github.com/eclipse-basyx/basyx-go-components/pkg/aasregistryapi"
	"github.com/go-chi/chi/v5"
)

func loadEvalInput(filename string) (auth.EvalInput, error) {
	var input auth.EvalInput

	file, err := os.Open(filename)
	if err != nil {
		return input, err
	}
	defer func() {
		_ = file.Close()
	}()

	if err := json.NewDecoder(file).Decode(&input); err != nil {
		return input, err
	}

	return input, nil
}

func pretty(b []byte) []byte {
	var v any
	_ = json.Unmarshal(b, &v)
	out, _ := json.MarshalIndent(v, "", "  ")
	return out
}

func canon(b []byte) []byte {
	var v any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return b
	}
	out, _ := json.Marshal(v)
	return out
}

func normJSON(v any) []byte {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	return bytes.TrimSpace(buf.Bytes())
}

type adaptCase struct {
	Input     string `json:"input"`
	Expected  string `json:"expected"`
	EvalInput string `json:"eval,omitempty"`
}

// response envelope we assert against
type resp struct {
	Ok          bool                  `json:"ok"`
	Reason      auth.DecisionCode     `json:"reason"`
	QueryFilter *sanitizedQueryFilter `json:"queryFilter,omitempty"`
}

type sanitizedQueryFilter struct {
	Formula *grammar.LogicalExpression                                  `json:"Formula,omitempty"`
	Filters map[grammar.FragmentStringPattern]grammar.LogicalExpression `json:"Filters,omitempty"`
}

func sanitizeQueryFilter(qf *auth.QueryFilter) *sanitizedQueryFilter {
	if qf == nil {
		return nil
	}
	sanitized := &sanitizedQueryFilter{Formula: qf.Formula}
	if len(qf.Filters) == 0 {
		return sanitized
	}
	sanitized.Filters = make(map[grammar.FragmentStringPattern]grammar.LogicalExpression, len(qf.Filters))
	for fragment, predicate := range qf.Filters {
		sanitized.Filters[fragment] = flattenFragmentFilterPredicate(predicate)
	}
	return sanitized
}

func flattenFragmentFilterPredicate(predicate auth.FragmentFilterPredicate) grammar.LogicalExpression {
	if predicate.Condition != nil {
		return *predicate.Condition
	}
	if len(predicate.And) > 0 {
		children := make([]grammar.LogicalExpression, 0, len(predicate.And))
		for _, child := range predicate.And {
			children = append(children, flattenFragmentFilterPredicate(child))
		}
		return grammar.LogicalExpression{And: children}
	}
	children := make([]grammar.LogicalExpression, 0, len(predicate.Or))
	for _, child := range predicate.Or {
		children = append(children, flattenFragmentFilterPredicate(child))
	}
	return grammar.LogicalExpression{Or: children}
}

// TestAdaptLEForBackend loads cases from unit_tests/adapt_le/testcases.json
// Each case provides paths (relative to that base) to the input logical expression,
// expected adapted expression, and optional context (claims/now).
func TestAdaptLEForBackend(t *testing.T) {
	t.Parallel()
	manifest := "testcases.json"

	rawManifest, err := os.ReadFile(manifest)
	if err != nil {
		t.Skipf("no manifest at %s: %v", manifest, err)
		return
	}

	var cases []adaptCase
	if err := json.Unmarshal(rawManifest, &cases); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(cases) == 0 {
		t.Fatalf("empty manifest: %s", manifest)
	}

	apiRouter := chi.NewRouter()
	smCtrl := apis.NewAssetAdministrationShellRegistryAPIAPIController(nil, "/*")
	for _, rt := range smCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	for _, c := range cases {
		t.Run(c.Input, func(t *testing.T) {

			raw, err := os.ReadFile(c.Input)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}

			model, err := auth.ParseAccessModel(raw, apiRouter, "")
			if err != nil {
				t.Fatalf("model input: %v", err)
			}

			// Load optional eval input (ctx) if provided
			var evalInput auth.EvalInput
			if c.EvalInput != "" {
				fmt.Println("eval")
				evalInput, err = loadEvalInput(c.EvalInput)
				if err != nil {
					t.Fatalf("eval input: %v", err)
				}
			}

			ok, reason, qf := model.AuthorizeWithFilter(evalInput)

			got := normJSON(resp{
				Ok:          ok,
				Reason:      reason,
				QueryFilter: sanitizeQueryFilter(qf),
			})

			want, err := os.ReadFile(c.Expected)
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}

			if !bytes.Equal(canon(got), canon(bytes.TrimSpace(want))) {
				t.Fatalf("adapt mismatch\n--- got ---\n%s\n--- want ---\n%s", pretty(got), pretty(want))
			}
			t.Log("ok: adaptLEForBackend matched expected output")
		})
	}
}
