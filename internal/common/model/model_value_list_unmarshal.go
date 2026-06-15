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

package model

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
)

// UnmarshalJSON implements custom unmarshaling for ValueList that can handle both
// a direct array of ValueReferencePair and an object with valueReferencePair field
func (vl *ValueList) UnmarshalJSON(data []byte) error {
	// First try to unmarshal as array directly
	var pairs []*ValueReferencePair
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(data, &pairs); err == nil {
		vl.ValueReferencePairs = pairs
		return nil
	}

	// If that fails, try to unmarshal as object with valueReferencePair field
	var obj struct {
		ValueReferencePairs []*ValueReferencePair `json:"valueReferencePairs"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("value list must be either array of value reference pairs or object with valueReferencePairs field: %w", err)
	}

	vl.ValueReferencePairs = obj.ValueReferencePairs
	return nil
}
