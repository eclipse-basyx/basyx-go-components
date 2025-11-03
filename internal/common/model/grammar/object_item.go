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

// Package grammar defines the data structures for representing object items in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package grammar

import (
	"encoding/json"
	"fmt"
)

// ObjectItem represents an item in the grammar model with a specific type and value.
// It is used to encapsulate different kinds of objects in the AAS structure, where
// the Kind field specifies the category of the object and Value contains its identifier.
type ObjectItem struct {
	// Kind specifies the type/category of the object (e.g., ROUTE, IDENTIFIABLE)
	Kind OBJECTTYPE
	// Value contains the string identifier or value associated with this object
	Value string
}

// Route represents an OBJECTTYPE for route objects in the grammar model.
// Route objects typically represent paths or endpoints in the AAS structure.
const Route OBJECTTYPE = "ROUTE"

// Identifiable represents an OBJECTTYPE for identifiable objects in the grammar model.
// Identifiable objects have a unique identifier and can be referenced globally.
const Identifiable OBJECTTYPE = "IDENTIFIABLE"

// Refarable represents an OBJECTTYPE for referable objects in the grammar model.
// Referable objects can be referenced within a namespace but may not be globally unique.
const Refarable OBJECTTYPE = "REFERABLE"

// Fragment represents an OBJECTTYPE for fragment objects in the grammar model.
// Fragment objects represent parts or segments of a larger structure.
const Fragment OBJECTTYPE = "FRAGMENT"

// Descriptor represents an OBJECTTYPE for descriptor objects in the grammar model.
// Descriptor objects provide metadata or descriptive information about other objects.
const Descriptor OBJECTTYPE = "DESCRIPTOR"

// UnmarshalJSON implements the json.Unmarshaler interface for ObjectItem.
//
// This custom unmarshaler expects JSON in the format: {"TYPE": "value"} where TYPE
// is one of the allowed OBJECTTYPE constants (ROUTE, IDENTIFIABLE, REFERABLE, FRAGMENT,
// DESCRIPTOR) and value is a string identifier.
//
// Example JSON:
//
//	{"ROUTE": "/api/submodels/123"}
//	{"IDENTIFIABLE": "https://example.com/ids/sm001"}
//
// Parameters:
//   - b: JSON byte slice to unmarshal
//
// Returns:
//   - error: An error if the JSON format is invalid, if there isn't exactly one key,
//     if the key is not an allowed OBJECTTYPE, or if the value is not a string
func (o *ObjectItem) UnmarshalJSON(b []byte) error {
	// Expect a single-key object with a string value.
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	if len(raw) != 1 {
		return fmt.Errorf("ObjectItem: expected exactly one key, got %d", len(raw))
	}

	// Convert allowed keys into a quick lookup map for validation
	allowed := map[string]struct{}{
		"ROUTE":        {},
		"IDENTIFIABLE": {},
		"REFERABLE":    {},
		"FRAGMENT":     {},
		"DESCRIPTOR":   {},
	}

	for k, v := range raw {
		if _, ok := allowed[k]; !ok {
			return fmt.Errorf("ObjectItem: invalid key %q (allowed: ROUTE, IDENTIFIABLE, REFERABLE, FRAGMENT, DESCRIPTOR)", k)
		}

		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("ObjectItem: value for %q must be a string", k)
		}

		o.Kind = OBJECTTYPE(k)
		o.Value = s
		break
	}

	return nil
}
