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

type AttributeItem struct {
	Kind  ATTRTYPE
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

func (a *AttributeItem) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
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
