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

// Package grammar defines the data structures for representing the AAS Access Rule Language.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package grammar

import (
	"encoding/json"
	"fmt"
	"strings"
)

type AccessPermissionRule struct {
	// ACL corresponds to the JSON schema field "ACL".
	ACL *ACL `json:"ACL,omitempty" yaml:"ACL,omitempty" mapstructure:"ACL,omitempty"`

	// FILTER corresponds to the JSON schema field "FILTER".
	FILTER *AccessPermissionRuleFILTER `json:"FILTER,omitempty" yaml:"FILTER,omitempty" mapstructure:"FILTER,omitempty"`

	// FORMULA corresponds to the JSON schema field "FORMULA".
	FORMULA *LogicalExpression `json:"FORMULA,omitempty" yaml:"FORMULA,omitempty" mapstructure:"FORMULA,omitempty"`

	// OBJECTS corresponds to the JSON schema field "OBJECTS".
	OBJECTS []ObjectItem `json:"OBJECTS,omitempty" yaml:"OBJECTS,omitempty" mapstructure:"OBJECTS,omitempty"`

	// USEACL corresponds to the JSON schema field "USEACL".
	USEACL *string `json:"USEACL,omitempty" yaml:"USEACL,omitempty" mapstructure:"USEACL,omitempty"`

	// USEFORMULA corresponds to the JSON schema field "USEFORMULA".
	USEFORMULA *string `json:"USEFORMULA,omitempty" yaml:"USEFORMULA,omitempty" mapstructure:"USEFORMULA,omitempty"`

	// USEOBJECTS corresponds to the JSON schema field "USEOBJECTS".
	USEOBJECTS []string `json:"USEOBJECTS,omitempty" yaml:"USEOBJECTS,omitempty" mapstructure:"USEOBJECTS,omitempty"`
}

func (j *AccessPermissionRule) UnmarshalJSON(value []byte) error {

	type Plain AccessPermissionRule
	var plain Plain

	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}

	isStrSet := func(p *string) bool {
		return p != nil && strings.TrimSpace(*p) != ""
	}

	hasACL := plain.ACL != nil
	hasUseACL := isStrSet(plain.USEACL)
	hasFormula := plain.FORMULA != nil
	hasUseFormula := isStrSet(plain.USEFORMULA)

	if hasACL == hasUseACL {
		if hasACL {
			return fmt.Errorf("AccessPermissionRule: only one of ACL or USEACL may be defined, not both")
		}
		return fmt.Errorf("AccessPermissionRule: exactly one of ACL or USEACL must be defined")
	}

	if hasFormula == hasUseFormula {
		if hasFormula {
			return fmt.Errorf("AccessPermissionRule: only one of FORMULA or USEFORMULA may be defined, not both")
		}
		return fmt.Errorf("AccessPermissionRule: exactly one of FORMULA or USEFORMULA must be defined")
	}

	*j = AccessPermissionRule(plain)
	return nil
}
