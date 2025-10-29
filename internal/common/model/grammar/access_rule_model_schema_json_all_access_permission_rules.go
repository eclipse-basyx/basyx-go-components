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

// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package grammar

type AccessRuleModelSchemaJsonAllAccessPermissionRules struct {
	// DEFACLS corresponds to the JSON schema field "DEFACLS".
	DEFACLS []AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem `json:"DEFACLS,omitempty" yaml:"DEFACLS,omitempty" mapstructure:"DEFACLS,omitempty"`

	// DEFATTRIBUTES corresponds to the JSON schema field "DEFATTRIBUTES".
	DEFATTRIBUTES []AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem `json:"DEFATTRIBUTES,omitempty" yaml:"DEFATTRIBUTES,omitempty" mapstructure:"DEFATTRIBUTES,omitempty"`

	// DEFFORMULAS corresponds to the JSON schema field "DEFFORMULAS".
	DEFFORMULAS []AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem `json:"DEFFORMULAS,omitempty" yaml:"DEFFORMULAS,omitempty" mapstructure:"DEFFORMULAS,omitempty"`

	// DEFOBJECTS corresponds to the JSON schema field "DEFOBJECTS".
	DEFOBJECTS []AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem `json:"DEFOBJECTS,omitempty" yaml:"DEFOBJECTS,omitempty" mapstructure:"DEFOBJECTS,omitempty"`

	// Rules corresponds to the JSON schema field "rules".
	Rules []AccessPermissionRule `json:"rules" yaml:"rules" mapstructure:"rules"`
}
