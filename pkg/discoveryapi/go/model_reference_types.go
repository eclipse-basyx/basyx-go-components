/*
 * DotAAS Part 2 | HTTP/REST | Discovery Service Specification
 *
 * The entire Full Profile of the Discovery Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) April 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

import (
	"fmt"
)

type ReferenceTypes string

// List of ReferenceTypes
const (
	EXTERNAL_REFERENCE ReferenceTypes = "ExternalReference"
	MODEL_REFERENCE    ReferenceTypes = "ModelReference"
)

// AllowedReferenceTypesEnumValues is all the allowed values of ReferenceTypes enum
var AllowedReferenceTypesEnumValues = []ReferenceTypes{
	"ExternalReference",
	"ModelReference",
}

// validReferenceTypesEnumValue provides a map of ReferenceTypess for fast verification of use input
var validReferenceTypesEnumValues = map[ReferenceTypes]struct{}{
	"ExternalReference": {},
	"ModelReference":    {},
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v ReferenceTypes) IsValid() bool {
	_, ok := validReferenceTypesEnumValues[v]
	return ok
}

// NewReferenceTypesFromValue returns a pointer to a valid ReferenceTypes
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewReferenceTypesFromValue(v string) (ReferenceTypes, error) {
	ev := ReferenceTypes(v)
	if ev.IsValid() {
		return ev, nil
	}

	return "", fmt.Errorf("invalid value '%v' for ReferenceTypes: valid values are %v", v, AllowedReferenceTypesEnumValues)
}

// AssertReferenceTypesRequired checks if the required fields are not zero-ed
func AssertReferenceTypesRequired(obj ReferenceTypes) error {
	return nil
}

// AssertReferenceTypesConstraints checks if the values respects the defined constraints
func AssertReferenceTypesConstraints(obj ReferenceTypes) error {
	return nil
}
