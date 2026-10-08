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

func conditionalShellDescriptor(t *testing.T, id string, idShort string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"id":        id,
		"idShort":   idShort,
		"assetKind": "Instance",
		"endpoints": []any{map[string]any{
			"interface":           "AAS-3.0",
			"protocolInformation": map[string]any{"href": "https://example.com/shells/" + common.EncodeString(id)},
		}},
	})
	require.NoError(t, err)
	return body
}

func conditionalSubmodelDescriptor(t *testing.T, id string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"id": id,
		"endpoints": []any{map[string]any{
			"interface":           "SUBMODEL-3.0",
			"protocolInformation": map[string]any{"href": "https://example.com/submodels/" + common.EncodeString(id)},
		}},
	})
	require.NoError(t, err)
	return body
}

func TestConditionalShellDescriptorLifecycle(t *testing.T) {
	id := fmt.Sprintf("urn:etag:shell-descriptor:%d", time.Now().UnixNano())
	testenv.RunConditionalLifecycle(t, testenv.ConditionalLifecycle{
		ResourceURL: aasRegistryBaseURL + "/shell-descriptors/" + common.EncodeString(id),
		Create: func(t *testing.T) testenv.HTTPResult {
			return testenv.DoHTTP(t, http.MethodPost, aasRegistryBaseURL+"/shell-descriptors", conditionalShellDescriptor(t, id, "Created"), nil)
		},
		CreatedStatus: http.StatusCreated,
		UpdateMethod:  http.MethodPut,
		UpdateBody:    conditionalShellDescriptor(t, id, "Updated"),
		UpdatedStatus: http.StatusNoContent,
		DeletedStatus: http.StatusNoContent,
	})
}

func TestConditionalNestedSubmodelDescriptorsShareTheShellDescriptorRevision(t *testing.T) {
	id := fmt.Sprintf("urn:etag:nested:%d", time.Now().UnixNano())
	shellURL := aasRegistryBaseURL + "/shell-descriptors/" + common.EncodeString(id)
	created := testenv.DoHTTP(t, http.MethodPost, aasRegistryBaseURL+"/shell-descriptors", conditionalShellDescriptor(t, id, "Nested"), nil)
	require.Equal(t, http.StatusCreated, created.Status, string(created.Body))
	require.NotEmpty(t, created.Header.Get("ETag"))
	t.Cleanup(func() { testenv.DoHTTP(t, http.MethodDelete, shellURL, nil, nil) })

	before := testenv.DoHTTP(t, http.MethodGet, shellURL, nil, nil).Header.Get("ETag")
	submodelID := id + ":sm"
	stale := testenv.DoHTTP(t, http.MethodPost, shellURL+"/submodel-descriptors", conditionalSubmodelDescriptor(t, submodelID), map[string]string{"If-Match": `"1-00000000"`})
	require.Equal(t, http.StatusPreconditionFailed, stale.Status, string(stale.Body))
	added := testenv.DoHTTP(t, http.MethodPost, shellURL+"/submodel-descriptors", conditionalSubmodelDescriptor(t, submodelID), map[string]string{"If-Match": before})
	require.Equal(t, http.StatusCreated, added.Status, string(added.Body))

	after := testenv.DoHTTP(t, http.MethodGet, shellURL, nil, nil).Header.Get("ETag")
	require.NotEqual(t, before, after)
	nestedURL := shellURL + "/submodel-descriptors/" + common.EncodeString(submodelID)
	nested := testenv.DoHTTP(t, http.MethodGet, nestedURL, nil, nil)
	require.Equal(t, http.StatusOK, nested.Status)
	require.Equal(t, http.StatusPreconditionFailed, testenv.DoHTTP(t, http.MethodDelete, nestedURL, nil, map[string]string{"If-Match": before}).Status)
	require.Equal(t, http.StatusNoContent, testenv.DoHTTP(t, http.MethodDelete, nestedURL, nil, map[string]string{"If-Match": nested.Header.Get("ETag")}).Status)
}
