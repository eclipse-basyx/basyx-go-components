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

// Package grammar defines the data structures for representing time literal patterns in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"fmt"
	"regexp"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// TimeLiteralPattern represents a time literal pattern in the format "HH:MM" or "HH:MM:SS".
type TimeLiteralPattern string

// UnmarshalJSON implements json.Unmarshaler.
func (j *TimeLiteralPattern) UnmarshalJSON(value []byte) error {
	type Plain TimeLiteralPattern
	var plain Plain
	if err := common.UnmarshalAndDisallowUnknownFields(value, &plain); err != nil {
		return err
	}
	if matched, _ := regexp.MatchString(`^[0-9][0-9]:[0-9][0-9](:[0-9][0-9])?$`, string(plain)); !matched {
		return fmt.Errorf("field %s pattern match: must match %s", "", `^[0-9][0-9]:[0-9][0-9](:[0-9][0-9])?$`)
	}
	*j = TimeLiteralPattern(plain)
	return nil
}
