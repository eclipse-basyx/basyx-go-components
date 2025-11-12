package auth

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// No update flag: tests assert against files in the expected/ directory.

type adaptCtx struct {
	Claims map[string]any `json:"claims"`
	Now    string         `json:"now"`
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
	Input    string `json:"input"`
	Expected string `json:"expected"`
	Ctx      string `json:"ctx,omitempty"`
}

// TestAdaptLEForBackend loads cases from unit_tests/adapt_le/testcases.json
// Each case provides paths (relative to that base) to the input logical expression,
// expected adapted expression, and optional context (claims/now).
func TestAdaptLEForBackend(t *testing.T) {
	t.Parallel()
	base := filepath.Join("unit_tests", "adapt_le")
	manifest := filepath.Join(base, "testcases.json")

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

    for _, c := range cases {
        c := c
        t.Run(c.Input, func(t *testing.T) {
            inPath := filepath.Join(base, c.Input)
            expPath := filepath.Join(base, c.Expected)
            if c.Ctx != "" {
                t.Logf("case: input=%s expected=%s ctx=%s", inPath, expPath, filepath.Join(base, c.Ctx))
            } else {
                t.Logf("case: input=%s expected=%s", inPath, expPath)
            }

			raw, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			var le grammar.LogicalExpression
			if err := json.Unmarshal(raw, &le); err != nil {
				t.Fatalf("unmarshal logical expression: %v", err)
			}

			claims := Claims{}
			now := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			if c.Ctx != "" {
				ctxPath := filepath.Join(base, c.Ctx)
				if ctxB, err := os.ReadFile(ctxPath); err == nil {
					var ctx adaptCtx
					if err := json.Unmarshal(ctxB, &ctx); err != nil {
						t.Fatalf("unmarshal ctx: %v", err)
					}
					if ctx.Claims != nil {
						claims = ctx.Claims
					}
					if ctx.Now != "" {
						p, err := time.Parse(time.RFC3339, ctx.Now)
						if err != nil {
							t.Fatalf("parse now: %v", err)
						}
						now = p
					}
				}
			}

            adapted, _ := adaptLEForBackend(le, claims, now)
            got := normJSON(adapted)

			want, err := os.ReadFile(expPath)
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
