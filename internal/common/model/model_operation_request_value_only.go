/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

type OperationRequestValueOnly struct {

	// The ValueOnly serialization (patternProperties and propertyNames will be supported probably with OpenApi 3.1). For the full description of the generic JSON validation schema see the ValueOnly-Serialization as defined in the 'Specification of the Asset Administration Shell - Part 2'.
	InoutputArguments map[string]interface{} `json:"inoutputArguments,omitempty"`

	// The ValueOnly serialization (patternProperties and propertyNames will be supported probably with OpenApi 3.1). For the full description of the generic JSON validation schema see the ValueOnly-Serialization as defined in the 'Specification of the Asset Administration Shell - Part 2'.
	InputArguments map[string]interface{} `json:"inputArguments,omitempty"`

	ClientTimeoutDuration string `json:"clientTimeoutDuration" validate:"regexp=^-?P((([0-9]+Y([0-9]+M)?([0-9]+D)?|([0-9]+M)([0-9]+D)?|([0-9]+D))(T(([0-9]+H)([0-9]+M)?([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+M)([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+(\\\\.[0-9]+)?S)))?)|(T(([0-9]+H)([0-9]+M)?([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+M)([0-9]+(\\\\.[0-9]+)?S)?|([0-9]+(\\\\.[0-9]+)?S))))$"`
}

// AssertOperationRequestValueOnlyRequired checks if the required fields are not zero-ed
func AssertOperationRequestValueOnlyRequired(obj OperationRequestValueOnly) error {
	elements := map[string]interface{}{
		"clientTimeoutDuration": obj.ClientTimeoutDuration,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	return nil
}

// AssertOperationRequestValueOnlyConstraints checks if the values respects the defined constraints
func AssertOperationRequestValueOnlyConstraints(obj OperationRequestValueOnly) error {
	return nil
}
