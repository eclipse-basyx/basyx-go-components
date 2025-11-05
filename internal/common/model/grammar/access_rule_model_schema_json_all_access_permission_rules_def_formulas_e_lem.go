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

// AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem represents a formula definition
// element within the access permission rules schema.
//
// This structure defines a named logical expression (formula) that can be referenced and reused
// across multiple access permission rules. Formulas allow for defining complex conditions once
// and referencing them by name, promoting consistency and maintainability in access control policies.
//
// A formula consists of:
//   - Name: A unique identifier for the formula (required)
//   - Formula: A LogicalExpression defining the condition logic (required)
//
// The Formula field can contain complex logical expressions with AND, OR, NOT operators,
// comparison operations, and nested conditions to evaluate access control decisions.
//
// Example JSON:
//
//	{
//	  "name": "IsSensorOwner",
//	  "formula": {
//	    "$and": [
//	      {"$eq": [{"CLAIM": "role"}, "owner"]},
//	      {"$eq": [{"REFERENCE": "$sm#idShort"}, "SensorData"]}
//	    ]
//	  }
//	}
type AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem struct {
	// Formula corresponds to the JSON schema field "formula".
	Formula LogicalExpression `json:"formula" yaml:"formula" mapstructure:"formula"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name" yaml:"name" mapstructure:"name"`
}

// UnmarshalJSON implements the json.Unmarshaler interface for AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem.
//
// This custom unmarshaler validates that both required fields are present in the JSON object:
//   - "formula": The logical expression defining the condition (required)
//   - "name": The unique identifier for this formula definition (required)
//
// Both fields are mandatory to ensure that formulas are properly defined and can be referenced
// by access permission rules.
//
// Parameters:
//   - value: JSON byte slice containing the formula definition element to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid or if either of the required fields ("formula" or "name")
//     is missing. Returns nil on successful unmarshaling and validation.
func (j *AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["formula"]; raw != nil && !ok {
		return fmt.Errorf("field formula in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem: required")
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem: required")
	}
	type Plain AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem(plain)
	return nil
}
