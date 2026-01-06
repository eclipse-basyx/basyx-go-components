package grammar

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fidTestCase struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"` // "scalar" or "fragment"
	Input         string `json:"input"`
	Expected      string `json:"expected,omitempty"`
	ShouldFail    bool   `json:"should_fail"`
	ErrorContains string `json:"error_contains,omitempty"`
}

type expectedBinding struct {
	Alias string `json:"alias"`
	Index int    `json:"index"`
}

type expectedScalar struct {
	Column   string            `json:"column"`
	Bindings []expectedBinding `json:"bindings"`
}

type expectedFragment struct {
	Bindings []expectedBinding `json:"bindings"`
}

func mustReadFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return b
}

func canonicalJSON(t *testing.T, b []byte) []byte {
	t.Helper()
	var v any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return out
}

func marshalPretty(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestFieldIdentifierProcessing_FromManifest(t *testing.T) {
	t.Parallel()

	base := filepath.Join("testdata", "fieldidentifier_processing")
	manifestPath := filepath.Join(base, "testcases.json")

	raw := mustReadFile(t, manifestPath)
	var cases []fidTestCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("unmarshal manifest %s: %v", manifestPath, err)
	}
	if len(cases) == 0 {
		t.Fatalf("empty manifest: %s", manifestPath)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			inputPath := filepath.Join(base, tc.Input)
			fieldStr := strings.TrimSpace(string(mustReadFile(t, inputPath)))
			if fieldStr == "" {
				t.Fatalf("empty input in %s", inputPath)
			}

			switch tc.Kind {
			case "scalar":
				f := ModelStringPattern(fieldStr)
				got, err := ResolveScalarFieldToSQL(&f)

				if tc.ShouldFail {
					if err == nil {
						t.Fatalf("expected error, got nil")
					}
					if tc.ErrorContains != "" && !strings.Contains(err.Error(), tc.ErrorContains) {
						t.Fatalf("expected error to contain %q, got %v", tc.ErrorContains, err)
					}
					return
				}

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tc.Expected == "" {
					t.Fatalf("missing expected for passing case")
				}

				var want expectedScalar
				wantPath := filepath.Join(base, tc.Expected)
				if err := json.Unmarshal(mustReadFile(t, wantPath), &want); err != nil {
					t.Fatalf("unmarshal expected %s: %v", wantPath, err)
				}

				gotExp := expectedScalar{Column: got.Column}
				for _, b := range got.ArrayBindings {
					gotExp.Bindings = append(gotExp.Bindings, expectedBinding{Alias: b.Alias, Index: b.Index})
				}
				if gotExp.Bindings == nil {
					gotExp.Bindings = []expectedBinding{}
				}

				gotJSON := canonicalJSON(t, marshalPretty(t, gotExp))
				wantJSON := canonicalJSON(t, bytes.TrimSpace(mustReadFile(t, wantPath)))
				if !bytes.Equal(gotJSON, wantJSON) {
					t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", marshalPretty(t, gotExp), bytes.TrimSpace(mustReadFile(t, wantPath)))
				}

			case "fragment":
				f := FragmentStringPattern(fieldStr)
				got, err := ResolveFragmentFieldToSQL(&f)

				if tc.ShouldFail {
					if err == nil {
						t.Fatalf("expected error, got nil")
					}
					if tc.ErrorContains != "" && !strings.Contains(err.Error(), tc.ErrorContains) {
						t.Fatalf("expected error to contain %q, got %v", tc.ErrorContains, err)
					}
					return
				}

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tc.Expected == "" {
					t.Fatalf("missing expected for passing case")
				}

				wantPath := filepath.Join(base, tc.Expected)
				var want expectedFragment
				if err := json.Unmarshal(mustReadFile(t, wantPath), &want); err != nil {
					t.Fatalf("unmarshal expected %s: %v", wantPath, err)
				}

				gotExp := expectedFragment{}
				for _, b := range got {
					gotExp.Bindings = append(gotExp.Bindings, expectedBinding{Alias: b.Alias, Index: b.Index})
				}
				if gotExp.Bindings == nil {
					gotExp.Bindings = []expectedBinding{}
				}

				gotJSON := canonicalJSON(t, marshalPretty(t, gotExp))
				wantJSON := canonicalJSON(t, bytes.TrimSpace(mustReadFile(t, wantPath)))
				if !bytes.Equal(gotJSON, wantJSON) {
					t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", marshalPretty(t, gotExp), bytes.TrimSpace(mustReadFile(t, wantPath)))
				}

			default:
				t.Fatalf("unknown kind %q (expected scalar|fragment)", tc.Kind)
			}
		})
	}
}
