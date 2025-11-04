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

// Package grammar defines the data structures for representing access control lists in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package grammar

// ACL (Access Control List) defines the authorization decision and permissions for AAS resources.
//
// An ACL specifies whether access should be allowed or denied and what specific operations
// (rights) are permitted on Asset Administration Shell (AAS) resources. It combines an access
// decision with a set of permissions and optional attribute-based conditions.
//
// Core Components:
//
//   - ACCESS: The authorization decision - either ALLOW or DENY (required)
//     Determines the baseline access policy for matching requests.
//
//   - RIGHTS: Array of specific permissions granted or denied (required)
//     Examples: READ, WRITE, DELETE, EXECUTE
//     Specifies which operations are controlled by this ACL.
//
//   - ATTRIBUTES/USEATTRIBUTES: Optional attribute constraints (inline or by reference)
//     Can reference user claims, global attributes, or AAS element properties to
//     add conditional logic to the access decision.
//
// Attribute Handling:
// ACLs can either define attributes inline (ATTRIBUTES) or reference a previously
// defined attribute collection by name (USEATTRIBUTES). These attributes provide
// context for access decisions, such as user roles, timestamps, or resource properties.
//
// Example JSON (with inline attributes):
//
//	{
//	  "ACCESS": "ALLOW",
//	  "RIGHTS": ["READ", "WRITE"],
//	  "ATTRIBUTES": [
//	    {"CLAIM": "role"},
//	    {"GLOBAL": "LOCALNOW"}
//	  ]
//	}
//
// Example JSON (with attribute reference):
//
//	{
//	  "ACCESS": "DENY",
//	  "RIGHTS": ["DELETE", "EXECUTE"],
//	  "USEATTRIBUTES": "AdminAttributes"
//	}
type ACL struct {
	// ACCESS corresponds to the JSON schema field "ACCESS".
	ACCESS ACLACCESS `json:"ACCESS" yaml:"ACCESS" mapstructure:"ACCESS"`

	// ATTRIBUTES corresponds to the JSON schema field "ATTRIBUTES".
	ATTRIBUTES []AttributeItem `json:"ATTRIBUTES,omitempty" yaml:"ATTRIBUTES,omitempty" mapstructure:"ATTRIBUTES,omitempty"`

	// RIGHTS corresponds to the JSON schema field "RIGHTS".
	RIGHTS []RightsEnum `json:"RIGHTS" yaml:"RIGHTS" mapstructure:"RIGHTS"`

	// USEATTRIBUTES corresponds to the JSON schema field "USEATTRIBUTES".
	USEATTRIBUTES *string `json:"USEATTRIBUTES,omitempty" yaml:"USEATTRIBUTES,omitempty" mapstructure:"USEATTRIBUTES,omitempty"`
}
