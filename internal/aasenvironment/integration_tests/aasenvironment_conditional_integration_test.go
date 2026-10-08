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
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func mustJSONBody(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	return body
}

func concurrencyValidator(etag string) string {
	parts := strings.Split(strings.Trim(etag, `"`), "-")
	if len(parts) < 2 {
		return etag
	}
	return parts[0] + "-" + parts[1]
}

func TestConditionalEnvironmentMatchesStandaloneRevisions(t *testing.T) {
	suffix := time.Now().UnixNano()
	submodelID := fmt.Sprintf("urn:etag:env:sm:%d", suffix)
	shellID := fmt.Sprintf("urn:etag:env:aas:%d", suffix)
	submodelURL := aasEnvBaseURL + "/submodels/" + common.EncodeString(submodelID)
	shellURL := aasEnvBaseURL + "/shells/" + common.EncodeString(shellID)
	superpathURL := shellURL + "/submodels/" + common.EncodeString(submodelID)
	submodel := func(value string) []byte {
		return mustJSONBody(t, map[string]any{
			"modelType": "Submodel", "id": submodelID, "idShort": "EnvSubmodel",
			"submodelElements": []any{map[string]any{"modelType": "Property", "idShort": "Prop", "valueType": "xs:string", "value": value}},
		})
	}
	require.Equal(t, http.StatusCreated, testenv.DoHTTP(t, http.MethodPost, aasEnvBaseURL+"/submodels", submodel("initial"), nil).Status)
	t.Cleanup(func() { testenv.DoHTTP(t, http.MethodDelete, submodelURL, nil, nil) })
	shell := testenv.DoHTTP(t, http.MethodPost, aasEnvBaseURL+"/shells", mustJSONBody(t, map[string]any{
		"modelType": "AssetAdministrationShell", "id": shellID, "idShort": "EnvShell",
		"assetInformation": map[string]any{"assetKind": "Instance", "globalAssetId": shellID + ":asset"},
		"submodels":        []any{map[string]any{"type": "ModelReference", "keys": []any{map[string]any{"type": "Submodel", "value": submodelID}}}},
	}), nil)
	require.Equal(t, http.StatusCreated, shell.Status, string(shell.Body))
	t.Cleanup(func() { testenv.DoHTTP(t, http.MethodDelete, shellURL, nil, nil) })

	standalone := testenv.DoHTTP(t, http.MethodGet, submodelURL, nil, nil).Header.Get("ETag")
	superpath := testenv.DoHTTP(t, http.MethodGet, superpathURL, nil, nil).Header.Get("ETag")
	require.NotEmpty(t, standalone)
	require.Equal(t, concurrencyValidator(standalone), concurrencyValidator(superpath))

	patched := testenv.DoHTTP(t, http.MethodPatch, superpathURL+"/submodel-elements/Prop/$value", mustJSONBody(t, "superpath"), map[string]string{"If-Match": standalone})
	require.Equal(t, http.StatusNoContent, patched.Status, string(patched.Body))
	require.Equal(t, http.StatusPreconditionFailed, testenv.DoHTTP(t, http.MethodPut, submodelURL, submodel("put"), map[string]string{"If-Match": standalone}).Status)
	current := testenv.DoHTTP(t, http.MethodGet, submodelURL, nil, nil).Header.Get("ETag")
	put := testenv.DoHTTP(t, http.MethodPut, submodelURL, submodel("put"), map[string]string{"If-Match": current})
	require.Equal(t, http.StatusNoContent, put.Status, string(put.Body))

	shellETag := testenv.DoHTTP(t, http.MethodGet, shellURL, nil, nil).Header.Get("ETag")
	require.Equal(t, http.StatusPreconditionFailed, testenv.DoHTTP(t, http.MethodDelete, shellURL, nil, map[string]string{"If-Match": standalone}).Status)
	require.Equal(t, http.StatusNoContent, testenv.DoHTTP(t, http.MethodDelete, shellURL, nil, map[string]string{"If-Match": shellETag}).Status)
}

func TestUploadWritesResourceRevisions(t *testing.T) {
	db, err := common.NewDatabaseConnection(integrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	before := testenv.MaxResourceRevision(t, db, "aas")

	content, err := os.ReadFile("testdata/IESEDriveMotorDM3000.aasx")
	require.NoError(t, err)
	status, body := uploadAASXPayload(t, content, "IESEDriveMotorDM3000.aasx")
	require.Equal(t, http.StatusOK, status, string(body))

	require.Greater(t, testenv.MaxResourceRevision(t, db, "aas"), before)
}
