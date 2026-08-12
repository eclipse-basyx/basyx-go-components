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

package persistence

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
)

type aasReconciliationMetadata struct {
	CoreChanged             bool                        `json:"coreChanged"`
	PayloadChanged          bool                        `json:"payloadChanged"`
	AssetInformationChanged bool                        `json:"assetInformationChanged"`
	ThumbnailChanged        bool                        `json:"thumbnailChanged"`
	IDShort                 *string                     `json:"idShort"`
	Category                *string                     `json:"category"`
	Description             json.RawMessage             `json:"description"`
	DisplayName             json.RawMessage             `json:"displayName"`
	Administration          json.RawMessage             `json:"administration"`
	EmbeddedDataSpecs       json.RawMessage             `json:"embeddedDataSpecifications"`
	Extensions              json.RawMessage             `json:"extensions"`
	DerivedFrom             json.RawMessage             `json:"derivedFrom"`
	AssetKind               int                         `json:"assetKind"`
	GlobalAssetID           *string                     `json:"globalAssetId"`
	AssetType               *string                     `json:"assetType"`
	Thumbnail               *aasReconciliationThumbnail `json:"thumbnail"`
}

type aasReconciliationThumbnail struct {
	Path        string  `json:"path"`
	ContentType *string `json:"contentType"`
}

type aasSpecificAssetIDChanges struct {
	Core           bool `json:"core"`
	Payload        bool `json:"payload"`
	External       bool `json:"external"`
	SupplementalID bool `json:"supplementalId"`
}

type aasSpecificAssetIDRow struct {
	Position                int                                        `json:"position"`
	MatchPosition           int                                        `json:"matchPosition"`
	Name                    string                                     `json:"name"`
	Value                   string                                     `json:"value"`
	SemanticID              json.RawMessage                            `json:"semanticId"`
	ExternalSubjectID       *submodelelements.ReconciliationReference  `json:"externalSubjectId"`
	SupplementalSemanticIDs []submodelelements.ReconciliationReference `json:"supplementalSemanticIds"`
	Changes                 aasSpecificAssetIDChanges                  `json:"changes"`
}

type aasSubmodelReferenceChanges struct {
	Core    bool `json:"core"`
	Payload bool `json:"payload"`
	Keys    bool `json:"keys"`
}

type aasSubmodelReferenceRow struct {
	Position      int                                           `json:"position"`
	MatchPosition int                                           `json:"matchPosition"`
	MatchIdentity *string                                       `json:"matchIdentity"`
	Identity      *string                                       `json:"identity"`
	Type          int                                           `json:"type"`
	Payload       json.RawMessage                               `json:"payload"`
	Keys          []submodelelements.ReconciliationReferenceKey `json:"keys"`
	Changes       aasSubmodelReferenceChanges                   `json:"changes"`
}

type aasSpecificAssetIDDelete struct {
	MatchPosition int `json:"matchPosition"`
}

type aasSubmodelReferenceDelete struct {
	MatchPosition int     `json:"matchPosition"`
	MatchIdentity *string `json:"matchIdentity"`
}

type aasSubmodelReferenceMatch struct {
	PreviousIndex int
	Identity      *string
}

type aasReconciliationPlan struct {
	Metadata         aasReconciliationMetadata    `json:"metadata"`
	SpecificUpdates  []aasSpecificAssetIDRow      `json:"specificUpdates"`
	SpecificInserts  []aasSpecificAssetIDRow      `json:"specificInserts"`
	SpecificDeletes  []aasSpecificAssetIDDelete   `json:"specificDeletes"`
	ReferenceUpdates []aasSubmodelReferenceRow    `json:"referenceUpdates"`
	ReferenceInserts []aasSubmodelReferenceRow    `json:"referenceInserts"`
	ReferenceDeletes []aasSubmodelReferenceDelete `json:"referenceDeletes"`
}

type aasReconciliationJSONRow[T any] struct {
	Row T `json:"row"`
}

