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

// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )

package model

import (
	"encoding/json"

	"github.com/FriedJannik/aas-go-sdk/types"
)

// AmbiguousSubmodelElementValue represents a submodel element value that can take multiple forms.
// It is used to handle cases where the exact type of the submodel element value is not known
// at compile time, allowing for dynamic interpretation as either a SubmodelElementListValue
// or a MultiLanguagePropertyValue.
//
// Why? In the case that a SubmodelElementList contains only SubmodelElementCollection which
// then only contain Properties - the JSON representation is identical to a MultiLanguageProperty
// and thus the deserialization is ambiguous.
type AmbiguousSubmodelElementValue []map[string]any

// MarshalValueOnly serializes SubmodelElementListValue in Value-Only format - This method shall never be called
func (s AmbiguousSubmodelElementValue) MarshalValueOnly() ([]byte, error) {
	result := make([]json.RawMessage, 0, len(s))
	return json.Marshal(result)
}

// MarshalJSON implements custom JSON marshaling for SubmodelElementListValue
func (s AmbiguousSubmodelElementValue) MarshalJSON() ([]byte, error) {
	return s.MarshalValueOnly()
}

// ConvertToSubmodelElementListValue converts the ambiguous value to a SubmodelElementListValue
//
// Returns an error if the conversion fails
func (s AmbiguousSubmodelElementValue) ConvertToSubmodelElementListValue() (SubmodelElementListValue, error) {
	result := make(SubmodelElementListValue, 0, len(s))
	for _, item := range s {
		var element SubmodelElementValue
		elementBytes, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(elementBytes, &element)
		if err != nil {
			return nil, err
		}
		result = append(result, element)
	}
	return result, nil
}

// ConvertToMultiLanguagePropertyValue converts the ambiguous value to a MultiLanguagePropertyValue
//
// Returns an error if the conversion fails
func (s AmbiguousSubmodelElementValue) ConvertToMultiLanguagePropertyValue() (MultiLanguagePropertyValue, error) {
	result := make(MultiLanguagePropertyValue, 0, len(s))
	for _, item := range s {
		var mlpItem map[string]string
		itemBytes, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(itemBytes, &mlpItem)
		if err != nil {
			return nil, err
		}
		result = append(result, mlpItem)
	}
	return result, nil
}

// GetModelType returns an empty string as AmbiguousSubmodelElementValue does not have a specific model type
func (s AmbiguousSubmodelElementValue) GetModelType() types.ModelType {
	return 100
}
