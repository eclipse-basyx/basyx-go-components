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

package bench

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

func adminHeaders(t *testing.T) map[string]string {
	t.Helper()
	token, err := testenv.NewPasswordGrantTokenProvider(keycloakTokenURL, "basyx-ui", 10*time.Second).
		GetAccessToken(&testenv.TokenCredentials{User: "admin", Password: "pwd"})
	require.NoError(t, err)
	return map[string]string{"Authorization": "Bearer " + token}
}

func TestConditionalDigitalTwinShellDescriptorLifecycle(t *testing.T) {
	headers := adminHeaders(t)
	id := fmt.Sprintf("urn:etag:dtr:%d", time.Now().UnixNano())
	body := func(idShort string) []byte {
		encoded, err := json.Marshal(map[string]any{
			"id":               id,
			"idShort":          idShort,
			"globalAssetId":    id + ":asset",
			"specificAssetIds": []any{map[string]any{"name": "partInstanceId", "value": id}},
		})
		require.NoError(t, err)
		return encoded
	}
	testenv.RunConditionalLifecycle(t, testenv.ConditionalLifecycle{
		ResourceURL: BaseURL + "/shell-descriptors/" + common.EncodeString(id),
		Create: func(t *testing.T) testenv.HTTPResult {
			return testenv.DoHTTP(t, http.MethodPost, BaseURL+"/shell-descriptors", body("Created"), headers)
		},
		CreatedStatus: http.StatusCreated,
		UpdateMethod:  http.MethodPut,
		UpdateBody:    body("Updated"),
		UpdatedStatus: http.StatusNoContent,
		DeletedStatus: http.StatusNoContent,
		Headers:       headers,
	})
}

func TestConditionalDigitalTwinAssetLinksFollowTheDescriptor(t *testing.T) {
	headers := adminHeaders(t)
	id := fmt.Sprintf("urn:etag:dtr-links:%d", time.Now().UnixNano())
	descriptorURL := BaseURL + "/shell-descriptors/" + common.EncodeString(id)
	linksURL := BaseURL + "/lookup/shells/" + common.EncodeString(id)
	body, err := json.Marshal(map[string]any{
		"id":               id,
		"idShort":          "Links",
		"specificAssetIds": []any{map[string]any{"name": "partInstanceId", "value": id}},
	})
	require.NoError(t, err)
	created := testenv.DoHTTP(t, http.MethodPost, BaseURL+"/shell-descriptors", body, headers)
	require.Equal(t, http.StatusCreated, created.Status, string(created.Body))
	t.Cleanup(func() { testenv.DoHTTP(t, http.MethodDelete, descriptorURL, nil, headers) })

	descriptorETag := testenv.DoHTTP(t, http.MethodGet, descriptorURL, nil, headers).Header.Get("ETag")
	require.NotEmpty(t, descriptorETag)
	links := testenv.DoHTTP(t, http.MethodGet, linksURL, nil, headers)
	require.Equal(t, http.StatusOK, links.Status, string(links.Body))
	etag := links.Header.Get("ETag")
	require.NotEmpty(t, etag)

	added, err := json.Marshal([]any{map[string]any{"name": "customerPartId", "value": id}})
	require.NoError(t, err)
	stale := testenv.DoHTTP(t, http.MethodPost, linksURL, added, map[string]string{"Authorization": headers["Authorization"], "If-Match": `"1-00000000"`})
	require.Equal(t, http.StatusPreconditionFailed, stale.Status, string(stale.Body))
	updated := testenv.DoHTTP(t, http.MethodPost, linksURL, added, map[string]string{"Authorization": headers["Authorization"], "If-Match": etag})
	require.Equal(t, http.StatusCreated, updated.Status, string(updated.Body))
	require.NotEqual(t, etag, testenv.DoHTTP(t, http.MethodGet, linksURL, nil, headers).Header.Get("ETag"))

	overwrite := testenv.DoHTTP(t, http.MethodPut, descriptorURL, body, map[string]string{"Authorization": headers["Authorization"], "If-Match": descriptorETag})
	require.Equal(t, http.StatusPreconditionFailed, overwrite.Status, "asset links added through discovery change the descriptor")
}
