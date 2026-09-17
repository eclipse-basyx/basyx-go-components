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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
)

func (s *DPPRepositoryService) prepareSelectiveDPPUpdate(
	ctx context.Context,
	patch dppDocument,
	merged dppDocument,
	header dppHeader,
	resolved resolvedDPP,
	currentContent []types.ISubmodel,
	current dppDocument,
) (preparedDPPUpdate, error) {
	mergedContent, err := selectedContentSubmodelsForHeader(merged, resolved.metadata.ID(), resolved.submodels)
	if err != nil {
		return preparedDPPUpdate{}, err
	}
	header.LastUpdate, err = timestampAfterDPPContent(header.LastUpdate, mergedContent)
	if err != nil {
		return preparedDPPUpdate{}, err
	}
	merged[headerLastUpdate] = header.LastUpdate.Format(time.RFC3339Nano)
	metadata, err := updatedDPPMetadata(resolved.metadata, current, merged, header)
	if err != nil {
		return preparedDPPUpdate{}, err
	}
	update := preparedDPPUpdate{
		descriptorAAS:       resolved.aas,
		submodels:           []types.ISubmodel{metadata},
		newSubmodelIDs:      make(map[string]struct{}),
		retainedSubmodelIDs: make(map[string]struct{}),
	}
	currentBySection, err := contentSubmodelsBySection(current, resolved.metadata.ID(), currentContent)
	if err != nil {
		return preparedDPPUpdate{}, err
	}
	mergedBySection, err := contentSubmodelsBySection(merged, resolved.metadata.ID(), mergedContent)
	if err != nil {
		return preparedDPPUpdate{}, err
	}
	detachedIDsBySection, err := contentSubmodelIDsBySection(current, resolved.metadata.ID(), resolved.submodels)
	if err != nil {
		return preparedDPPUpdate{}, err
	}
	planner := dppContentUpdatePlanner{
		service:              s,
		ctx:                  ctx,
		patch:                patch,
		mergedSections:       contentSections(merged),
		header:               header,
		current:              current,
		currentBySection:     currentBySection,
		mergedBySection:      mergedBySection,
		detachedIDsBySection: detachedIDsBySection,
		update:               &update,
	}
	if err = planner.plan(); err != nil {
		return preparedDPPUpdate{}, err
	}
	detachedSubmodelIDs := withoutRetainedDPPSubmodels(
		planner.detachedSubmodelIDs, update.submodels, update.retainedSubmodelIDs,
	)
	update.aas, err = updatedDPPAAS(resolved.aas, current, merged, detachedSubmodelIDs, update.newSubmodelIDs)
	if err != nil {
		return preparedDPPUpdate{}, err
	}
	return update, nil
}

type dppContentUpdatePlanner struct {
	service              *DPPRepositoryService
	ctx                  context.Context
	patch                dppDocument
	mergedSections       map[string]any
	header               dppHeader
	current              dppDocument
	currentBySection     map[string]types.ISubmodel
	mergedBySection      map[string]types.ISubmodel
	detachedIDsBySection map[string][]string
	detachedSubmodelIDs  []string
	update               *preparedDPPUpdate
}

func (p *dppContentUpdatePlanner) plan() error {
	for _, sectionName := range sortedKeys(contentSections(p.patch)) {
		if err := p.planSection(sectionName); err != nil {
			return err
		}
	}
	return nil
}

func (p *dppContentUpdatePlanner) planSection(sectionName string) error {
	currentSubmodel, currentValue, mergeFromCurrent, err := p.service.resolveDPPContentUpdateBase(
		p.ctx, sectionName, p.current, p.currentBySection, p.mergedBySection, p.update,
	)
	if err != nil {
		return err
	}
	mergedSection, exists := p.mergedSections[sectionName]
	if !exists {
		if currentSubmodel != nil {
			p.detachedSubmodelIDs = append(p.detachedSubmodelIDs, p.detachedIDsBySection[sectionName]...)
		}
		return nil
	}
	if currentSubmodel != nil {
		p.update.retainedSubmodelIDs[currentSubmodel.ID()] = struct{}{}
	}
	if mergeFromCurrent {
		mergedSection, err = mergeRemappedDPPContent(currentValue, p.patch[sectionName])
		if err != nil {
			return err
		}
	}
	if dppValuesEqual(currentValue, mergedSection) {
		return nil
	}
	if currentSubmodel == nil {
		return planNewDPPContentSubmodel(sectionName, mergedSection, p.mergedSections, p.header, p.update)
	}
	changed, err := updatedDPPContentSubmodel(
		currentSubmodel, currentValue, mergedSection, p.patch[sectionName], p.header.LastUpdate,
	)
	if err != nil {
		return err
	}
	if err = p.service.preserveManagedAttachments(
		p.ctx, []types.ISubmodel{changed}, []types.ISubmodel{currentSubmodel},
	); err != nil {
		return err
	}
	p.update.submodels = appendOrReplacePlannedDPPSubmodel(p.update.submodels, changed)
	return nil
}

