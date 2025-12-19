/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */
//nolint:all//nolint:all
package model

import (
	"encoding/json"
	"fmt"
)

// UnmarshalSubmodelElementValue attempts to deserialize JSON into the appropriate SubmodelElementValue type.
// The function inspects the JSON structure to determine the correct concrete type.
func UnmarshalSubmodelElementValue(data []byte) (SubmodelElementValue, error) {
	// First, try to unmarshal into a generic map to inspect the structure
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		// If it's not an object, it might be a simple PropertyValue (just a string)
		var strVal string
		if err := json.Unmarshal(data, &strVal); err == nil {
			return PropertyValue{Value: strVal}, nil
		}
		// Try to parse it as ambiguous type
		var ambiguous AmbiguousSubmodelElementValue
		err := json.Unmarshal(data, &ambiguous)
		if err == nil {
			return ambiguous, nil
		}
		value, err := parseSubmodelElementListValue(data)
		if err == nil {
			return value, nil
		}
		return nil, fmt.Errorf("failed to unmarshal SubmodelElementValue: %w", err)
	}

	// Detect type based on structure
	if observed, hasObserved := raw["observed"]; hasObserved {
		// BasicEventElementValue
		if obsMap, ok := observed.(map[string]interface{}); ok {
			if _, hasType := obsMap["type"]; hasType {
				var val BasicEventElementValue
				if err := json.Unmarshal(data, &val); err != nil {
					return nil, err
				}
				return val, nil
			}
		}
	}

	if contentType, hasContentType := raw["contentType"]; hasContentType {
		// FileValue or BlobValue
		if _, ok := contentType.(string); ok {
			if value, hasValue := raw["value"]; hasValue {
				if valueStr, ok := value.(string); ok && valueStr != "" {
					// FileValue (value is required and non-empty)
					var val FileValue
					if err := json.Unmarshal(data, &val); err != nil {
						return nil, err
					}
					return val, nil
				}
			}
			// BlobValue (value is optional)
			var val BlobValue
			if err := json.Unmarshal(data, &val); err != nil {
				return nil, err
			}
			return val, nil
		}
	}

	if _, hasMin := raw["min"]; hasMin {
		// RangeValue
		if _, hasMax := raw["max"]; hasMax {
			var val RangeValue
			if err := json.Unmarshal(data, &val); err != nil {
				return nil, err
			}
			return val, nil
		}
	}

	if _, hasKeys := raw["keys"]; hasKeys {
		// Could be ReferenceElementValue or part of RelationshipElement
		if _, hasFirst := raw["first"]; hasFirst {
			if _, hasSecond := raw["second"]; hasSecond {
				// RelationshipElementValue or AnnotatedRelationshipElementValue
				if _, hasAnnotations := raw["annotations"]; hasAnnotations {
					var val AnnotatedRelationshipElementValue
					if err := json.Unmarshal(data, &val); err != nil {
						return nil, err
					}
					return val, nil
				}
				var val RelationshipElementValue
				if err := json.Unmarshal(data, &val); err != nil {
					return nil, err
				}
				return val, nil
			}
		}
		// Just a ReferenceElementValue
		var val ReferenceElementValue
		if err := json.Unmarshal(data, &val); err != nil {
			return nil, err
		}
		return val, nil
	}

	if _, hasFirst := raw["first"]; hasFirst {
		if _, hasSecond := raw["second"]; hasSecond {
			// RelationshipElementValue or AnnotatedRelationshipElementValue
			if annotationsRaw, hasAnnotations := raw["annotations"]; hasAnnotations {
				// Manually parse AnnotatedRelationshipElementValue to handle annotations recursively
				var val AnnotatedRelationshipElementValue

				// Parse first and second
				firstBytes, _ := json.Marshal(raw["first"])
				if err := json.Unmarshal(firstBytes, &val.First); err != nil {
					return nil, fmt.Errorf("failed to unmarshal first: %w", err)
				}

				secondBytes, _ := json.Marshal(raw["second"])
				if err := json.Unmarshal(secondBytes, &val.Second); err != nil {
					return nil, fmt.Errorf("failed to unmarshal second: %w", err)
				}

				// Parse annotations as SubmodelElementCollectionValue
				annotationsBytes, _ := json.Marshal(annotationsRaw)
				annotations, err := parseSubmodelElementCollectionValue(annotationsBytes)
				if err != nil {
					return nil, fmt.Errorf("failed to unmarshal annotations: %w", err)
				}
				val.Annotations = annotations.(SubmodelElementCollectionValue)

				return val, nil
			}
			var val RelationshipElementValue
			if err := json.Unmarshal(data, &val); err != nil {
				return nil, err
			}
			return val, nil
		}
	}

	if _, hasEntityType := raw["entityType"]; hasEntityType {
		// EntityValue
		var val EntityValue
		if err := json.Unmarshal(data, &val); err != nil {
			return nil, err
		}
		return val, nil
	}

	// Check if all values are SubmodelElementValue types (nested structure)
	// This could be SubmodelElementCollectionValue or SubmodelValue
	return parseSubmodelElementCollectionValue(data)
}

// parseSubmodelElementCollectionValue attempts to parse a map of SubmodelElementValues
func parseSubmodelElementCollectionValue(data []byte) (SubmodelElementValue, error) {
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return nil, err
	}

	result := make(SubmodelElementCollectionValue)
	for key, rawValue := range rawMap {
		value, err := UnmarshalSubmodelElementValue(rawValue)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal element '%s': %w", key, err)
		}
		result[key] = value
	}
	return result, nil
}

// parseSubmodelElementListValue attempts to parse an array of SubmodelElementValues
func parseSubmodelElementListValue(data []byte) (SubmodelElementValue, error) {
	var rawArray []json.RawMessage
	if err := json.Unmarshal(data, &rawArray); err != nil {
		return nil, err
	}

	result := make(SubmodelElementListValue, 0, len(rawArray))
	for i, rawValue := range rawArray {
		value, err := UnmarshalSubmodelElementValue(rawValue)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal element at index %d: %w", i, err)
		}
		result = append(result, value)
	}
	return result, nil
}
