/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](https://industrialdigitaltwin.org/en/content-hub/aasspecifications).   Copyright: Industrial Digital Twin Association (IDTA) 2025
 *
 * API version: V3.1.1_SSP-001
 * Contact: info@idtwin.org
 */
//nolint:all
package model

import (
	"errors"
	"strings"
)

// Resource type of Resource
type Resource struct {
	Path string `json:"path"`

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
	if err := AssertStringConstraints(obj.Path); err != nil {
		return err
	}
	if err := AssertStringConstraints(obj.ContentType); err != nil {
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

// AssertStringConstraints checks if a string meets specific constraints.
// Modify this function to include any additional constraints as needed.
func AssertStringConstraints(value string) error {
	// Example constraint: Check if the string length is within a specific range.
	if len(value) > 255 {
		return errors.New("field exceeds maximum length of 255 characters")
	}
	return nil
}
