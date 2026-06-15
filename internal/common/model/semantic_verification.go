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
	"log"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/verification"
)

// ValidateWithMode executes semantic verification based on the selected mode.
// In off mode, verification is skipped. In permissive mode, verification errors
// are logged as warnings. In strict mode, verification errors are returned.
func ValidateWithMode(mode VerificationMode, warningContext string, verify func(func(*verification.VerificationError) bool), strictErrorFactory func(string) error) error {
	normalizedMode := NormalizeVerificationMode(mode)
	if normalizedMode == VerificationModeOff {
		return nil
	}

	validationErrors := make([]string, 0)
	verify(func(verErr *verification.VerificationError) bool {
		validationErrors = append(validationErrors, verErr.Error())
		return false
	})

	if len(validationErrors) == 0 {
		return nil
	}

	joined := strings.Join(validationErrors, "; ")
	if normalizedMode == VerificationModePermissive {
		log.Printf("WARN: %s: %s", warningContext, joined)
		return nil
	}

	if strictErrorFactory != nil {
		return strictErrorFactory(joined)
	}
	return errors.New(joined)
}
