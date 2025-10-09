/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

type ReferenceElementValue struct {
	Type ReferenceTypes `json:"type,omitempty"`

	Keys []Key `json:"keys,omitempty"`
}

// AssertReferenceElementValueRequired checks if the required fields are not zero-ed
func AssertReferenceElementValueRequired(obj ReferenceElementValue) error {
	for _, el := range obj.Keys {
		if err := AssertKeyRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertReferenceElementValueConstraints checks if the values respects the defined constraints
func AssertReferenceElementValueConstraints(obj ReferenceElementValue) error {
	for _, el := range obj.Keys {
		if err := AssertKeyConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
