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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

const selectiveTechnicalSemantic = "urn:example:selective:technical"
const selectiveOtherSemantic = "urn:example:selective:other"

func TestPrepareDPPMetadataUpdateLeavesContentAndAASUntouched(t *testing.T) {
	resolved := selectiveUpdateFixture()
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	before, err := json.Marshal(current)
	require.NoError(t, err)

	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{headerDppStatus: "archived"})

	require.Nil(t, update.aas)
	require.Len(t, update.submodels, 1)
	metadata := update.submodels[0]
	require.Equal(t, resolved.metadata.ID(), metadata.ID())
	require.Equal(t, resolved.metadata.Extensions(), metadata.Extensions())
	header, err := composeHeader(metadata)
	require.NoError(t, err)
	require.Equal(t, "archived", header[headerDppStatus])
	require.NotEqual(t, current[headerLastUpdate], header[headerLastUpdate])
	after, err := json.Marshal(current)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after), "planning must not mutate the source document")
	original, err := composeHeader(resolved.metadata)
	require.NoError(t, err)
	require.Equal(t, "active", original[headerDppStatus])
}

func TestPrepareDPPContentUpdatePreservesExistingIdentityAndTypedMetadata(t *testing.T) {
	resolved := selectiveUpdateFixture()
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	technical := resolved.submodels[1]
	original := technical.SubmodelElements()[0].(*types.Property)

	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{
		selectiveTechnicalSemantic: map[string]any{"temperature": json.Number("24.5")},
	})

	require.Nil(t, update.aas)
	require.Len(t, update.submodels, 2)
	changed := selectivePlannedSubmodel(t, update, technical.ID())
	require.Equal(t, technical.IDShort(), changed.IDShort())
	require.Equal(t, technical.Extensions(), changed.Extensions())
	require.Equal(t, technical.Administration().Version(), changed.Administration().Version())
	require.Equal(t, technical.Administration().CreatedAt(), changed.Administration().CreatedAt())
	property := changed.SubmodelElements()[0].(*types.Property)
	require.Equal(t, "24.5", *property.Value())
	require.Equal(t, original.ValueType(), property.ValueType())
	require.Equal(t, original.SemanticID(), property.SemanticID())
	require.Equal(t, original.Extensions(), property.Extensions())
	require.Equal(t, "20.5", *original.Value(), "planning must not mutate existing models")
	require.Equal(t, technical.SubmodelElements()[1], changed.SubmodelElements()[1])
}

func TestPrepareDPPUnchangedContentDoesNotScheduleContentWrite(t *testing.T) {
	resolved := selectiveUpdateFixture()
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{
		selectiveTechnicalSemantic: map[string]any{"temperature": json.Number("20.5")},
	})
	require.Nil(t, update.aas)
	require.Len(t, update.submodels, 1)
	require.Equal(t, resolved.metadata.ID(), update.submodels[0].ID())
}

func TestPrepareDPPMappedHeaderUpdatePreservesAASAttributes(t *testing.T) {
	resolved := selectiveUpdateFixture()
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{
		headerUniqueProductIdentifier: "urn:example:new-product",
		headerGranularity:             "Batch",
	})
	require.NotNil(t, update.aas)
	require.Equal(t, resolved.aasID, update.aas.ID())
	require.Equal(t, resolved.aas.IDShort(), update.aas.IDShort())
	require.Equal(t, resolved.aas.Extensions(), update.aas.Extensions())
	require.Equal(t, resolved.aas.Administration(), update.aas.Administration())
	require.Equal(t, resolved.aas.Submodels(), update.aas.Submodels())
	require.Equal(t, types.AssetKindBatch, update.aas.AssetInformation().AssetKind())
	require.Equal(t, "urn:example:new-product", *update.aas.AssetInformation().GlobalAssetID())
	require.Equal(t, resolved.aas.AssetInformation().SpecificAssetIDs(), update.aas.AssetInformation().SpecificAssetIDs())
	require.Len(t, update.submodels, 1)
	require.Equal(t, "urn:example:product", *resolved.aas.AssetInformation().GlobalAssetID())
}

func TestPrepareDPPNestedMergePreservesSiblingAndCollectionMetadata(t *testing.T) {
	resolved := selectiveUpdateFixture()
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{
		selectiveTechnicalSemantic: map[string]any{"details": map[string]any{"manufacturer": "Updated"}},
	})
	changed := selectivePlannedSubmodel(t, update, resolved.submodels[1].ID())
	details := changed.SubmodelElements()[1].(*types.SubmodelElementCollection)
	original := resolved.submodels[1].SubmodelElements()[1].(*types.SubmodelElementCollection)
	require.Equal(t, original.SemanticID(), details.SemanticID())
	require.Equal(t, "Updated", *details.Value()[0].(*types.Property).Value())
	require.Equal(t, original.Value()[0].SemanticID(), details.Value()[0].SemanticID())
	require.Equal(t, original.Value()[1], details.Value()[1])
	require.Equal(t, resolved.submodels[1].SubmodelElements()[0], changed.SubmodelElements()[0])
}

