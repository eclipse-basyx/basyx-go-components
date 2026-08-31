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
	"time"
)

const (
	schemaAssetFull       = "metamodel-assetChangeEvent.v1.schema.json"
	schemaAssetCompact    = "metamodel-assetChangeEventCompact.v1.schema.json"
	schemaAASFull         = "metamodel-aasChangeEvent.v1.schema.json"
	schemaAASCompact      = "metamodel-aasChangeEventCompact.v1.schema.json"
	schemaSubmodelFull    = "metamodel-submodelChangeEvent.v1.schema.json"
	schemaSubmodelCompact = "metamodel-submodelChangeEventCompact.v1.schema.json"

	sourceSuffixAsset    = "/lookup/shells"
	sourceSuffixAAS      = "/shells"
	sourceSuffixSubmodel = "/submodels"
)

// Builder assembles FeedEvent envelopes for repository write hooks.
type Builder struct {
	sourceBaseURL string
	schemaBaseURL string
	now           func() time.Time
}

// NewBuilder creates a CloudEvents feed builder.
func NewBuilder(cfg Config) *Builder {
	return &Builder{
		sourceBaseURL: trimTrailingSlash(cfg.SourceBaseURL),
		schemaBaseURL: trimTrailingSlash(cfg.SchemaBaseURL),
		now:           func() time.Time { return time.Now().UTC() },
	}
}

// AssetCreated builds an asset.created feed event.
func (b *Builder) AssetCreated(globalAssetID string, aasID string, submodelIDs []string) (FeedEvent, error) {
	return b.assetEvent(TypeAssetCreated, globalAssetID, aasID, submodelIDs)
}

// AssetUpdated builds an asset.updated feed event.
func (b *Builder) AssetUpdated(globalAssetID string, aasID string, submodelIDs []string) (FeedEvent, error) {
	return b.assetEvent(TypeAssetUpdated, globalAssetID, aasID, submodelIDs)
}

// AssetDeleted builds an asset.deleted feed event.
func (b *Builder) AssetDeleted(globalAssetID string, aasID string, submodelIDs []string) (FeedEvent, error) {
	return b.assetEvent(TypeAssetDeleted, globalAssetID, aasID, submodelIDs)
}

// AASCreated builds an aas.created feed event.
func (b *Builder) AASCreated(aasID, globalAssetID string, submodelIDs []string) (FeedEvent, error) {
	return b.aasEvent(TypeAASCreated, aasID, globalAssetID, submodelIDs)
}

// AASUpdated builds an aas.updated feed event.
func (b *Builder) AASUpdated(aasID, globalAssetID string, submodelIDs []string) (FeedEvent, error) {
	return b.aasEvent(TypeAASUpdated, aasID, globalAssetID, submodelIDs)
}

// AASDeleted builds an aas.deleted feed event.
func (b *Builder) AASDeleted(aasID, globalAssetID string, submodelIDs []string) (FeedEvent, error) {
	return b.aasEvent(TypeAASDeleted, aasID, globalAssetID, submodelIDs)
}

// SubmodelCreated builds a submodel.created feed event.
func (b *Builder) SubmodelCreated(submodelID, semanticID string, globalAssetIDs []string) (FeedEvent, error) {
	return b.submodelEvent(TypeSubmodelCreated, submodelID, semanticID, globalAssetIDs)
}

// SubmodelUpdated builds a submodel.updated feed event.
func (b *Builder) SubmodelUpdated(submodelID, semanticID string, globalAssetIDs []string) (FeedEvent, error) {
	return b.submodelEvent(TypeSubmodelUpdated, submodelID, semanticID, globalAssetIDs)
}

// SubmodelDeleted builds a submodel.deleted feed event.
func (b *Builder) SubmodelDeleted(submodelID, semanticID string, globalAssetIDs []string) (FeedEvent, error) {
	return b.submodelEvent(TypeSubmodelDeleted, submodelID, semanticID, globalAssetIDs)
}

// PCN builds an io.admin-shell.pcn.v1 feed event for Product Change Notifications.
// recordIDShorts lists idShort values of Records entries referenced by the event.
func (b *Builder) PCN(submodelID, semanticID string, recordIDShorts []string) (FeedEvent, error) {
	if semanticID == "" {
		semanticID = SemanticIDPCN
	}
	semRef := externalReference(semanticID)
	recordValues := make([]any, 0, len(recordIDShorts))
	for _, idShort := range recordIDShorts {
		if idShort == "" {
			continue
		}
		recordValues = append(recordValues, map[string]any{"idShort": idShort})
	}
	full := map[string]any{
		"semanticId": semRef,
		"submodelElements": []any{
			map[string]any{
				"idShort": "Records",
				"value":   recordValues,
			},
		},
	}
	compact := map[string]any{
		"submodelId": submodelID,
		"semanticId": semRef,
	}
	return b.build(TypePCN, submodelID, sourceSuffixSubmodel, schemaSubmodelFull, schemaSubmodelCompact, full, compact)
}

