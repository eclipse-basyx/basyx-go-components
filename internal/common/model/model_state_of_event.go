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
//
//nolint:all
func AssertStateOfEventRequired(obj StateOfEvent) error {
	return nil
}

// AssertStateOfEventConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertStateOfEventConstraints(obj StateOfEvent) error {
	return nil
}
