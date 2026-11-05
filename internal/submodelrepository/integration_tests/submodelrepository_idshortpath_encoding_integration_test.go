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
	"net/http"
	"net/url"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSubmodelElementByEncodedIDShortPath(t *testing.T) {
	h := newIDShortHarness(t, "off", idShortService{name: "standalone"})
	id := "urn:idshortpath:encoding:" + uuid.NewString()
	h.create(t, testenv.IDShortPathEncodingSubmodel(id), http.StatusCreated)
	elements := h.path(id) + "/submodel-elements/"

	for _, testCase := range testenv.IDShortPathEncodingCases() {
		t.Run(testCase.Name, func(t *testing.T) {
			element := h.json(t, http.MethodGet, elements+testCase.Segment, nil, http.StatusOK)
			require.Equal(t, testCase.IDShort, element["idShort"])
		})
	}

	t.Run("location of created element", func(t *testing.T) {
		body := map[string]any{"modelType": "Property", "valueType": "xs:string", "idShort": "x;y"}
		response := h.request(t, http.MethodPost, elements+"Coll", mustJSON(t, body), "application/json")
		require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
		location, err := url.Parse(response.Header().Get("Location"))
		require.NoError(t, err)
		require.Equal(t, "x;y", h.json(t, http.MethodGet, location.EscapedPath(), nil, http.StatusOK)["idShort"])
	})

	t.Run("delete by encoded path", func(t *testing.T) {
		h.json(t, http.MethodDelete, elements+"Coll.a%2Fb", nil, http.StatusNoContent)
		h.json(t, http.MethodGet, elements+"Coll.a%2Fb", nil, http.StatusNotFound)
	})
}
