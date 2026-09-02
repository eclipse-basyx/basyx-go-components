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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"fmt"
	"net/http"
	"testing"
)

func TestMapPersistenceErrorPreservesWrappedDPPHTTPError(t *testing.T) {
	err := fmt.Errorf(
		"DPP-CONTENT-FULL convert submodel: %w",
		newDPPHTTPError(
			http.StatusInternalServerError,
			"DPP-FILEURL-MISSINGEXTERNALURL",
			"managed attachment requires a valid general.externalUrl",
		),
	)

	response := mapPersistenceError(err, http.StatusNotFound)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("mapPersistenceError() status = %d, want %d", response.Code, http.StatusInternalServerError)
	}

	result, ok := response.Body.(Result)
	if !ok {
		t.Fatalf("mapPersistenceError() body type = %T, want Result", response.Body)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("mapPersistenceError() messages = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Code != "DPP-FILEURL-MISSINGEXTERNALURL" {
		t.Fatalf(
			"mapPersistenceError() code = %q, want %q",
			result.Messages[0].Code,
			"DPP-FILEURL-MISSINGEXTERNALURL",
		)
	}
}
