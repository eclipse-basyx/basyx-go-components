/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type EmbeddedDataSpecification struct {
	DataSpecificationContent DataSpecificationContentChoice `json:"dataSpecificationContent"`

	DataSpecification Reference `json:"dataSpecification"`
}

// AssertEmbeddedDataSpecificationRequired checks if the required fields are not zero-ed
func AssertEmbeddedDataSpecificationRequired(obj EmbeddedDataSpecification) error {
	elements := map[string]interface{}{
		"dataSpecificationContent": obj.DataSpecificationContent,
		"dataSpecification":        obj.DataSpecification,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	if err := AssertDataSpecificationContentChoiceRequired(obj.DataSpecificationContent); err != nil {
		return err
	}
	if err := AssertReferenceRequired(obj.DataSpecification); err != nil {
		return err
	}
	return nil
}

// AssertEmbeddedDataSpecificationConstraints checks if the values respects the defined constraints
func AssertEmbeddedDataSpecificationConstraints(obj EmbeddedDataSpecification) error {
	if err := AssertDataSpecificationContentChoiceConstraints(obj.DataSpecificationContent); err != nil {
		return err
	}
	if err := AssertReferenceConstraints(obj.DataSpecification); err != nil {
		return err
	}
	return nil
}
