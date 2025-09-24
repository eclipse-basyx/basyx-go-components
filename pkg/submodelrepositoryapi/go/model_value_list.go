/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type ValueList struct {
	ValueReferencePairs []ValueReferencePair `json:"valueReferencePairs"`
}

// AssertValueListRequired checks if the required fields are not zero-ed
func AssertValueListRequired(obj ValueList) error {
	elements := map[string]interface{}{
		"valueReferencePairs": obj.ValueReferencePairs,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	for _, el := range obj.ValueReferencePairs {
		if err := AssertValueReferencePairRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertValueListConstraints checks if the values respects the defined constraints
func AssertValueListConstraints(obj ValueList) error {
	for _, el := range obj.ValueReferencePairs {
		if err := AssertValueReferencePairConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
