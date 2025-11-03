/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// RangeValue  type of RangeValue
type RangeValue struct {
	Max RangeValueType `json:"max,omitempty"`

	Min RangeValueType `json:"min,omitempty"`
}

// AssertRangeValueRequired checks if the required fields are not zero-ed
func AssertRangeValueRequired(obj RangeValue) error {
	if err := AssertRangeValueTypeRequired(obj.Max); err != nil {
		return err
	}
	if err := AssertRangeValueTypeRequired(obj.Min); err != nil {
		return err
	}
	return nil
}

// AssertRangeValueConstraints checks if the values respects the defined constraints
func AssertRangeValueConstraints(obj RangeValue) error {
	if err := AssertRangeValueTypeConstraints(obj.Max); err != nil {
		return err
	}
	if err := AssertRangeValueTypeConstraints(obj.Min); err != nil {
		return err
	}
	return nil
}