func (s *DPPRepositoryService) resolveDPPContentUpdateBase(
	ctx context.Context,
	sectionName string,
	current dppDocument,
	currentBySection map[string]types.ISubmodel,
	mergedBySection map[string]types.ISubmodel,
	update *preparedDPPUpdate,
) (types.ISubmodel, any, bool, error) {
	submodel := currentBySection[sectionName]
	value := current[sectionName]
	remapped := false
	if submodel == nil {
		submodel = mergedBySection[sectionName]
		var err error
		value, err = s.serializedDPPContent(ctx, submodel)
		if err != nil {
			return nil, nil, false, err
		}
		remapped = submodel != nil
	}
	planned := plannedDPPSubmodel(update, submodel)
	if planned == nil {
		return submodel, value, remapped, nil
	}
	value, err := s.serializedDPPContent(ctx, planned)
	if err != nil {
		return nil, nil, false, err
	}
	return planned, value, true, nil
}

func plannedDPPSubmodel(update *preparedDPPUpdate, current types.ISubmodel) types.ISubmodel {
	if current == nil {
		return nil
	}
	for _, submodel := range update.submodels {
		if submodel.ID() == current.ID() {
			return submodel
		}
	}
	return nil
}

func appendOrReplacePlannedDPPSubmodel(planned []types.ISubmodel, submodel types.ISubmodel) []types.ISubmodel {
	for index, existing := range planned {
		if existing.ID() == submodel.ID() {
			planned[index] = submodel
			return planned
		}
	}
	return append(planned, submodel)
}

func planNewDPPContentSubmodel(
	sectionName string,
	section any,
	mergedSections map[string]any,
	header dppHeader,
	update *preparedDPPUpdate,
) error {
	semanticID, err := semanticIDForNewDPPSection(sectionName, section, mergedSections, header.ContentSpecificationIDs)
	if err != nil {
		return err
	}
	created, err := buildContentSubmodel(header.DigitalProductPassportID, sectionName, semanticID, section)
	if err != nil {
		return err
	}
	setNewDPPSubmodelAdministration(created, header.LastUpdate)
	update.submodels = append(update.submodels, created)
	update.newSubmodelIDs[created.ID()] = struct{}{}
	return nil
}

func semanticIDForNewDPPSection(
	sectionName string,
	section any,
	mergedSections map[string]any,
	contentSpecificationIDs []string,
) (string, error) {
	if len(contentSpecificationIDs) == 0 {
		return "", nil
	}
	for _, specificationID := range contentSpecificationIDs {
		if sectionName == specificationID {
			return specificationID, nil
		}
	}
	semanticID, explicit, err := explicitSemanticIDForSection(sectionName, section)
	if err != nil {
		return "", err
	}
	if explicit {
		return validateDPPSectionSemanticID(sectionName, semanticID, contentSpecificationIDs)
	}
	if len(mergedSections) == 1 && len(contentSpecificationIDs) == 1 {
		return contentSpecificationIDs[0], nil
	}
	return "", fmt.Errorf("DPP-SEMSPEC-EXPLICIT content sections must define dictionaryReference when multiple contentSpecificationIds or content sections are present")
}

func validateDPPSectionSemanticID(sectionName string, semanticID string, specificationIDs []string) (string, error) {
	for _, specificationID := range specificationIDs {
		if semanticID == specificationID {
			return semanticID, nil
		}
	}
	return "", fmt.Errorf("DPP-SEMSPEC-UNKNOWN section %s dictionaryReference must be listed in contentSpecificationIds", sectionName)
}

