/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

import (
	"fmt"
)

type Direction string

// List of Direction
const (
	DIRECTION_INPUT  Direction = "input"
	DIRECTION_OUTPUT Direction = "output"
)

// AllowedDirectionEnumValues is all the allowed values of Direction enum
var AllowedDirectionEnumValues = []Direction{
	"input",
	"output",
}

// validDirectionEnumValue provides a map of Directions for fast verification of use input
var validDirectionEnumValues = map[Direction]struct{}{
	"input":  {},
	"output": {},
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v Direction) IsValid() bool {
	_, ok := validDirectionEnumValues[v]
	return ok
}

// NewDirectionFromValue returns a pointer to a valid Direction
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewDirectionFromValue(v string) (Direction, error) {
	ev := Direction(v)
	if ev.IsValid() {
		return ev, nil
	}

	return "", fmt.Errorf("invalid value '%v' for Direction: valid values are %v", v, AllowedDirectionEnumValues)
}

// AssertDirectionRequired checks if the required fields are not zero-ed
func AssertDirectionRequired(obj Direction) error {
	return nil
}

// AssertDirectionConstraints checks if the values respects the defined constraints
func AssertDirectionConstraints(obj Direction) error {
	return nil
}
