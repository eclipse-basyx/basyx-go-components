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

// Package grammar defines the data structures for representing object items in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"fmt"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// AttributeItem represents an attribute in the AAS access control grammar with its type and value.
//
// An attribute consists of a Kind (CLAIM, GLOBAL, or REFERENCE) and a Value string.
// The Kind determines how the Value should be interpreted:
//   - CLAIM: Value is a claim name from authentication tokens (e.g., "sub", "role")
//   - GLOBAL: Value must be one of the predefined global constants (LOCALNOW, UTCNOW, CLIENTNOW, ANONYMOUS)
//   - REFERENCE: Value is a reference to an AAS model element (e.g., "$sm#idShort")
//
// Example JSON representations:
//   - {"CLAIM": "sub"} - refers to the subject claim from a JWT
//   - {"GLOBAL": "LOCALNOW"} - refers to the current local time
//   - {"REFERENCE": "$sm#id"} - refers to the submodel ID
type AttributeItem struct {
	// Kind specifies the type of attribute (CLAIM, GLOBAL, or REFERENCE)
	Kind ATTRTYPE
	// Value contains the attribute identifier or reference string
	Value string
}

var allowedAttrKeys = map[string]struct{}{
	string(ATTRCLAIM):     {},
	string(ATTRGLOBAL):    {},
	string(ATTRREFERENCE): {},
}

var allowedGlobalVals = map[string]struct{}{
	"LOCALNOW":  {},
	"UTCNOW":    {},
	"CLIENTNOW": {},
	"ANONYMOUS": {},
}

// UnmarshalJSON implements the json.Unmarshaler interface for AttributeItem.
//
// This custom unmarshaler validates the JSON structure and enforces constraints:
//   - Expects exactly one key-value pair in the JSON object
//   - Key must be one of: "CLAIM", "GLOBAL", or "REFERENCE"
//   - Value must be a string
//   - For GLOBAL attributes, the value must be one of the allowed constants:
//     LOCALNOW, UTCNOW, CLIENTNOW, or ANONYMOUS
//
// Example valid JSON:
//   - {"CLAIM": "username"}
//   - {"GLOBAL": "LOCALNOW"}
//   - {"REFERENCE": "$sm#idShort"}
//
// Example invalid JSON:
//   - {"GLOBAL": "INVALID"} - not an allowed global value
//   - {"CLAIM": "user", "GLOBAL": "LOCALNOW"} - more than one key
//   - {"UNKNOWN": "value"} - invalid key
//
// Parameters:
//   - b: JSON byte slice containing the attribute item to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid, has multiple keys, uses an invalid
//     key or global value, or if the value is not a string. Returns nil on success.
func (a *AttributeItem) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := common.UnmarshalAndDisallowUnknownFields(b, &raw); err != nil {
		return err
	}
	if len(raw) != 1 {
		return fmt.Errorf("AttributeItem: expected exactly one key, got %d", len(raw))
	}

	for k, v := range raw {
		if _, ok := allowedAttrKeys[k]; !ok {
			return fmt.Errorf("AttributeItem: invalid key %q (allowed: CLAIM, GLOBAL, REFERENCE)", k)
		}

		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("AttributeItem: value for %q must be a string", k)
		}

		if k == string(ATTRGLOBAL) {
			if _, ok := allowedGlobalVals[s]; !ok {
				return fmt.Errorf("AttributeItem: GLOBAL must be one of LOCALNOW, UTCNOW, CLIENTNOW, ANONYMOUS (got %q)", s)
			}
		}

		a.Kind = ATTRTYPE(k)
		a.Value = s
		break
	}
	return nil
}
