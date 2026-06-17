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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	basyxmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const (
	dppMetadataIDShort     = "DppMetadata"
	dppMetadataSemanticID  = "https://admin-shell.io/idta/cds/dppMetadata/1"
	dppMetadataSemanticURN = "urn:samm:io.admin-shell.idta.dpp_meta:1.0.0#DppMetadata"
)

var dppMetadataSemanticIDs = map[string]struct{}{
	dppMetadataSemanticID:  {},
	dppMetadataSemanticURN: {},
}

func buildAAS(header dppHeader, submodelRefs []types.IReference) types.IAssetAdministrationShell {
	assetInformation := types.NewAssetInformation(types.AssetKindInstance)
	assetInformation.SetGlobalAssetID(&header.UniqueProductIdentifier)

	aas := types.NewAssetAdministrationShell(header.DigitalProductPassportID, assetInformation)
	idShort := sanitizeIDShort(header.DigitalProductPassportID, "Dpp")
	aas.SetIDShort(&idShort)
	aas.SetSubmodels(submodelRefs)
	return aas
}

func buildMetadataSubmodel(dppID string, header dppHeader) types.ISubmodel {
	id := metadataSubmodelID(dppID)
	submodel := types.NewSubmodel(id)
	idShort := dppMetadataIDShort
	submodel.SetIDShort(&idShort)
	submodel.SetSemanticID(globalReference(dppMetadataSemanticID))
	submodel.SetSubmodelElements([]types.ISubmodelElement{
		stringProperty(headerDigitalProductPassportID, header.DigitalProductPassportID),
		stringProperty(headerUniqueProductIdentifier, header.UniqueProductIdentifier),
		stringProperty(headerGranularity, header.Granularity),
		stringProperty(headerDppSchemaVersion, header.DppSchemaVersion),
		stringProperty(headerDppStatus, header.DppStatus),
		stringProperty(headerLastUpdate, header.LastUpdate.UTC().Format(time.RFC3339Nano)),
		stringProperty(headerEconomicOperatorID, header.EconomicOperatorID),
		stringProperty(headerFacilityID, header.FacilityID),
		stringList(headerContentSpecificationIDs, header.ContentSpecificationIDs),
	})
	return submodel
}

func buildContentSubmodel(dppID string, sectionName string, semanticID string, value any) (types.ISubmodel, error) {
	submodel := types.NewSubmodel(contentSubmodelID(dppID, sectionName))
	idShort := upperFirst(sectionName)
	submodel.SetIDShort(&idShort)
	if semanticID != "" {
		submodel.SetSemanticID(globalReference(semanticID))
	}

	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("DPP-BUILDSM-CONTENTSECTION content section %s must be a JSON object", sectionName)
	}
	elements := make([]types.ISubmodelElement, 0, len(object))
	keys := sortedKeys(object)
	for _, key := range keys {
		element, err := inferElement(key, object[key])
		if err != nil {
			return nil, err
		}
		elements = append(elements, element)
	}
	submodel.SetSubmodelElements(elements)
	return submodel, nil
}

func inferElement(idShort string, value any) (types.ISubmodelElement, error) {
	switch typed := value.(type) {
	case map[string]any:
		if isFileObject(typed) {
			return fileElement(idShort, typed), nil
		}
		return collectionElement(idShort, typed)
	case []any:
		return listElement(idShort, typed)
	case string:
		return scalarProperty(idShort, typed, types.DataTypeDefXSDString), nil
	case bool:
		return scalarProperty(idShort, strconv.FormatBool(typed), types.DataTypeDefXSDBoolean), nil
	case json.Number:
		return numberProperty(idShort, typed), nil
	case nil:
		return scalarProperty(idShort, "", types.DataTypeDefXSDString), nil
	default:
		return scalarProperty(idShort, fmt.Sprint(typed), types.DataTypeDefXSDString), nil
	}
}

func collectionElement(idShort string, values map[string]any) (types.ISubmodelElement, error) {
	collection := types.NewSubmodelElementCollection()
	collection.SetIDShort(&idShort)
	keys := sortedKeys(values)
	elements := make([]types.ISubmodelElement, 0, len(keys))
	for _, key := range keys {
		element, err := inferElement(key, values[key])
		if err != nil {
			return nil, err
		}
		elements = append(elements, element)
	}
	collection.SetValue(elements)
	return collection, nil
}

