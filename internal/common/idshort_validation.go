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

package common

import (
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// IDShortRequirement describes the idShort type of the AAS metamodel for error
// messages.
const IDShortRequirement = "an idShort must have 2 to 128 characters, contain only letters, digits, '-' and '_', start with a letter and not end with '-'"

// ValidateOptionalIDShort enforces the idShort type of the AAS metamodel in
// every verification mode, because idShorts are stored as segments of
// idShortPaths and must stay addressable in URLs. Like everywhere else in the
// repositories, an empty idShort counts as omitted, and an omitted one is valid.
func ValidateOptionalIDShort(errorScope string, subject string, idShort *string) error {
	if idShort == nil || *idShort == "" || model.IsValidIDShort(*idShort) {
		return nil
	}
	return NewErrInvalidIDShort(errorScope, subject, *idShort)
}

// NewErrInvalidIDShort creates the 400 error for an idShort that violates the
// idShort type of the AAS metamodel. The error code is errorScope-INVALIDIDSHORT.
// The message quotes at most the first model.MaxIDShortLength bytes of idShort,
// so that an overlong idShort does not inflate the response.
func NewErrInvalidIDShort(errorScope string, subject string, idShort string) error {
	if len(idShort) > model.MaxIDShortLength {
		idShort = strings.ToValidUTF8(idShort[:model.MaxIDShortLength], "") + "..."
	}
	return NewErrBadRequest(errorScope + "-INVALIDIDSHORT " + subject + " has the invalid idShort '" + idShort + "': " + IDShortRequirement)
}
