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
	"net/http"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

func TestAPIParameterConformance(t *testing.T) {
	token, err := testenv.NewPasswordGrantTokenProvider(keycloakTokenURL, "basyx-ui", 10*time.Second).GetAccessToken(&testenv.TokenCredentials{User: "admin", Password: "pwd"})
	if err != nil {
		t.Fatal(err)
	}
	testenv.RunAPIParameterConformance(t, BaseURL, http.Header{"Authorization": {"Bearer " + token}}, []testenv.APIConformanceEndpoint{
		{Path: "/shell-descriptors", Filters: []string{"assetIds", "assetType"}, IdentifierPath: "/shell-descriptors/"},
		{Path: "/lookup/shells", Filters: []string{"assetIds"}},
		{Path: "/lookup/shellsByAssetLink", Method: "POST", Body: "[]"},
	})
}
