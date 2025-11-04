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

// AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem represents an object definition
// element within the access permission rules schema.
//
// This structure defines a named collection of objects that can be referenced by access rules.
// Objects can be defined inline using the Objects field or referenced from other definitions
// using the USEOBJECTS field. This allows for reusable object definitions across multiple rules.
//
// Fields:
//   - Name: A unique identifier for this object definition (required)
//   - Objects: An array of ObjectItem instances defining specific objects inline (optional)
//   - USEOBJECTS: An array of names referencing other object definitions to include (optional)
//
// Example JSON:
//
//	{
//	  "name": "SensorSubmodels",
//	  "objects": [
//	    {"IDENTIFIABLE": "https://example.com/sm/sensor1"},
//	    {"ROUTE": "/api/submodels/temp"}
//	  ]
//	}
//
// Or using references:
//
//	{
//	  "name": "CombinedObjects",
//	  "USEOBJECTS": ["SensorSubmodels", "ActuatorSubmodels"]
//	}
type AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem struct {
	// USEOBJECTS corresponds to the JSON schema field "USEOBJECTS".
	USEOBJECTS []string `json:"USEOBJECTS,omitempty" yaml:"USEOBJECTS,omitempty" mapstructure:"USEOBJECTS,omitempty"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name" yaml:"name" mapstructure:"name"`

	// Objects corresponds to the JSON schema field "objects".
	Objects []ObjectItem `json:"objects,omitempty" yaml:"objects,omitempty" mapstructure:"objects,omitempty"`
}

// UnmarshalJSON implements the json.Unmarshaler interface for AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem.
//
// This custom unmarshaler validates that the required "name" field is present in the JSON object.
// The name field is mandatory as it serves as the unique identifier for the object definition.
//
// Parameters:
//   - value: JSON byte slice containing the object definition element to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid or if the required "name" field is missing.
//     Returns nil on successful unmarshaling and validation.
func (j *AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem: required")
	}
	type Plain AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem(plain)
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface for AccessRuleModelSchemaJsonAllAccessPermissionRules.
//
// This custom unmarshaler validates that the required "rules" field is present in the JSON object.
// The rules field is mandatory as it contains the array of access permission rules that define
// the access control policy.
//
// Parameters:
//   - value: JSON byte slice containing the access permission rules to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid or if the required "rules" field is missing.
//     Returns nil on successful unmarshaling and validation.
func (j *AccessRuleModelSchemaJSONAllAccessPermissionRules) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["rules"]; raw != nil && !ok {
		return fmt.Errorf("field rules in AccessRuleModelSchemaJsonAllAccessPermissionRules: required")
	}
	type Plain AccessRuleModelSchemaJSONAllAccessPermissionRules
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJSONAllAccessPermissionRules(plain)
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface for AccessRuleModelSchemaJson.
//
// This custom unmarshaler validates that the required "AllAccessPermissionRules" field is present
// in the JSON object. This field is mandatory as it contains the root access permission rules
// configuration for the entire access control model.
//
// Parameters:
//   - value: JSON byte slice containing the access rule model schema to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid or if the required "AllAccessPermissionRules" field is missing.
//     Returns nil on successful unmarshaling and validation.
func (j *AccessRuleModelSchemaJSON) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["AllAccessPermissionRules"]; raw != nil && !ok {
		return fmt.Errorf("field AllAccessPermissionRules in AccessRuleModelSchemaJson: required")
	}
	type Plain AccessRuleModelSchemaJSON
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJSON(plain)
	return nil
}
