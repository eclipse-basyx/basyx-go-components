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
	"bytes"
	"encoding/json"
	"encoding/xml"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strings"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/xmlization"
	aasx "github.com/aas-core-works/aas-package3-golang/v2"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var idShortExportFormats = []string{"application/json", "application/xml", "application/aasx+json", "application/aasx+xml"}

func TestSubmodelIDShortSerialization(t *testing.T) {
	h := newIDShortHarness(t, "strict", idShortService{name: "environment", environment: true})
	for _, testCase := range []struct {
		name  string
		value *string
	}{
		{name: "omitted"}, {name: "populated", value: new("ExportedName")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			id := h.seed(t, testCase.value)
			for _, format := range idShortExportFormats {
				t.Run(format, func(t *testing.T) {
					assertIDShortEnvironment(t, format, h.serialize(t, id, format), id, testCase.value)
				})
			}
		})
	}
	t.Run("stored empty remains empty", func(t *testing.T) {
		id := h.seed(t, new(""))
		h.assertState(t, id, new(""))
		for _, format := range []string{"application/json", "application/xml"} {
			assertIDShortEnvironment(t, format, h.serialize(t, id, format), id, new(""))
		}
	})
}

func TestSubmodelIDShortImport(t *testing.T) {
	h := newIDShortHarness(t, "strict", idShortService{name: "environment", environment: true})
	for _, format := range idShortExportFormats {
		t.Run(format, func(t *testing.T) { h.testImport(t, format) })
	}
	for _, testCase := range []struct {
		name  string
		value any
	}{
		{name: "null"}, {name: "empty", value: ""},
	} {
		t.Run("JSON import rejects "+testCase.name, func(t *testing.T) {
			id := "urn:idshort:submodel:" + uuid.NewString()
			h.track(t, id)
			payload, err := json.Marshal(map[string]any{"submodels": []any{
				map[string]any{"modelType": "Submodel", "id": id, "idShort": testCase.value},
			}})
			require.NoError(t, err)
			h.upload(t, "application/json", payload, http.StatusBadRequest)
			h.json(t, http.MethodGet, h.path(id), nil, http.StatusNotFound)
		})
	}
}

func (h *idShortHarness) serialize(t *testing.T, id, format string) []byte {
	t.Helper()
	request := httptest.NewRequestWithContext(h.ctx, http.MethodGet,
		"/serialization?submodelIds="+common.EncodeString(id)+"&includeConceptDescriptions=false", nil)
	request.Header.Set("Accept", format)
	response := httptest.NewRecorder()
	h.handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, format, response.Header().Get("Content-Type"))
	return response.Body.Bytes()
}

func assertIDShortEnvironment(t *testing.T, format string, payload []byte, id string, expected *string) {
	t.Helper()
	if strings.Contains(format, "aasx") {
		reader, err := aasx.NewPackaging().OpenReadFromStream(bytes.NewReader(payload))
		require.NoError(t, err)
		defer func() { require.NoError(t, reader.Close()) }()
		specs, err := reader.Specs()
		require.NoError(t, err)
		require.Len(t, specs, 1)
		payload, err = specs[0].ReadAllBytes()
		require.NoError(t, err)
	}
	if strings.HasSuffix(format, "json") {
		var environment struct {
			Submodels []map[string]any `json:"submodels"`
		}
		require.NoError(t, json.Unmarshal(payload, &environment))
		require.Len(t, environment.Submodels, 1)
		require.Equal(t, id, environment.Submodels[0]["id"])
		assertIDShort(t, environment.Submodels[0], expected)
		return
	}
	var environment struct {
		Submodels []struct {
			ID      string  `xml:"id"`
			IDShort *string `xml:"idShort"`
		} `xml:"submodels>submodel"`
	}
	require.NoError(t, xml.Unmarshal(payload, &environment))
	require.Len(t, environment.Submodels, 1)
	require.Equal(t, id, environment.Submodels[0].ID)
	require.Equal(t, expected, environment.Submodels[0].IDShort)
}

func (h *idShortHarness) testImport(t *testing.T, format string) {
	t.Helper()
	id := "urn:idshort:submodel:" + uuid.NewString()
	h.track(t, id)
	payload := idShortEnvironmentFixture(t, format, id)
	h.upload(t, format, payload, http.StatusOK)
	h.assertState(t, id, nil)
	assertIDShortEnvironment(t, format, h.serialize(t, id, format), id, nil)
	if format == "application/json" {
		h.json(t, http.MethodPut, h.path(id), idShortPayload(id, new("ImportedName")), http.StatusNoContent)
		h.upload(t, format, payload, http.StatusOK)
		h.assertState(t, id, nil)
	}
}

func (h *idShortHarness) upload(t *testing.T, format string, content []byte, status int) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	extension := "json"
	if strings.Contains(format, "aasx") {
		extension = "aasx"
	} else if format == "application/xml" {
		extension = "xml"
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="idshort.`+extension+`"`)
	header.Set("Content-Type", format)
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	response := h.request(t, http.MethodPost, "/upload", body.Bytes(), writer.FormDataContentType())
	require.Equal(t, status, response.Code, response.Body.String())
}

func idShortEnvironmentFixture(t *testing.T, format, id string) []byte {
	t.Helper()
	submodel := types.NewSubmodel(id)
	environment := types.NewEnvironment()
	environment.SetSubmodels([]types.ISubmodel{submodel})
	var content bytes.Buffer
	specType := "application/json"
	extension := "json"
	if strings.HasSuffix(format, "xml") {
		specType = "application/xml"
		extension = "xml"
		encoder := xml.NewEncoder(&content)
		require.NoError(t, xmlization.Marshal(encoder, environment, true))
		require.NoError(t, encoder.Flush())
	} else {
		jsonable, err := jsonization.ToJsonable(environment)
		require.NoError(t, err)
		require.NoError(t, json.NewEncoder(&content).Encode(jsonable))
	}
	if !strings.Contains(format, "aasx") {
		return content.Bytes()
	}
	var destination bytes.Buffer
	writer, err := aasx.NewPackaging().CreateWriter(&destination)
	require.NoError(t, err)
	uri, err := url.Parse("/aasx/environment." + extension)
	require.NoError(t, err)
	spec, err := writer.PutPartFromStream(uri, specType, &content)
	require.NoError(t, err)
	require.NoError(t, writer.MakeSpec(spec))
	require.NoError(t, writer.Close())
	return destination.Bytes()
}
