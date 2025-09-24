/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type HasExtensions struct {
	Extensions []Extension `json:"extensions,omitempty"`
}

// AssertHasExtensionsRequired checks if the required fields are not zero-ed
func AssertHasExtensionsRequired(obj HasExtensions) error {
	for _, el := range obj.Extensions {
		if err := AssertExtensionRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertHasExtensionsConstraints checks if the values respects the defined constraints
func AssertHasExtensionsConstraints(obj HasExtensions) error {
	for _, el := range obj.Extensions {
		if err := AssertExtensionConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
