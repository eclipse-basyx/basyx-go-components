/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */
//nolint:all
package model

import "encoding/json"

// SubmodelValue represents the Value-Only serialization of a Submodel.
// According to spec: A submodel is serialized as an unnamed JSON object with all
// submodel elements serialized according to their respective types.
type SubmodelValue map[string]SubmodelElementValue

// MarshalValueOnly serializes SubmodelValue in Value-Only format
func (s SubmodelValue) MarshalValueOnly() ([]byte, error) {
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

// MarshalJSON implements custom JSON marshaling for SubmodelValue
func (s SubmodelValue) MarshalJSON() ([]byte, error) {
	return s.MarshalValueOnly()
}

// UnmarshalJSON implements custom JSON unmarshaling for SubmodelValue
func (s *SubmodelValue) UnmarshalJSON(data []byte) error {
	// Unmarshal into raw messages first
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// Initialize the map if needed
	if *s == nil {
		*s = make(SubmodelValue)
	}

	// Process each element using the SubmodelElementValue deserializer
	for key, rawValue := range rawMap {
		value, err := UnmarshalSubmodelElementValue(rawValue)
		if err != nil {
			return err
		}
		(*s)[key] = value
	}

	return nil
}

// GetModelType returns the model type name for Submodel
func (s SubmodelValue) GetModelType() string {
	return "Submodel"
}

// AssertSubmodelValueRequired checks if the required fields are not zero-ed
func AssertSubmodelValueRequired(_ SubmodelValue) error {
	// Submodel value itself is optional, individual elements are validated by their own types
	return nil
}

// AssertSubmodelValueConstraints checks if the values respects the defined constraints
func AssertSubmodelValueConstraints(_ SubmodelValue) error {
	// Constraints are validated by individual element types
	return nil
}
