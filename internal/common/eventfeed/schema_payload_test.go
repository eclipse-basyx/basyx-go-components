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
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// schemaDir holds verbatim copies of the JSON Schema documents advertised via
// the CloudEvents "dataschema" attribute. Payloads are validated against these
// documents rather than against hand-written shape assertions, so a change to
// either the builder or the published schemas shows up here.
const schemaDir = "testdata/schemas"

const (
	testAASID      = "https://example.com/ids/aas/1"
	testAssetID    = "https://example.com/ids/asset/1"
	testSubmodelID = "https://example.com/ids/sm/1"
	testSemanticID = "https://admin-shell.io/idta/ProductChangeNotifications/1/0"
)

func TestGeneratedPayloadsMatchAdvertisedSchema(t *testing.T) {
	b := NewBuilder(DefaultConfig())
	pcnRecordValue := pcnRecordValueOnly(t)

	cases := []struct {
		name  string
		build func() (FeedEvent, error)
	}{
		{
			name: "aas",
			build: func() (FeedEvent, error) {
				return b.AASCreated(testAASID, testAssetID, []SubmodelRef{{SubmodelID: testSubmodelID, SemanticID: testSemanticID}})
			},
		},
		{
			name: "aas-without-asset-and-submodels",
			build: func() (FeedEvent, error) {
				return b.AASDeleted(testAASID, "", nil)
			},
		},
		{
			name: "aas-submodel-without-semanticId",
			build: func() (FeedEvent, error) {
				return b.AASUpdated(testAASID, testAssetID, []SubmodelRef{{SubmodelID: testSubmodelID}})
			},
		},
		{
			name: "asset",
			build: func() (FeedEvent, error) {
				return b.AssetUpdated(testAssetID, testAASID, []SubmodelRef{{SubmodelID: testSubmodelID, SemanticID: testSemanticID}})
			},
		},
		{
			name: "asset-without-aas",
			build: func() (FeedEvent, error) {
				return b.AssetDeleted(testAssetID, "", nil)
			},
		},
		{
			name: "submodel",
			build: func() (FeedEvent, error) {
				return b.SubmodelCreated(testSubmodelID, testSemanticID, []string{testAssetID})
			},
		},
		{
			name: "submodel-without-semanticId",
			build: func() (FeedEvent, error) {
				return b.SubmodelUpdated(testSubmodelID, "", nil)
			},
		},
		{
			name: "pcn",
			build: func() (FeedEvent, error) {
				return b.PCN(testSubmodelID, []string{testAssetID}, pcnRecordValue)
			},
		},
		{
			name: "pcn-without-assets",
			build: func() (FeedEvent, error) {
				return b.PCN(testSubmodelID, nil, pcnRecordValue)
			},
		},
	}

	schemas := loadAdvertisedSchemas(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := tc.build()
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			schemas.assertValid(t, ev.DataSchemaFull, ev.DataFull)
			schemas.assertValid(t, ev.DataSchemaCompact, ev.DataCompact)
		})
	}
}

// TestAdvertisedSchemasAreVendored guards against a schema constant that is
// changed without adding the matching document under testdata.
func TestAdvertisedSchemasAreVendored(t *testing.T) {
	schemas := loadAdvertisedSchemas(t)
	for _, eventType := range allEventTypes() {
		full, compact := schemaPairForType(eventType, DefaultConfig().SchemaBaseURL)
		for _, url := range []string{full, compact} {
			if _, ok := schemas.byName[path.Base(url)]; !ok {
				t.Errorf("no vendored schema for %s (event type %s)", url, eventType)
			}
		}
	}
}

// TestPCNAdvertisesSingleSchema pins the PCN event to one schema document:
// the compact payload is the identification subset of the full one, so both
// presentations advertise pcnNotificationEvent.v1 and there is no separate
// compact schema.
func TestPCNAdvertisesSingleSchema(t *testing.T) {
	cfg := DefaultConfig()
	ev, err := NewBuilder(cfg).PCN(testSubmodelID, []string{testAssetID}, pcnRecordValueOnly(t))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if ev.DataSchemaFull != ev.DataSchemaCompact {
		t.Fatalf("full=%s compact=%s want the same schema", ev.DataSchemaFull, ev.DataSchemaCompact)
	}
	if got := path.Base(ev.DataSchemaFull); got != schemaPCN {
		t.Fatalf("dataschema=%s want %s", got, schemaPCN)
	}
	full, compact := schemaPairForType(TypePCN, cfg.SchemaBaseURL)
	if full != compact {
		t.Fatalf("capabilities advertise full=%s compact=%s want the same schema", full, compact)
	}
	// Both payloads must validate against that single document.
	schemas := loadAdvertisedSchemas(t)
	schemas.assertValid(t, ev.DataSchemaFull, ev.DataFull)
	schemas.assertValid(t, ev.DataSchemaCompact, ev.DataCompact)
}

type advertisedSchemas struct {
	byName map[string]*jsonschema.Schema
}