func TestPrepareDPPOptionalHeaderRemovalPreservesOtherMetadata(t *testing.T) {
	resolved := selectiveUpdateFixture()
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	require.Contains(t, current, headerFacilityID)
	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{headerFacilityID: nil})
	require.Nil(t, update.aas)
	require.Len(t, update.submodels, 1)
	header, err := composeHeader(update.submodels[0])
	require.NoError(t, err)
	require.NotContains(t, header, headerFacilityID)
	require.Equal(t, current[headerEconomicOperatorID], header[headerEconomicOperatorID])
}

func TestPrepareDPPContentSelectionUpdateDoesNotRewriteContent(t *testing.T) {
	resolved := selectiveUpdateFixture()
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{
		headerContentSpecificationIDs: []any{selectiveTechnicalSemantic},
	})
	require.Nil(t, update.aas)
	require.Len(t, update.submodels, 1)
	resolved.metadata = update.submodels[0]
	resolved.submodels[0] = update.submodels[0]
	selected, err := selectedResolvedContentSubmodels(resolved)
	require.NoError(t, err)
	require.Len(t, selected, 1)
	require.Equal(t, "urn:shared:technical", selected[0].ID())
}

func TestPrepareDPPStatusUpdatePreservesExplicitEmptyContentSelection(t *testing.T) {
	resolved := selectiveUpdateFixture()
	selection := metadataElementByIDShort(t, resolved.metadata, headerContentSpecificationIDs).(*types.SubmodelElementList)
	selection.SetValue([]types.ISubmodelElement{})
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{headerDppStatus: "archived"})
	require.Len(t, update.submodels, 1)
	actual := metadataElementByIDShort(t, update.submodels[0], headerContentSpecificationIDs)
	require.Equal(t, selection, actual)
}

func TestPrepareDPPListExpansionPreservesDeclaredItemType(t *testing.T) {
	resolved := selectiveUpdateFixture()
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{
		selectiveTechnicalSemantic: map[string]any{"readings": []any{json.Number("1.5"), json.Number("2.5")}},
	})
	technical := selectivePlannedSubmodel(t, update, resolved.submodels[1].ID())
	list := metadataElementByIDShort(t, technical, "readings").(*types.SubmodelElementList)
	require.Equal(t, types.DataTypeDefXSDFloat, *list.ValueTypeListElement())
	require.Len(t, list.Value(), 2)
	for _, item := range list.Value() {
		property, ok := item.(*types.Property)
		require.True(t, ok)
		require.Equal(t, types.DataTypeDefXSDFloat, property.ValueType())
		require.Nil(t, property.IDShort())
	}
}

func TestPrepareDPPSectionRemovalPreservesOtherReferencesAndContent(t *testing.T) {
	resolved := selectiveUpdateFixture()
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{selectiveTechnicalSemantic: nil})
	require.NotNil(t, update.aas)
	require.Len(t, update.submodels, 1)
	require.Len(t, update.aas.Submodels(), 2)
	require.True(t, referenceListContains(update.aas.Submodels(), resolved.metadata.ID()))
	require.True(t, referenceListContains(update.aas.Submodels(), resolved.submodels[2].ID()))
	require.False(t, referenceListContains(update.aas.Submodels(), resolved.submodels[1].ID()))
	require.Len(t, resolved.aas.Submodels(), 3)
}

func TestDecodeDPPPatchPreservesOptionalHeaderRemovals(t *testing.T) {
	patch, err := decodeDPPPatchDocument([]byte(`{"facilityId":null,"contentSpecificationIds":null}`))
	require.NoError(t, err)
	require.Contains(t, patch, headerFacilityID)
	require.Nil(t, patch[headerFacilityID])
	require.Contains(t, patch, headerContentSpecificationIDs)
	require.Nil(t, patch[headerContentSpecificationIDs])
}

func TestDecodeDPPPatchRejectsInvalidHeadersBeforePersistence(t *testing.T) {
	for _, payload := range []string{
		`{"dppStatus":null}`,
		`{"digitalProductPassportId":null}`,
		`{"granularity":"Unknown"}`,
		`{"facilityId":123}`,
	} {
		t.Run(payload, func(t *testing.T) {
			_, err := decodeDPPPatchDocument([]byte(payload))
			require.Error(t, err)
		})
	}
}

