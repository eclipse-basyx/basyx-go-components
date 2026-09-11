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

import "github.com/eclipse-basyx/basyx-go-components/internal/common/model"

// ExecutionState identifies the lifecycle state of an asynchronous operation.
type ExecutionState = model.ExecutionState

// ExecutionState values identify the lifecycle state of an asynchronous operation.
const (
	EXECUTIONSTATE_INITIATED ExecutionState = model.EXECUTIONSTATE_INITIATED
	EXECUTIONSTATE_RUNNING   ExecutionState = model.EXECUTIONSTATE_RUNNING
	EXECUTIONSTATE_COMPLETED ExecutionState = model.EXECUTIONSTATE_COMPLETED
	EXECUTIONSTATE_CANCELED  ExecutionState = model.EXECUTIONSTATE_CANCELED
	EXECUTIONSTATE_FAILED    ExecutionState = model.EXECUTIONSTATE_FAILED
	EXECUTIONSTATE_TIMEOUT   ExecutionState = model.EXECUTIONSTATE_TIMEOUT
)

// AllowedExecutionStateEnumValues contains every supported asynchronous execution state.
var AllowedExecutionStateEnumValues = model.AllowedExecutionStateEnumValues

// NewExecutionStateFromValue parses and validates an execution state value.
func NewExecutionStateFromValue(value string) (ExecutionState, error) {
	return model.NewExecutionStateFromValue(value)
}

// AssertExecutionStateRequired validates required ExecutionState fields.
func AssertExecutionStateRequired(state ExecutionState) error {
	return model.AssertExecutionStateRequired(state)
}

// AssertExecutionStateConstraints validates ExecutionState constraints.
func AssertExecutionStateConstraints(state ExecutionState) error {
	return model.AssertExecutionStateConstraints(state)
}
