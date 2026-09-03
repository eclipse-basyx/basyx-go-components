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
	"strings"
	"testing"
	"time"
)

func TestBuilderAASCreatedPayloads(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SourceBaseURL = "https://example.com/api"
	cfg.SchemaBaseURL = "https://schemas.example.com"
	b := NewBuilder(cfg)
	b.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }

	ev, err := b.AASCreated("aas-1", "asset-1", []SubmodelRef{
		{SubmodelID: "sm-1", SemanticID: "sem-1"},
		{SubmodelID: "sm-2"},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if ev.Type != TypeAASCreated {
		t.Fatalf("type=%s", ev.Type)
	}
	if ev.Subject != "aas-1" {
		t.Fatalf("subject=%s", ev.Subject)
	}
	if !strings.HasSuffix(ev.Source, "/shells") {
		t.Fatalf("source=%s", ev.Source)
	}
	if !strings.Contains(ev.DataSchemaFull, "metamodel-aasChangeEvent.v1.schema.json") {
		t.Fatalf("schema full=%s", ev.DataSchemaFull)
	}

	var full map[string]any
	if err := json.Unmarshal([]byte(ev.DataFull), &full); err != nil {
		t.Fatalf("full json: %v", err)
	}
	if full["aasId"] != "aas-1" || full["globalAssetId"] != "asset-1" {
		t.Fatalf("full payload=%v", full)
	}
	submodels, ok := full["submodels"].([]any)
	if !ok || len(submodels) != 2 {
		t.Fatalf("submodels=%v", full["submodels"])
	}
	firstSubmodel, ok := submodels[0].(map[string]any)
	if !ok {
		t.Fatalf("submodels[0]=%v", submodels[0])
	}
	if firstSubmodel["type"] != "ExternalReference" {
		t.Fatalf("submodels[0] type=%v", firstSubmodel["type"])
	}
	submodelKeys, ok := firstSubmodel["keys"].([]any)
	if !ok || len(submodelKeys) != 1 {
		t.Fatalf("submodels[0] keys=%v", firstSubmodel["keys"])
	}
	submodelKey, ok := submodelKeys[0].(map[string]any)
	if !ok || submodelKey["type"] != "Submodel" || submodelKey["value"] != "sm-1" {
		t.Fatalf("submodels[0] key=%v", submodelKeys[0])
	}
	referredSemanticID, ok := firstSubmodel["referredSemanticId"].(map[string]any)
	if !ok {
		t.Fatalf("referredSemanticId=%v", firstSubmodel["referredSemanticId"])
	}
	if referredSemanticID["type"] != "ExternalReference" {
		t.Fatalf("referredSemanticId type=%v", referredSemanticID["type"])
	}
	keys, ok := referredSemanticID["keys"].([]any)
	if !ok || len(keys) != 1 {
		t.Fatalf("referredSemanticId keys=%v", referredSemanticID["keys"])
	}
	key, ok := keys[0].(map[string]any)
	if !ok || key["type"] != "GlobalReference" || key["value"] != "sem-1" {
		t.Fatalf("referredSemanticId key=%v", keys[0])
	}
	secondSubmodel, ok := submodels[1].(map[string]any)
	if !ok {
		t.Fatalf("submodels[1]=%v", submodels[1])
	}
	if _, hasReferredSemanticID := secondSubmodel["referredSemanticId"]; hasReferredSemanticID {
		t.Fatalf("submodels[1] must not expose referredSemanticId when none is recorded: %v", secondSubmodel)
	}
	var compact map[string]any
	if err := json.Unmarshal([]byte(ev.DataCompact), &compact); err != nil {
		t.Fatalf("compact json: %v", err)
	}
	if compact["aasId"] != "aas-1" {
		t.Fatalf("compact payload=%v", compact)
	}
	if _, ok := compact["globalAssetId"]; ok {
		t.Fatalf("compact must not include globalAssetId")
	}
}

func TestBuilderSubmodelUpdated(t *testing.T) {
	b := NewBuilder(DefaultConfig())
	ev, err := b.SubmodelUpdated("sm-1", "https://semantic", []string{"asset-1"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if ev.Type != TypeSubmodelUpdated {
		t.Fatalf("type=%s", ev.Type)
	}
	var full map[string]any
	if err := json.Unmarshal([]byte(ev.DataFull), &full); err != nil {
		t.Fatalf("json: %v", err)
	}
	ids, ok := full["globalAssetIds"].([]any)
	if !ok || len(ids) != 1 || ids[0] != "asset-1" {
		t.Fatalf("globalAssetIds=%v", full["globalAssetIds"])
	}
}

func TestBuilderDeletedAndPCN(t *testing.T) {
	b := NewBuilder(DefaultConfig())

	aasDel, err := b.AASDeleted("aas-1", "asset-1", []SubmodelRef{{SubmodelID: "sm-1"}})
	if err != nil {
		t.Fatalf("aas deleted: %v", err)
	}
	if aasDel.Type != TypeAASDeleted {
		t.Fatalf("type=%s", aasDel.Type)
	}

	assetDel, err := b.AssetDeleted("asset-1", "aas-1", nil)
	if err != nil {
		t.Fatalf("asset deleted: %v", err)
	}
	if assetDel.Type != TypeAssetDeleted {
		t.Fatalf("type=%s", assetDel.Type)
	}

	smDel, err := b.SubmodelDeleted("sm-1", SemanticIDPCN, nil)
	if err != nil {
		t.Fatalf("sm deleted: %v", err)
	}
	if smDel.Type != TypeSubmodelDeleted {
		t.Fatalf("type=%s", smDel.Type)
	}

	pcn, err := b.PCN("sm-pcn", []string{"asset-1"}, map[string]any{"ManufacturerChangeID": "CN123456"})
	if err != nil {
		t.Fatalf("pcn: %v", err)
	}
	if pcn.Type != TypePCN {
		t.Fatalf("type=%s", pcn.Type)
	}
	var full map[string]any
	if err := json.Unmarshal([]byte(pcn.DataFull), &full); err != nil {
		t.Fatalf("json: %v", err)
	}
	if full["submodelId"] != "sm-pcn" {
		t.Fatalf("submodelId=%v", full["submodelId"])
	}
	globalAssetIDs, ok := full["globalAssetIds"].([]any)
	if !ok || len(globalAssetIDs) != 1 || globalAssetIDs[0] != "asset-1" {
		t.Fatalf("globalAssetIds=%v", full["globalAssetIds"])
	}
	record, ok := full["record"].(map[string]any)
	if !ok || record["ManufacturerChangeID"] != "CN123456" {
		t.Fatalf("record=%v", full["record"])
	}
	if !IsPCNSemanticID(SemanticIDPCN) || IsPCNSemanticID("other") {
		t.Fatalf("IsPCNSemanticID mismatch")
	}
	if !IsPCNSemanticID("0173-1#01-AHE582#005") {
		t.Fatalf("IsPCNSemanticID should ignore version segment")
	}
	if IsPCNSemanticID("0173-1#01-AHE581#003") {
		t.Fatalf("IsPCNSemanticID should not match a different irdi code")
	}
}
