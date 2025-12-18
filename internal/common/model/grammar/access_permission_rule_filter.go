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

// AccessPermissionRuleFILTER represents an optional filter component within an access permission rule.
//
// Filters provide fine-grained control over which AAS resources or resource fragments an access
// permission rule applies to. They enable more precise access control by allowing rules to target
// specific subsets of resources based on conditions or path fragments.
//
// A filter can specify:
//
//   - CONDITION: An inline logical expression that resources must satisfy to match the filter.
//     This allows dynamic filtering based on resource properties, metadata, or context attributes.
//
//   - USEFORMULA: A reference to a previously defined logical formula (by name) that will be
//     used as the filter condition. This promotes reusability of common filter conditions.
//
//   - FRAGMENT: A path or identifier fragment that narrows down which parts of a resource
//     the rule applies to (e.g., specific submodel elements within a submodel).
//
// These fields are all optional and can be combined to create complex filtering logic.
// For example, a rule might apply only to specific fragments of resources that also
// satisfy certain conditions.
//
// Example JSON (with condition):
//
//	{
//	  "CONDITION": {
//	    "operator": "==",
//	    "left": {"attribute": "idShort"},
//	    "right": {"value": "Temperature"}
//	  },
//	  "FRAGMENT": "properties/currentValue"
//	}
//
// Example JSON (with formula reference):
//
//	{
//	  "USEFORMULA": "SensitiveDataCondition",
//	  "FRAGMENT": "properties/*"
//	}
type AccessPermissionRuleFILTER struct {
	// CONDITION corresponds to the JSON schema field "CONDITION".
	CONDITION *LogicalExpression `json:"CONDITION,omitempty" yaml:"CONDITION,omitempty" mapstructure:"CONDITION,omitempty"`

	// FRAGMENT corresponds to the JSON schema field "FRAGMENT".
	FRAGMENT *FragmentStringPattern `json:"FRAGMENT,omitempty" yaml:"FRAGMENT,omitempty" mapstructure:"FRAGMENT,omitempty"`

	// USEFORMULA corresponds to the JSON schema field "USEFORMULA".
	USEFORMULA *string `json:"USEFORMULA,omitempty" yaml:"USEFORMULA,omitempty" mapstructure:"USEFORMULA,omitempty"`
}