func listElement(idShort string, values []any) (types.ISubmodelElement, error) {
	listType := types.AASSubmodelElementsProperty
	if len(values) > 0 {
		listType = listElementType(values[0])
	}
	list := types.NewSubmodelElementList(listType)
	list.SetIDShort(&idShort)

	elements := make([]types.ISubmodelElement, 0, len(values))
	for index, value := range values {
		element, err := inferElement("", value)
		if err != nil {
			return nil, err
		}
		if idShorter, ok := element.(interface{ SetIDShort(*string) }); ok {
			empty := ""
			if listType == types.AASSubmodelElementsSubmodelElementCollection {
				empty = fmt.Sprintf("%s%d", idShort, index)
			}
			idShorter.SetIDShort(&empty)
		}
		elements = append(elements, element)
	}
	list.SetValue(elements)
	return list, nil
}

func listElementType(value any) types.AASSubmodelElements {
	if object, ok := value.(map[string]any); ok {
		if isFileObject(object) {
			return types.AASSubmodelElementsFile
		}
		return types.AASSubmodelElementsSubmodelElementCollection
	}
	return types.AASSubmodelElementsProperty
}

func stringProperty(idShort string, value string) types.ISubmodelElement {
	return scalarProperty(idShort, value, types.DataTypeDefXSDString)
}

func scalarProperty(idShort string, value string, valueType types.DataTypeDefXSD) types.ISubmodelElement {
	property := types.NewProperty(valueType)
	property.SetIDShort(&idShort)
	property.SetValue(&value)
	return property
}

func numberProperty(idShort string, value json.Number) types.ISubmodelElement {
	if _, err := value.Int64(); err == nil {
		return scalarProperty(idShort, value.String(), types.DataTypeDefXSDLong)
	}
	return scalarProperty(idShort, value.String(), types.DataTypeDefXSDDouble)
}

func stringList(idShort string, values []string) types.ISubmodelElement {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	element, _ := listElement(idShort, items)
	return element
}

func fileElement(idShort string, values map[string]any) types.ISubmodelElement {
	file := types.NewFile()
	file.SetIDShort(&idShort)
	if url, ok := values["url"].(string); ok {
		file.SetValue(&url)
	}
	if contentType, ok := values["contentType"].(string); ok {
		file.SetContentType(&contentType)
	}
	return file
}

func isFileObject(values map[string]any) bool {
	_, hasURL := values["url"].(string)
	_, hasContentType := values["contentType"].(string)
	return hasURL && hasContentType
}

func metadataSubmodelID(dppID string) string {
	return dppID + "/submodels/DppMetadata"
}

func contentSubmodelID(dppID string, sectionName string) string {
	return dppID + "/submodels/" + upperFirst(sectionName)
}

func submodelReference(submodelID string) types.IReference {
	return types.NewReference(types.ReferenceTypesModelReference, []types.IKey{
		types.NewKey(types.KeyTypesSubmodel, submodelID),
	})
}

func globalReference(value string) types.IReference {
	return types.NewReference(types.ReferenceTypesExternalReference, []types.IKey{
		types.NewKey(types.KeyTypesGlobalReference, value),
	})
}

func referenceLastValue(ref types.IReference) string {
	if ref == nil || len(ref.Keys()) == 0 {
		return ""
	}
	return ref.Keys()[len(ref.Keys())-1].Value()
}

func hasDPPMetadataSemanticID(submodel types.ISubmodel) bool {
	if submodel == nil || submodel.SemanticID() == nil {
		return false
	}
	_, ok := dppMetadataSemanticIDs[referenceLastValue(submodel.SemanticID())]
	return ok
}

func composeHeader(metadata types.ISubmodel) (dppDocument, error) {
	valueOnly, err := basyxmodel.SubmodelToValueOnly(metadata)
	if err != nil {
		return nil, fmt.Errorf("DPP-COMPOSE-METAVALUE convert metadata value-only: %w", err)
	}
	raw, err := json.Marshal(valueOnly)
	if err != nil {
		return nil, fmt.Errorf("DPP-COMPOSE-METAMARSHAL marshal metadata value-only: %w", err)
	}
	var header dppDocument
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, fmt.Errorf("DPP-COMPOSE-METAUNMARSHAL unmarshal metadata value-only: %w", err)
	}
	normalizeValueOnly(header)
	return header, nil
}