// assertValid validates payload against the schema advertised in dataschema.
// Format assertion stays off (the draft 2020-12 default), so "format": "uri"
// remains an annotation.
func (s advertisedSchemas) assertValid(t *testing.T, dataschema, payload string) {
	t.Helper()
	name := path.Base(dataschema)
	schema, ok := s.byName[name]
	if !ok {
		t.Fatalf("no vendored schema for advertised dataschema %s", dataschema)
	}
	instance, err := jsonschema.UnmarshalJSON(strings.NewReader(payload))
	if err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if err = schema.Validate(instance); err != nil {
		t.Fatalf("payload does not satisfy %s: %v\npayload=%s", name, err, payload)
	}
}

func loadAdvertisedSchemas(t *testing.T) advertisedSchemas {
	t.Helper()
	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		t.Fatalf("read schema dir: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(schemaDir, entry.Name()))
		if readErr != nil {
			t.Fatalf("read %s: %v", entry.Name(), readErr)
		}
		doc, unmarshalErr := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if unmarshalErr != nil {
			t.Fatalf("parse %s: %v", entry.Name(), unmarshalErr)
		}
		if addErr := compiler.AddResource(schemaResourceURL(entry.Name()), doc); addErr != nil {
			t.Fatalf("add %s: %v", entry.Name(), addErr)
		}
		names = append(names, entry.Name())
	}
	out := advertisedSchemas{byName: make(map[string]*jsonschema.Schema, len(names))}
	for _, name := range names {
		schema, compileErr := compiler.Compile(schemaResourceURL(name))
		if compileErr != nil {
			t.Fatalf("compile %s: %v", name, compileErr)
		}
		out.byName[name] = schema
	}
	return out
}

// schemaResourceURL mirrors the "$id" of the vendored documents so intra-schema
// "$ref"s resolve without network access.
func schemaResourceURL(name string) string {
	return "https://admin-shell.io/events/schemas/" + name
}

// pcnRecordValueOnly produces the Value-Only record exactly the way the
// mutation sink does: a full PCN Record submodel element converted through
// PCNNewRecordValuesFromSubmodel.
func pcnRecordValueOnly(t *testing.T) any {
	t.Helper()
	sm := pcnSubmodelWithListRecordElements(t, fullPCNRecordElement())
	values := PCNNewRecordValuesFromSubmodel(nil, sm)
	if len(values) != 1 {
		t.Fatalf("expected exactly one PCN record value, got %d", len(values))
	}
	return values[0]
}

func fullPCNRecordElement() *types.SubmodelElementCollection {
	record := types.NewSubmodelElementCollection()
	record.SetValue([]types.ISubmodelElement{
		smeCollection("Manufacturer",
			smeMultiLanguageProperty("ManufacturerName", "en", "Example Corp"),
			smeCollection("PhysicalAddress",
				smeMultiLanguageProperty("Street", "en", "Example Street 1"),
				smeMultiLanguageProperty("CityTown", "en", "Example City"),
			),
		),
		smeStringProperty("ManufacturerChangeID", "CN1"),
		smeStringProperty("PcnType", "PCN"),
		smeList("ReasonsOfChange",
			smeCollection("ReasonOfChange",
				smeStringProperty("ReasonClassificationSystem", "VDMA24903"),
				smeStringProperty("ReasonId", "RAWM"),
			),
		),
		smeList("ItemCategories",
			smeCollection("ItemCategory",
				smeStringProperty("ItemClassificationSystem", "VDMA24903"),
				smeStringProperty("ItemCategory", "ELME"),
			),
		),
		smeCollection("PcnChangeInformation",
			smeMultiLanguageProperty("ChangeTitle", "en", "Material change"),
			smeMultiLanguageProperty("ChangeDetail", "en", "Switch to lead-free solder"),
		),
		smeStringProperty("DateOfRecord", "2026-06-01T12:00:00Z"),
		smeCollection("ItemOfChange",
			smeMultiLanguageProperty("ManufacturerProductFamily", "en", "Series X"),
			smeMultiLanguageProperty("ManufacturerProductDesignation", "en", "X-1000"),
		),
	})
	return record
}

func smeStringProperty(idShort, value string) types.ISubmodelElement {
	prop := types.NewProperty(types.DataTypeDefXSDString)
	id := idShort
	v := value
	prop.SetIDShort(&id)
	prop.SetValue(&v)
	return prop
}

func smeMultiLanguageProperty(idShort, language, text string) types.ISubmodelElement {
	mlp := types.NewMultiLanguageProperty()
	id := idShort
	mlp.SetIDShort(&id)
	mlp.SetValue([]types.ILangStringTextType{types.NewLangStringTextType(language, text)})
	return mlp
}

func smeCollection(idShort string, children ...types.ISubmodelElement) types.ISubmodelElement {
	sec := types.NewSubmodelElementCollection()
	id := idShort
	sec.SetIDShort(&id)
	sec.SetValue(children)
	return sec
}

func smeList(idShort string, children ...types.ISubmodelElement) types.ISubmodelElement {
	sel := types.NewSubmodelElementList(types.AASSubmodelElementsSubmodelElementCollection)
	id := idShort
	sel.SetIDShort(&id)
	sel.SetValue(children)
	return sel
}
