/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

import (
	"encoding/json"
	"fmt"
)

type OperationVariable struct {
	Value SubmodelElement `json:"value"`
}

// AssertOperationVariableRequired checks if the required fields are not zero-ed
func AssertOperationVariableRequired(obj OperationVariable) error {
	elements := map[string]interface{}{
		"value": obj.Value,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	//TODO VALIDATION
	// if err := AssertSubmodelElementChoiceRequired(obj.Value); err != nil {
	// 	return err
	// }
	return nil
}

// AssertOperationVariableConstraints checks if the values respects the defined constraints
func AssertOperationVariableConstraints(obj OperationVariable) error {
	//TODO VALIDATION
	// if err := AssertSubmodelElementChoiceConstraints(obj.Value); err != nil {
	// 	return err
	// }
	return nil
}

func (ov *OperationVariable) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with the same fields but Value as json.RawMessage
	type Alias OperationVariable
	aux := &struct {
		Value json.RawMessage `json:"value"`
		*Alias
	}{
		Alias: (*Alias)(ov),
	}

	// Unmarshal into the temporary struct
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Now process the Value field manually
	if aux.Value != nil {
		element, err := UnmarshalSubmodelElement(aux.Value)
		if err != nil {
			return fmt.Errorf("failed to unmarshal Value field: %w", err)
		}
		ov.Value = element
	}

	return nil
}
