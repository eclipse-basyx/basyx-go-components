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

// ExecutionState  type of ExecutionState
type ExecutionState string

// List of ExecutionState
//
//nolint:all
const (
	EXECUTIONSTATE_INITIATED ExecutionState = "Initiated"
	EXECUTIONSTATE_RUNNING   ExecutionState = "Running"
	EXECUTIONSTATE_COMPLETED ExecutionState = "Completed"
	EXECUTIONSTATE_CANCELED  ExecutionState = "Canceled"
	EXECUTIONSTATE_FAILED    ExecutionState = "Failed"
	EXECUTIONSTATE_TIMEOUT   ExecutionState = "Timeout"
)

// AllowedExecutionStateEnumValues is all the allowed values of ExecutionState enum
var AllowedExecutionStateEnumValues = []ExecutionState{
	"Initiated",
	"Running",
	"Completed",
	"Canceled",
	"Failed",
	"Timeout",
}

// validExecutionStateEnumValue provides a map of ExecutionStates for fast verification of use input
var validExecutionStateEnumValues = map[ExecutionState]struct{}{
	"Initiated": {},
	"Running":   {},
	"Completed": {},
	"Canceled":  {},
	"Failed":    {},
	"Timeout":   {},
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v ExecutionState) IsValid() bool {
	_, ok := validExecutionStateEnumValues[v]
	return ok
}

// NewExecutionStateFromValue returns a pointer to a valid ExecutionState
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewExecutionStateFromValue(v string) (ExecutionState, error) {
	ev := ExecutionState(v)
	if ev.IsValid() {
		return ev, nil
	}

	return "", fmt.Errorf("invalid value '%v' for ExecutionState: valid values are %v", v, AllowedExecutionStateEnumValues)
}

// AssertExecutionStateRequired checks if the required fields are not zero-ed
func AssertExecutionStateRequired(_ ExecutionState) error {
	return nil
}

// AssertExecutionStateConstraints checks if the values respects the defined constraints
func AssertExecutionStateConstraints(_ ExecutionState) error {
	return nil
}
