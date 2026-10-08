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
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func conditionalShell(t *testing.T, id string, idShort string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"modelType":        "AssetAdministrationShell",
		"id":               id,
		"idShort":          idShort,
		"assetInformation": map[string]any{"assetKind": "Instance", "globalAssetId": id + ":asset"},
	})
	require.NoError(t, err)
	return body
}

func TestConditionalShellLifecycle(t *testing.T) {
	id := fmt.Sprintf("urn:etag:shell:%d", time.Now().UnixNano())
	testenv.RunConditionalLifecycle(t, testenv.ConditionalLifecycle{
		ResourceURL: aasRepositoryBaseURL + "/shells/" + common.EncodeString(id),
		Create: func(t *testing.T) testenv.HTTPResult {
			return testenv.DoHTTP(t, http.MethodPost, aasRepositoryBaseURL+"/shells", conditionalShell(t, id, "Created"), nil)
		},
		CreatedStatus: http.StatusCreated,
		UpdateMethod:  http.MethodPut,
		UpdateBody:    conditionalShell(t, id, "Updated"),
		UpdatedStatus: http.StatusNoContent,
		DeletedStatus: http.StatusNoContent,
	})
}

func TestConditionalShellPartsShareTheShellRevision(t *testing.T) {
	id := fmt.Sprintf("urn:etag:shell-parts:%d", time.Now().UnixNano())
	shellURL := aasRepositoryBaseURL + "/shells/" + common.EncodeString(id)
	created := testenv.DoHTTP(t, http.MethodPost, aasRepositoryBaseURL+"/shells", conditionalShell(t, id, "Parts"), nil)
	require.Equal(t, http.StatusCreated, created.Status, string(created.Body))
	t.Cleanup(func() { testenv.DoHTTP(t, http.MethodDelete, shellURL, nil, nil) })

	shell := testenv.DoHTTP(t, http.MethodGet, shellURL, nil, nil)
	assetInformation := testenv.DoHTTP(t, http.MethodGet, shellURL+"/asset-information", nil, nil)
	require.Equal(t, http.StatusOK, assetInformation.Status)
	require.NotEqual(t, shell.Header.Get("ETag"), assetInformation.Header.Get("ETag"))

	update, err := json.Marshal(map[string]any{"assetKind": "Instance", "globalAssetId": id + ":changed"})
	require.NoError(t, err)
	stale := testenv.DoHTTP(t, http.MethodPut, shellURL+"/asset-information", update, map[string]string{"If-Match": `"1-00000000"`})
	require.Equal(t, http.StatusPreconditionFailed, stale.Status, string(stale.Body))
	updated := testenv.DoHTTP(t, http.MethodPut, shellURL+"/asset-information", update, map[string]string{"If-Match": assetInformation.Header.Get("ETag")})
	require.Equal(t, http.StatusNoContent, updated.Status, string(updated.Body))

	reference, err := json.Marshal(map[string]any{"type": "ModelReference", "keys": []any{map[string]any{"type": "Submodel", "value": id + ":sm"}}})
	require.NoError(t, err)
	staleReference := testenv.DoHTTP(t, http.MethodPost, shellURL+"/submodel-refs", reference, map[string]string{"If-Match": shell.Header.Get("ETag")})
	require.Equal(t, http.StatusPreconditionFailed, staleReference.Status, "the asset information update must change the shell revision")

	current := testenv.DoHTTP(t, http.MethodGet, shellURL, nil, nil).Header.Get("ETag")
	added := testenv.DoHTTP(t, http.MethodPost, shellURL+"/submodel-refs", reference, map[string]string{"If-Match": current})
	require.Equal(t, http.StatusCreated, added.Status, string(added.Body))
	require.NotEmpty(t, added.Header.Get("ETag"))
	require.NotEqual(t, current, testenv.DoHTTP(t, http.MethodGet, shellURL, nil, nil).Header.Get("ETag"))
}

func TestWildcardsOfThumbnailUploadsReferToTheThumbnail(t *testing.T) {
	id := fmt.Sprintf("urn:etag:thumbnail:%d", time.Now().UnixNano())
	shellURL := aasRepositoryBaseURL + "/shells/" + common.EncodeString(id)
	created := testenv.DoHTTP(t, http.MethodPost, aasRepositoryBaseURL+"/shells", conditionalShell(t, id, "Thumbnail"), nil)
	require.Equal(t, http.StatusCreated, created.Status, string(created.Body))
	t.Cleanup(func() { testenv.DoHTTP(t, http.MethodDelete, shellURL, nil, nil) })

	image, err := os.ReadFile("testFiles/marcus.gif")
	require.NoError(t, err)
	thumbnailURL := shellURL + "/asset-information/thumbnail"
	upload := func(headers map[string]string) int {
		return testenv.DoMultipartUpload(t, thumbnailURL, "marcus.gif", image, headers).Status
	}
	require.Equal(t, http.StatusPreconditionFailed, upload(map[string]string{"If-Match": "*"}), "If-Match: * requires the thumbnail, not only its shell")
	require.Equal(t, http.StatusNoContent, upload(map[string]string{"If-None-Match": "*"}))
	require.Equal(t, http.StatusPreconditionFailed, upload(map[string]string{"If-None-Match": "*"}))
	require.Equal(t, http.StatusNoContent, upload(map[string]string{"If-Match": "*"}))
}
