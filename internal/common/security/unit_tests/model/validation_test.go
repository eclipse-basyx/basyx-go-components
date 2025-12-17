/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// nolint:all
package access_control_model_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// ----- test config & helpers -----

var update = flag.Bool("update", false, "update golden files")

type testCase struct {
	File          string `json:"file"`
	Type          string `json:"type"` // e.g., "ACL", "AccessRuleModelSchemaJson", etc.
	ShouldFail    bool   `json:"should_fail"`
	ErrorContains string `json:"error_contains,omitempty"`
	Golden        string `json:"golden,omitempty"`
}

// Map of type name -> factory that returns a new pointer to the struct to unmarshal into.
func typeFactory(typeName string) (any, error) {
	switch typeName {
	case "ACL":
		var v grammar.ACL
		return &v, nil
	case "AccessPermissionRule":
		var v grammar.AccessPermissionRule
		return &v, nil
	case "AccessRuleModelSchemaJson":
		var v grammar.AccessRuleModelSchemaJSON
		return &v, nil
	case "AccessRuleModelSchemaJsonAllAccessPermissionRules":
		var v grammar.AccessRuleModelSchemaJSONAllAccessPermissionRules
		return &v, nil
	case "AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem":
		var v grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem
		return &v, nil
	case "AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem":
		var v grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem
		return &v, nil
	case "AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem":
		var v grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem
		return &v, nil
	case "AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem":
		var v grammar.AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem
		return &v, nil
	case "LogicalExpression":
		var v grammar.LogicalExpression
		return &v, nil
	case "MatchExpression":
		var v grammar.MatchExpression
		return &v, nil
	case "Value":
		var v grammar.Value
		return &v, nil
	case "StringValue":
		var v grammar.StringValue
		return &v, nil
	// add other leaf types here as needed
	default:
		return nil, fmt.Errorf("unknown type %q in test manifest", typeName)
	}
}

func mustReadFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return b
}

func normalizeJSON(t *testing.T, v any) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	return bytes.TrimSpace(buf.Bytes())
}

func discoverFallback(t *testing.T, base string) []testCase {
	t.Helper()
	var out []testCase
	defaultType := "AccessRuleModelSchemaJson"

	addDir := func(dir string, shouldFail bool) {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				t.Fatalf("walk %s: %v", dir, err)
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
				tc := testCase{
					File:       strings.TrimPrefix(path, base+string(filepath.Separator)),
					Type:       defaultType,
					ShouldFail: shouldFail,
				}
				// If it's in pass/, also look for a matching golden by name in golden/
				if !shouldFail {
					goldenName := filepath.Join("golden", d.Name()+".golden")
					if _, err := os.Stat(filepath.Join(base, goldenName)); err == nil {
						tc.Golden = goldenName
					}
				}
				out = append(out, tc)
			}
			return nil
		})
	}

	addDir(filepath.Join(base, "pass"), false)
	addDir(filepath.Join(base, "fail"), true)
	return out
}

func loadManifest(base string) ([]testCase, error) {
	manifest := filepath.Join(base, "testcases.json")
	b, err := os.ReadFile(manifest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	var cases []testCase
	if err := json.Unmarshal(b, &cases); err != nil {
		return nil, fmt.Errorf("bad manifest %s: %w", manifest, err)
	}
	return cases, nil
}

// ----- the actual test -----

func TestJSONValidation(t *testing.T) {
	t.Parallel()

	base := "testdata"

	var cases []testCase
	if m, err := loadManifest(base); err == nil {
		cases = m
	} else if errors.Is(err, os.ErrNotExist) {
		cases = discoverFallback(t, base)
	} else {
		t.Fatalf("loading manifest: %v", err)
	}

	if len(cases) == 0 {
		t.Fatalf("no test cases found. Add files under testdata/pass or testdata/fail, or create testdata/testcases.json")
	}

	for _, tc := range cases {
		tc := tc // capture
		name := tc.File
		if tc.ShouldFail {
			name = "FAIL_" + name
		} else {
			name = "PASS_" + name
		}

		t.Run(name, func(t *testing.T) {
			// t.Parallel()

			fullPath := filepath.Join(base, tc.File)
			raw := mustReadFile(t, fullPath)

			target, err := typeFactory(tc.Type)
			if err != nil {
				t.Fatalf("typeFactory(%s): %v", tc.Type, err)
			}

			unmarshalErr := json.Unmarshal(raw, target)

			if tc.ShouldFail {
				if unmarshalErr == nil {
					t.Fatalf("expected failure, but unmarshal succeeded into %T", target)
				}
				if tc.ErrorContains != "" && !strings.Contains(unmarshalErr.Error(), tc.ErrorContains) {
					t.Fatalf("expected error to contain %q, got: %v", tc.ErrorContains, unmarshalErr)
				}
				return
			}

			// Should succeed
			if unmarshalErr != nil {
				t.Fatalf("unexpected unmarshal error: %v", unmarshalErr)
			}

			// Optional golden check
			if tc.Golden != "" {
				got := normalizeJSON(t, target)
				goldenPath := filepath.Join(base, tc.Golden)

				if *update {
					if err := os.WriteFile(goldenPath, append(got, '\n'), 0o644); err != nil {
						t.Fatalf("updating golden %s: %v", goldenPath, err)
					}
				} else {
					want := bytes.TrimSpace(mustReadFile(t, goldenPath))
					gotCanon, err := canonicalizeJSON(got)
					if err != nil {
						t.Fatalf("canonicalizing got: %v", err)
					}
					wantCanon, err := canonicalizeJSON(want)
					if err != nil {
						t.Fatalf("canonicalizing golden %s: %v", goldenPath, err)
					}

					if !bytes.Equal(gotCanon, wantCanon) {
						t.Fatalf("golden mismatch (whitespace/formatting ignored).\n--- got (pretty) ---\n%s\n--- want (pretty) ---\n%s",
							prettyJSON(got), prettyJSON(want))
					}
				}
			}
		})
	}
}

// canonicalizeJSON parses JSON (ignoring whitespace) and re-encodes it in a
// stable, minimal form so byte-by-byte compares ignore formatting differences.
func canonicalizeJSON(b []byte) ([]byte, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber() // preserve numbers like 1 vs 1.0
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	// Minimal (no indent) for stable canonical bytes
	return json.Marshal(v)
}

// prettyJSON is just for readable diffs in failure messages.
func prettyJSON(b []byte) string {
	var v any
	if json.Unmarshal(b, &v) != nil {
		// Not JSON? return as-is
		return string(b)
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out)
}
