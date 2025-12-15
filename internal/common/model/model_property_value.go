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

// PropertyValue represents the Value-Only serialization of a Property.
// According to spec: Property is serialized as ${Property/value} where value is a string.
type PropertyValue struct {
	Value string `json:"-"`
}

// MarshalValueOnly serializes PropertyValue in Value-Only format (just the value string)
func (p PropertyValue) MarshalValueOnly() ([]byte, error) {
	return json.Marshal(p.Value)
}

// MarshalJSON implements custom JSON marshaling for PropertyValue
func (p PropertyValue) MarshalJSON() ([]byte, error) {
	return p.MarshalValueOnly()
}

// UnmarshalJSON implements custom JSON unmarshaling for PropertyValue
func (p *PropertyValue) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &p.Value)
}

// AssertPropertyValueRequired checks if the required fields are not zero-ed
func AssertPropertyValueRequired(obj PropertyValue) error {
	if obj.Value == "" {
		return &RequiredError{Field: "value"}
	}
	return nil
}

// AssertPropertyValueConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertPropertyValueConstraints(obj PropertyValue) error {
	return nil
}
