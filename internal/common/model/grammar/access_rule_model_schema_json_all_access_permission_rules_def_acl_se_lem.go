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
	"fmt"

	jsoniter "github.com/json-iterator/go"
)

// AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem represents an Access Control List (ACL)
// definition element within the access permission rules schema.
//
// This structure defines a named ACL that can be referenced and reused across multiple access
// permission rules. ACLs encapsulate complete access control logic, including rules, conditions,
// and permissions that govern access to Asset Administration Shell (AAS) resources.
//
// An ACL definition consists of:
//   - Name: A unique identifier for the ACL (required)
//   - Acl: The complete ACL structure containing access rules and permissions (required)
//
// ACLs provide a way to modularize and centralize access control logic, making it easier to:
//   - Apply consistent security policies across multiple resources
//   - Update access rules in a single location
//   - Maintain and audit access control configurations
//   - Reference common permission patterns by name
//
// Example JSON:
//
//	{
//	  "name": "AdminACL",
//	  "acl": {
//	    "access": "ALLOW",
//	    "rules": [
//	      {
//	        "permission": "READ",
//	        "condition": "role == 'admin'"
//	      }
//	    ]
//	  }
//	}
type AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem struct {
	// ACL corresponds to the JSON schema field "acl".
	ACL ACL `json:"acl" yaml:"acl" mapstructure:"acl"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name" yaml:"name" mapstructure:"name"`
}

// UnmarshalJSON implements the json.Unmarshaler interface for AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem.
//
// This custom unmarshaler validates that both required fields are present in the JSON object:
//   - "acl": The ACL structure containing access control rules and permissions (required)
//   - "name": The unique identifier for this ACL definition (required)
//
// Both fields are mandatory to ensure that ACL definitions are complete and can be properly
// referenced and applied within the access permission rules system.
//
// Parameters:
//   - value: JSON byte slice containing the ACL definition element to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid or if either of the required fields ("acl" or "name")
//     is missing. Returns nil on successful unmarshaling and validation.
func (j *AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["acl"]; raw != nil && !ok {
		return fmt.Errorf("field acl in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem: required")
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem: required")
	}
	type Plain AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem(plain)
	return nil
}
