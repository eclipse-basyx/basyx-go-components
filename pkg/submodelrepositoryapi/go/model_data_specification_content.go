/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type DataSpecificationContent struct {
	ModelType ModelType `json:"modelType"`
}

// AssertDataSpecificationContentRequired checks if the required fields are not zero-ed
func AssertDataSpecificationContentRequired(obj DataSpecificationContent) error {
	elements := map[string]interface{}{
		"modelType": obj.ModelType,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	return nil
}

// AssertDataSpecificationContentConstraints checks if the values respects the defined constraints
func AssertDataSpecificationContentConstraints(obj DataSpecificationContent) error {
	return nil
}
