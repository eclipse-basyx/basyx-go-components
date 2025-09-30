/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type GetSubmodelElementsValueResult struct {
	PagingMetadata PagedResultPagingMetadata `json:"paging_metadata,omitempty"`

	Result []SubmodelElementValue `json:"result,omitempty"`
}

// AssertGetSubmodelElementsValueResultRequired checks if the required fields are not zero-ed
func AssertGetSubmodelElementsValueResultRequired(obj GetSubmodelElementsValueResult) error {
	if err := AssertPagedResultPagingMetadataRequired(obj.PagingMetadata); err != nil {
		return err
	}
	for _, el := range obj.Result {
		if err := AssertSubmodelElementValueRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertGetSubmodelElementsValueResultConstraints checks if the values respects the defined constraints
func AssertGetSubmodelElementsValueResultConstraints(obj GetSubmodelElementsValueResult) error {
	if err := AssertPagedResultPagingMetadataConstraints(obj.PagingMetadata); err != nil {
		return err
	}
	for _, el := range obj.Result {
		if err := AssertSubmodelElementValueConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
