/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// Package common provides utility functions and shared components
// used across the BaSyx Go components implementation.
//
//nolint:revive
package common

import (
	"strings"
	"time"
)

// GetCurrentTimestamp returns the current timestamp in RFC3339 format.
// This function generates a timestamp string that is compliant with
// ISO 8601 and suitable for logging, API responses, and data serialization.
//
// Returns:
//   - A string representation of the current time in RFC3339 format
//     (e.g., "2006-01-02T15:04:05Z07:00")
//
// Example:
//
//	timestamp := GetCurrentTimestamp()
//	// Returns: "2025-11-03T13:45:30Z"
func GetCurrentTimestamp() string {
	timestamp := time.Now().Format(time.RFC3339)
	return timestamp
}

// NormalizeBasePath normalizes a URL path to ensure consistent formatting
// for API endpoints and routing. It handles common path formatting issues
// such as missing leading slashes and trailing slashes.
//
// The function applies the following transformations:
//   - Empty strings and single "/" are normalized to "/"
//   - Adds a leading "/" if missing
//   - Removes trailing "/" (except for root path)
//
// Parameters:
//   - p: The path string to normalize
//
// Returns:
//   - A normalized path string with proper leading slash and no trailing slash
//
// Examples:
//
//	NormalizeBasePath("")        // Returns: "/"
//	NormalizeBasePath("/")       // Returns: "/"
//	NormalizeBasePath("api")     // Returns: "/api"
//	NormalizeBasePath("/api/")   // Returns: "/api"
//	NormalizeBasePath("/api/v1") // Returns: "/api/v1"
func NormalizeBasePath(p string) string {
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}
