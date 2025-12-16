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

// SubmodelElementListValue represents the Value-Only serialization of a SubmodelElementList.
// According to spec: Serialized as a named JSON array with ${SubmodelElementList/idShort} as the name.
// The elements in the JSON array are the ValueOnly serializations preserving order.
type SubmodelElementListValue []SubmodelElementValue

// MarshalValueOnly serializes SubmodelElementListValue in Value-Only format
func (s SubmodelElementListValue) MarshalValueOnly() ([]byte, error) {
	result := make([]json.RawMessage, 0, len(s))
	for _, value := range s {
		if value != nil {
			data, err := value.MarshalValueOnly()
			if err != nil {
				return nil, err
			}
			result = append(result, data)
		}
	}
	return json.Marshal(result)
}

// MarshalJSON implements custom JSON marshaling for SubmodelElementListValue
func (s SubmodelElementListValue) MarshalJSON() ([]byte, error) {
	return s.MarshalValueOnly()
}

// AssertSubmodelElementListValueRequired checks if the required fields are not zero-ed
func AssertSubmodelElementListValueRequired(_ SubmodelElementListValue) error {
	// List itself is optional, individual elements are validated by their own types
	return nil
}

// AssertSubmodelElementListValueConstraints checks if the values respects the defined constraints
func AssertSubmodelElementListValueConstraints(_ SubmodelElementListValue) error {
	// Constraints are validated by individual element types
	return nil
}
