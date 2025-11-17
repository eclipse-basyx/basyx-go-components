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
func SubmodelElementToValueOnly(element gen.SubmodelElement) interface{} {
	switch e := element.(type) {
	case *gen.Property:
		if e.Value != "" {
			return e.Value
		}
		return nil
	case *gen.MultiLanguageProperty:
		if e.Value != nil {
			return e.Value
		}
		return nil
	case *gen.Range:
		return map[string]interface{}{
			"min": e.Min,
			"max": e.Max,
		}
	case *gen.File:
		return e.Value
	case *gen.Blob:
		return e.Value
	case *gen.ReferenceElement:
		if e.Value != nil {
			return ReferenceToValueOnly(*e.Value)
		}
		return nil
	case *gen.RelationshipElement:
		return map[string]interface{}{
			"first":  ReferenceToValueOnly(*e.First),
			"second": ReferenceToValueOnly(*e.Second),
		}
	case *gen.AnnotatedRelationshipElement:
		result := map[string]interface{}{
			"first":  ReferenceToValueOnly(*e.First),
			"second": ReferenceToValueOnly(*e.Second),
		}
		if len(e.Annotations) > 0 {
			annotations := make(map[string]interface{})
			for _, annotation := range e.Annotations {
				annotations[annotation.GetIdShort()] = SubmodelElementToValueOnly(annotation)
			}
			result["annotations"] = annotations
		}
		return result
	case *gen.Capability:
		return map[string]interface{}{}
	case *gen.SubmodelElementCollection:
		result := make(map[string]interface{})
		for _, elem := range e.Value {
			result[elem.GetIdShort()] = SubmodelElementToValueOnly(elem)
		}
		return result
	case *gen.SubmodelElementList:
		result := make([]interface{}, len(e.Value))
		for i, elem := range e.Value {
			result[i] = SubmodelElementToValueOnly(elem)
		}
		return result
	case *gen.Entity:
		result := map[string]interface{}{
			"entityType": e.EntityType,
		}
		if e.GlobalAssetID != "" {
			result["globalAssetId"] = e.GlobalAssetID
		}
		if len(e.SpecificAssetIds) > 0 {
			result["specificAssetIds"] = e.SpecificAssetIds
		}
		if len(e.Statements) > 0 {
			statements := make(map[string]interface{})
			for _, stmt := range e.Statements {
				statements[stmt.GetIdShort()] = SubmodelElementToValueOnly(stmt)
			}
			result["statements"] = statements
		}
		return result
	case *gen.Operation:
		// Operations don't have values in value-only representation
		return nil
	default:
		// For unknown types, return nil
		return nil
	}
}

// ReferenceToValueOnly converts a Reference to its value-only representation.
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
func SubmodelsToValueOnly(submodels []gen.Submodel) []map[string]interface{} {
	result := make([]map[string]interface{}, len(submodels))

	for i, submodel := range submodels {
		result[i] = SubmodelToValueOnly(submodel)
	}

	return result
}

