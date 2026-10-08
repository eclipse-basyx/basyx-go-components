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

package conditional

import (
	"errors"
	"net/http"
)

// PreconditionError reports a failed (412) or missing (428) precondition.
// It carries its HTTP status, so error responses keep it even when a caller
// passes a different fallback status.
type PreconditionError struct {
	status  int
	message string
}

func newPreconditionFailed(message string) *PreconditionError {
	return &PreconditionError{status: http.StatusPreconditionFailed, message: message}
}

func newPreconditionRequired(message string) *PreconditionError {
	return &PreconditionError{status: http.StatusPreconditionRequired, message: message}
}

func (e *PreconditionError) Error() string {
	return http.StatusText(e.status) + ": " + e.message
}

// HTTPStatus returns 412 or 428.
func (e *PreconditionError) HTTPStatus() int {
	return e.status
}

// IsPreconditionError reports whether err is a failed or missing precondition.
func IsPreconditionError(err error) bool {
	var preconditionErr *PreconditionError
	return errors.As(err, &preconditionErr)
}

// IsPreconditionFailed reports whether err is a failed precondition (412).
func IsPreconditionFailed(err error) bool {
	var preconditionErr *PreconditionError
	return errors.As(err, &preconditionErr) && preconditionErr.status == http.StatusPreconditionFailed
}

// IsPreconditionRequired reports whether err is a missing precondition (428).
func IsPreconditionRequired(err error) bool {
	var preconditionErr *PreconditionError
	return errors.As(err, &preconditionErr) && preconditionErr.status == http.StatusPreconditionRequired
}

func errIfMatchFailed() error {
	return newPreconditionFailed("COMMON-CONDREQ-IFMATCH the resource does not match If-Match")
}

func errIfNoneMatchFailed() error {
	return newPreconditionFailed("COMMON-CONDREQ-IFNONEMATCH the resource matches If-None-Match")
}

func errIfMatchRequired() error {
	return newPreconditionRequired("COMMON-CONDREQ-IFMATCHREQUIRED If-Match is required to change this resource")
}
