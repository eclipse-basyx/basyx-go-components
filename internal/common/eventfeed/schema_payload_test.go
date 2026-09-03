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

package eventfeed

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestGeneratedPayloadsMatchAdvertisedSchema(t *testing.T) {
	b := NewBuilder(DefaultConfig())
	cases := []struct {
		name    string
		build   func() (FeedEvent, error)
		regular func(map[string]any) error
		compact func(map[string]any) error
	}{
		{
			name: "aas",
			build: func() (FeedEvent, error) {
				return b.AASCreated("aas-1", "asset-1", []SubmodelRef{{SubmodelID: "sm-1", SemanticID: "sem-1"}})
			},
			regular: validateAASRegular,
			compact: validateAASCompact,
		},
		{
			name: "asset",
			build: func() (FeedEvent, error) {
				return b.AssetUpdated("asset-1", "aas-1", []SubmodelRef{{SubmodelID: "sm-1", SemanticID: "sem-1"}})
			},
			regular: validateAssetRegular,
			compact: validateAssetCompact,
		},
		{
			name: "submodel",
			build: func() (FeedEvent, error) {
				return b.SubmodelCreated("sm-1", "https://semantic", []string{"asset-1"})
			},
			regular: validateSubmodelRegular,
			compact: validateSubmodelCompact,
		},
		{
			name: "submodel-missing-semanticId",
			build: func() (FeedEvent, error) {
				return b.SubmodelUpdated("sm-1", "", nil)
			},
			regular: validateSubmodelRegular,
			compact: validateSubmodelCompact,
		},
		{
			name: "pcn",
			build: func() (FeedEvent, error) {
				return b.PCN("sm-pcn", []string{"asset-1"}, map[string]any{"ManufacturerChangeID": "CN1"})
			},
			regular: validatePCNRegular,
			compact: validatePCNCompact,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := tc.build()
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			assertPayload(t, ev.DataFull, tc.regular)
			assertPayload(t, ev.DataCompact, tc.compact)
		})
	}
}

func assertPayload(t *testing.T, raw string, validate func(map[string]any) error) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	if err := validate(payload); err != nil {
		t.Fatalf("%v payload=%s", err, raw)
	}
}

func validateAASRegular(p map[string]any) error {
	if err := requireString(p, "aasId"); err != nil {
		return err
	}
	if err := optionalString(p, "globalAssetId"); err != nil {
		return err
	}
	return validateReferredSemanticIDs(p["submodels"])
}

func validateAASCompact(p map[string]any) error {
	if err := requireExactKeys(p, "aasId"); err != nil {
		return err
	}
	return requireString(p, "aasId")
}

func validateAssetRegular(p map[string]any) error {
	if err := requireString(p, "globalAssetId"); err != nil {
		return err
	}
	if err := validateReferredSemanticIDs(p["submodels"]); err != nil {
		return err
	}
	refs, ok := p["aasRefs"].([]any)
	if !ok {
		return fmt.Errorf("aasRefs must be an array")
	}
	for _, ref := range refs {
		if err := validateReference(ref, "AssetAdministrationShell"); err != nil {
			return err
		}
	}
	return nil
}

func validateAssetCompact(p map[string]any) error {
	if err := requireExactKeys(p, "globalAssetId"); err != nil {
		return err
	}
	return requireString(p, "globalAssetId")
}

func validateSubmodelRegular(p map[string]any) error {
	if err := requireString(p, "submodelId"); err != nil {
		return err
	}
	if err := optionalStringArray(p, "globalAssetIds"); err != nil {
		return err
	}
	if _, ok := p["semanticId"]; !ok {
		return nil
	}
	return validateReference(p["semanticId"], "GlobalReference")
}

func validateSubmodelCompact(p map[string]any) error {
	if err := requireString(p, "submodelId"); err != nil {
		return err
	}
	if _, ok := p["semanticId"]; !ok {
		return requireExactKeys(p, "submodelId")
	}
	if err := requireExactKeys(p, "submodelId", "semanticId"); err != nil {
		return err
	}
	return validateReference(p["semanticId"], "GlobalReference")
}

func validatePCNRegular(p map[string]any) error {
	if err := requireString(p, "submodelId"); err != nil {
		return err
	}
	if err := optionalStringArray(p, "globalAssetIds"); err != nil {
		return err
	}
	if _, ok := p["record"]; !ok {
		return fmt.Errorf("record is required")
	}
	return nil
}

func validatePCNCompact(p map[string]any) error {
	if err := requireExactKeys(p, "submodelId", "globalAssetIds"); err != nil {
		return err
	}
	if err := requireString(p, "submodelId"); err != nil {
		return err
	}
	return optionalStringArray(p, "globalAssetIds")
}

func validateReferredSemanticIDs(raw any) error {
	items, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("submodels must be an array")
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("submodels entry must be an object")
		}
		_, hasReferredSemanticID := m["referredSemanticId"]
		if hasReferredSemanticID {
			if err := requireExactKeys(m, "type", "keys", "referredSemanticId"); err != nil {
				return err
			}
			if err := validateReference(m["referredSemanticId"], "GlobalReference"); err != nil {
				return err
			}
		} else if err := requireExactKeys(m, "type", "keys"); err != nil {
			return err
		}
		if err := validateReferenceShape(m, "Submodel"); err != nil {
			return err
		}
	}
	return nil
}

func validateReference(raw any, keyType string) error {
	m, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("reference must be an object")
	}
	if err := requireExactKeys(m, "type", "keys"); err != nil {
		return err
	}
	return validateReferenceShape(m, keyType)
}

// validateReferenceShape checks a reference's "type"/"keys" fields without
// requiring that those be the object's only keys, so it can also validate a
// "submodels" entry that carries an extra optional "referredSemanticId".
func validateReferenceShape(m map[string]any, keyType string) error {
	keys, ok := m["keys"].([]any)
	if !ok || len(keys) < 1 {
		return fmt.Errorf("keys must have at least one entry")
	}
	for _, key := range keys {
		km, ok := key.(map[string]any)
		if !ok {
			return fmt.Errorf("key must be an object")
		}
		if err := requireExactKeys(km, "type", "value"); err != nil {
			return err
		}
		if km["type"] != keyType {
			return fmt.Errorf("key type=%v want %s", km["type"], keyType)
		}
		if _, ok := km["value"].(string); !ok {
			return fmt.Errorf("key value must be a string")
		}
	}
	return nil
}

func requireString(m map[string]any, key string) error {
	v, ok := m[key].(string)
	if !ok || v == "" {
		return fmt.Errorf("%s must be a non-empty string", key)
	}
	return nil
}

func optionalString(m map[string]any, key string) error {
	if _, ok := m[key]; !ok {
		return nil
	}
	if _, ok := m[key].(string); !ok {
		return fmt.Errorf("%s must be a string", key)
	}
	return nil
}

func optionalStringArray(m map[string]any, key string) error {
	raw, ok := m[key]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("%s must be an array", key)
	}
	for _, item := range items {
		if _, ok := item.(string); !ok {
			return fmt.Errorf("%s entries must be strings", key)
		}
	}
	return nil
}

func requireExactKeys(m map[string]any, keys ...string) error {
	if len(m) != len(keys) {
		return fmt.Errorf("unexpected keys %v want %v", keysOf(m), keys)
	}
	for _, key := range keys {
		if _, ok := m[key]; !ok {
			return fmt.Errorf("missing key %s", key)
		}
	}
	return nil
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
