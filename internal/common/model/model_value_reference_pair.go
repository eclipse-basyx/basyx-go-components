/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// ValueReferencePair struct representing a pair of a value and its reference ID
type ValueReferencePair struct {
	Value string `json:"value" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	ValueID *Reference `json:"valueId"`
}

// AssertValueReferencePairRequired checks if the required fields are not zero-ed
func AssertValueReferencePairRequired(obj ValueReferencePair) error {
	elements := map[string]interface{}{
		"value":   obj.Value,
		"valueId": obj.ValueID,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}
	if obj.ValueID != nil {
		if err := AssertReferenceRequired(*obj.ValueID); err != nil {
			return err
		}
	}
	return nil
}

// AssertValueReferencePairConstraints checks if the values respects the defined constraints
func AssertValueReferencePairConstraints(obj ValueReferencePair) error {
	if obj.ValueID != nil {
		if err := AssertReferenceConstraints(*obj.ValueID); err != nil {
			return err
		}
	}
	return nil
}
