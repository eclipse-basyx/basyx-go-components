/*
 * DotAAS Part 2 | HTTP/REST | Discovery Service Specification
 *
 * The entire Full Profile of the Discovery Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) April 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type PagedResultPagingMetadata struct {
	Cursor string `json:"cursor,omitempty"`
}

// AssertPagedResultPagingMetadataRequired checks if the required fields are not zero-ed
func AssertPagedResultPagingMetadataRequired(obj PagedResultPagingMetadata) error {
	return nil
}

// AssertPagedResultPagingMetadataConstraints checks if the values respects the defined constraints
func AssertPagedResultPagingMetadataConstraints(obj PagedResultPagingMetadata) error {
	return nil
}
