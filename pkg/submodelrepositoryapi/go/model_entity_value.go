/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type EntityValue struct {
	EntityType EntityType `json:"entityType"`

	GlobalAssetId string `json:"globalAssetId,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	SpecificAssetIds []map[string]interface{} `json:"specificAssetIds,omitempty"`

	Statements []map[string]interface{} `json:"statements,omitempty"`
}

// AssertEntityValueRequired checks if the required fields are not zero-ed
func AssertEntityValueRequired(obj EntityValue) error {
	elements := map[string]interface{}{
		"entityType": obj.EntityType,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	return nil
}

// AssertEntityValueConstraints checks if the values respects the defined constraints
func AssertEntityValueConstraints(obj EntityValue) error {
	return nil
}
