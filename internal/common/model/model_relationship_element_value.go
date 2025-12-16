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

// RelationshipElementValue A relationship element value consisting of two reference values.
type RelationshipElementValue struct {
	First ReferenceValue `json:"first"`

	Second ReferenceValue `json:"second"`
}

// MarshalValueOnly serializes RelationshipElementValue in Value-Only format
func (r RelationshipElementValue) MarshalValueOnly() ([]byte, error) {
	type Alias RelationshipElementValue
	return json.Marshal((Alias)(r))
}

// MarshalJSON implements custom JSON marshaling for RelationshipElementValue
func (r RelationshipElementValue) MarshalJSON() ([]byte, error) {
	return r.MarshalValueOnly()
}

// AssertRelationshipElementValueRequired checks if the required fields are not zero-ed
func AssertRelationshipElementValueRequired(obj RelationshipElementValue) error {
	elements := map[string]interface{}{
		"first":  obj.First,
		"second": obj.Second,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	if err := AssertReferenceValueRequired(obj.First); err != nil {
		return err
	}
	if err := AssertReferenceValueRequired(obj.Second); err != nil {
		return err
	}
	return nil
}

// AssertRelationshipElementValueConstraints checks if the values respects the defined constraints
func AssertRelationshipElementValueConstraints(obj RelationshipElementValue) error {
	if err := AssertReferenceValueConstraints(obj.First); err != nil {
		return err
	}
	if err := AssertReferenceValueConstraints(obj.Second); err != nil {
		return err
	}
	return nil
}
