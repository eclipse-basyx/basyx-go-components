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

type AssetKind string

// List of AssetKind
const (
	ASSETKIND_INSTANCE       AssetKind = "Instance"
	ASSETKIND_NOT_APPLICABLE AssetKind = "NotApplicable"
	ASSETKIND_TYPE           AssetKind = "Type"
)

// AllowedAssetKindEnumValues is all the allowed values of AssetKind enum
var AllowedAssetKindEnumValues = []AssetKind{
	"Instance",
	"NotApplicable",
	"Type",
}

// validAssetKindEnumValue provides a map of AssetKinds for fast verification of use input
var validAssetKindEnumValues = map[AssetKind]struct{}{
	"Instance":      {},
	"NotApplicable": {},
	"Type":          {},
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v AssetKind) IsValid() bool {
	_, ok := validAssetKindEnumValues[v]
	return ok
}

// NewAssetKindFromValue returns a pointer to a valid AssetKind
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewAssetKindFromValue(v string) (AssetKind, error) {
	ev := AssetKind(v)
	if ev.IsValid() {
		return ev, nil
	}

	return "", fmt.Errorf("invalid value '%v' for AssetKind: valid values are %v", v, AllowedAssetKindEnumValues)
}

// AssertAssetKindRequired checks if the required fields are not zero-ed
func AssertAssetKindRequired(obj AssetKind) error {
	return nil
}

// AssertAssetKindConstraints checks if the values respects the defined constraints
func AssertAssetKindConstraints(obj AssetKind) error {
	return nil
}
