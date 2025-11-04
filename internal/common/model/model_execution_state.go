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
//
//nolint:all
func AssertExecutionStateRequired(obj ExecutionState) error {
	return nil
}

// AssertExecutionStateConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertExecutionStateConstraints(obj ExecutionState) error {
	return nil
}