type aasReconciliationPlanJSON struct {
	Metadata         aasReconciliationMetadata                              `json:"metadata"`
	SpecificUpdates  []aasReconciliationJSONRow[aasSpecificAssetIDRow]      `json:"specificUpdates"`
	SpecificInserts  []aasReconciliationJSONRow[aasSpecificAssetIDRow]      `json:"specificInserts"`
	SpecificDeletes  []aasReconciliationJSONRow[aasSpecificAssetIDDelete]   `json:"specificDeletes"`
	ReferenceUpdates []aasReconciliationJSONRow[aasSubmodelReferenceRow]    `json:"referenceUpdates"`
	ReferenceInserts []aasReconciliationJSONRow[aasSubmodelReferenceRow]    `json:"referenceInserts"`
	ReferenceDeletes []aasReconciliationJSONRow[aasSubmodelReferenceDelete] `json:"referenceDeletes"`
}

type aasReconciliationOptions struct {
	PreserveExistingManagedThumbnail bool
}

func (p aasReconciliationPlan) hasLiveMutation() bool {
	return p.Metadata.CoreChanged || p.Metadata.PayloadChanged || p.Metadata.AssetInformationChanged ||
		p.Metadata.ThumbnailChanged || len(p.SpecificUpdates) > 0 || len(p.SpecificInserts) > 0 ||
		len(p.SpecificDeletes) > 0 || len(p.ReferenceUpdates) > 0 || len(p.ReferenceInserts) > 0 ||
		len(p.ReferenceDeletes) > 0
}

func (p aasReconciliationPlan) marshal() ([]byte, error) {
	encoded, err := json.Marshal(aasReconciliationPlanJSON{
		Metadata:         p.Metadata,
		SpecificUpdates:  wrapAASReconciliationRows(p.SpecificUpdates),
		SpecificInserts:  wrapAASReconciliationRows(p.SpecificInserts),
		SpecificDeletes:  wrapAASReconciliationRows(p.SpecificDeletes),
		ReferenceUpdates: wrapAASReconciliationRows(p.ReferenceUpdates),
		ReferenceInserts: wrapAASReconciliationRows(p.ReferenceInserts),
		ReferenceDeletes: wrapAASReconciliationRows(p.ReferenceDeletes),
	})
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-RECON-MARSHALPLAN " + err.Error())
	}
	return encoded, nil
}

func wrapAASReconciliationRows[T any](rows []T) []aasReconciliationJSONRow[T] {
	result := make([]aasReconciliationJSONRow[T], len(rows))
	for index, row := range rows {
		result[index] = aasReconciliationJSONRow[T]{Row: row}
	}
	return result
}

func buildAASReconciliationPlan(
	previous types.IAssetAdministrationShell,
	target types.IAssetAdministrationShell,
	options aasReconciliationOptions,
) (aasReconciliationPlan, error) {
	metadata, err := buildAASReconciliationMetadata(previous, target, options)
	if err != nil {
		return aasReconciliationPlan{}, err
	}
	previousSpecific, err := buildAASSpecificAssetIDRows(previous.AssetInformation().SpecificAssetIDs())
	if err != nil {
		return aasReconciliationPlan{}, err
	}
	targetSpecific, err := buildAASSpecificAssetIDRows(target.AssetInformation().SpecificAssetIDs())
	if err != nil {
		return aasReconciliationPlan{}, err
	}
	specificUpdates, specificInserts, specificDeletes := reconcileAASSpecificAssetIDs(previousSpecific, targetSpecific)

	previousReferences, err := buildAASSubmodelReferenceRows(previous.Submodels())
	if err != nil {
		return aasReconciliationPlan{}, err
	}
	targetReferences, err := buildAASSubmodelReferenceRows(target.Submodels())
	if err != nil {
		return aasReconciliationPlan{}, err
	}
	referenceUpdates, referenceInserts, referenceDeletes := reconcileAASSubmodelReferences(previousReferences, targetReferences)
	return aasReconciliationPlan{
		Metadata:         metadata,
		SpecificUpdates:  specificUpdates,
		SpecificInserts:  specificInserts,
		SpecificDeletes:  specificDeletes,
		ReferenceUpdates: referenceUpdates,
		ReferenceInserts: referenceInserts,
		ReferenceDeletes: referenceDeletes,
	}, nil
}

