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

	"github.com/FriedJannik/aas-go-sdk/types"
)

// RelationshipElementValue A relationship element value consisting of two reference values.
type RelationshipElementValue struct {
	First types.IReference `json:"first,omitempty"`

	Second types.IReference `json:"second,omitempty"`
}

// MarshalValueOnly serializes RelationshipElementValue in Value-Only format
func (r RelationshipElementValue) MarshalValueOnly() ([]byte, error) {
	type Alias RelationshipElementValue
	return json.Marshal((Alias)(r))
}

// MarshalJSON implements custom JSON marshaling for RelationshipElementValue
func (r RelationshipElementValue) MarshalJSON() ([]byte, error) {
	return r.MarshalValueOnly()
}

// GetModelType returns the model type name for RelationshipElement
func (r RelationshipElementValue) GetModelType() types.ModelType {
	return types.ModelTypeRelationshipElement
}
