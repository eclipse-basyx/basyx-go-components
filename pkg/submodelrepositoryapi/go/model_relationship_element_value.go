/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type RelationshipElementValue struct {
	First ReferenceValue `json:"first"`

	Second ReferenceValue `json:"second"`
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