func buildAASReconciliationMetadata(
	previous types.IAssetAdministrationShell,
	target types.IAssetAdministrationShell,
	options aasReconciliationOptions,
) (aasReconciliationMetadata, error) {
	previousValues, err := aasReconciliationMetadataValues(previous, true, options)
	if err != nil {
		return aasReconciliationMetadata{}, err
	}
	targetValues, err := aasReconciliationMetadataValues(target, false, options)
	if err != nil {
		return aasReconciliationMetadata{}, err
	}
	if options.PreserveExistingManagedThumbnail && targetValues.Thumbnail != nil && !isExternalThumbnailPath(targetValues.Thumbnail.Path) {
		if previousValues.Thumbnail == nil || targetValues.Thumbnail.Path != previousValues.Thumbnail.Path {
			targetValues.Thumbnail = nil
		} else if targetValues.Thumbnail.ContentType == nil {
			targetValues.Thumbnail.ContentType = previousValues.Thumbnail.ContentType
		}
	}
	targetValues.CoreChanged = !reflect.DeepEqual(previousValues.IDShort, targetValues.IDShort) ||
		!reflect.DeepEqual(previousValues.Category, targetValues.Category)
	targetValues.PayloadChanged = !reflect.DeepEqual(previousValues.Description, targetValues.Description) ||
		!reflect.DeepEqual(previousValues.DisplayName, targetValues.DisplayName) ||
		!reflect.DeepEqual(previousValues.Administration, targetValues.Administration) ||
		!reflect.DeepEqual(previousValues.EmbeddedDataSpecs, targetValues.EmbeddedDataSpecs) ||
		!reflect.DeepEqual(previousValues.Extensions, targetValues.Extensions) ||
		!reflect.DeepEqual(previousValues.DerivedFrom, targetValues.DerivedFrom)
	targetValues.AssetInformationChanged = previousValues.AssetKind != targetValues.AssetKind ||
		!reflect.DeepEqual(previousValues.GlobalAssetID, targetValues.GlobalAssetID) ||
		!reflect.DeepEqual(previousValues.AssetType, targetValues.AssetType)
	targetValues.ThumbnailChanged = !reflect.DeepEqual(previousValues.Thumbnail, targetValues.Thumbnail)
	return targetValues, nil
}

func aasReconciliationMetadataValues(
	aas types.IAssetAdministrationShell,
	previous bool,
	options aasReconciliationOptions,
) (aasReconciliationMetadata, error) {
	payload, err := jsonizeAssetAdministrationShellPayload(aas)
	if err != nil {
		return aasReconciliationMetadata{}, err
	}
	assetInformation := aas.AssetInformation()
	thumbnail := normalizeAASThumbnail(assetInformation.DefaultThumbnail())
	if !previous && thumbnail != nil && !isExternalThumbnailPath(thumbnail.Path) && !options.PreserveExistingManagedThumbnail {
		thumbnail = nil
	}
	return aasReconciliationMetadata{
		IDShort:           aas.IDShort(),
		Category:          aas.Category(),
		Description:       aasNullableRawJSON(payload.description),
		DisplayName:       aasNullableRawJSON(payload.displayName),
		Administration:    aasNullableRawJSON(payload.administrativeInformation),
		EmbeddedDataSpecs: aasNullableRawJSON(payload.embeddedDataSpecification),
		Extensions:        aasNullableRawJSON(payload.extensions),
		DerivedFrom:       aasNullableRawJSON(payload.derivedFrom),
		AssetKind:         int(assetInformation.AssetKind()),
		GlobalAssetID:     assetInformation.GlobalAssetID(),
		AssetType:         assetInformation.AssetType(),
		Thumbnail:         thumbnail,
	}, nil
}

func aasNullableRawJSON(value *string) json.RawMessage {
	if value == nil || !json.Valid([]byte(*value)) {
		return json.RawMessage("null")
	}
	return json.RawMessage(*value)
}

func normalizeAASThumbnail(resource types.IResource) *aasReconciliationThumbnail {
	if resource == nil || strings.TrimSpace(resource.Path()) == "" {
		return nil
	}
	return &aasReconciliationThumbnail{Path: resource.Path(), ContentType: resource.ContentType()}
}