func (s *DPPRepositoryService) serializedDPPContent(ctx context.Context, submodel types.ISubmodel) (any, error) {
	if submodel == nil {
		return nil, nil
	}
	serializationContext := dppSerializationContext{}
	if submodelContainsFile(submodel) {
		var err error
		serializationContext, err = s.serializationContext(ctx, submodel.ID(), false)
		if err != nil {
			return nil, fmt.Errorf("DPP-UPDDPP-SERIALIZECONTEXT load submodel %s attachment paths: %w", submodel.ID(), err)
		}
	}
	content, err := compressedContentWithContext(submodel, serializationContext)
	if err != nil {
		return nil, fmt.Errorf("DPP-UPDDPP-SERIALIZECONTENT serialize submodel %s: %w", submodel.ID(), err)
	}
	return content, nil
}

func mergeRemappedDPPContent(current any, patch any) (any, error) {
	currentCopy, err := cloneDPPValue(current)
	if err != nil {
		return nil, fmt.Errorf("DPP-UPDDPP-CLONEREMAPPED clone remapped content: %w", err)
	}
	return applyMergePatch(currentCopy, patch), nil
}

func withoutRetainedDPPSubmodels(
	detached []string,
	planned []types.ISubmodel,
	retainedIDs map[string]struct{},
) []string {
	plannedIDs := make(map[string]struct{}, len(planned)+len(retainedIDs))
	for _, submodel := range planned {
		plannedIDs[submodel.ID()] = struct{}{}
	}
	for id := range retainedIDs {
		plannedIDs[id] = struct{}{}
	}
	result := make([]string, 0, len(detached))
	for _, id := range detached {
		if _, retained := plannedIDs[id]; !retained {
			result = append(result, id)
		}
	}
	return result
}

func timestampAfterDPPContent(candidate time.Time, content []types.ISubmodel) (time.Time, error) {
	latest := candidate
	for _, submodel := range content {
		timestamp, err := dppContentSubmodelTimestamp(submodel)
		if err != nil {
			return time.Time{}, err
		}
		latest = timestampAfter(latest, timestamp)
	}
	return latest, nil
}

func contentSubmodelIDsBySection(
	header dppDocument,
	metadataID string,
	submodels []types.ISubmodel,
) (map[string][]string, error) {
	specificationSet, err := contentSpecificationSetFromHeader(header)
	if err != nil {
		return nil, err
	}
	bySection := make(map[string][]string, len(submodels))
	for _, submodel := range submodels {
		if submodel.ID() == metadataID {
			continue
		}
		sectionName := compressedContentSectionName(submodel, specificationSet)
		bySection[sectionName] = append(bySection[sectionName], submodel.ID())
	}
	return bySection, nil
}

func contentSubmodelsBySection(
	header dppDocument,
	metadataID string,
	submodels []types.ISubmodel,
) (map[string]types.ISubmodel, error) {
	specificationSet, err := contentSpecificationSetFromHeader(header)
	if err != nil {
		return nil, err
	}
	bySection := make(map[string]types.ISubmodel, len(submodels))
	for _, submodel := range submodels {
		if submodel.ID() != metadataID {
			bySection[compressedContentSectionName(submodel, specificationSet)] = submodel
		}
	}
	return bySection, nil
}

func updatedDPPMetadata(
	current types.ISubmodel,
	currentDocument dppDocument,
	mergedDocument dppDocument,
	header dppHeader,
) (types.ISubmodel, error) {
	updated, err := cloneDPPSubmodel(current)
	if err != nil {
		return nil, fmt.Errorf("DPP-UPDDPP-CLONEMETADATA clone metadata: %w", err)
	}
	target := buildMetadataSubmodelWithID(current.ID(), header)
	if _, present := mergedDocument[headerContentSpecificationIDs]; present && len(header.ContentSpecificationIDs) == 0 {
		target.SetSubmodelElements(append(target.SubmodelElements(), metadataContentSpecificationIDs(nil)))
	}
	changedFields := changedDPPHeaderFields(currentDocument, mergedDocument)
	updated.SetSubmodelElements(reconcileMetadataElements(updated.SubmodelElements(), target.SubmodelElements(), changedFields))
	updated.SetAdministration(updatedDPPAdministration(current, header.LastUpdate))
	return updated, nil
}

func changedDPPHeaderFields(current dppDocument, merged dppDocument) map[string]struct{} {
	changed := map[string]struct{}{headerLastUpdate: {}}
	for field := range dppHeaderFields {
		if !dppValuesEqual(current[field], merged[field]) {
			changed[field] = struct{}{}
		}
	}
	return changed
}

