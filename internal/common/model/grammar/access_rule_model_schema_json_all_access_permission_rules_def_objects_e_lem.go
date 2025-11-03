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

// Package grammar defines the data structures for representing all access permission rules in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package grammar

import (
	"encoding/json"
	"fmt"
)

type AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem struct {
	// USEOBJECTS corresponds to the JSON schema field "USEOBJECTS".
	USEOBJECTS []string `json:"USEOBJECTS,omitempty" yaml:"USEOBJECTS,omitempty" mapstructure:"USEOBJECTS,omitempty"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name" yaml:"name" mapstructure:"name"`

	// Objects corresponds to the JSON schema field "objects".
	Objects []ObjectItem `json:"objects,omitempty" yaml:"objects,omitempty" mapstructure:"objects,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem: required")
	}
	type Plain AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem(plain)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *AccessRuleModelSchemaJsonAllAccessPermissionRules) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["rules"]; raw != nil && !ok {
		return fmt.Errorf("field rules in AccessRuleModelSchemaJsonAllAccessPermissionRules: required")
	}
	type Plain AccessRuleModelSchemaJsonAllAccessPermissionRules
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJsonAllAccessPermissionRules(plain)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *AccessRuleModelSchemaJson) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["AllAccessPermissionRules"]; raw != nil && !ok {
		return fmt.Errorf("field AllAccessPermissionRules in AccessRuleModelSchemaJson: required")
	}
	type Plain AccessRuleModelSchemaJson
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJson(plain)
	return nil
}
