/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// FileValue type of FileValue
type FileValue struct {
	ContentType string `json:"contentType"`

	Value string `json:"value"`
}

// AssertFileValueRequired checks if the required fields are not zero-ed
func AssertFileValueRequired(obj FileValue) error {
	elements := map[string]interface{}{
		"contentType": obj.ContentType,
		"value":       obj.Value,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	return nil
}

// AssertFileValueConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertFileValueConstraints(obj FileValue) error {
	return nil
}
