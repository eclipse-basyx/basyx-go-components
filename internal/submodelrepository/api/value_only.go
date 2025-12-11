/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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
// Author: Jannik Fried (Fraunhofer IESE)

package api

import (
	"fmt"
	"reflect"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
)

// SubmodelElementToValueOnly converts a SubmodelElement to its value-only representation
// according to the AAS specification IDTA-01001.
//
// Value-only serialization is a simplified representation that focuses on the actual values
// rather than the complete metadata structure. It provides a more compact and human-readable
// format suitable for data exchange scenarios where metadata is not required.
//
// The conversion follows these principles:
//   - Properties: Return the raw value as a string
//   - MultiLanguageProperty: Return array of {language: text} objects
//   - Range: Return {min, max} object
//   - File/Blob: Return {contentType, value} object
//   - References: Return {type, keys} structure
//   - Collections: Return nested map of idShort to values
//   - Lists: Return array preserving element order and indices
//   - Entity: Return {entityType, globalAssetId, specificAssetIds, statements} object
//   - Relationships: Return {first, second} references, optionally with annotations
//   - Operations and nil/empty elements: Return nil
//
// Returns nil for elements without meaningful values (Operations, empty collections, etc.)
// to optimize the serialized output size.
func SubmodelElementToValueOnly(element gen.SubmodelElement) interface{} {
	switch e := element.(type) {
	case *gen.Property:
		return e.ToValueOnly()
	case *gen.MultiLanguageProperty:
		return e.ToValueOnly()
	case *gen.Range:
		return e.ToValueOnly()
	case *gen.File:
		return e.ToValueOnly()
	case *gen.Blob:
		return e.ToValueOnly()
	case *gen.ReferenceElement:
		return e.ToValueOnly(referenceToInterface)
	case *gen.RelationshipElement:
		return e.ToValueOnly(referenceToInterface)
	case *gen.AnnotatedRelationshipElement:
		return e.ToValueOnly(referenceToInterface, serializeAnnotations)
	case *gen.SubmodelElementCollection:
		return e.ToValueOnly(serializeElements)
	case *gen.SubmodelElementList:
		return e.ToValueOnly(serializeElementsList)
	case *gen.Entity:
		return e.ToValueOnly(serializeElements)
	case *gen.BasicEventElement:
		return e.ToValueOnly(referenceToInterface)
	default:
		// For unknown types, return nil
		return nil
	}
}

// referenceToInterface wraps ReferenceToValueOnly to match the expected signature.
func referenceToInterface(ref gen.Reference) interface{} {
	return ReferenceToValueOnly(ref)
}