func compressedContent(submodel types.ISubmodel) (any, error) {
	valueOnly, err := basyxmodel.SubmodelToValueOnly(submodel)
	if err != nil {
		return nil, fmt.Errorf("DPP-CONTENT-COMPRESSED convert submodel value-only: %w", err)
	}
	raw, err := json.Marshal(valueOnly)
	if err != nil {
		return nil, fmt.Errorf("DPP-CONTENT-MARSHAL marshal submodel value-only: %w", err)
	}
	var content any
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, fmt.Errorf("DPP-CONTENT-UNMARSHAL unmarshal submodel value-only: %w", err)
	}
	normalizeValueOnly(content)
	return content, nil
}

func fullContent(submodel types.ISubmodel) (any, error) {
	jsonable, err := jsonization.ToJsonable(submodel)
	if err != nil {
		return nil, fmt.Errorf("DPP-CONTENT-FULL convert submodel normal serialization: %w", err)
	}
	return aasNormalToDPPExpanded(jsonable), nil
}

func normalizeValueOnly(value any) {
	switch typed := value.(type) {
	case map[string]any:
		if rawValue, ok := typed["value"]; ok && typed["contentType"] != nil {
			typed["url"] = rawValue
			delete(typed, "value")
		}
		for _, child := range typed {
			normalizeValueOnly(child)
		}
	case []any:
		for index, child := range typed {
			if langMap, ok := child.(map[string]any); ok && len(langMap) == 1 {
				for language, text := range langMap {
					typed[index] = map[string]any{"language": language, "value": text}
				}
			}
			normalizeValueOnly(typed[index])
		}
	}
}

func aasNormalToDPPExpanded(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			switch key {
			case "idShort":
				result["elementId"] = child
			case "modelType":
				result["objectType"] = child
			case "semanticId":
				result["dictionaryReference"] = referenceJSONToString(child)
			case "valueType":
				result["valueDataType"] = aasValueTypeToXSD(child)
			case "submodelElements":
				result["elements"] = aasNormalToDPPExpanded(child)
			case "contentType":
				result[key] = child
			case "value":
				result[key] = aasNormalToDPPExpanded(child)
			default:
				result[key] = aasNormalToDPPExpanded(child)
			}
		}
		return result
	case []any:
		items := make([]any, 0, len(typed))
		for _, child := range typed {
			items = append(items, aasNormalToDPPExpanded(child))
		}
		return items
	default:
		return value
	}
}

func referenceJSONToString(value any) any {
	object, ok := value.(map[string]any)
	if !ok {
		return value
	}
	keys, ok := object["keys"].([]any)
	if !ok || len(keys) == 0 {
		return value
	}
	lastKey, ok := keys[len(keys)-1].(map[string]any)
	if !ok {
		return value
	}
	if keyValue, ok := lastKey["value"].(string); ok {
		return keyValue
	}
	return value
}

func aasValueTypeToXSD(value any) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	text = strings.TrimPrefix(text, "xs:")
	text = strings.TrimPrefix(text, "xsd:")
	return "xsd:" + strings.ToLower(text[:1]) + text[1:]
}

func semanticIDForSection(sectionName string, contentSpecificationIDs []string) (string, error) {
	if len(contentSpecificationIDs) == 0 {
		return "", nil
	}
	normalized := strings.ToLower(sectionName)
	var matches []string
	for _, id := range contentSpecificationIDs {
		candidate := strings.ToLower(strings.TrimSpace(id))
		if strings.HasPrefix(candidate, normalized) || strings.Contains(candidate, normalized+" ") || strings.Contains(candidate, normalized+"-") {
			matches = append(matches, id)
		}
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("DPP-SEMSPEC-AMBIGUOUS contentSpecificationIds are ambiguous for section %s", sectionName)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(contentSpecificationIDs) == 1 {
		return contentSpecificationIDs[0], nil
	}
	return "", fmt.Errorf("DPP-SEMSPEC-MISSING no contentSpecificationId matches section %s", sectionName)
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sanitizeIDShort(value string, fallback string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			_, _ = builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return fallback
	}
	return builder.String()
}
