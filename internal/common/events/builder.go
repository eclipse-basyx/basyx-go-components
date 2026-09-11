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

package events

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
	// PCN events advertise a single schema for both presentations: the
	// compact payload is the identification subset of the full one, and the
	// schema leaves "record" optional.
	schemaPCN = "pcnNotificationEvent.v1.schema.json"

	sourceSuffixAsset    = "/lookup/shells"
	sourceSuffixAAS      = "/shells"
	sourceSuffixSubmodel = "/submodels"
)

// Builder constructs FeedEvent values for the change types the Event Feed
// API advertises.
type Builder struct {
	sourceBaseURL string
	schemaBaseURL string
	now           func() time.Time
}

// NewBuilder creates a Builder configured with cfg's source and schema base URLs.
func NewBuilder(cfg Config) *Builder {
	return &Builder{
		sourceBaseURL: trimTrailingSlash(cfg.SourceBaseURL),
		schemaBaseURL: trimTrailingSlash(cfg.SchemaBaseURL),
		now:           func() time.Time { return time.Now().UTC() },
	}
}

// AssetCreated builds an asset.created feed event.
func (b *Builder) AssetCreated(globalAssetID string, aasID string, submodels []SubmodelRef) (FeedEvent, error) {
	return b.assetEvent(TypeAssetCreated, globalAssetID, aasID, submodels)
}

// AssetUpdated builds an asset.updated feed event.
func (b *Builder) AssetUpdated(globalAssetID string, aasID string, submodels []SubmodelRef) (FeedEvent, error) {
	return b.assetEvent(TypeAssetUpdated, globalAssetID, aasID, submodels)
}

// AssetDeleted builds an asset.deleted feed event.
func (b *Builder) AssetDeleted(globalAssetID string, aasID string, submodels []SubmodelRef) (FeedEvent, error) {
	return b.assetEvent(TypeAssetDeleted, globalAssetID, aasID, submodels)
}

// AASCreated builds an aas.created feed event.
func (b *Builder) AASCreated(aasID, globalAssetID string, submodels []SubmodelRef) (FeedEvent, error) {
	return b.aasEvent(TypeAASCreated, aasID, globalAssetID, submodels)
}

// AASUpdated builds an aas.updated feed event.
func (b *Builder) AASUpdated(aasID, globalAssetID string, submodels []SubmodelRef) (FeedEvent, error) {
	return b.aasEvent(TypeAASUpdated, aasID, globalAssetID, submodels)
}

