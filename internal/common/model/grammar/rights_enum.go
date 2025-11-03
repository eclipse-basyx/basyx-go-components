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

// Package grammar defines the data structures for representing rights enumerations in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package grammar

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type RightsEnum string

const RightsEnumALL RightsEnum = "ALL"
const RightsEnumCREATE RightsEnum = "CREATE"
const RightsEnumDELETE RightsEnum = "DELETE"
const RightsEnumEXECUTE RightsEnum = "EXECUTE"
const RightsEnumREAD RightsEnum = "READ"
const RightsEnumTREE RightsEnum = "TREE"
const RightsEnumUPDATE RightsEnum = "UPDATE"
const RightsEnumVIEW RightsEnum = "VIEW"

var enumValues_RightsEnum = []interface{}{
	"CREATE",
	"READ",
	"UPDATE",
	"DELETE",
	"EXECUTE",
	"VIEW",
	"ALL",
	"TREE",
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *RightsEnum) UnmarshalJSON(value []byte) error {
	var v string
	if err := json.Unmarshal(value, &v); err != nil {
		return err
	}
	var ok bool
	for _, expected := range enumValues_RightsEnum {
		if reflect.DeepEqual(v, expected) {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("invalid value (expected one of %#v): %#v", enumValues_RightsEnum, v)
	}
	*j = RightsEnum(v)
	return nil
}
