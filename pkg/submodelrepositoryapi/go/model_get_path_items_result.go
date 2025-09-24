/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type GetPathItemsResult struct {
	PagingMetadata PagedResultPagingMetadata `json:"paging_metadata,omitempty"`

	Result []string `json:"result,omitempty"`
}

// AssertGetPathItemsResultRequired checks if the required fields are not zero-ed
func AssertGetPathItemsResultRequired(obj GetPathItemsResult) error {
	if err := AssertPagedResultPagingMetadataRequired(obj.PagingMetadata); err != nil {
		return err
	}
	return nil
}

// AssertGetPathItemsResultConstraints checks if the values respects the defined constraints
func AssertGetPathItemsResultConstraints(obj GetPathItemsResult) error {
	if err := AssertPagedResultPagingMetadataConstraints(obj.PagingMetadata); err != nil {
		return err
	}
	return nil
}
