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
