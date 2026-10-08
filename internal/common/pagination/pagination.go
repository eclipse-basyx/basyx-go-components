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

// Package pagination holds the limits shared by every paginated endpoint.
package pagination

import "math"

const (
	// MaxSupportedLimit is the largest page size the services can serve.
	//
	// Page queries fetch one extra row to detect a following page, so the
	// largest supported page size leaves room for that row within an int32.
	MaxSupportedLimit = math.MaxInt32 - 1

	// maxBufferCapacity bounds the rows reserved before any row is read.
	maxBufferCapacity = 1024
)

// BufferCapacity returns the initial slice capacity for a page of up to
// limit rows.
//
// A large page size must not reserve memory before the database returned a
// single row; slices grow on demand beyond the returned capacity.
//
// Parameters:
//   - limit: Number of rows the page may hold, including lookahead rows.
//
// Returns:
//   - int: Capacity between 0 and the internal bound.
func BufferCapacity(limit int) int {
	return min(max(limit, 0), maxBufferCapacity)
}
