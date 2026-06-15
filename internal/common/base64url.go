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
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Encode takes a byte slice and returns a base64 URL-encoded string
// This encoding is URL and filename safe as specified in RFC 4648
func Encode(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	// Replace standard Base64 characters with URL-safe variants
	encoded = strings.ReplaceAll(encoded, "+", "-")
	encoded = strings.ReplaceAll(encoded, "/", "_")
	// Remove padding characters
	encoded = strings.TrimRight(encoded, "=")
	return encoded
}

// Decode takes a base64 URL-encoded string and returns the decoded bytes
// Returns an error if the input is not properly encoded
func Decode(encoded string) ([]byte, error) {
	// Restore the standard Base64 format
	standardB64 := strings.ReplaceAll(encoded, "-", "+")
	standardB64 = strings.ReplaceAll(standardB64, "_", "/")

	// Add padding if needed
	switch len(standardB64) % 4 {
	case 2:
		standardB64 += "=="
	case 3:
		standardB64 += "="
	}

	// Decode
	return base64.StdEncoding.DecodeString(standardB64)
}

// EncodeString is a convenience function that takes a string,
// converts it to bytes, and returns a base64 URL-encoded string
func EncodeString(data string) string {
	return Encode([]byte(data))
}

// DecodeString is a convenience function that decodes a base64 URL-encoded
// string and returns the decoded string
// Returns an error if the input is not properly encoded
func DecodeString(encoded string) (string, error) {
	bytes, err := Decode(encoded)
	if err != nil {
		return "", fmt.Errorf("not base64url encoded (%w)", err)
	}

	// Validate UTF-8
	if !utf8.Valid(bytes) {
		return "", fmt.Errorf("decoded value contains non-UTF8 bytes")
	}
	return string(bytes), nil
}