func reconcileMetadataElements(
	current []types.ISubmodelElement,
	target []types.ISubmodelElement,
	changedFields map[string]struct{},
) []types.ISubmodelElement {
	targetByIDShort := elementsByIDShort(target)
	result := make([]types.ISubmodelElement, 0, len(current)+len(target))
	for _, element := range current {
		idShort := idShortValue(element)
		if _, changed := changedFields[idShort]; !changed {
			result = append(result, element)
			continue
		}
		replacement := targetByIDShort[idShort]
		if replacement != nil {
			result = append(result, reconcileDPPElement(element, replacement))
			delete(targetByIDShort, idShort)
		}
	}
	for _, element := range target {
		idShort := idShortValue(element)
		if _, changed := changedFields[idShort]; changed && targetByIDShort[idShort] != nil {
			result = append(result, element)
		}
	}
	return result
}

func updatedDPPContentSubmodel(
	current types.ISubmodel,
	currentValue any,
	mergedValue any,
	patchValue any,
	timestamp time.Time,
) (types.ISubmodel, error) {
	currentObject := dppObjectFromAny(currentValue)
	mergedObject := dppObjectFromAny(mergedValue)
	patchObject := dppObjectFromAny(patchValue)
	if currentObject == nil || mergedObject == nil || patchObject == nil {
		return nil, fmt.Errorf("DPP-UPDDPP-CONTENTTYPE content section for submodel %s must be a JSON object", current.ID())
	}
	updated, err := cloneDPPSubmodel(current)
	if err != nil {
		return nil, fmt.Errorf("DPP-UPDDPP-CLONECONTENT clone submodel %s: %w", current.ID(), err)
	}
	elements, err := applyDPPElementObjectPatch(updated.SubmodelElements(), currentObject, mergedObject, patchObject)
	if err != nil {
		return nil, err
	}
	updated.SetSubmodelElements(elements)
	updated.SetAdministration(updatedDPPAdministration(current, timestamp))
	return updated, nil
}

func applyDPPElementObjectPatch(
	elements []types.ISubmodelElement,
	current map[string]any,
	merged map[string]any,
	patch map[string]any,
) ([]types.ISubmodelElement, error) {
	result := make([]types.ISubmodelElement, 0, len(elements)+len(patch))
	result, err := updateExistingDPPElements(result, elements, current, merged, patch)
	if err != nil {
		return nil, err
	}
	return appendNewDPPElements(result, merged, patch, elementsByIDShort(elements))
}

func updateExistingDPPElements(
	result []types.ISubmodelElement,
	elements []types.ISubmodelElement,
	current map[string]any,
	merged map[string]any,
	patch map[string]any,
) ([]types.ISubmodelElement, error) {
	for _, element := range elements {
		idShort := idShortValue(element)
		patchValue, affected := patch[idShort]
		if !affected || dppValuesEqual(current[idShort], merged[idShort]) {
			result = append(result, element)
			continue
		}
		mergedValue, exists := merged[idShort]
		if !exists {
			continue
		}
		updated, err := updateDPPElement(element, current[idShort], mergedValue, patchValue)
		if err != nil {
			return nil, err
		}
		result = append(result, updated)
	}
	return result, nil
}

func appendNewDPPElements(
	result []types.ISubmodelElement,
	merged map[string]any,
	patch map[string]any,
	existing map[string]types.ISubmodelElement,
) ([]types.ISubmodelElement, error) {
	for _, idShort := range sortedKeys(patch) {
		if existing[idShort] != nil {
			continue
		}
		mergedValue, exists := merged[idShort]
		if !exists {
			continue
		}
		element, err := inferElement(idShort, mergedValue)
		if err != nil {
			return nil, err
		}
		result = append(result, element)
	}
	return result, nil
}

func updateDPPElement(current types.ISubmodelElement, currentValue any, mergedValue any, patchValue any) (types.ISubmodelElement, error) {
	if collection, ok := current.(*types.SubmodelElementCollection); ok {
		currentObject := dppObjectFromAny(currentValue)
		mergedObject := dppObjectFromAny(mergedValue)
		patchObject := dppObjectFromAny(patchValue)
		if currentObject != nil && mergedObject != nil && patchObject != nil && !isFileObject(mergedObject) {
			children, err := applyDPPElementObjectPatch(collection.Value(), currentObject, mergedObject, patchObject)
			if err != nil {
				return nil, err
			}
			collection.SetValue(children)
			return collection, nil
		}
	}
	if list, ok := current.(*types.SubmodelElementList); ok {
		if values, isArray := mergedValue.([]any); isArray && len(values) == 0 {
			list.SetValue([]types.ISubmodelElement{})
			return list, nil
		}
	}
	replacement, err := inferElement(idShortValue(current), mergedValue)
	if err != nil {
		return nil, err
	}
	return reconcileDPPElement(current, replacement), nil
}

