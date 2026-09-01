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

// Package eventfeed implements the CloudEvents event feed API.
package eventfeed

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	schemaAssetFull       = "metamodel-assetChangeEvent.v1.schema.json"
	schemaAssetCompact    = "metamodel-assetChangeEventCompact.v1.schema.json"
	schemaAASFull         = "metamodel-aasChangeEvent.v1.schema.json"
	schemaAASCompact      = "metamodel-aasChangeEventCompact.v1.schema.json"
	schemaSubmodelFull    = "metamodel-submodelChangeEvent.v1.schema.json"
	schemaSubmodelCompact = "metamodel-submodelChangeEventCompact.v1.schema.json"
	schemaPCNFull         = "pcnNotificationEvent.v1.schema.json"
	schemaPCNCompact      = "pcnNotificationEventCompact.v1.schema.json"

	sourceSuffixAsset    = "/lookup/shells"
	sourceSuffixAAS      = "/shells"
	sourceSuffixSubmodel = "/submodels"
)

// Builder creates event feed records from repository changes.
type Builder struct {
	sourceBaseURL string
	schemaBaseURL string
	now           func() time.Time
}

// NewBuilder creates a Builder from the supplied event feed configuration.
func NewBuilder(cfg Config) *Builder {
	return &Builder{
		sourceBaseURL: trimTrailingSlash(cfg.SourceBaseURL),
		schemaBaseURL: trimTrailingSlash(cfg.SchemaBaseURL),
		now:           func() time.Time { return time.Now().UTC() },
	}
}

// AssetCreated builds an asset-created event.
func (b *Builder) AssetCreated(globalAssetID string, aasID string, submodelSemanticIDs []string) (FeedEvent, error) {
	return b.assetEvent(TypeAssetCreated, globalAssetID, aasID, submodelSemanticIDs)
}

// AssetUpdated builds an asset-updated event.
func (b *Builder) AssetUpdated(globalAssetID string, aasID string, submodelSemanticIDs []string) (FeedEvent, error) {
	return b.assetEvent(TypeAssetUpdated, globalAssetID, aasID, submodelSemanticIDs)
}

// AssetDeleted builds an asset-deleted event.
func (b *Builder) AssetDeleted(globalAssetID string, aasID string, submodelSemanticIDs []string) (FeedEvent, error) {
	return b.assetEvent(TypeAssetDeleted, globalAssetID, aasID, submodelSemanticIDs)
}

// AASCreated builds an Asset Administration Shell-created event.
func (b *Builder) AASCreated(aasID, globalAssetID string, submodelSemanticIDs []string) (FeedEvent, error) {
	return b.aasEvent(TypeAASCreated, aasID, globalAssetID, submodelSemanticIDs)
}

// AASUpdated builds an Asset Administration Shell-updated event.
func (b *Builder) AASUpdated(aasID, globalAssetID string, submodelSemanticIDs []string) (FeedEvent, error) {
	return b.aasEvent(TypeAASUpdated, aasID, globalAssetID, submodelSemanticIDs)
}

// AASDeleted builds an Asset Administration Shell-deleted event.
func (b *Builder) AASDeleted(aasID, globalAssetID string, submodelSemanticIDs []string) (FeedEvent, error) {
	return b.aasEvent(TypeAASDeleted, aasID, globalAssetID, submodelSemanticIDs)
}

// SubmodelCreated builds a submodel-created event.
func (b *Builder) SubmodelCreated(submodelID, semanticID string, globalAssetIDs []string) (FeedEvent, error) {
	return b.submodelEvent(TypeSubmodelCreated, submodelID, semanticID, globalAssetIDs)
}

// SubmodelUpdated builds a submodel-updated event.
func (b *Builder) SubmodelUpdated(submodelID, semanticID string, globalAssetIDs []string) (FeedEvent, error) {
	return b.submodelEvent(TypeSubmodelUpdated, submodelID, semanticID, globalAssetIDs)
}

// SubmodelDeleted builds a submodel-deleted event.
func (b *Builder) SubmodelDeleted(submodelID, semanticID string, globalAssetIDs []string) (FeedEvent, error) {
	return b.submodelEvent(TypeSubmodelDeleted, submodelID, semanticID, globalAssetIDs)
}

// PCN builds an io.admin-shell.pcn.v1 feed event for a single changed Product Change Notification record. record is the record's Value-Only representation.
func (b *Builder) PCN(submodelID string, globalAssetIDs []string, record any) (FeedEvent, error) {
	ids := normalizeGlobalAssetIDs(globalAssetIDs)
	full := map[string]any{
		"submodelId":     submodelID,
		"globalAssetIds": ids,
		"record":         record,
	}
	compact := map[string]any{
		"submodelId":     submodelID,
		"globalAssetIds": ids,
	}
	return b.build(TypePCN, submodelID, sourceSuffixSubmodel, schemaPCNFull, schemaPCNCompact, full, compact)
}

// IsPCNSemanticID reports whether semanticID identifies a Product Change Notifications submodel.
func IsPCNSemanticID(semanticID string) bool {
	return irdiCode(semanticID) == irdiCode(SemanticIDPCN)
}

// irdiCode extracts the code segment of an ECLASS IRDI (the part between the
// two "#" separators). It returns "" if semanticID is not in that form.
func irdiCode(semanticID string) string {
	parts := strings.Split(semanticID, "#")
	if len(parts) != 3 {
		return ""
	}
	return parts[1]
}

func (b *Builder) assetEvent(eventType, globalAssetID, aasID string, submodelSemanticIDs []string) (FeedEvent, error) {
	if globalAssetID == "" {
		globalAssetID = aasID
	}
	aasRefs := []any{}
	if aasID != "" {
		aasRefs = append(aasRefs, modelReference("AssetAdministrationShell", aasID))
	}
	full := map[string]any{
		"globalAssetId": globalAssetID,
		"submodels":     referredSemanticIDs(submodelSemanticIDs),
		"aasRefs":       aasRefs,
	}
	compact := map[string]any{"globalAssetId": globalAssetID}
	return b.build(eventType, globalAssetID, sourceSuffixAsset, schemaAssetFull, schemaAssetCompact, full, compact)
}

func (b *Builder) aasEvent(eventType, aasID, globalAssetID string, submodelSemanticIDs []string) (FeedEvent, error) {
	full := map[string]any{
		"aasId":         aasID,
		"globalAssetId": globalAssetID,
		"submodels":     referredSemanticIDs(submodelSemanticIDs),
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

func referredSemanticIDs(semanticIDs []string) []any {
	out := make([]any, 0, len(semanticIDs))
	for _, semanticID := range semanticIDs {
		if semanticID == "" {
			continue
		}
		out = append(out, map[string]any{"referredSemanticId": externalReference(semanticID)})
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
	case TypePCN:
		return base + "/" + schemaPCNFull, base + "/" + schemaPCNCompact
	default:
		return base + "/" + schemaSubmodelFull, base + "/" + schemaSubmodelCompact
	}
}
