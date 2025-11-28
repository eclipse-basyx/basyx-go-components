package abacenginetest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	apis "github.com/eclipse-basyx/basyx-go-components/pkg/aasregistryapi"
	"github.com/go-chi/chi/v5"
)

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
	Ok          bool              `json:"ok"`
	Reason      auth.DecisionCode `json:"reason"`
	QueryFilter *auth.QueryFilter `json:"queryFilter,omitempty"`
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

			model, err := auth.ParseAccessModel(raw, apiRouter)
			if err != nil {
				t.Fatalf("model input: %v", err)
			}

			// Load optional eval input (ctx) if provided
			var evalInput auth.EvalInput
			if c.EvalInput != "" {
				fmt.Println("eval")
				evalInput, err = auth.LoadEvalInput(c.EvalInput)
				if err != nil {
					t.Fatalf("eval input: %v", err)
				}
			}

			ok, reason, qf := model.AuthorizeWithFilter(evalInput)

			got := normJSON(resp{
				Ok:          ok,
				Reason:      reason,
				QueryFilter: qf,
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

func TestParseAccessModelInvalid(t *testing.T) {
	t.Parallel()

	apiRouter := chi.NewRouter()
	smCtrl := apis.NewAssetAdministrationShellRegistryAPIAPIController(nil, "/*")
	for _, rt := range smCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	cases := []struct {
		name     string
		file     string
		wantPart string
	}{
		{"missing_root", "invalid_missing_root.json", "AllAccessPermissionRules"},
		{"missing_acl", "invalid_missing_acl.json", "exactly one of ACL or USEACL"},
		{"both_acl_and_useacl", "invalid_both_acl_and_useacl.json", "only one of ACL or USEACL"},
		{"missing_formula", "invalid_missing_formula.json", "exactly one of FORMULA or USEFORMULA"},
		{"useacl_unknown", "invalid_useacl_unknown.json", `USEACL "missing" not found`},
		{"useformula_unknown", "invalid_useformula_unknown.json", `USEFORMULA "missing" not found`},
		{"useattributes_unknown", "invalid_useattributes_unknown.json", `USEATTRIBUTES "missing" not found`},
		{"useobjects_unknown", "invalid_useobjects_unknown.json", `USEOBJECTS "missing" not found`},
		{"useobjects_cycle", "invalid_useobjects_cycle.json", `circular USEOBJECTS reference involving "A"`},
		{"duplicate_defacls", "invalid_duplicate_defacls.json", `DEFACLS: duplicate name "dup"`},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join("invalid", c.file)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}

			_, err = auth.ParseAccessModel(raw, apiRouter)
			if err == nil {
				t.Fatalf("expected error but got nil")
			}
			if c.wantPart != "" && !strings.Contains(err.Error(), c.wantPart) {
				t.Fatalf("error mismatch\nwant contains: %q\ngot: %v", c.wantPart, err)
			}
		})
	}
}

func TestAuthorizeWithFilterResilience(t *testing.T) {
	t.Parallel()

	apiRouter := chi.NewRouter()
	smCtrl := apis.NewAssetAdministrationShellRegistryAPIAPIController(nil, "/*")
	for _, rt := range smCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	type resilientCase struct {
		name string
		json string
		eval auth.EvalInput
	}

	baseEval := auth.EvalInput{
		Method: "GET",
		Path:   "/shell-descriptors",
		Claims: auth.Claims{"role": "viewer"},
	}

	cases := []resilientCase{
		{
			name: "invalid_regex_pattern_does_not_panic",
			json: `{
				"AllAccessPermissionRules": {
					"rules": [{
						"ACL": {"ACCESS":"ALLOW","RIGHTS":["READ"]},
						"OBJECTS": [{"ROUTE": "/shell-descriptors"}],
						"FORMULA": {"$regex": [ {"$strVal": "foo"}, {"$strVal": "["} ]}
					}]
				}
			}`,
			eval: baseEval,
		},
		{
			name: "non_numeric_compare_does_not_panic",
			json: `{
				"AllAccessPermissionRules": {
					"rules": [{
						"ACL": {"ACCESS":"ALLOW","RIGHTS":["READ"]},
						"OBJECTS": [{"ROUTE": "/shell-descriptors"}],
						"FORMULA": {"$gt": [ {"$attribute":{"CLAIM":"role"}}, {"$strVal":"not-a-number"} ]}
					}]
				}
			}`,
			eval: baseEval,
		},
		{
			name: "invalid_time_compare_does_not_panic",
			json: `{
				"AllAccessPermissionRules": {
					"rules": [{
						"ACL": {"ACCESS":"ALLOW","RIGHTS":["READ"]},
						"OBJECTS": [{"ROUTE": "/shell-descriptors"}],
						"FORMULA": {"$gt": [ {"$strVal":"not-a-time"}, {"$strVal":"2020-01-01T00:00:00Z"} ]}
					}]
				}
			}`,
			eval: baseEval,
		},
		{
			name: "unknown_global_token_does_not_panic",
			json: `{
				"AllAccessPermissionRules": {
					"rules": [{
						"ACL": {"ACCESS":"ALLOW","RIGHTS":["READ"]},
						"OBJECTS": [{"ROUTE": "/shell-descriptors"}],
						"FORMULA": {"$eq": [ {"$attribute":{"GLOBAL":"unknown"}}, {"$strVal":"x"} ]}
					}]
				}
			}`,
			eval: baseEval,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			model, err := auth.ParseAccessModel([]byte(c.json), apiRouter)
			if err != nil {
				t.Fatalf("parse model: %v", err)
			}

			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("AuthorizeWithFilter panicked: %v", r)
				}
			}()

			_, _, _ = model.AuthorizeWithFilter(c.eval)
		})
	}
}