// UpdateSubmodelFromValueOnly updates a submodel with values from a value-only representation.
// This function modifies the submodel elements in place based on the provided value-only data.
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
func UpdateSubmodelElementFromValueOnly(element gen.SubmodelElement, value interface{}) error {
	switch e := element.(type) {
	case *gen.Property:
		if strValue, ok := value.(string); ok {
			e.Value = strValue
		} else {
			return fmt.Errorf("invalid value type for Property: expected string, got %T", value)
		}
	case *gen.MultiLanguageProperty:
		if langStringSlice, ok := value.([]gen.LangStringTextType); ok {
			e.Value = langStringSlice
		} else {
			return fmt.Errorf("invalid value type for MultiLanguageProperty: expected []LangStringTextType, got %T", value)
		}
	case *gen.Range:
		if rangeMap, ok := value.(map[string]interface{}); ok {
			if minVal, exists := rangeMap["min"]; exists {
				if minStr, ok := minVal.(string); ok {
					e.Min = minStr
				}
			}
			if maxVal, exists := rangeMap["max"]; exists {
				if maxStr, ok := maxVal.(string); ok {
					e.Max = maxStr
				}
			}
		} else {
			return fmt.Errorf("invalid value type for Range: expected map, got %T", value)
		}
	case *gen.File:
		if strValue, ok := value.(string); ok {
			e.Value = strValue
		} else {
			return fmt.Errorf("invalid value type for File: expected string, got %T", value)
		}
	case *gen.Blob:
		if strValue, ok := value.(string); ok {
			e.Value = strValue
		} else {
			return fmt.Errorf("invalid value type for Blob: expected string, got %T", value)
		}
	case *gen.ReferenceElement:
		if refMap, ok := value.(map[string]interface{}); ok {
			ref, err := ValueOnlyToReference(refMap)
			if err != nil {
				return fmt.Errorf("failed to convert value-only to Reference: %w", err)
			}
			e.Value = &ref
		} else {
			return fmt.Errorf("invalid value type for ReferenceElement: expected map, got %T", value)
		}
	case *gen.RelationshipElement:
		if relMap, ok := value.(map[string]interface{}); ok {
			if firstMap, exists := relMap["first"]; exists {
				if firstRefMap, ok := firstMap.(map[string]interface{}); ok {
					firstRef, err := ValueOnlyToReference(firstRefMap)
					if err != nil {
						return fmt.Errorf("failed to convert first reference: %w", err)
					}
					e.First = &firstRef
				}
			}
			if secondMap, exists := relMap["second"]; exists {
				if secondRefMap, ok := secondMap.(map[string]interface{}); ok {
					secondRef, err := ValueOnlyToReference(secondRefMap)
					if err != nil {
						return fmt.Errorf("failed to convert second reference: %w", err)
					}
					e.Second = &secondRef
				}
			}
		} else {
			return fmt.Errorf("invalid value type for RelationshipElement: expected map, got %T", value)
		}
	case *gen.SubmodelElementCollection:
		if collectionMap, ok := value.(map[string]interface{}); ok {
			for _, elem := range e.Value {
				elemIDShort := elem.GetIdShort()
				if elemValue, exists := collectionMap[elemIDShort]; exists {
					if err := UpdateSubmodelElementFromValueOnly(elem, elemValue); err != nil {
						return fmt.Errorf("failed to update collection element %s: %w", elemIDShort, err)
					}
				}
			}
		} else {
			return fmt.Errorf("invalid value type for SubmodelElementCollection: expected map, got %T", value)
		}
	case *gen.SubmodelElementList:
		if listSlice, ok := value.([]interface{}); ok {
			if len(listSlice) != len(e.Value) {
				return fmt.Errorf("list length mismatch: expected %d elements, got %d", len(e.Value), len(listSlice))
			}
			for i, elemValue := range listSlice {
				if err := UpdateSubmodelElementFromValueOnly(e.Value[i], elemValue); err != nil {
					return fmt.Errorf("failed to update list element %d: %w", i, err)
				}
			}
		} else {
			return fmt.Errorf("invalid value type for SubmodelElementList: expected slice, got %T", value)
		}
	case *gen.Entity:
		if entityMap, ok := value.(map[string]interface{}); ok {
			if entityType, exists := entityMap["entityType"]; exists {
				if et, ok := entityType.(gen.EntityType); ok {
					e.EntityType = et
				}
			}
			if globalAssetID, exists := entityMap["globalAssetId"]; exists {
				if gid, ok := globalAssetID.(string); ok {
					e.GlobalAssetID = gid
				}
			}
			if specificAssetIDs, exists := entityMap["specificAssetIds"]; exists {
				if sids, ok := specificAssetIDs.([]gen.SpecificAssetID); ok {
					e.SpecificAssetIds = sids
				}
			}
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
		} else {
			return fmt.Errorf("invalid value type for Entity: expected map, got %T", value)
		}
	default:
		// For unsupported element types, do nothing
		return fmt.Errorf("unsupported element type: %T", reflect.TypeOf(element))
	}
	return nil
}

// ValueOnlyToReference converts a value-only reference representation back to a Reference.
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

// SubmodelElementValueToValueOnly converts a SubmodelElementValue back to a value-only representation
// This is the inverse of SubmodelElementToValueOnly and used to accept typed SubmodelElementValue
// payloads on value-only patch endpoints.
func SubmodelElementValueToValueOnly(val gen.SubmodelElementValue) interface{} {
	// For simple string values
	if val.Value != "" {
		return val.Value
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
		return result
	}

	// For entity representation
	if val.EntityType != "" || val.GlobalAssetID != "" {
		m := make(map[string]interface{})
		if val.EntityType != "" {
			m["entityType"] = val.EntityType
		}
		if val.GlobalAssetID != "" {
			m["globalAssetId"] = val.GlobalAssetID
		}
		if len(val.SpecificAssetIds) > 0 {
			// reuse as-is; should map to interface slice (OK for Unmarshal in UpdateSubmodelElementFromValueOnly)
			m["specificAssetIds"] = val.SpecificAssetIds
		}
		if len(val.Statements) > 0 {
			m["statements"] = val.Statements
		}
		return m
	}

	// Nothing matched -> return nil so the update does nothing
	return nil
}

// ReferenceValueToValueOnly converts ReferenceValue to the value-only representation used by the updater
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
