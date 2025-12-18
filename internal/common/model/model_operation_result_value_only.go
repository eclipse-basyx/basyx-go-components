/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// OperationResultValueOnly type of OperationResultValueOnly
type OperationResultValueOnly struct {
	Messages []Message `json:"messages,omitempty"`

	ExecutionState ExecutionState `json:"executionState,omitempty"`

	Success bool `json:"success,omitempty"`

	// The ValueOnly serialization (patternProperties and propertyNames will probably be supported with OpenApi 3.1). For the full description of the generic JSON validation schema see the ValueOnly-Serialization as defined in the 'Specification of the Asset Administration Shell - Part 2'.
	InoutputArguments map[string]interface{} `json:"inoutputArguments,omitempty"`

	// The ValueOnly serialization (patternProperties and propertyNames will probably be supported with OpenApi 3.1). For the full description of the generic JSON validation schema see the ValueOnly-Serialization as defined in the 'Specification of the Asset Administration Shell - Part 2'.
	OutputArguments map[string]interface{} `json:"outputArguments,omitempty"`
}

// AssertOperationResultValueOnlyRequired checks if the required fields are not zero-ed
func AssertOperationResultValueOnlyRequired(obj OperationResultValueOnly) error {
	for _, el := range obj.Messages {
		if err := AssertMessageRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertOperationResultValueOnlyConstraints checks if the values respects the defined constraints
func AssertOperationResultValueOnlyConstraints(obj OperationResultValueOnly) error {
	for _, el := range obj.Messages {
		if err := AssertMessageConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
