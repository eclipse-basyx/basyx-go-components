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

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem represents an attribute definition
// element within the access permission rules schema.
//
// This structure defines a named collection of attributes that can be referenced and reused across
// multiple access permission rules. Attribute definitions allow for grouping related attributes
// together, promoting consistency and reducing duplication in access control policies.
//
// An attribute definition consists of:
//   - Name: A unique identifier for the attribute collection (required)
//   - Attributes: An array of AttributeItem instances defining specific attributes (required)
//
// Attributes can be of three types:
//   - CLAIM: Attributes from authentication tokens (e.g., user roles, permissions)
//   - GLOBAL: System-wide attributes (e.g., LOCALNOW, UTCNOW, ANONYMOUS)
//   - REFERENCE: References to AAS model elements (e.g., "$sm#idShort")
//
// Example JSON:
//
//	{
//	  "name": "UserContext",
//	  "attributes": [
//	    {"CLAIM": "sub"},
//	    {"CLAIM": "role"},
//	    {"GLOBAL": "LOCALNOW"}
//	  ]
//	}
type AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem struct {
	// Attributes corresponds to the JSON schema field "attributes".
	Attributes []AttributeItem `json:"attributes" yaml:"attributes" mapstructure:"attributes"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name" yaml:"name" mapstructure:"name"`
}

// UnmarshalJSON implements the json.Unmarshaler interface for AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem.
//
// This custom unmarshaler validates that both required fields are present in the JSON object:
//   - "attributes": The array of AttributeItem instances (required)
//   - "name": The unique identifier for this attribute definition (required)
//
// Both fields are mandatory to ensure that attribute collections are properly defined and can
// be referenced by access permission rules.
//
// Parameters:
//   - value: JSON byte slice containing the attribute definition element to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid or if either of the required fields ("attributes" or "name")
//     is missing. Returns nil on successful unmarshaling and validation.
func (j *AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := common.UnmarshalAndDisallowUnknownFields(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["attributes"]; raw != nil && !ok {
		return fmt.Errorf("field attributes in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem: required")
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem: required")
	}
	type Plain AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem
	var plain Plain
	if err := common.UnmarshalAndDisallowUnknownFields(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem(plain)
	return nil
}