func reconcileDPPElement(current types.ISubmodelElement, target types.ISubmodelElement) types.ISubmodelElement {
	switch currentTyped := current.(type) {
	case *types.Property:
		if targetTyped, ok := target.(*types.Property); ok {
			currentTyped.SetValue(targetTyped.Value())
			return currentTyped
		}
	case *types.MultiLanguageProperty:
		if targetTyped, ok := target.(*types.MultiLanguageProperty); ok {
			currentTyped.SetValue(targetTyped.Value())
			return currentTyped
		}
	case *types.File:
		if targetTyped, ok := target.(*types.File); ok {
			currentTyped.SetValue(targetTyped.Value())
			currentTyped.SetContentType(targetTyped.ContentType())
			currentTyped.SetExtensions(replaceDPPFileExtensions(currentTyped.Extensions(), targetTyped.Extensions()))
			return currentTyped
		}
	case *types.SubmodelElementCollection:
		if targetTyped, ok := target.(*types.SubmodelElementCollection); ok {
			currentTyped.SetValue(reconcileDPPElementSlices(currentTyped.Value(), targetTyped.Value()))
			return currentTyped
		}
	case *types.SubmodelElementList:
		if targetTyped, ok := target.(*types.SubmodelElementList); ok {
			currentTyped.SetValue(reconcileDPPListValues(currentTyped, targetTyped.Value()))
			return currentTyped
		}
	}
	preserveElementMetadata(current, target)
	return target
}

func reconcileDPPElementSlices(current []types.ISubmodelElement, target []types.ISubmodelElement) []types.ISubmodelElement {
	currentByIDShort := elementsByIDShort(current)
	result := make([]types.ISubmodelElement, 0, len(target))
	for _, targetElement := range target {
		currentElement := currentByIDShort[idShortValue(targetElement)]
		if currentElement == nil {
			result = append(result, targetElement)
			continue
		}
		result = append(result, reconcileDPPElement(currentElement, targetElement))
	}
	return result
}

func reconcileDPPListValues(current *types.SubmodelElementList, target []types.ISubmodelElement) []types.ISubmodelElement {
	result := make([]types.ISubmodelElement, 0, len(target))
	for index, targetElement := range target {
		if index >= len(current.Value()) {
			result = append(result, normalizedDPPListItem(current, targetElement))
			continue
		}
		result = append(result, reconcileDPPElement(current.Value()[index], targetElement))
	}
	return result
}

func normalizedDPPListItem(list *types.SubmodelElementList, element types.ISubmodelElement) types.ISubmodelElement {
	if property, ok := element.(*types.Property); ok && list.ValueTypeListElement() != nil {
		property.SetValueType(*list.ValueTypeListElement())
	}
	return element
}

func replaceDPPFileExtensions(current []types.IExtension, target []types.IExtension) []types.IExtension {
	result := make([]types.IExtension, 0, len(current)+len(target))
	for _, extension := range current {
		if extension.Name() != dppResourceTitleExtensionName && extension.Name() != dppLanguageExtensionName {
			result = append(result, extension)
		}
	}
	return append(result, target...)
}

func elementsByIDShort(elements []types.ISubmodelElement) map[string]types.ISubmodelElement {
	result := make(map[string]types.ISubmodelElement, len(elements))
	for _, element := range elements {
		result[idShortValue(element)] = element
	}
	return result
}

