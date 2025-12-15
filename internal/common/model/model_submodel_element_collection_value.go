/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

import "encoding/json"

// SubmodelElementCollectionValue represents the Value-Only serialization of a SubmodelElementCollection.
// According to spec: Serialized as named JSON object with ${SubmodelElementCollection/idShort} as the name.
// The elements are serialized according to their respective type with ${SubmodelElement/idShort} as the name.
type SubmodelElementCollectionValue map[string]SubmodelElementValue

// MarshalValueOnly serializes SubmodelElementCollectionValue in Value-Only format
func (s SubmodelElementCollectionValue) MarshalValueOnly() ([]byte, error) {
	result := make(map[string]json.RawMessage)
	for key, value := range s {
		if value != nil {
			data, err := value.MarshalValueOnly()
			if err != nil {
				return nil, err
			}
			result[key] = data
		}
	}
	return json.Marshal(result)
}

// MarshalJSON implements custom JSON marshaling for SubmodelElementCollectionValue
func (s SubmodelElementCollectionValue) MarshalJSON() ([]byte, error) {
	return s.MarshalValueOnly()
}

// AssertSubmodelElementCollectionValueRequired checks if the required fields are not zero-ed
func AssertSubmodelElementCollectionValueRequired(obj SubmodelElementCollectionValue) error {
	// Collection itself is optional, individual elements are validated by their own types
	return nil
}

// AssertSubmodelElementCollectionValueConstraints checks if the values respects the defined constraints
func AssertSubmodelElementCollectionValueConstraints(obj SubmodelElementCollectionValue) error {
	// Constraints are validated by individual element types
	return nil
}