// serializeElements converts a slice of SubmodelElements to a value-only map representation.
// Used by SubmodelElementCollection and Entity.
func serializeElements(elements []gen.SubmodelElement) interface{} {
	result := make(map[string]interface{})
	for _, elem := range elements {
		if value := SubmodelElementToValueOnly(elem); value != nil {
			result[elem.GetIdShort()] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// serializeElementsList converts a slice of SubmodelElements to a value-only array representation.
// Used by SubmodelElementList. Preserves nil values to maintain array indices.
func serializeElementsList(elements []gen.SubmodelElement) interface{} {
	result := make([]interface{}, 0, len(elements))
	for _, elem := range elements {
		result = append(result, SubmodelElementToValueOnly(elem))
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// serializeAnnotations converts annotation SubmodelElements to a value-only map representation.
// Used by AnnotatedRelationshipElement.
func serializeAnnotations(annotations []gen.SubmodelElement) interface{} {
	annMap := make(map[string]interface{})
	for _, annotation := range annotations {
		if annValue := SubmodelElementToValueOnly(annotation); annValue != nil {
			annMap[annotation.GetIdShort()] = annValue
		}
	}
	if len(annMap) > 0 {
		return annMap
	}
	return nil
}

// ReferenceToValueOnly converts a Reference to its value-only representation.
//
// A reference in value-only format includes:
//   - type: The reference type (e.g., "ModelReference", "ExternalReference")
//   - keys: Array of key objects, each containing type and value
//   - referredSemanticId: Optional nested reference for semantic context
//
// Example output:
//
//	{
//	  "type": "ModelReference",
//	  "keys": [
//	    {"type": "Submodel", "value": "urn:example:submodel:123"}
//	  ]
//	}
func ReferenceToValueOnly(ref gen.Reference) map[string]interface{} {
	result := make(map[string]interface{})
	result["type"] = ref.Type

	if len(ref.Keys) > 0 {
		keys := make([]map[string]interface{}, len(ref.Keys))
		for i, key := range ref.Keys {
			keys[i] = map[string]interface{}{
				"type":  key.Type,
				"value": key.Value,
			}
		}
		result["keys"] = keys
	}

	if ref.ReferredSemanticID != nil {
		result["referredSemanticId"] = ReferenceToValueOnly(*ref.ReferredSemanticID)
	}

	return result
}

// SubmodelToValueOnly converts a Submodel to its value-only representation
// according to the AAS specification IDTA-01001.
//
// The resulting map contains the submodel's elements indexed by their idShort,
// with each element converted to its value-only form. Elements without meaningful
// values (nil, empty, or Operations) are excluded from the output.
//
// Parameters:
//   - submodel: The complete Submodel object to convert
//
// Returns:
//   - A map where keys are element idShorts and values are the value-only representations
//
// Example output:
//
//	{
//	  "Temperature": "25.5",
//	  "Unit": [{"en": "Celsius"}],
//	  "Range": {"min": "0", "max": "100"}
//	}
func SubmodelToValueOnly(submodel gen.Submodel) map[string]interface{} {
	result := make(map[string]interface{})

	for _, element := range submodel.SubmodelElements {
		value := SubmodelElementToValueOnly(element)
		if value != nil {
			result[element.GetIdShort()] = value
		}
	}

	return result
}

// SubmodelsToValueOnly converts a slice of Submodels to their value-only representations.
//
// This is a batch conversion function useful for APIs that return multiple submodels.
// Each submodel is independently converted using SubmodelToValueOnly.
//
// Parameters:
//   - submodels: Slice of Submodel objects to convert
//
// Returns:
//   - Slice of maps, each representing one submodel in value-only format
func SubmodelsToValueOnly(submodels []gen.Submodel) []map[string]interface{} {
	result := make([]map[string]interface{}, len(submodels))

	for i, submodel := range submodels {
		result[i] = SubmodelToValueOnly(submodel)
	}

	return result
}

// UpdateSubmodelFromValueOnly updates a submodel with values from a value-only representation.
//
// This function performs partial updates: only elements present in the valueOnly map are updated.
// Elements not included in the map remain unchanged. The function modifies the submodel in place.
//
// The update process:
//  1. Iterates through all submodel elements
//  2. Looks for matching idShort in the valueOnly map
//  3. Updates found elements using UpdateSubmodelElementFromValueOnly
//  4. Skips elements not present in the map (no-op)
//
// Parameters:
//   - submodel: Pointer to the Submodel to update (modified in place)
//   - valueOnly: Map of idShort to value-only representations
//
// Returns:
//   - error: If any element update fails, returns detailed error with element idShort
//
// Example:
//
//	err := UpdateSubmodelFromValueOnly(&mySubmodel, map[string]interface{}{
//	    "Temperature": "30.0",
//	    "Pressure": {"min": "1", "max": "10"},
//	})
func UpdateSubmodelFromValueOnly(submodel *gen.Submodel, valueOnly map[string]interface{}) error {
	for _, element := range submodel.SubmodelElements {
		idShort := element.GetIdShort()
		if value, exists := valueOnly[idShort]; exists {
			if err := UpdateSubmodelElementFromValueOnly(element, value); err != nil {
				return fmt.Errorf("failed to update element %s: %w", idShort, err)
			}
		}
	}
	return nil
}

// UpdateSubmodelElementFromValueOnly updates a submodel element with a value from its value-only representation.
//
// This is the core deserialization function that converts value-only data back into strongly-typed
// SubmodelElement structures. It performs type-safe conversion with validation.
//
// Supported element types and their expected value formats:
//   - Property: string value
//   - MultiLanguageProperty: array of {language: text} objects
//   - Range: {min: string, max: string} object
//   - File/Blob: {contentType: string, value: string} object
//   - ReferenceElement: {type: string, keys: [...]} object
//   - RelationshipElement: {first: {...}, second: {...}} object
//   - AnnotatedRelationshipElement: {first: {...}, second: {...}, annotations: {...}} object
//   - SubmodelElementCollection: {idShort: value, ...} object
//   - SubmodelElementList: [value1, value2, ...] array
//   - Entity: {entityType: string, globalAssetId: string, ...} object
//   - BasicEventElement: {observed: {...}} object
//
// Parameters:
//   - element: The SubmodelElement to update (modified in place)
//   - value: The value-only representation (from JSON deserialization)
//
// Returns:
//   - error: Type mismatch errors, conversion errors, or "unsupported element type" for unknown types
func UpdateSubmodelElementFromValueOnly(element gen.SubmodelElement, value interface{}) error {
	switch e := element.(type) {
	case *gen.Property:
		return e.UpdateFromValueOnly(value)
	case *gen.MultiLanguageProperty:
		return e.UpdateFromValueOnly(value)
	case *gen.Range:
		return e.UpdateFromValueOnly(value)
	case *gen.File:
		return e.UpdateFromValueOnly(value)
	case *gen.Blob:
		return e.UpdateFromValueOnly(value)
	case *gen.ReferenceElement:
		return e.UpdateFromValueOnly(value, ValueOnlyToReferencePtr)
	case *gen.RelationshipElement:
		return e.UpdateFromValueOnly(value, ValueOnlyToReferencePtr)
	case *gen.AnnotatedRelationshipElement:
		return updateAnnotatedRelationshipElement(e, value)
	case *gen.SubmodelElementCollection:
		return updateSubmodelElementCollection(e, value)
	case *gen.SubmodelElementList:
		return updateSubmodelElementList(e, value)
	case *gen.Entity:
		return updateEntity(e, value)
	case *gen.BasicEventElement:
		return e.UpdateFromValueOnly(value, ValueOnlyToReferencePtr)
	case *gen.Capability:
		return e.UpdateFromValueOnly(value)
	case *gen.Operation:
		return e.UpdateFromValueOnly(value)
	default:
		return fmt.Errorf("unsupported element type: %T", reflect.TypeOf(element))
	}
}

// updateRelationshipReferences updates the first and second references of a relationship element.
// This shared function is used by both RelationshipElement and AnnotatedRelationshipElement.
// Updates are performed in-place via double pointers. Returns error if reference conversion fails.
func updateRelationshipReferences(first, second **gen.Reference, relMap map[string]interface{}) error {
	if firstMap, exists := relMap["first"]; exists {
		if firstRefMap, ok := firstMap.(map[string]interface{}); ok {
			firstRef, err := ValueOnlyToReference(firstRefMap)
			if err != nil {
				return fmt.Errorf("failed to convert first reference: %w", err)
			}
			*first = &firstRef
		}
	}
	if secondMap, exists := relMap["second"]; exists {
		if secondRefMap, ok := secondMap.(map[string]interface{}); ok {
			secondRef, err := ValueOnlyToReference(secondRefMap)
			if err != nil {
				return fmt.Errorf("failed to convert second reference: %w", err)
			}
			*second = &secondRef
		}
	}
	return nil
}

// updateAnnotatedRelationshipElement updates an AnnotatedRelationshipElement from a value-only representation.
// Expects a map with "first", "second" references, and optional "annotations" map.
// Annotations are recursively updated by matching idShort keys.
func updateAnnotatedRelationshipElement(e *gen.AnnotatedRelationshipElement, value interface{}) error {
	relMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid value type for AnnotatedRelationshipElement: expected map, got %T", value)
	}

	// Update first and second references
	if err := updateRelationshipReferences(&e.First, &e.Second, relMap); err != nil {
		return err
	}

	// Update annotations
	if annotationsMap, exists := relMap["annotations"]; exists {
		if annMap, ok := annotationsMap.(map[string]interface{}); ok {
			for _, ann := range e.Annotations {
				annIDShort := ann.GetIdShort()
				if annValue, exists := annMap[annIDShort]; exists {
					if err := UpdateSubmodelElementFromValueOnly(ann, annValue); err != nil {
						return fmt.Errorf("failed to update annotation %s: %w", annIDShort, err)
					}
				}
			}
		}
	}
	return nil
}

// updateSubmodelElementCollection updates a SubmodelElementCollection from a value-only representation.
// Expects a map where keys are element idShorts. Elements are updated recursively in place.
func updateSubmodelElementCollection(e *gen.SubmodelElementCollection, value interface{}) error {
	collectionMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid value type for SubmodelElementCollection: expected map, got %T", value)
	}

	for _, elem := range e.Value {
		elemIDShort := elem.GetIdShort()
		if elemValue, exists := collectionMap[elemIDShort]; exists {
			if err := UpdateSubmodelElementFromValueOnly(elem, elemValue); err != nil {
				return fmt.Errorf("failed to update collection element %s: %w", elemIDShort, err)
			}
		}
	}
	return nil
}

// updateSubmodelElementList updates a SubmodelElementList from a value-only representation.
// The incoming array can have any length:
// - If shorter than existing list: only update the provided elements, keep the rest unchanged
// - If same length: update all elements in place
// - If longer than existing list: update existing elements and append new ones
// Note: To remove elements, provide an array with fewer elements than the current list
func updateSubmodelElementList(e *gen.SubmodelElementList, value interface{}) error {
	listSlice, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("invalid value type for SubmodelElementList: expected slice, got %T", value)
	}

	// Update existing elements up to the length of the incoming array
	minLen := len(e.Value)
	if len(listSlice) < minLen {
		minLen = len(listSlice)
	}

	for i := 0; i < minLen; i++ {
		elemValue := listSlice[i]
		// Skip nil values to preserve existing element at this index
		if elemValue == nil {
			continue
		}
		if err := UpdateSubmodelElementFromValueOnly(e.Value[i], elemValue); err != nil {
			return fmt.Errorf("failed to update list element %d: %w", i, err)
		}
	}

	// If incoming array is longer, append new elements
	// Note: We need to create new SubmodelElements from the value-only representation
	// This is a limitation - we can only update existing elements, not add new ones
	// because we don't know what type of SubmodelElement to create
	if len(listSlice) > len(e.Value) {
		return fmt.Errorf("cannot add new elements to SubmodelElementList via value-only update: incoming array has %d elements but list has %d. Use the full API to add elements", len(listSlice), len(e.Value))
	}

	return nil
}

// updateEntity updates an Entity from a value-only representation.
// Expects a map with optional fields: entityType, globalAssetId, specificAssetIds, statements.
// Statements are recursively updated by matching idShort keys.
func updateEntity(e *gen.Entity, value interface{}) error {
	entityMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid value type for Entity: expected map, got %T", value)
	}

	// Update entityType
	if entityType, exists := entityMap["entityType"]; exists {
		if et, ok := entityType.(gen.EntityType); ok {
			e.EntityType = et
		} else if etStr, ok := entityType.(string); ok {
			e.EntityType = gen.EntityType(etStr)
		}
	}

	// Update globalAssetId
	if globalAssetID, exists := entityMap["globalAssetId"]; exists {
		if gid, ok := globalAssetID.(string); ok {
			e.GlobalAssetID = gid
		}
	}

	// Update specificAssetIds
	if err := updateSpecificAssetIDs(e, entityMap); err != nil {
		return err
	}

	// Update statements
	if statementsMap, exists := entityMap["statements"]; exists {
		if stmtMap, ok := statementsMap.(map[string]interface{}); ok {
			for _, stmt := range e.Statements {
				stmtIDShort := stmt.GetIdShort()
				if stmtValue, exists := stmtMap[stmtIDShort]; exists {
					if err := UpdateSubmodelElementFromValueOnly(stmt, stmtValue); err != nil {
						return fmt.Errorf("failed to update entity statement %s: %w", stmtIDShort, err)
					}
				}
			}
		}
	}
	return nil
}

// updateSpecificAssetIDs updates the SpecificAssetIds field of an Entity.
// Handles both []interface{} (from JSON) and []gen.SpecificAssetID (direct type).
func updateSpecificAssetIDs(e *gen.Entity, entityMap map[string]interface{}) error {
	specificAssetIDs, exists := entityMap["specificAssetIds"]
	if !exists {
		return nil
	}

	sidsSlice, ok := specificAssetIDs.([]interface{})
	if !ok {
		if sids, ok := specificAssetIDs.([]gen.SpecificAssetID); ok {
			e.SpecificAssetIds = sids
		}
		return nil
	}

	var sids []gen.SpecificAssetID
	for _, sidRaw := range sidsSlice {
		sidMap, ok := sidRaw.(map[string]interface{})
		if !ok {
			continue
		}

		sid := gen.SpecificAssetID{}
		if name, ok := sidMap["name"].(string); ok {
			sid.Name = name
		}
		if value, ok := sidMap["value"].(string); ok {
			sid.Value = value
		}

		// Parse optional ExternalSubjectID
		if extSubjID, exists := sidMap["externalSubjectId"]; exists {
			if extSubjIDMap, ok := extSubjID.(map[string]interface{}); ok {
				if extSubjRef, err := ValueOnlyToReference(extSubjIDMap); err == nil {
					sid.ExternalSubjectID = &extSubjRef
				}
			}
		}

		// Parse optional SemanticID
		if semID, exists := sidMap["semanticId"]; exists {
			if semIDMap, ok := semID.(map[string]interface{}); ok {
				if semRef, err := ValueOnlyToReference(semIDMap); err == nil {
					sid.SemanticID = &semRef
				}
			}
		}

		sids = append(sids, sid)
	}
	e.SpecificAssetIds = sids
	return nil
}

// ValueOnlyToReference converts a value-only reference representation back to a Reference.
//
// This function handles the deserialization of reference objects from JSON, where types
// are represented as strings rather than strongly-typed enums. It gracefully handles both
// direct enum values and string representations.
//
// Expected input format:
//
//	{
//	  "type": "ModelReference",
//	  "keys": [
//	    {"type": "Submodel", "value": "urn:example:123"}
//	  ],
//	  "referredSemanticId": { ... }  // Optional nested reference
//	}
//
// Parameters:
//   - refMap: Map containing the reference data from JSON deserialization
//
// Returns:
//   - gen.Reference: The reconstructed Reference object
//   - error: If nested referredSemanticId conversion fails
func ValueOnlyToReference(refMap map[string]interface{}) (gen.Reference, error) {
	ref := gen.Reference{}

	if refType, exists := refMap["type"]; exists {
		if rt, ok := refType.(gen.ReferenceTypes); ok {
			ref.Type = rt
		} else if rtStr, ok := refType.(string); ok {
			ref.Type = gen.ReferenceTypes(rtStr)
		}
	}

	if keys, exists := refMap["keys"]; exists {
		if keysSlice, ok := keys.([]interface{}); ok {
			ref.Keys = make([]gen.Key, len(keysSlice))
			for i, keyMap := range keysSlice {
				if km, ok := keyMap.(map[string]interface{}); ok {
					key := gen.Key{}
					if keyType, exists := km["type"]; exists {
						if kt, ok := keyType.(gen.KeyTypes); ok {
							key.Type = kt
						} else if ktStr, ok := keyType.(string); ok {
							key.Type = gen.KeyTypes(ktStr)
						}
					}
					if keyValue, exists := km["value"]; exists {
						if kv, ok := keyValue.(string); ok {
							key.Value = kv
						}
					}
					ref.Keys[i] = key
				}
			}
		}
	}

	if referredSemanticID, exists := refMap["referredSemanticId"]; exists {
		if rsidMap, ok := referredSemanticID.(map[string]interface{}); ok {
			rsid, err := ValueOnlyToReference(rsidMap)
			if err != nil {
				return ref, err
			}
			ref.ReferredSemanticID = &rsid
		}
	}

	return ref, nil
}

// SubmodelElementValueToValueOnly converts a SubmodelElementValue back to a value-only representation.
//
// This is a specialized conversion function used for PATCH endpoints that accept typed
// SubmodelElementValue payloads. It bridges the gap between the API's SubmodelElementValue
// type and the internal value-only representation used by UpdateSubmodelElementFromValueOnly.
//
// The function performs intelligent detection of the element type based on which fields
// are populated in the SubmodelElementValue struct:
//   - Observed field present → BasicEventElement
//   - ContentType field present → File or Blob
//   - Value field present → Property
//   - Min/Max fields present → Range
//   - First/Second fields present → Relationship or AnnotatedRelationship
//   - EntityType field present → Entity
//
// Parameters:
//   - val: The SubmodelElementValue from the API request
//
// Returns:
//   - interface{}: The value-only representation, or nil if no fields are populated
func SubmodelElementValueToValueOnly(val gen.SubmodelElementValue) interface{} {
	// Check if there are fields that indicate this is NOT a simple Property value
	// These fields belong to RelationshipElements, Events, Entities, or Ranges
	hasNonPropertyFields := !openapi.IsZeroValue(val.First) || !openapi.IsZeroValue(val.Second) ||
		!openapi.IsZeroValue(val.Observed) || !openapi.IsZeroValue(val.Min) || !openapi.IsZeroValue(val.Max) ||
		val.EntityType != "" || len(val.Annotations) > 0

	// For simple string values (Property) - prioritize this if no contentType or if other fields are present
	// If value field exists AND (no contentType OR other non-Property fields exist), treat as Property
	if val.Value != "" && (val.ContentType == "" || hasNonPropertyFields) {
		return val.Value
	}

	// For File/Blob (contentType + value, and no other significant fields)
	if val.ContentType != "" && !hasNonPropertyFields {
		result := map[string]interface{}{
			"contentType": val.ContentType,
		}
		if val.Value != "" {
			result["value"] = val.Value
		}
		return result
	}

	// For BasicEventElement (observed)
	if !openapi.IsZeroValue(val.Observed) {
		return map[string]interface{}{
			"observed": ReferenceValueToValueOnly(val.Observed),
		}
	}

	// For ranges
	if !openapi.IsZeroValue(val.Min) || !openapi.IsZeroValue(val.Max) {
		res := make(map[string]interface{})
		if !openapi.IsZeroValue(val.Min) {
			res["min"] = val.Min
		}
		if !openapi.IsZeroValue(val.Max) {
			res["max"] = val.Max
		}
		return res
	}

	// For relationships (first/second)
	if !openapi.IsZeroValue(val.First) || !openapi.IsZeroValue(val.Second) {
		result := make(map[string]interface{})
		if !openapi.IsZeroValue(val.First) {
			result["first"] = ReferenceValueToValueOnly(val.First)
		}
		if !openapi.IsZeroValue(val.Second) {
			result["second"] = ReferenceValueToValueOnly(val.Second)
		}
		// For AnnotatedRelationshipElement
		if len(val.Annotations) > 0 {
			result["annotations"] = val.Annotations
		}
		return result
	}

	// For entity representation
	if val.EntityType != "" || val.GlobalAssetID != "" {
		m := make(map[string]interface{})
		if len(val.Statements) > 0 {
			m["statements"] = val.Statements
		}
		if val.GlobalAssetID != "" {
			m["globalAssetId"] = val.GlobalAssetID
		}
		if len(val.SpecificAssetIds) > 0 {
			m["specificAssetIds"] = val.SpecificAssetIds
		}
		if val.EntityType != "" {
			m["entityType"] = val.EntityType
		}
		return m
	}

	// Nothing matched -> return nil so the update does nothing
	return nil
}

// ReferenceValueToValueOnly converts ReferenceValue to the value-only representation used by the updater.
//
// This helper function is used by SubmodelElementValueToValueOnly to convert reference fields
// from the API's ReferenceValue type to the map-based value-only format.
//
// Parameters:
//   - rv: The ReferenceValue from the API request
//
// Returns:
//   - map[string]interface{}: The value-only reference representation
func ReferenceValueToValueOnly(rv gen.ReferenceValue) map[string]interface{} {
	res := make(map[string]interface{})
	if rv.Type != "" {
		res["type"] = rv.Type
	}
	if len(rv.Keys) > 0 {
		keys := make([]map[string]interface{}, len(rv.Keys))
		for i, k := range rv.Keys {
			keys[i] = map[string]interface{}{"type": k.Type, "value": k.Value}
		}
		res["keys"] = keys
	}
	return res
}

// ValueOnlyToReferencePtr is a wrapper that converts a value-only representation to a Reference pointer.
// Used as a parameter for UpdateFromValueOnly methods that require reference deserialization.
func ValueOnlyToReferencePtr(value interface{}) (*gen.Reference, error) {
	refMap, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid reference value: expected map, got %T", value)
	}
	ref, err := ValueOnlyToReference(refMap)
	if err != nil {
		return nil, err
	}
	return &ref, nil
}
