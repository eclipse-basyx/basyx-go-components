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

// BlobValue type of Blob
type BlobValue struct {
	ContentType string `json:"contentType"`

	Value string `json:"value,omitempty"`
}

// MarshalValueOnly serializes BlobValue in Value-Only format
func (b BlobValue) MarshalValueOnly() ([]byte, error) {
	type Alias BlobValue
	return json.Marshal((Alias)(b))
}

// MarshalJSON implements custom JSON marshaling for BlobValue
func (b BlobValue) MarshalJSON() ([]byte, error) {
	return b.MarshalValueOnly()
}

// AssertBlobValueRequired checks if the required fields are not zero-ed
func AssertBlobValueRequired(obj BlobValue) error {
	elements := map[string]interface{}{
		"contentType": obj.ContentType,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	return nil
}

// AssertBlobValueConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertBlobValueConstraints(obj BlobValue) error {
	return nil
}
