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

// ReferenceTypes Reference type of Reference
type ReferenceTypes string

// List of ReferenceTypes
//
//nolint:all
const (
	REFERENCETYPES_EXTERNAL_REFERENCE ReferenceTypes = "ExternalReference"
	REFERENCETYPES_MODEL_REFERENCE    ReferenceTypes = "ModelReference"
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
//
//nolint:all
func AssertReferenceTypesRequired(obj ReferenceTypes) error {
	return nil
}

// AssertReferenceTypesConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertReferenceTypesConstraints(obj ReferenceTypes) error {
	return nil
}
