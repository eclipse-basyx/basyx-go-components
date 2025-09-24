/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type HasKind1 struct {
	Kind ModellingKind `json:"kind,omitempty"`
}

// AssertHasKind1Required checks if the required fields are not zero-ed
func AssertHasKind1Required(obj HasKind1) error {
	return nil
}

// AssertHasKind1Constraints checks if the values respects the defined constraints
func AssertHasKind1Constraints(obj HasKind1) error {
	return nil
}
