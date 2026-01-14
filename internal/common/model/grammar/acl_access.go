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

// Package grammar defines the data structures for representing access control lists in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"fmt"
	"reflect"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// ACLACCESS defines the access mode for an Access Control List (ACL) entry.
//
// This type determines whether access is granted or denied for a particular ACL rule.
// The access mode works in conjunction with RIGHTS to define what operations are permitted.
//
// Valid values:
//   - ALLOW: Grants access according to the specified rights
//   - DISABLED: Denies access regardless of rights (ACL is disabled)
type ACLACCESS string

// ACLACCESSALLOW represents the access mode that grants permissions.
// When an ACL has ACCESS set to ALLOW, the specified RIGHTS determine what operations
// are permitted for the matching resources.
const ACLACCESSALLOW ACLACCESS = "ALLOW"

// ACLACCESSDISABLED represents the access mode that denies all access.
// When an ACL has ACCESS set to DISABLED, all access is denied regardless of the
// specified RIGHTS. This effectively disables the ACL rule.
const ACLACCESSDISABLED ACLACCESS = "DISABLED"

var enumValuesACLACCESS = []interface{}{
	"ALLOW",
	"DISABLED",
}

// UnmarshalJSON implements the json.Unmarshaler interface for ACLACCESS.
//
// This custom unmarshaler validates that the JSON string value is one of the allowed
// ACLACCESS enum values: "ALLOW" or "DISABLED". Any other value will result in an error.
//
// Parameters:
//   - value: JSON byte slice containing the string value to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid or if the value is not one of the allowed
//     enum values. Returns nil on successful unmarshaling and validation.
func (j *ACLACCESS) UnmarshalJSON(value []byte) error {
	var v string
	if err := common.UnmarshalAndDisallowUnknownFields(value, &v); err != nil {
		return err
	}
	var ok bool
	for _, expected := range enumValuesACLACCESS {
		if reflect.DeepEqual(v, expected) {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("invalid value (expected one of %#v): %#v", enumValuesACLACCESS, v)
	}
	*j = ACLACCESS(v)
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface for ACL.
//
// This custom unmarshaler validates that the ACL JSON object contains the required fields:
//   - ACCESS: The access mode (ALLOW or DISABLED)
//   - RIGHTS: The rights/permissions being granted or denied
//
// Both fields are mandatory and their absence will result in an error.
//
// Parameters:
//   - value: JSON byte slice containing the ACL object to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid or if required fields (ACCESS or RIGHTS) are missing.
//     Returns nil on successful unmarshaling and validation.
func (j *ACL) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := common.UnmarshalAndDisallowUnknownFields(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["ACCESS"]; raw != nil && !ok {
		return fmt.Errorf("field ACCESS in ACL: required")
	}
	if _, ok := raw["RIGHTS"]; raw != nil && !ok {
		return fmt.Errorf("field RIGHTS in ACL: required")
	}
	type Plain ACL
	var plain Plain
	if err := common.UnmarshalAndDisallowUnknownFields(value, &plain); err != nil {
		return err
	}
	*j = ACL(plain)
	return nil
}
