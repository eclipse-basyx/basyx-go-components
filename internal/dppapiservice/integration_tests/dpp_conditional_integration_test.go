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

package integration_tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const dppRepresentationETagPattern = `^"\d+-[0-9a-f]{16}-[0-9a-f]{16}"$`

// assertDPPConditionalRequests checks that a DPP's entity tag covers its
// shell and all its Submodels, including changes made outside the DPP API.
func assertDPPConditionalRequests(t *testing.T, baseURL string, aasBaseURL string, encodedDPPID string, elementPath string, submodelID string) {
	t.Helper()
	dppURL := baseURL + "/v1/dpps/" + encodedDPPID
	read := testenv.DoHTTP(t, http.MethodGet, dppURL, nil, nil)
	require.Equal(t, http.StatusOK, read.Status)
	etag := read.Header.Get("ETag")
	require.Regexp(t, dppRepresentationETagPattern, etag)
	require.Equal(t, http.StatusNotModified, testenv.DoHTTP(t, http.MethodGet, dppURL, nil, map[string]string{"If-None-Match": etag}).Status)

	element := testenv.DoHTTP(t, http.MethodGet, dppURL+"/elements/"+elementPath, nil, nil)
	require.Equal(t, http.StatusOK, element.Status)
	require.Equal(t, dppValidator(etag), dppValidator(element.Header.Get("ETag")))

	patch := mustMarshal(t, map[string]any{"facilityId": "facility-etag"})
	require.Equal(t, http.StatusPreconditionFailed, testenv.DoHTTP(t, http.MethodPatch, dppURL, patch, map[string]string{"If-Match": `"1-0000000000000000"`}).Status)
	require.Equal(t, etag, testenv.DoHTTP(t, http.MethodGet, dppURL, nil, nil).Header.Get("ETag"))
	patched := testenv.DoHTTP(t, http.MethodPatch, dppURL, patch, map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, patched.Status, string(patched.Body))
	require.Regexp(t, `^"\d+-[0-9a-f]{16}"$`, patched.Header.Get("ETag"))
	afterPatch := testenv.DoHTTP(t, http.MethodGet, dppURL, nil, nil).Header.Get("ETag")
	require.NotEqual(t, etag, afterPatch)
	require.Equal(t, dppValidator(patched.Header.Get("ETag")), dppValidator(afterPatch))

	standalone := testenv.DoHTTP(t, http.MethodPatch,
		aasBaseURL+"/submodels/"+common.EncodeString(submodelID)+"/submodel-elements/energyClass/$value", mustMarshal(t, "A"), nil)
	require.Equal(t, http.StatusNoContent, standalone.Status, string(standalone.Body))
	afterStandalone := testenv.DoHTTP(t, http.MethodGet, dppURL, nil, nil).Header.Get("ETag")
	require.NotEqual(t, afterPatch, afterStandalone, "a change of a member Submodel must change the DPP entity tag")
	staleElement := testenv.DoHTTP(t, http.MethodPatch, dppURL+"/elements/"+elementPath, mustMarshal(t, 122), map[string]string{"If-Match": afterPatch})
	require.Equal(t, http.StatusPreconditionFailed, staleElement.Status, string(staleElement.Body))
}

func dppValidator(etag string) string {
	parts := strings.Split(strings.Trim(etag, `"`), "-")
	if len(parts) < 2 {
		return etag
	}
	return parts[0] + "-" + parts[1]
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	return body
}
