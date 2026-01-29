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

// Package grammar defines the data structures for representing attribute types in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

// ATTRTYPE defines the type classification for attributes in the AAS access control grammar.
//
// Attributes in the access control system can be categorized by their source and scope:
//   - CLAIM: Attributes derived from authentication claims (e.g., JWT claims, user identity)
//   - GLOBAL: Attributes that are globally accessible across the system
//   - REFERENCE: Attributes that reference AAS model elements or their properties
//
// This type is used to distinguish between different kinds of attributes when evaluating
// access control rules and policies.
type ATTRTYPE string

const (
	// ATTRCLAIM represents attributes derived from authentication claims.
	// These are typically extracted from authentication tokens (e.g., JWT) and include
	// information about the authenticated user or client, such as user ID, roles, or permissions.
	ATTRCLAIM ATTRTYPE = "CLAIM"

	// ATTRGLOBAL represents globally accessible attributes.
	// These attributes are available system-wide and not tied to specific AAS elements
	// or authentication contexts.
	ATTRGLOBAL ATTRTYPE = "GLOBAL"

	// ATTRREFERENCE represents attributes that reference AAS model elements.
	// These attributes point to specific elements within the Asset Administration Shell
	// structure, such as submodels, properties, or other referable elements.
	ATTRREFERENCE ATTRTYPE = "REFERENCE"
)
