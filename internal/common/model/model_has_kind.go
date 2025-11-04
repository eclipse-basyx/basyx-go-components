/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// HasKind type of HasKind
type HasKind struct {
	Kind ModellingKind `json:"kind,omitempty"`
}

// AssertHasKindRequired checks if the required fields are not zero-ed
//
//nolint:all
func AssertHasKindRequired(obj HasKind) error {
	return nil
}

// AssertHasKindConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertHasKindConstraints(obj HasKind) error {
	return nil
}
