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

// RangeValue  type of RangeValue
type RangeValue struct {
	Max string `json:"max,omitempty"`

	Min string `json:"min,omitempty"`
}

// MarshalValueOnly serializes RangeValue in Value-Only format
func (r RangeValue) MarshalValueOnly() ([]byte, error) {
	type Alias RangeValue
	return json.Marshal((Alias)(r))
}

// MarshalJSON implements custom JSON marshaling for RangeValue
func (r RangeValue) MarshalJSON() ([]byte, error) {
	return r.MarshalValueOnly()
}

// AssertRangeValueRequired checks if the required fields are not zero-ed
func AssertRangeValueRequired(_ RangeValue) error {
	// Min and max are optional in value-only representation
	return nil
}

// AssertRangeValueConstraints checks if the values respects the defined constraints
func AssertRangeValueConstraints(_ RangeValue) error {
	return nil
}
