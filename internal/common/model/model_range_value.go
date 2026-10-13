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
 * API version: V3.2.0
 * Contact: info@idtwin.org
 */
//nolint:all
package model

import (
	"encoding/json"

	"github.com/FriedJannik/aas-go-sdk/types"
)

// RangeValue represents the Value-Only serialization of a Range.
// Both bounds are serialized with the JSON type that the ValueOnly mapping assigns to its valueType.
type RangeValue struct {
	Max       *string
	Min       *string
	ValueType types.DataTypeDefXSD
}

// MarshalValueOnly serializes RangeValue in Value-Only format
func (r RangeValue) MarshalValueOnly() ([]byte, error) {
	return json.Marshal(struct {
		Max any `json:"max"`
		Min any `json:"min"`
	}{
		Max: ValueOnlyScalarToJSON(r.Max, r.ValueType),
		Min: ValueOnlyScalarToJSON(r.Min, r.ValueType),
	})
}

// MarshalJSON implements custom JSON marshaling for RangeValue
func (r RangeValue) MarshalJSON() ([]byte, error) {
	return r.MarshalValueOnly()
}

// GetModelType returns the model type name for Range
func (r RangeValue) GetModelType() types.ModelType {
	return types.ModelTypeRange
}
