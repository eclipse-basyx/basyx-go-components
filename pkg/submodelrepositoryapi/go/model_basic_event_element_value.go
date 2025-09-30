/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type BasicEventElementValue struct {
	Observed ReferenceValue `json:"observed"`
}

// AssertBasicEventElementValueRequired checks if the required fields are not zero-ed
func AssertBasicEventElementValueRequired(obj BasicEventElementValue) error {
	elements := map[string]interface{}{
		"observed": obj.Observed,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	if err := AssertReferenceValueRequired(obj.Observed); err != nil {
		return err
	}
	return nil
}

// AssertBasicEventElementValueConstraints checks if the values respects the defined constraints
func AssertBasicEventElementValueConstraints(obj BasicEventElementValue) error {
	if err := AssertReferenceValueConstraints(obj.Observed); err != nil {
		return err
	}
	return nil
}
