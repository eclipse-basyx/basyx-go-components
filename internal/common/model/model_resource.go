/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

import (
	"errors"
	"strings"
)

// Resource type of Resource
type Resource struct {
	Path string `json:"path" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	ContentType string `json:"contentType,omitempty"`
}

// AssertResourceRequired checks if the required fields are not zero-ed
func AssertResourceRequired(obj Resource) error {
	elements := map[string]interface{}{
		"path": obj.Path,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	if err := AssertIdShortRequired(obj.ContentType); err != nil {
		return err
	}
	return nil
}

// AssertResourceConstraints checks if the values respects the defined constraints
func AssertResourceConstraints(obj Resource) error {
	if err := AssertstringConstraints(obj.ContentType); err != nil {
		return err
	}
	return nil
}

// AssertIdShortRequired checks if a string is not empty.
//
//nolint:all
func AssertIdShortRequired(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("field is required and cannot be empty")
	}
	return nil
}

// AssertstringConstraints checks if a string meets specific constraints.
// Modify this function to include any additional constraints as needed.
func AssertstringConstraints(value string) error {
	// Example constraint: Check if the string length is within a specific range.
	if len(value) > 255 {
		return errors.New("field exceeds maximum length of 255 characters")
	}
	return nil
}
