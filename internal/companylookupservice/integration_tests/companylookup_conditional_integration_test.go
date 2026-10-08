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

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func TestConditionalCompanyLifecycle(t *testing.T) {
	domain := fmt.Sprintf("etag%d.example.com", time.Now().UnixNano())
	body := func(name string) []byte {
		encoded, err := json.Marshal(map[string]any{
			"idShort": "etagCompany",
			"name":    name,
			"domain":  domain,
			"endpoints": []any{map[string]any{
				"interface":           "AAS-REGISTRY-3.0",
				"protocolInformation": map[string]any{"href": "https://" + domain + "/aas-registry", "endpointProtocol": "HTTPS"},
			}},
		})
		require.NoError(t, err)
		return encoded
	}
	testenv.RunConditionalLifecycle(t, testenv.ConditionalLifecycle{
		ResourceURL: companyLookupBaseURL + "/companies/" + common.EncodeString(domain),
		Create: func(t *testing.T) testenv.HTTPResult {
			return testenv.DoHTTP(t, http.MethodPost, companyLookupBaseURL+"/companies", body("Created"), nil)
		},
		CreatedStatus: http.StatusCreated,
		UpdateMethod:  http.MethodPut,
		UpdateBody:    body("Updated"),
		UpdatedStatus: http.StatusOK,
		DeletedStatus: http.StatusNoContent,
	})
}
