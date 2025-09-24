/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type GetSubmodelElementsMetadataResult struct {
	PagingMetadata PagedResultPagingMetadata `json:"paging_metadata,omitempty"`

	Result []SubmodelElementMetadata `json:"result,omitempty"`
}

// AssertGetSubmodelElementsMetadataResultRequired checks if the required fields are not zero-ed
func AssertGetSubmodelElementsMetadataResultRequired(obj GetSubmodelElementsMetadataResult) error {
	if err := AssertPagedResultPagingMetadataRequired(obj.PagingMetadata); err != nil {
		return err
	}
	for _, el := range obj.Result {
		if err := AssertSubmodelElementMetadataRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertGetSubmodelElementsMetadataResultConstraints checks if the values respects the defined constraints
func AssertGetSubmodelElementsMetadataResultConstraints(obj GetSubmodelElementsMetadataResult) error {
	if err := AssertPagedResultPagingMetadataConstraints(obj.PagingMetadata); err != nil {
		return err
	}
	for _, el := range obj.Result {
		if err := AssertSubmodelElementMetadataConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
