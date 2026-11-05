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
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func TestSubmodelElementByEncodedIDShortPathAasRepository(t *testing.T) {
	submodelEndpoint := createEncodedIDShortPathSuperpath(t)
	elements := submodelEndpoint + "/submodel-elements/"

	for _, testCase := range testenv.IDShortPathEncodingCases() {
		t.Run(testCase.Name, func(t *testing.T) {
			element, statusCode, err := getJSONResponse(elements + testCase.Segment)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, statusCode, "%v", element)
			require.Equal(t, testCase.IDShort, element["idShort"])
		})
	}

	t.Run("location of created element", func(t *testing.T) {
		body := `{"modelType":"Property","valueType":"xs:string","idShort":"x;y"}`
		_, statusCode, headers, err := postJSONResponse(elements+"Coll", body)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, statusCode)
		element, statusCode, err := getJSONResponse(headers.Get("Location"))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, statusCode, "%v", element)
		require.Equal(t, "x;y", element["idShort"])
	})

	t.Run("delete by encoded path", func(t *testing.T) {
		statusCode, body, err := rawJSONRequest(http.MethodDelete, elements+"Coll.a%2Fb", "")
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, statusCode, string(body))
		statusCode, body, err = rawJSONRequest(http.MethodGet, elements+"Coll.a%2Fb", "")
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, statusCode, string(body))
	})
}

func createEncodedIDShortPathSuperpath(t *testing.T) string {
	t.Helper()
	suffix := time.Now().UnixNano()
	aasID := fmt.Sprintf("https://example.com/ids/aas/idshortpath-encoding-%d", suffix)
	statusCode, err := createAASForThumbnailTest(aasRepositoryBaseURL, aasID)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode)
	shellEndpoint := aasRepositoryBaseURL + "/shells/" + base64.RawURLEncoding.EncodeToString([]byte(aasID))
	t.Cleanup(func() { requireDeleted(t, shellEndpoint) })

	submodelID := fmt.Sprintf("https://example.com/ids/sm/idshortpath-encoding-%d", suffix)
	submodelEndpoint := shellEndpoint + "/submodels/" + base64.RawURLEncoding.EncodeToString([]byte(submodelID))
	body, err := json.Marshal(testenv.IDShortPathEncodingSubmodel(submodelID))
	require.NoError(t, err)
	_, statusCode, _, err = putJSONResponse(submodelEndpoint, string(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, statusCode)
	t.Cleanup(func() { requireDeleted(t, submodelEndpoint) })
	return submodelEndpoint
}

func requireDeleted(t *testing.T, endpoint string) {
	t.Helper()
	statusCode, body, err := rawJSONRequest(http.MethodDelete, endpoint, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, statusCode, string(body))
}
