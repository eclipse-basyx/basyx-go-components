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

//nolint:all
package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"
	"time"

	commonjws "github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	"github.com/stretchr/testify/require"
	jose "gopkg.in/go-jose/go-jose.v2"
)

func TestAASEnvironmentSignedReadsReturnPlainTextJWS(t *testing.T) {
	resetDatabase(t)

	aasID := fmt.Sprintf("https://example.com/ids/aasenv/signed-%d", time.Now().UnixNano())
	encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(aasID))
	submodelID := fmt.Sprintf("urn:example:aasenv:submodel:signed:%d", time.Now().UnixNano())
	encodedSubmodelID := base64.RawURLEncoding.EncodeToString([]byte(submodelID))

	t.Cleanup(func() {
		doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodDelete, aasEnvBaseURL+"/shells/"+encodedAASID, nil)
		doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodDelete, aasEnvBaseURL+"/submodels/"+encodedSubmodelID, nil)
	})

	status, body, _ := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPost, aasEnvBaseURL+"/shells", map[string]any{
		"id":               aasID,
		"modelType":        "AssetAdministrationShell",
		"assetInformation": map[string]any{"assetKind": "Instance"},
	})
	require.Equal(t, http.StatusCreated, status, string(body))
	status, body, _ = doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodPost, aasEnvBaseURL+"/submodels", map[string]any{
		"id":        submodelID,
		"modelType": "Submodel",
	})
	require.Equal(t, http.StatusCreated, status, string(body))

	requirePlainTextJWSContaining(t, aasEnvBaseURL+"/shells/"+encodedAASID+"/$signed", aasID)
	requirePlainTextJWSContaining(t, aasEnvBaseURL+"/submodels/"+encodedSubmodelID+"/$signed", submodelID)
}

func requirePlainTextJWSContaining(t *testing.T, endpoint string, identifier string) {
	t.Helper()

	status, body, headers := doAASEnvRequest(t, aasEnvNoRedirectClient, http.MethodGet, endpoint, nil)
	require.Equal(t, http.StatusOK, status, string(body))
	require.Equal(t, "text/plain", headers.Get("Content-Type"))

	privateKey, err := commonjws.LoadPrivateKey("../../submodelrepository/integration_tests/docker_compose/rsa-key.pem")
	require.NoError(t, err)
	signed, err := jose.ParseSigned(string(body))
	require.NoError(t, err, "response=%s", body)
	payload, err := signed.Verify(&privateKey.PublicKey)
	require.NoError(t, err)
	require.Contains(t, string(payload), identifier)
}
