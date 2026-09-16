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

package integration_tests

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func testDPPLongIdentifierLifecycle(
	t *testing.T,
	client *http.Client,
	baseURL, aasBaseURL, idSuffix string,
	now time.Time,
) {
	t.Helper()
	tests := []struct {
		name       string
		identifier string
		length     int
	}{
		{
			name: "157 character URL identifier",
			identifier: longDPPIntegrationIdentifier(
				"https://manufacturer.example/digital-product-passports/"+idSuffix+"/series-a/item?variant=blue&serial=",
				157,
			),
			length: 157,
		},
		{
			name: "175 character punctuation and non-ASCII-leading identifier",
			identifier: longDPPIntegrationIdentifier(
				"製品/urn:example:dpp:"+idSuffix+":series_b.item-",
				175,
			),
			length: 175,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := utf8.RuneCountInString(test.identifier); got != test.length {
				t.Fatalf("identifier length = %d, want %d", got, test.length)
			}
			productID := "https://manufacturer.example/products/" + idSuffix + "/" + strconv.Itoa(index)
			created := doJSON(
				t,
				client,
				http.MethodPost,
				baseURL+"/v1/dpps",
				lifecycleDPPDocument(test.identifier, productID, now.Add(time.Duration(index)*time.Second)),
				http.StatusCreated,
			)
			assertJSONPathEquals(t, created, "digitalProductPassportId", test.identifier)

			encodedIdentifier := encodedPathParam(test.identifier)
			read := doJSON(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedIdentifier, nil, http.StatusOK)
			assertJSONPathEquals(t, read, "digitalProductPassportId", test.identifier)
			aas := doJSON(
				t,
				client,
				http.MethodGet,
				aasBaseURL+"/shells/"+common.EncodeString(test.identifier),
				nil,
				http.StatusOK,
			)
			assertJSONPathEquals(t, aas, "id", test.identifier)
			assertJSONPathEquals(t, aas, "idShort", "DPP")

			doJSON(t, client, http.MethodDelete, baseURL+"/v1/dpps/"+encodedIdentifier, nil, http.StatusNoContent)
			doJSONAny(t, client, http.MethodGet, baseURL+"/v1/dpps/"+encodedIdentifier, nil, http.StatusNotFound)
			doJSONAny(
				t,
				client,
				http.MethodGet,
				aasBaseURL+"/shells/"+common.EncodeString(test.identifier),
				nil,
				http.StatusNotFound,
			)
		})
	}
}

func longDPPIntegrationIdentifier(prefix string, length int) string {
	return prefix + strings.Repeat("x", length-utf8.RuneCountInString(prefix))
}
