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

// Package grammar defines the data structures for representing the AAS Access Rule Language.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

// AccessRuleModelSchemaJSON represents the root structure of the AAS Access Rule Language schema.
//
// This is the top-level entry point for defining access control policies in the Asset Administration
// Shell (AAS) ecosystem. It encapsulates the complete specification of access permission rules,
// including all definitions and rule configurations that govern access to AAS resources.
//
// The structure serves as a wrapper around AllAccessPermissionRules, which contains:
//   - Reusable component definitions (ACLs, formulas, attributes, objects)
//   - The actual access permission rules that enforce security policies
//
// Usage:
// This type is typically used to:
//   - Parse JSON configuration files containing access control policies
//   - Serialize access rule configurations to JSON format
//   - Serve as the data model for access control policy management systems
//   - Validate and process access control specifications for AAS implementations
//
// The schema follows a declarative approach, allowing security administrators to define
// comprehensive access control policies in a structured, readable JSON format that can be
// version-controlled, audited, and dynamically updated.
//
// Example JSON:
//
//	{
//	  "AllAccessPermissionRules": {
//	    "DEFACLS": [...],
//	    "DEFFORMULAS": [...],
//	    "DEFOBJECTS": [...],
//	    "DEFATTRIBUTES": [...],
//	    "rules": [
//	      {
//	        "USEACL": "AdminACL",
//	        "USEFORMULA": "IsAdmin",
//	        "USEOBJECTS": ["CriticalSubmodels"]
//	      }
//	    ]
//	  }
//	}
type AccessRuleModelSchemaJSON struct {
	// AllAccessPermissionRules corresponds to the JSON schema field
	// "AllAccessPermissionRules".
	AllAccessPermissionRules AccessRuleModelSchemaJSONAllAccessPermissionRules `json:"AllAccessPermissionRules" yaml:"AllAccessPermissionRules" mapstructure:"AllAccessPermissionRules"`
}