func isExternalThumbnailPath(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

func buildAASSpecificAssetIDRows(assetIDs []types.ISpecificAssetID) ([]aasSpecificAssetIDRow, error) {
	rows := make([]aasSpecificAssetIDRow, 0, len(assetIDs))
	for position, assetID := range assetIDs {
		semanticID, err := common.BuildReferencePayload(assetID.SemanticID())
		if err != nil {
			return nil, common.NewInternalServerError("AASREPO-RECON-SPECIFICSEMANTIC " + err.Error())
		}
		external, err := submodelelements.BuildReconciliationReference(assetID.ExternalSubjectID(), false)
		if err != nil {
			return nil, common.NewInternalServerError("AASREPO-RECON-SPECIFICEXTERNAL " + err.Error())
		}
		supplemental, err := submodelelements.BuildReconciliationReferences(assetID.SupplementalSemanticIDs())
		if err != nil {
			return nil, common.NewInternalServerError("AASREPO-RECON-SPECIFICSUPPLEMENTAL " + err.Error())
		}
		rows = append(rows, aasSpecificAssetIDRow{
			Position: position, MatchPosition: position, Name: assetID.Name(), Value: assetID.Value(),
			SemanticID: semanticID, ExternalSubjectID: external, SupplementalSemanticIDs: supplemental,
		})
	}
	return rows, nil
}

func reconcileAASSpecificAssetIDs(
	previous []aasSpecificAssetIDRow,
	target []aasSpecificAssetIDRow,
) ([]aasSpecificAssetIDRow, []aasSpecificAssetIDRow, []aasSpecificAssetIDDelete) {
	updates := make([]aasSpecificAssetIDRow, 0)
	inserts := make([]aasSpecificAssetIDRow, 0)
	for position := range target {
		row := target[position]
		if position >= len(previous) {
			row.Changes = allAASSpecificAssetIDChanges()
			inserts = append(inserts, row)
			continue
		}
		row.MatchPosition = previous[position].Position
		row.Changes = aasSpecificAssetIDChanges{
			Core:           previous[position].Position != row.Position || previous[position].Name != row.Name || previous[position].Value != row.Value,
			Payload:        !reflect.DeepEqual(previous[position].SemanticID, row.SemanticID),
			External:       !reflect.DeepEqual(previous[position].ExternalSubjectID, row.ExternalSubjectID),
			SupplementalID: !reflect.DeepEqual(previous[position].SupplementalSemanticIDs, row.SupplementalSemanticIDs),
		}
		if hasAASSpecificAssetIDChanges(row.Changes) {
			updates = append(updates, row)
		}
	}
	deletes := make([]aasSpecificAssetIDDelete, 0, max(0, len(previous)-len(target)))
	for position := len(target); position < len(previous); position++ {
		deletes = append(deletes, aasSpecificAssetIDDelete{MatchPosition: previous[position].Position})
	}
	return updates, inserts, deletes
}

func allAASSpecificAssetIDChanges() aasSpecificAssetIDChanges {
	return aasSpecificAssetIDChanges{Core: true, Payload: true, External: true, SupplementalID: true}
}

func hasAASSpecificAssetIDChanges(changes aasSpecificAssetIDChanges) bool {
	return changes.Core || changes.Payload || changes.External || changes.SupplementalID
}

func buildAASSubmodelReferenceRows(references []types.IReference) ([]aasSubmodelReferenceRow, error) {
	rows := make([]aasSubmodelReferenceRow, 0, len(references))
	for position, reference := range references {
		normalized, err := submodelelements.BuildReconciliationReference(reference, true)
		if err != nil {
			return nil, common.NewInternalServerError("AASREPO-RECON-SMREFERENCE " + err.Error())
		}
		if normalized == nil {
			continue
		}
		identity := aasSubmodelReferenceIdentity(reference)
		rows = append(rows, aasSubmodelReferenceRow{
			Position: position, MatchPosition: position, Identity: identity,
			Type: normalized.Type, Payload: normalized.Payload, Keys: normalized.Keys,
		})
	}
	return rows, nil
}

func aasSubmodelReferenceIdentity(reference types.IReference) *string {
	if reference == nil {
		return nil
	}
	var identity *string
	for _, key := range reference.Keys() {
		if key.Type() != types.KeyTypesSubmodel || strings.TrimSpace(key.Value()) == "" {
			continue
		}
		if identity != nil {
			return nil
		}
		value := key.Value()
		identity = &value
	}
	return identity
}

func aasReferencesContainKeyValue(references []types.IReference, value string) bool {
	for _, reference := range references {
		for _, key := range reference.Keys() {
			if key.Value() == value {
				return true
			}
		}
	}
	return false
}

func removeAASReferenceByKeyValue(references []types.IReference, value string) ([]types.IReference, bool) {
	result := make([]types.IReference, 0, len(references))
	removed := false
	for _, reference := range references {
		if !removed && aasReferencesContainKeyValue([]types.IReference{reference}, value) {
			removed = true
			continue
		}
		result = append(result, reference)
	}
	return result, removed
}

func reconcileAASSubmodelReferences(
	previous []aasSubmodelReferenceRow,
	target []aasSubmodelReferenceRow,
) ([]aasSubmodelReferenceRow, []aasSubmodelReferenceRow, []aasSubmodelReferenceDelete) {
	previousIdentities := uniqueAASReferenceIdentities(previous)
	targetIdentities := uniqueAASReferenceIdentities(target)
	matches, usedPrevious := matchAASSubmodelReferences(previous, target, previousIdentities, targetIdentities)
	updates, inserts := buildAASSubmodelReferenceMutations(previous, target, matches)
	deletes := buildAASSubmodelReferenceDeletes(previous, usedPrevious, previousIdentities)
	return updates, inserts, deletes
}

func matchAASSubmodelReferences(
	previous []aasSubmodelReferenceRow,
	target []aasSubmodelReferenceRow,
	previousIdentities map[string]int,
	targetIdentities map[string]int,
) ([]aasSubmodelReferenceMatch, []bool) {
	matches := make([]aasSubmodelReferenceMatch, len(target))
	for index := range matches {
		matches[index].PreviousIndex = -1
	}
	reservedPrevious := reserveSemanticAASSubmodelReferenceMatches(matches, target, len(previous), previousIdentities, targetIdentities)
	usedPrevious := append([]bool(nil), reservedPrevious...)
	for targetIndex := range target {
		if matches[targetIndex].PreviousIndex >= 0 || targetIndex >= len(previous) || usedPrevious[targetIndex] {
			continue
		}
		matches[targetIndex].PreviousIndex = targetIndex
		usedPrevious[targetIndex] = true
	}
	return matches, usedPrevious
}

func reserveSemanticAASSubmodelReferenceMatches(
	matches []aasSubmodelReferenceMatch,
	target []aasSubmodelReferenceRow,
	previousCount int,
	previousIdentities map[string]int,
	targetIdentities map[string]int,
) []bool {
	reservedPrevious := make([]bool, previousCount)
	for targetIndex, row := range target {
		if row.Identity == nil {
			continue
		}
		previousIndex, previousUnique := previousIdentities[*row.Identity]
		_, targetUnique := targetIdentities[*row.Identity]
		if !previousUnique || !targetUnique {
			continue
		}
		matches[targetIndex] = aasSubmodelReferenceMatch{PreviousIndex: previousIndex, Identity: row.Identity}
		reservedPrevious[previousIndex] = true
	}
	return reservedPrevious
}

func buildAASSubmodelReferenceMutations(
	previous []aasSubmodelReferenceRow,
	target []aasSubmodelReferenceRow,
	matches []aasSubmodelReferenceMatch,
) ([]aasSubmodelReferenceRow, []aasSubmodelReferenceRow) {
	updates := make([]aasSubmodelReferenceRow, 0)
	inserts := make([]aasSubmodelReferenceRow, 0)
	for targetIndex := range target {
		row := target[targetIndex]
		previousIndex := matches[targetIndex].PreviousIndex
		if previousIndex < 0 {
			row.Changes = allAASSubmodelReferenceChanges()
			inserts = append(inserts, row)
			continue
		}
		row.MatchIdentity = matches[targetIndex].Identity
		oldRow := previous[previousIndex]
		row.MatchPosition = oldRow.Position
		row.Changes = aasSubmodelReferenceChanges{
			Core:    oldRow.Position != row.Position || oldRow.Type != row.Type,
			Payload: !reflect.DeepEqual(oldRow.Payload, row.Payload),
			Keys:    !reflect.DeepEqual(oldRow.Keys, row.Keys),
		}
		if hasAASSubmodelReferenceChanges(row.Changes) {
			updates = append(updates, row)
		}
	}
	return updates, inserts
}

func buildAASSubmodelReferenceDeletes(
	previous []aasSubmodelReferenceRow,
	usedPrevious []bool,
	previousIdentities map[string]int,
) []aasSubmodelReferenceDelete {
	deletes := make([]aasSubmodelReferenceDelete, 0)
	for index, row := range previous {
		if usedPrevious[index] {
			continue
		}
		deletes = append(deletes, aasSubmodelReferenceDelete{
			MatchPosition: row.Position,
			MatchIdentity: uniqueAASSubmodelReferenceIdentity(row.Identity, previousIdentities),
		})
	}
	return deletes
}

func uniqueAASSubmodelReferenceIdentity(identity *string, identities map[string]int) *string {
	if identity == nil {
		return nil
	}
	if _, unique := identities[*identity]; !unique {
		return nil
	}
	return identity
}

func uniqueAASReferenceIdentities(rows []aasSubmodelReferenceRow) map[string]int {
	counts := make(map[string]int)
	indexes := make(map[string]int)
	for index, row := range rows {
		if row.Identity == nil {
			continue
		}
		counts[*row.Identity]++
		indexes[*row.Identity] = index
	}
	result := make(map[string]int)
	for identity, count := range counts {
		if count == 1 {
			result[identity] = indexes[identity]
		}
	}
	return result
}

func allAASSubmodelReferenceChanges() aasSubmodelReferenceChanges {
	return aasSubmodelReferenceChanges{Core: true, Payload: true, Keys: true}
}

func hasAASSubmodelReferenceChanges(changes aasSubmodelReferenceChanges) bool {
	return changes.Core || changes.Payload || changes.Keys
}

func cloneAssetAdministrationShell(aas types.IAssetAdministrationShell) (types.IAssetAdministrationShell, error) {
	jsonable, err := jsonization.ToJsonable(aas)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-RECON-CLONETOJSON " + err.Error())
	}
	cloned, err := jsonization.AssetAdministrationShellFromJsonable(jsonable)
	if err != nil {
		return nil, common.NewInternalServerError("AASREPO-RECON-CLONEFROMJSON " + err.Error())
	}
	return cloned, nil
}