func updatedDPPAAS(
	current types.IAssetAdministrationShell,
	currentDocument dppDocument,
	mergedDocument dppDocument,
	detachedIDs []string,
	newSubmodelIDs map[string]struct{},
) (types.IAssetAdministrationShell, error) {
	productIDChanged := !dppValuesEqual(currentDocument[headerUniqueProductIdentifier], mergedDocument[headerUniqueProductIdentifier])
	granularityChanged := !dppValuesEqual(currentDocument[headerGranularity], mergedDocument[headerGranularity])
	if !productIDChanged && !granularityChanged && len(detachedIDs) == 0 && len(newSubmodelIDs) == 0 {
		return nil, nil
	}
	if current == nil {
		return nil, fmt.Errorf("DPP-UPDDPP-MISSINGAAS resolved DPP has no owner AAS")
	}
	updated, err := cloneDPPAAS(current)
	if err != nil {
		return nil, fmt.Errorf("DPP-UPDDPP-CLONEAAS clone AAS %s: %w", current.ID(), err)
	}
	if productIDChanged || granularityChanged {
		if err = applyMappedDPPHeaderToAAS(updated, mergedDocument, productIDChanged, granularityChanged); err != nil {
			return nil, err
		}
	}
	updated.SetSubmodels(updatedDPPSubmodelReferences(updated.Submodels(), detachedIDs, newSubmodelIDs))
	return updated, nil
}

func applyMappedDPPHeaderToAAS(
	aas types.IAssetAdministrationShell,
	document dppDocument,
	productIDChanged bool,
	granularityChanged bool,
) error {
	assetInformation := aas.AssetInformation()
	if assetInformation == nil {
		return fmt.Errorf("DPP-UPDDPP-ASSETINFO AAS %s has no assetInformation", aas.ID())
	}
	if productIDChanged {
		productID, ok := document[headerUniqueProductIdentifier].(string)
		if !ok || productID == "" {
			return fmt.Errorf("DPP-UPDDPP-PRODUCTID merged DPP has no uniqueProductIdentifier")
		}
		assetInformation.SetGlobalAssetID(&productID)
	}
	if granularityChanged {
		granularity, ok := document[headerGranularity].(string)
		if !ok || granularity == "" {
			return fmt.Errorf("DPP-UPDDPP-GRANULARITY merged DPP has no granularity")
		}
		assetInformation.SetAssetKind(granularityAssetKind(granularity))
	}
	return nil
}

func updatedDPPSubmodelReferences(
	current []types.IReference,
	detachedIDs []string,
	newSubmodelIDs map[string]struct{},
) []types.IReference {
	detached := make(map[string]struct{}, len(detachedIDs))
	for _, id := range detachedIDs {
		detached[id] = struct{}{}
	}
	result := make([]types.IReference, 0, len(current)+len(newSubmodelIDs))
	included := make(map[string]struct{}, len(current)+len(newSubmodelIDs))
	for _, reference := range current {
		id := referenceLastValue(reference)
		if _, remove := detached[id]; remove {
			continue
		}
		result = append(result, reference)
		included[id] = struct{}{}
	}
	newIDs := make([]string, 0, len(newSubmodelIDs))
	for id := range newSubmodelIDs {
		newIDs = append(newIDs, id)
	}
	sort.Strings(newIDs)
	for _, id := range newIDs {
		if _, exists := included[id]; !exists {
			result = append(result, submodelReference(id))
		}
	}
	return result
}

func cloneDPPDocument(document dppDocument) (dppDocument, error) {
	cloned, err := cloneDPPValue(document)
	if err != nil {
		return nil, err
	}
	return dppObjectFromAny(cloned), nil
}

func cloneDPPValue(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var cloned any
	err = decoder.Decode(&cloned)
	return cloned, err
}

func cloneDPPSubmodel(submodel types.ISubmodel) (types.ISubmodel, error) {
	jsonable, err := jsonization.ToJsonable(submodel)
	if err != nil {
		return nil, err
	}
	return jsonization.SubmodelFromJsonable(jsonable)
}

func cloneDPPAAS(aas types.IAssetAdministrationShell) (types.IAssetAdministrationShell, error) {
	jsonable, err := jsonization.ToJsonable(aas)
	if err != nil {
		return nil, err
	}
	return jsonization.AssetAdministrationShellFromJsonable(jsonable)
}

func dppValuesEqual(left any, right any) bool {
	leftJSON, leftErr := json.Marshal(normalizedDPPComparisonValue(left))
	rightJSON, rightErr := json.Marshal(normalizedDPPComparisonValue(right))
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func normalizedDPPComparisonValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			result[key] = normalizedDPPComparisonValue(child)
		}
		return result
	case dppDocument:
		return normalizedDPPComparisonValue(map[string]any(typed))
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = normalizedDPPComparisonValue(child)
		}
		return result
	case json.Number:
		return typed.String()
	case float64:
		return json.Number(fmt.Sprintf("%v", typed)).String()
	case bool:
		return fmt.Sprintf("%t", typed)
	default:
		return value
	}
}
