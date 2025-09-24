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

type ModellingKind string

// List of ModellingKind
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
func AssertModellingKindRequired(obj ModellingKind) error {
	return nil
}

// AssertModellingKindConstraints checks if the values respects the defined constraints
func AssertModellingKindConstraints(obj ModellingKind) error {
	return nil
}
