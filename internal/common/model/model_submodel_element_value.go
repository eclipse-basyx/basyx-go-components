/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// SubmodelElementValue is the interface for all Value-Only serialization types.
// All concrete value types must implement this interface to provide Value-Only serialization
// according to the AAS specification.
type SubmodelElementValue interface {
	// MarshalValueOnly serializes the value in Value-Only format
	MarshalValueOnly() ([]byte, error)
}