func TestPrepareDPPAddingSemanticSelectionRetainsExistingSubmodel(t *testing.T) {
	for _, removeAlias := range []bool{false, true} {
		t.Run(map[bool]string{false: "keep omitted alias", true: "remove old alias"}[removeAlias], func(t *testing.T) {
			resolved := selectiveUpdateFixture()
			resolved.metadata.SetSubmodelElements(withoutSubmodelElement(resolved.metadata.SubmodelElements(), headerContentSpecificationIDs))
			current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
			require.NoError(t, err)
			oldKey := compressedContentSectionName(resolved.submodels[1], map[string]struct{}{})
			require.Contains(t, current, oldKey)
			patch := dppDocument{
				headerContentSpecificationIDs: []any{selectiveTechnicalSemantic, selectiveOtherSemantic},
				selectiveTechnicalSemantic:    map[string]any{"temperature": json.Number("24.5")},
			}
			if removeAlias {
				patch[oldKey] = nil
			}
			update := prepareSelectiveTestUpdate(t, resolved, current, patch)
			require.Nil(t, update.aas)
			require.Len(t, update.submodels, 2)
			changed := selectivePlannedSubmodel(t, update, resolved.submodels[1].ID())
			require.Equal(t, "24.5", *metadataElementByIDShort(t, changed, "temperature").(*types.Property).Value())
			require.Equal(t, resolved.submodels[1].SubmodelElements()[1], metadataElementByIDShort(t, changed, "details"))
		})
	}
}

func TestPrepareDPPSelectingReferencedContentRetainsExistingSubmodel(t *testing.T) {
	resolved := selectiveUpdateFixture()
	selection := metadataElementByIDShort(t, resolved.metadata, headerContentSpecificationIDs).(*types.SubmodelElementList)
	selection.SetValue([]types.ISubmodelElement{scalarProperty("", selectiveOtherSemantic, types.DataTypeDefXSDString)})
	selection.Value()[0].SetIDShort(nil)
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	require.NotContains(t, current, selectiveTechnicalSemantic)
	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{
		headerContentSpecificationIDs: []any{selectiveTechnicalSemantic, selectiveOtherSemantic},
		selectiveTechnicalSemantic:    map[string]any{"temperature": json.Number("24.5")},
	})
	require.Nil(t, update.aas)
	require.Len(t, update.submodels, 2)
	changed := selectivePlannedSubmodel(t, update, resolved.submodels[1].ID())
	require.Equal(t, "24.5", *metadataElementByIDShort(t, changed, "temperature").(*types.Property).Value())
	require.Equal(t, resolved.submodels[1].SubmodelElements()[1], metadataElementByIDShort(t, changed, "details"))
}

func TestPrepareDPPSemanticNoOpRetainsOldAliasReferenceWithoutContentWrite(t *testing.T) {
	resolved := selectiveUpdateFixture()
	resolved.metadata.SetSubmodelElements(withoutSubmodelElement(resolved.metadata.SubmodelElements(), headerContentSpecificationIDs))
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	oldKey := compressedContentSectionName(resolved.submodels[1], map[string]struct{}{})
	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{
		headerContentSpecificationIDs: []any{selectiveTechnicalSemantic, selectiveOtherSemantic},
		oldKey:                        nil,
		selectiveTechnicalSemantic:    map[string]any{"temperature": json.Number("20.5")},
	})
	require.Nil(t, update.aas)
	require.Len(t, update.submodels, 1)
	require.Empty(t, update.detachedSubmodelIDs)
}

func TestPrepareDPPUpdateTimestampFollowsNewlySelectedContent(t *testing.T) {
	resolved := selectiveUpdateFixture()
	selection := metadataElementByIDShort(t, resolved.metadata, headerContentSpecificationIDs).(*types.SubmodelElementList)
	selection.SetValue([]types.ISubmodelElement{scalarProperty("", selectiveOtherSemantic, types.DataTypeDefXSDString)})
	selection.Value()[0].SetIDShort(nil)
	future := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
	setContentSubmodelUpdatedAt(resolved.submodels[1], future.Format(time.RFC3339Nano))
	current, err := composeResolvedDPP(resolved, REPRESENTATION_COMPRESSED)
	require.NoError(t, err)
	update := prepareSelectiveTestUpdate(t, resolved, current, dppDocument{
		headerContentSpecificationIDs: []any{selectiveTechnicalSemantic, selectiveOtherSemantic},
		selectiveTechnicalSemantic:    map[string]any{"temperature": json.Number("24.5")},
	})
	header, err := composeHeader(update.submodels[0])
	require.NoError(t, err)
	lastUpdate, err := time.Parse(time.RFC3339Nano, header[headerLastUpdate].(string))
	require.NoError(t, err)
	require.True(t, lastUpdate.After(future))
	changed := selectivePlannedSubmodel(t, update, resolved.submodels[1].ID())
	require.Equal(t, lastUpdate.Format(time.RFC3339Nano), *changed.Administration().UpdatedAt())
}

