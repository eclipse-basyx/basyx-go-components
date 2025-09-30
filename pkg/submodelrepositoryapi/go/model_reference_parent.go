/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type ReferenceParent struct {
	Type ReferenceTypes `json:"type"`

	Keys []Key `json:"keys"`
}

// AssertReferenceParentRequired checks if the required fields are not zero-ed
func AssertReferenceParentRequired(obj ReferenceParent) error {
	elements := map[string]interface{}{
		"type": obj.Type,
		"keys": obj.Keys,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	for _, el := range obj.Keys {
		if err := AssertKeyRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertReferenceParentConstraints checks if the values respects the defined constraints
func AssertReferenceParentConstraints(obj ReferenceParent) error {
	for _, el := range obj.Keys {
		if err := AssertKeyConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
