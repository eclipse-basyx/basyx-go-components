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

// Package grammar defines the data structures for representing all access permission rules in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

// AccessRuleModelSchemaJSONAllAccessPermissionRules represents the complete access control policy
// configuration for Asset Administration Shell (AAS) resources.
//
// This structure serves as the top-level container for the entire AAS Access Rule Language
// specification. It defines both reusable components (definitions) and the actual access
// permission rules that govern resource access.
//
// The structure is organized into two main categories:
//
// 1. Definitions (DEF* fields) - Reusable Components:
//   - DEFACLS: Named Access Control List (ACL) definitions that can be referenced by rules
//   - DEFATTRIBUTES: Named attribute collections (e.g., user roles, claims) for use in conditions
//   - DEFFORMULAS: Named logical expressions/formulas that can be reused across multiple rules
//   - DEFOBJECTS: Named object collections defining groups of AAS resources
//
// 2. Rules:
//   - Rules: The actual access permission rules that combine definitions to specify access control
//     policies. Each rule determines whether access to specific resources should be allowed or denied
//     based on conditions and ACLs.
//
// Design Pattern:
// The separation of definitions from rules follows the DRY (Don't Repeat Yourself) principle,
// allowing complex access control policies to be modularized and maintained efficiently. Rules
// can reference definitions by name (e.g., USEACL, USEFORMULA, USEOBJECTS), promoting consistency
// and simplifying policy updates.
//
// Example JSON:
//
//	{
//	  "DEFACLS": [
//	    {
//	      "name": "AdminACL",
//	      "acl": {"access": "ALLOW", "rules": [...]}
//	    }
//	  ],
//	  "DEFFORMULAS": [
//	    {
//	      "name": "IsAdmin",
//	      "formula": {"operator": "==", "left": {"CLAIM": "role"}, "right": {"value": "admin"}}
//	    }
//	  ],
//	  "DEFOBJECTS": [
//	    {
//	      "name": "CriticalSubmodels",
//	      "objects": [{"SUBMODEL": "sm1"}, {"SUBMODEL": "sm2"}]
//	    }
//	  ],
//	  "rules": [
//	    {
//	      "USEACL": "AdminACL",
//	      "USEFORMULA": "IsAdmin",
//	      "USEOBJECTS": ["CriticalSubmodels"]
//	    }
//	  ]
//	}
type AccessRuleModelSchemaJSONAllAccessPermissionRules struct {
	// DEFACLS corresponds to the JSON schema field "DEFACLS".
	DEFACLS []AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFACLSElem `json:"DEFACLS,omitempty" yaml:"DEFACLS,omitempty" mapstructure:"DEFACLS,omitempty"`

	// DEFATTRIBUTES corresponds to the JSON schema field "DEFATTRIBUTES".
	DEFATTRIBUTES []AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFATTRIBUTESElem `json:"DEFATTRIBUTES,omitempty" yaml:"DEFATTRIBUTES,omitempty" mapstructure:"DEFATTRIBUTES,omitempty"`

	// DEFFORMULAS corresponds to the JSON schema field "DEFFORMULAS".
	DEFFORMULAS []AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFFORMULASElem `json:"DEFFORMULAS,omitempty" yaml:"DEFFORMULAS,omitempty" mapstructure:"DEFFORMULAS,omitempty"`

	// DEFOBJECTS corresponds to the JSON schema field "DEFOBJECTS".
	DEFOBJECTS []AccessRuleModelSchemaJSONAllAccessPermissionRulesDEFOBJECTSElem `json:"DEFOBJECTS,omitempty" yaml:"DEFOBJECTS,omitempty" mapstructure:"DEFOBJECTS,omitempty"`

	// Rules corresponds to the JSON schema field "rules".
	Rules []AccessPermissionRule `json:"rules" yaml:"rules" mapstructure:"rules"`
}
