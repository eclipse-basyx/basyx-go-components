/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type Qualifiable struct {
	Qualifiers []Qualifier `json:"qualifiers,omitempty"`

	ModelType ModelType `json:"modelType"`
}

// AssertQualifiableRequired checks if the required fields are not zero-ed
func AssertQualifiableRequired(obj Qualifiable) error {
	elements := map[string]interface{}{
		"modelType": obj.ModelType,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	for _, el := range obj.Qualifiers {
		if err := AssertQualifierRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertQualifiableConstraints checks if the values respects the defined constraints
func AssertQualifiableConstraints(obj Qualifiable) error {
	for _, el := range obj.Qualifiers {
		if err := AssertQualifierConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