func buildEffectiveAssetInformationTarget(
	previous types.IAssetAdministrationShell,
	submitted types.IAssetInformation,
) (types.IAssetAdministrationShell, error) {
	target, err := cloneAssetAdministrationShell(previous)
	if err != nil {
		return nil, err
	}
	current := previous.AssetInformation()
	assetKind := submitted.AssetKind()
	if assetKind == 0 {
		assetKind = current.AssetKind()
	}
	effective := types.NewAssetInformation(assetKind)
	effective.SetGlobalAssetID(submitted.GlobalAssetID())
	if submitted.GlobalAssetID() == nil {
		effective.SetGlobalAssetID(current.GlobalAssetID())
	}
	effective.SetAssetType(submitted.AssetType())
	if submitted.AssetType() == nil {
		effective.SetAssetType(current.AssetType())
	}
	effective.SetSpecificAssetIDs(submitted.SpecificAssetIDs())
	if submitted.SpecificAssetIDs() == nil {
		effective.SetSpecificAssetIDs(current.SpecificAssetIDs())
	}
	effective.SetDefaultThumbnail(effectiveAssetInformationThumbnail(current.DefaultThumbnail(), submitted.DefaultThumbnail()))
	target.SetAssetInformation(effective)
	return target, nil
}

func effectiveAssetInformationThumbnail(current types.IResource, submitted types.IResource) types.IResource {
	if submitted == nil || !isExternalThumbnailPath(submitted.Path()) {
		return nil
	}
	contentType := submitted.ContentType()
	if contentType == nil && current != nil {
		contentType = current.ContentType()
	}
	thumbnail := types.NewResource(submitted.Path())
	thumbnail.SetContentType(contentType)
	return thumbnail
}
