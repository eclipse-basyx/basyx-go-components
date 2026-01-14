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

// Reference type of Reference
type Reference struct {
	Type ReferenceTypes `json:"type"`

	Keys []Key `json:"keys"`

	ReferredSemanticID *Reference `json:"referredSemanticId,omitempty"`
}

// AssertReferenceRequired checks if the required fields are not zero-ed
func AssertReferenceRequired(obj Reference) error {
	elements := map[string]interface{}{
		"type": obj.Type,
		"keys": obj.Keys,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	if len(obj.Keys) < 1 {
		return &RequiredError{Field: "keys must contain at least one element"}
	}

	for _, el := range obj.Keys {
		if err := AssertKeyRequired(el); err != nil {
			return err
		}
	}

	// Stack
	stack := make([]*Reference, 0)
	if obj.ReferredSemanticID != nil {
		stack = append(stack, obj.ReferredSemanticID)
	}
	for len(stack) > 0 {
		// Pop
		n := len(stack) - 1
		current := stack[n]
		stack = stack[:n]

		if err := AssertReferenceRequired(*current); err != nil {
			return err
		}

		if current.ReferredSemanticID != nil {
			stack = append(stack, current.ReferredSemanticID)
		}
	}

	return nil
}

// AssertReferenceConstraints checks if the values respects the defined constraints
func AssertReferenceConstraints(obj Reference) error {
	for _, el := range obj.Keys {
		if err := AssertKeyConstraints(el); err != nil {
			return err
		}
	}

	stack := make([]*Reference, 0)
	if obj.ReferredSemanticID != nil {
		stack = append(stack, obj.ReferredSemanticID)
	}
	for len(stack) > 0 {
		n := len(stack) - 1
		current := stack[n]
		stack = stack[:n]

		if err := AssertReferenceConstraints(*current); err != nil {
			return err
		}

		if current.ReferredSemanticID != nil {
			stack = append(stack, current.ReferredSemanticID)
		}
	}

	return nil
}
