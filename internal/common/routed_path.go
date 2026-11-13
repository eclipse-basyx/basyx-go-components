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
	"errors"
	"net/http"
	"net/url"
	"regexp"
)

// IDShortSegmentPattern matches one idShort inside an idShortPath.
const IDShortSegmentPattern = `[A-Za-z](?:[A-Za-z0-9_-]*[A-Za-z0-9_])?`

const concreteListIndexPattern = `(?:\[(?:0|[1-9][0-9]*)\])*`

var concreteIDShortPathRegex = regexp.MustCompile(`^` + IDShortSegmentPattern + concreteListIndexPattern +
	`(?:\.` + IDShortSegmentPattern + concreteListIndexPattern + `)*$`)

// IsConcreteIDShortPath reports whether value is an idShortPath that addresses
// exactly one SubmodelElement: idShorts joined by "." with numeric list
// indices. Wildcards, empty indices and surrounding whitespace are rejected.
func IsConcreteIDShortPath(value string) bool {
	return concreteIDShortPathRegex.MatchString(value)
}

// RoutedPath returns the path the router matches: RawPath when it is set,
// otherwise the already decoded Path.
func RoutedPath(requestURL *url.URL) string {
	if requestURL.RawPath == "" {
		return requestURL.Path
	}
	return requestURL.RawPath
}

// DecodeRoutedPathParam decodes a value taken from RoutedPath exactly once.
// Values from RawPath are still percent-encoded; values from Path are not.
func DecodeRoutedPathParam(requestURL *url.URL, value string) (string, error) {
	if requestURL.RawPath == "" {
		return value, nil
	}
	return url.PathUnescape(value)
}

// ParseRoutedIDShortPath decodes an idShortPath path parameter and accepts it
// only if it is a concrete idShortPath. A valid idShortPath contains neither
// "/" nor "%", so the result is exactly the value authorization checks.
func ParseRoutedIDShortPath(r *http.Request, value string) (string, error) {
	decoded, err := DecodeRoutedPathParam(r.URL, value)
	if err != nil {
		return "", errors.New("COMMON-IDSHORTPATHPARAM-DECODE idShortPath is not correctly percent-encoded")
	}
	if !IsConcreteIDShortPath(decoded) {
		return "", errors.New(`COMMON-IDSHORTPATHPARAM-GRAMMAR idShortPath must consist of idShorts joined by "." with optional list indices such as "[0]"`)
	}
	return decoded, nil
}
