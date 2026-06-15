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

package model

import (
	"errors"
	"regexp"
)

const (
	unicodeStringConstraintPattern = `^([\\x09\\x0a\\x0d\\x20-\\ud7ff\\ue000-\\ufffd]|\\ud800[\\udc00-\\udfff]|[\\ud801-\\udbfe][\\udc00-\\udfff]|\\udbff[\\udc00-\\udfff])*$`
	idShortConstraintPattern       = `^[a-zA-Z][a-zA-Z0-9_-]*[a-zA-Z0-9_]+$`
)

var idShortRegexp = regexp.MustCompile(idShortConstraintPattern)

func validateUnicodeStringConstraint(value string) error {
	for _, r := range value {
		if r == 0x9 || r == 0xA || r == 0xD {
			continue
		}
		if r >= 0x20 && r <= 0xD7FF {
			continue
		}
		if r >= 0xE000 && r <= 0xFFFD {
			continue
		}
		if r >= 0x10000 && r <= 0x10FFFF {
			continue
		}
		return errors.New(`must match "` + unicodeStringConstraintPattern + `"`)
	}
	return nil
}

func validateIDShortConstraint(value string) error {
	if !idShortRegexp.MatchString(value) {
		return errors.New(`must match "` + idShortConstraintPattern + `"`)
	}
	return nil
}