func prepareSelectiveTestUpdate(t *testing.T, resolved resolvedDPP, current dppDocument, patch dppDocument) preparedDPPUpdate {
	t.Helper()
	content, err := selectedResolvedContentSubmodels(resolved)
	require.NoError(t, err)
	ctx := common.ContextWithConfig(t.Context(), &common.Config{})
	service := NewDPPRepositoryService(nil, nil)
	update, err := service.prepareDPPUpdate(ctx, resolved.dppID, patch, resolved, content, current)
	require.NoError(t, err)
	return update
}

func selectivePlannedSubmodel(t *testing.T, update preparedDPPUpdate, id string) types.ISubmodel {
	t.Helper()
	for _, submodel := range update.submodels {
		if submodel.ID() == id {
			return submodel
		}
	}
	t.Fatalf("no planned submodel with existing ID %s", id)
	return nil
}

func selectiveUpdateFixture() resolvedDPP {
	header := dppHeader{
		DigitalProductPassportID: "urn:example:passport",
		UniqueProductIdentifier:  "urn:example:product",
		Granularity:              "Item",
		DppSchemaVersion:         "1.0.0",
		DppStatus:                "active",
		LastUpdate:               time.Date(2026, time.June, 1, 12, 0, 0, 0, time.UTC),
		EconomicOperatorID:       "urn:example:operator",
		FacilityID:               "urn:example:facility",
		ContentSpecificationIDs:  []string{selectiveTechnicalSemantic, selectiveOtherSemantic},
	}
	metadata := buildMetadataSubmodelWithID("urn:imported:metadata", header)
	metadata.SetExtensions([]types.IExtension{types.NewExtension("metadata-private")})
	technical := selectiveTechnicalSubmodel(header.LastUpdate)
	other := types.NewSubmodel("urn:shared:other")
	other.SetIDShort(selectiveString("Other"))
	other.SetSemanticID(globalReference(selectiveOtherSemantic))
	other.SetSubmodelElements([]types.ISubmodelElement{stringProperty("note", "unchanged")})
	submodels := []types.ISubmodel{metadata, technical, other}
	refs := []types.IReference{submodelReference(metadata.ID()), submodelReference(technical.ID()), submodelReference(other.ID())}
	aas := buildAASWithID(header, refs, "urn:example:owner")
	aas.SetIDShort(selectiveString("ImportedOwner"))
	aas.SetExtensions([]types.IExtension{types.NewExtension("owner-private")})
	administration := types.NewAdministrativeInformation()
	administration.SetVersion(selectiveString("42"))
	aas.SetAdministration(administration)
	aas.AssetInformation().SetSpecificAssetIDs([]types.ISpecificAssetID{types.NewSpecificAssetID("serial", "123")})
	return resolvedDPP{aas: aas, metadata: metadata, submodels: submodels, aasID: aas.ID(), dppID: header.DigitalProductPassportID}
}

func selectiveTechnicalSubmodel(timestamp time.Time) types.ISubmodel {
	technical := types.NewSubmodel("urn:shared:technical")
	technical.SetIDShort(selectiveString("OriginalTechnical"))
	technical.SetSemanticID(globalReference(selectiveTechnicalSemantic))
	technical.SetExtensions([]types.IExtension{types.NewExtension("technical-private")})
	setNewDPPSubmodelAdministration(technical, timestamp)
	technical.Administration().SetVersion(selectiveString("7"))
	temperature := scalarProperty("temperature", "20.5", types.DataTypeDefXSDFloat)
	temperature.SetSemanticID(globalReference("urn:example:temperature"))
	temperature.SetExtensions([]types.IExtension{types.NewExtension("sensor-private")})
	details := types.NewSubmodelElementCollection()
	details.SetIDShort(selectiveString("details"))
	details.SetSemanticID(globalReference("urn:example:details"))
	manufacturer := stringProperty("manufacturer", "Original")
	manufacturer.SetSemanticID(globalReference("urn:example:manufacturer"))
	details.SetValue([]types.ISubmodelElement{manufacturer, stringProperty("serial", "123")})
	empty := types.NewSubmodelElementList(types.AASSubmodelElementsProperty)
	empty.SetIDShort(selectiveString("readings"))
	valueType := types.DataTypeDefXSDFloat
	empty.SetValueTypeListElement(&valueType)
	empty.SetValue([]types.ISubmodelElement{})
	technical.SetSubmodelElements([]types.ISubmodelElement{temperature, details, empty})
	return technical
}

func selectiveString(value string) *string {
	return &value
}
