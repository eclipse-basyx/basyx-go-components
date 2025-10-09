/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

type GetSubmodelElementsResult struct {
	PagingMetadata PagedResultPagingMetadata `json:"paging_metadata,omitempty"`

	Result []SubmodelElement `json:"result"`
}

// AssertGetSubmodelElementsResultRequired checks if the required fields are not zero-ed
func AssertGetSubmodelElementsResultRequired(obj GetSubmodelElementsResult) error {
	if err := AssertPagedResultPagingMetadataRequired(obj.PagingMetadata); err != nil {
		return err
	}
	for _, el := range obj.Result {
		if err := AssertSubmodelElementRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertGetSubmodelElementsResultConstraints checks if the values respects the defined constraints
func AssertGetSubmodelElementsResultConstraints(obj GetSubmodelElementsResult) error {
	if err := AssertPagedResultPagingMetadataConstraints(obj.PagingMetadata); err != nil {
		return err
	}
	for _, el := range obj.Result {
		if err := AssertSubmodelElementConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