// AASDeleted builds an aas.deleted feed event.
func (b *Builder) AASDeleted(aasID, globalAssetID string, submodels []SubmodelRef) (FeedEvent, error) {
	return b.aasEvent(TypeAASDeleted, aasID, globalAssetID, submodels)
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

// PCN builds an io.admin-shell.pcn.v1 feed event for a single changed Product Change Notification record. record is the record's Value-Only representation.
func (b *Builder) PCN(submodelID string, globalAssetIDs []string, record any) (FeedEvent, error) {
	ids := normalizeGlobalAssetIDs(globalAssetIDs)
	full := map[string]any{
		"submodelId": submodelID,
		"record":     record,
	}
	compact := map[string]any{
		"submodelId": submodelID,
	}
	// globalAssetIds is optional and must not be an empty array: a submodel
	// that is not attached to any AAS carries no asset ids at all.
	if len(ids) > 0 {
		full["globalAssetIds"] = ids
		compact["globalAssetIds"] = ids
	}
	event, err := b.build(TypePCN, submodelID, sourceSuffixSubmodel, schemaPCN, schemaPCN, full, compact)
	if len(ids) == 0 {
		event.AuthorizationAASIDs = []string{}
	}
	return event, err
}

// IsPCNSemanticID reports whether semanticID refers to the IDTA Product
// Change Notifications submodel semantic id.
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

func (b *Builder) assetEvent(eventType, globalAssetID, aasID string, submodels []SubmodelRef) (FeedEvent, error) {
	if globalAssetID == "" {
		globalAssetID = aasID
	}
	aasRefs := []any{}
	if aasID != "" {
		aasRefs = append(aasRefs, modelReference("AssetAdministrationShell", aasID))
	}
	full := map[string]any{
		"globalAssetId": globalAssetID,
		"submodels":     submodelReferences(submodels),
		"aasRefs":       aasRefs,
	}
	compact := map[string]any{"globalAssetId": globalAssetID}
	event, err := b.build(eventType, globalAssetID, sourceSuffixAsset, schemaAssetFull, schemaAssetCompact, full, compact)
	event.AuthorizationAASIDs = []string{aasID}
	return event, err
}

func (b *Builder) aasEvent(eventType, aasID, globalAssetID string, submodels []SubmodelRef) (FeedEvent, error) {
	full := map[string]any{
		"aasId":     aasID,
		"submodels": submodelReferences(submodels),
	}
	// globalAssetId is optional for an AAS event and must not be an empty
	// string: the schema types it as a URI.
	if globalAssetID != "" {
		full["globalAssetId"] = globalAssetID
	}
	compact := map[string]any{"aasId": aasID}
	event, err := b.build(eventType, aasID, sourceSuffixAAS, schemaAASFull, schemaAASCompact, full, compact)
	event.AuthorizationAASIDs = []string{aasID}
	return event, err
}

func (b *Builder) submodelEvent(eventType, submodelID, semanticID string, globalAssetIDs []string) (FeedEvent, error) {
	ids := normalizeGlobalAssetIDs(globalAssetIDs)
	full := map[string]any{
		"submodelId":     submodelID,
		"globalAssetIds": ids,
	}
	compact := map[string]any{
		"submodelId": submodelID,
	}
	if semRef := externalReference(semanticID); semRef != nil {
		full["semanticId"] = semRef
		compact["semanticId"] = semRef
	}
	event, err := b.build(eventType, submodelID, sourceSuffixSubmodel, schemaSubmodelFull, schemaSubmodelCompact, full, compact)
	if len(ids) == 0 {
		event.AuthorizationAASIDs = []string{}
	}
	return event, err
}

func (b *Builder) build(
	eventType, subject, sourceSuffix, schemaFull, schemaCompact string,
	dataFull, dataCompact map[string]any,
) (FeedEvent, error) {
	now := b.now().UTC().Truncate(time.Microsecond)
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

// submodelReferences renders the "submodels" array shared by AAS and asset
// change events: one ModelReference entry per submodel, with an optional
// referredSemanticId. A submodel that has no semantic id recorded in the
// database is still listed, just without a referredSemanticId field.
func submodelReferences(submodels []SubmodelRef) []any {
	out := make([]any, 0, len(submodels))
	for _, submodel := range submodels {
		if submodel.SubmodelID == "" {
			continue
		}
		entry := modelReference("Submodel", submodel.SubmodelID)
		if semRef := externalReference(submodel.SemanticID); semRef != nil {
			entry["referredSemanticId"] = semRef
		}
		out = append(out, entry)
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

// externalReference renders an ExternalReference with a single GlobalReference
// key. It returns nil for a blank value: an AAS Key must carry a non-empty
// value, so an unknown semantic id is represented by omitting the field.
func externalReference(value string) map[string]any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return map[string]any{
		"type": "ExternalReference",
		"keys": []map[string]any{
			{"type": "GlobalReference", "value": value},
		},
	}
}

// AllEventTypes lists the supported CloudEvents types.
func AllEventTypes() []string {
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

// SchemaPairForType returns the regular and compact payload schema URLs.
func SchemaPairForType(eventType, schemaBase string) (full, compact string) {
	base := trimTrailingSlash(schemaBase)
	switch eventType {
	case TypeAssetCreated, TypeAssetUpdated, TypeAssetDeleted:
		return base + "/" + schemaAssetFull, base + "/" + schemaAssetCompact
	case TypeAASCreated, TypeAASUpdated, TypeAASDeleted:
		return base + "/" + schemaAASFull, base + "/" + schemaAASCompact
	case TypePCN:
		pcn := base + "/" + schemaPCN
		return pcn, pcn
	default:
		return base + "/" + schemaSubmodelFull, base + "/" + schemaSubmodelCompact
	}
}
