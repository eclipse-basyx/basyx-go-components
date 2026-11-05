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

//nolint:all - package name is not meaningless
package common

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

// URLPathParam returns the percent-decoded value of the chi path parameter
// name.
//
// Go keeps the client's encoding in RawPath when it differs from Go's own,
// for example "%3B" for ";", and chi then routes on RawPath and returns the
// parameter still encoded. Otherwise chi routes on the decoded Path, so the
// value is decoded exactly once in both cases.
//
// Parameters:
//   - r: Request routed by chi
//   - name: Name of the path parameter
//
// Returns:
//   - string: Decoded parameter value; the raw value if it is not a valid
//     escaping, which net/http already rejects before routing
func URLPathParam(r *http.Request, name string) string {
	value := chi.URLParam(r, name)
	if r.URL.RawPath == "" {
		return value
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}
