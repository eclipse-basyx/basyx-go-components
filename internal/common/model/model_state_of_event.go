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
	"fmt"
)

// StateOfEvent type of StateOfEvent
type StateOfEvent string

// List of StateOfEvent
//
//nolint:all
const (
	STATEOFEVENT_OFF StateOfEvent = "off"
	STATEOFEVENT_ON  StateOfEvent = "on"
)

// AllowedStateOfEventEnumValues is all the allowed values of StateOfEvent enum
var AllowedStateOfEventEnumValues = []StateOfEvent{
	"off",
	"on",
}

// validStateOfEventEnumValue provides a map of StateOfEvents for fast verification of use input
var validStateOfEventEnumValues = map[StateOfEvent]struct{}{
	"off": {},
	"on":  {},
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v StateOfEvent) IsValid() bool {
	_, ok := validStateOfEventEnumValues[v]
	return ok
}

// NewStateOfEventFromValue returns a pointer to a valid StateOfEvent
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewStateOfEventFromValue(v string) (StateOfEvent, error) {
	ev := StateOfEvent(v)
	if ev.IsValid() {
		return ev, nil
	}

	return "", fmt.Errorf("invalid value '%v' for StateOfEvent: valid values are %v", v, AllowedStateOfEventEnumValues)
}

// AssertStateOfEventRequired checks if the required fields are not zero-ed
func AssertStateOfEventRequired(_ StateOfEvent) error {
	return nil
}

// AssertStateOfEventConstraints checks if the values respects the defined constraints
func AssertStateOfEventConstraints(_ StateOfEvent) error {
	return nil
}
