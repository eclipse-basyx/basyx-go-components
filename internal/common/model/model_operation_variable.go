/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](https://industrialdigitaltwin.org/en/content-hub/aasspecifications).   Copyright: Industrial Digital Twin Association (IDTA) 2025
 *
 * API version: V3.1.1_SSP-001
 * Contact: info@idtwin.org
 */
//nolint:all
package model

import (
	"encoding/json"
	"fmt"

	jsoniter "github.com/json-iterator/go"
)

// OperationVariable type of OperationVariable
type OperationVariable struct {
	Value SubmodelElement `json:"value"`
}

// AssertOperationVariableRequired checks if the required fields are not zero-ed
func AssertOperationVariableRequired(obj OperationVariable) error {
	elements := map[string]any{
		"value": obj.Value,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	// TODO VALIDATION
	// if err := AssertSubmodelElementChoiceRequired(obj.Value); err != nil {
	// 	return err
	// }
	return nil
}

// AssertOperationVariableConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertOperationVariableConstraints(obj OperationVariable) error {
	// TODO VALIDATION
	// if err := AssertSubmodelElementChoiceConstraints(obj.Value); err != nil {
	// 	return err
	// }
	return nil
}

// UnmarshalJSON custom unmarshaler for OperationVariable to handle the Value field
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
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
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
