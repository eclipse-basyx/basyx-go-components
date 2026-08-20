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

package openapi

import "fmt"

// ExecutionState identifies the lifecycle state of an asynchronous operation.
type ExecutionState string

// ExecutionState values identify the lifecycle state of an asynchronous operation.
const (
	EXECUTIONSTATE_INITIATED ExecutionState = "Initiated"
	EXECUTIONSTATE_RUNNING   ExecutionState = "Running"
	EXECUTIONSTATE_COMPLETED ExecutionState = "Completed"
	EXECUTIONSTATE_CANCELED  ExecutionState = "Canceled"
	EXECUTIONSTATE_FAILED    ExecutionState = "Failed"
	EXECUTIONSTATE_TIMEOUT   ExecutionState = "Timeout"
)

// AllowedExecutionStateEnumValues contains every supported asynchronous execution state.
var AllowedExecutionStateEnumValues = []ExecutionState{
	EXECUTIONSTATE_INITIATED,
	EXECUTIONSTATE_RUNNING,
	EXECUTIONSTATE_COMPLETED,
	EXECUTIONSTATE_CANCELED,
	EXECUTIONSTATE_FAILED,
	EXECUTIONSTATE_TIMEOUT,
}

var validExecutionStateEnumValues = map[ExecutionState]struct{}{
	EXECUTIONSTATE_INITIATED: {},
	EXECUTIONSTATE_RUNNING:   {},
	EXECUTIONSTATE_COMPLETED: {},
	EXECUTIONSTATE_CANCELED:  {},
	EXECUTIONSTATE_FAILED:    {},
	EXECUTIONSTATE_TIMEOUT:   {},
}

// IsValid reports whether the execution state is supported.
func (state ExecutionState) IsValid() bool {
	_, found := validExecutionStateEnumValues[state]
	return found
}

// NewExecutionStateFromValue parses and validates an execution state value.
func NewExecutionStateFromValue(value string) (ExecutionState, error) {
	state := ExecutionState(value)
	if state.IsValid() {
		return state, nil
	}
	return "", fmt.Errorf("invalid value %q for ExecutionState: valid values are %v", value, AllowedExecutionStateEnumValues)
}

// AssertExecutionStateRequired validates required ExecutionState fields.
func AssertExecutionStateRequired(ExecutionState) error {
	return nil
}

// AssertExecutionStateConstraints validates ExecutionState constraints.
func AssertExecutionStateConstraints(ExecutionState) error {
	return nil
}
