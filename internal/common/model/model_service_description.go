/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// ServiceDescription - The Description object enables servers to present their capabilities to the clients, in particular which profiles they implement. At least one defined profile is required. Additional, proprietary attributes might be included. Nevertheless, the server must not expect that a regular client understands them.
type ServiceDescription struct {
	Profiles []string `json:"profiles,omitempty"`
}

// AssertServiceDescriptionRequired checks if the required fields are not zero-ed
func AssertServiceDescriptionRequired(obj ServiceDescription) error {
	return nil
}

// AssertServiceDescriptionConstraints checks if the values respects the defined constraints
func AssertServiceDescriptionConstraints(obj ServiceDescription) error {
	return nil
}