// IsPCNSemanticID reports whether semanticID identifies a PCN submodel.
func IsPCNSemanticID(semanticID string) bool {
	return semanticID == SemanticIDPCN
}

func (b *Builder) assetEvent(eventType, globalAssetID, aasID string, submodelIDs []string) (FeedEvent, error) {
	if globalAssetID == "" {
		globalAssetID = aasID
	}
	submodelRefs := make([]any, 0, len(submodelIDs))
	for _, id := range submodelIDs {
		submodelRefs = append(submodelRefs, modelReference("Submodel", id))
	}
	aasRefs := []any{}
	if aasID != "" {
		aasRefs = append(aasRefs, modelReference("AssetAdministrationShell", aasID))
	}
	full := map[string]any{
		"globalAssetId": globalAssetID,
		"submodels":     submodelRefs,
		"aasRefs":       aasRefs,
	}
	compact := map[string]any{"globalAssetId": globalAssetID}
	return b.build(eventType, globalAssetID, sourceSuffixAsset, schemaAssetFull, schemaAssetCompact, full, compact)
}

func (b *Builder) aasEvent(eventType, aasID, globalAssetID string, submodelIDs []string) (FeedEvent, error) {
	submodelRefs := make([]any, 0, len(submodelIDs))
	for _, id := range submodelIDs {
		submodelRefs = append(submodelRefs, modelReference("Submodel", id))
	}
	full := map[string]any{
		"aasId":         aasID,
		"globalAssetId": globalAssetID,
		"submodels":     submodelRefs,
	}
	compact := map[string]any{"aasId": aasID}
	return b.build(eventType, aasID, sourceSuffixAAS, schemaAASFull, schemaAASCompact, full, compact)
}

func (b *Builder) submodelEvent(eventType, submodelID, semanticID string, globalAssetIDs []string) (FeedEvent, error) {
	semRef := externalReference(semanticID)
	full := map[string]any{
		"submodelId":     submodelID,
		"semanticId":     semRef,
		"globalAssetIds": normalizeGlobalAssetIDs(globalAssetIDs),
	}
	compact := map[string]any{
		"submodelId": submodelID,
		"semanticId": semRef,
	}
	return b.build(eventType, submodelID, sourceSuffixSubmodel, schemaSubmodelFull, schemaSubmodelCompact, full, compact)
}

func (b *Builder) build(
	eventType, subject, sourceSuffix, schemaFull, schemaCompact string,
	dataFull, dataCompact map[string]any,
) (FeedEvent, error) {
	now := b.now()
	fullJSON, err := json.Marshal(dataFull)
	if err != nil {
		return FeedEvent{}, fmt.Errorf("EVENTFEED-BUILD-FULLJSON: %w", err)
	}
	compactJSON, err := json.Marshal(dataCompact)
	if err != nil {
		return FeedEvent{}, fmt.Errorf("EVENTFEED-BUILD-COMPACTJSON: %w", err)
	}
	return FeedEvent{
		ID:                newEventID(now),
		Type:              eventType,
		Subject:           subject,
		Source:            b.sourceBaseURL + sourceSuffix,
		Time:              now,
		DataSchemaFull:    b.schemaBaseURL + "/" + schemaFull,
		DataSchemaCompact: b.schemaBaseURL + "/" + schemaCompact,
		DataFull:          string(fullJSON),
		DataCompact:       string(compactJSON),
	}, nil
}

func normalizeGlobalAssetIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

func modelReference(keyType, value string) map[string]any {
	return map[string]any{
		"type": "ModelReference",
		"keys": []map[string]any{
			{"type": keyType, "value": value},
		},
	}
}

func externalReference(value string) map[string]any {
	if value == "" {
		return map[string]any{
			"type": "ExternalReference",
			"keys": []map[string]any{},
		}
	}
	return map[string]any{
		"type": "ExternalReference",
		"keys": []map[string]any{
			{"type": "GlobalReference", "value": value},
		},
	}
}

func allEventTypes() []string {
	return []string{
		TypeAssetCreated,
		TypeAssetUpdated,
		TypeAssetDeleted,
		TypeAASCreated,
		TypeAASUpdated,
		TypeAASDeleted,
		TypeSubmodelCreated,
		TypeSubmodelUpdated,
		TypeSubmodelDeleted,
		TypePCN,
	}
}

func schemaPairForType(eventType, schemaBase string) (full, compact string) {
	base := trimTrailingSlash(schemaBase)
	switch eventType {
	case TypeAssetCreated, TypeAssetUpdated, TypeAssetDeleted:
		return base + "/" + schemaAssetFull, base + "/" + schemaAssetCompact
	case TypeAASCreated, TypeAASUpdated, TypeAASDeleted:
		return base + "/" + schemaAASFull, base + "/" + schemaAASCompact
	default:
		return base + "/" + schemaSubmodelFull, base + "/" + schemaSubmodelCompact
	}
}
