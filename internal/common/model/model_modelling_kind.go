/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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
	"fmt"
)

// ModellingKind type of ModellingKind
type ModellingKind string

// List of ModellingKind
//
//nolint:all
const (
	MODELLINGKIND_INSTANCE ModellingKind = "Instance"
	MODELLINGKIND_TEMPLATE ModellingKind = "Template"
)

// AllowedModellingKindEnumValues is all the allowed values of ModellingKind enum
var AllowedModellingKindEnumValues = []ModellingKind{
	"Instance",
	"Template",
}

// validModellingKindEnumValue provides a map of ModellingKinds for fast verification of use input
var validModellingKindEnumValues = map[ModellingKind]struct{}{
	"Instance": {},
	"Template": {},
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v ModellingKind) IsValid() bool {
	_, ok := validModellingKindEnumValues[v]
	return ok
}

// NewModellingKindFromValue returns a pointer to a valid ModellingKind
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewModellingKindFromValue(v string) (ModellingKind, error) {
	ev := ModellingKind(v)
	if ev.IsValid() {
		return ev, nil
	}

	return "", fmt.Errorf("invalid value '%v' for ModellingKind: valid values are %v", v, AllowedModellingKindEnumValues)
}

// AssertModellingKindRequired checks if the required fields are not zero-ed
func AssertModellingKindRequired(_ ModellingKind) error {
	return nil
}

// AssertModellingKindConstraints checks if the values respects the defined constraints
func AssertModellingKindConstraints(_ ModellingKind) error {
	return nil
}
