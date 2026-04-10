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
// Author: Martin Stemmer ( Fraunhofer IESE )

//nolint:all
package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/aas-core-works/aas-core3.1-golang/xmlization"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	aasEnvironmentBaseURL = "http://127.0.0.1:6004"

	sampleAASID      = "urn:example:aas:env:1"
	sampleSubmodelID = "urn:example:submodel:env:1"
	sampleCDID       = "urn:example:cd:env:1"

	aasxRelationTypeOrigin        = "http://admin-shell.io/aasx/relationships/aasx-origin"
	aasxRelationTypeSpec          = "http://admin-shell.io/aasx/relationships/aas-spec"
	aasxRelationTypeSupplementary = "http://admin-shell.io/aasx/relationships/aas-suppl"

	opcRelationshipsContentType = "application/vnd.openxmlformats-package.relationships+xml"
	opcRelationshipsNamespace   = "http://schemas.openxmlformats.org/package/2006/relationships"
	opcContentTypesNamespace    = "http://schemas.openxmlformats.org/package/2006/content-types"
)

var sampleEnvironmentJSON = []byte(`{
  "assetAdministrationShells": [
    {
      "id": "urn:example:aas:env:1",
      "idShort": "DemoAAS",
      "modelType": "AssetAdministrationShell",
      "assetInformation": {
        "assetKind": "Instance"
      },
      "submodels": [
        {
          "type": "ModelReference",
          "keys": [
            {
              "type": "Submodel",
              "value": "urn:example:submodel:env:1"
            }
          ]
        }
      ]
    }
  ],
  "submodels": [
    {
      "id": "urn:example:submodel:env:1",
      "idShort": "DemoSubmodel",
      "modelType": "Submodel",
      "submodelElements": [
        {
          "idShort": "temperature",
          "modelType": "Property",
          "valueType": "xs:string",
          "value": "21"
        }
      ]
    }
  ],
  "conceptDescriptions": [
    {
      "id": "urn:example:cd:env:1",
      "idShort": "DemoCD",
      "modelType": "ConceptDescription"
    }
  ]
}`)

//go:embed aas/environment.json aas/ProductionPlanSFKL.aasx
var fixtureFiles embed.FS

func TestUploadEndpointMediaTypes(t *testing.T) {
	jsonPayload, xmlPayload, aasxXMLPayload, aasxXMLWithSupplementaryPayload, aasxJSONPayload := buildSampleEnvironmentPayloads(t)

	t.Run("Upload_JSON", func(t *testing.T) {
		resetDatabase(t)
		statusCode, body := uploadEnvironmentPayload(t, "application/json", jsonPayload)
		assert.Equal(t, http.StatusNoContent, statusCode, "response: %s", body)
	})

	t.Run("Upload_XML", func(t *testing.T) {
		resetDatabase(t)
		statusCode, body := uploadEnvironmentPayload(t, "application/xml", xmlPayload)
		assert.Equal(t, http.StatusNoContent, statusCode, "response: %s", body)
	})

	t.Run("Upload_AASX_XML_Media_Type", func(t *testing.T) {
		resetDatabase(t)
		statusCode, body := uploadEnvironmentPayload(t, "application/aasx+xml", aasxXMLWithSupplementaryPayload)
		assert.Equal(t, http.StatusNoContent, statusCode, "response: %s", body)
	})

	t.Run("Upload_AASX_XML_Alias_Media_Type", func(t *testing.T) {
		resetDatabase(t)
		statusCode, body := uploadEnvironmentPayload(t, "application/asset-administration-shell+xml", aasxXMLPayload)
		assert.Equal(t, http.StatusNoContent, statusCode, "response: %s", body)
	})

	t.Run("Upload_AASX_JSON_Media_Type", func(t *testing.T) {
		resetDatabase(t)
		statusCode, body := uploadEnvironmentPayload(t, "application/aasx+json", aasxJSONPayload)
		assert.Equal(t, http.StatusNoContent, statusCode, "response: %s", body)
	})

	t.Run("Upload_AASX_JSON_Alias_Media_Type", func(t *testing.T) {
		resetDatabase(t)
		statusCode, body := uploadEnvironmentPayload(t, "application/asset-administration-shell+json", aasxJSONPayload)
		assert.Equal(t, http.StatusNoContent, statusCode, "response: %s", body)
	})

	t.Run("Upload_JSON_Real_Example_File", func(t *testing.T) {
		resetDatabase(t)
		payload := mustReadFixtureFile(t, "environment.json")
		statusCode, body := uploadEnvironmentPayload(t, "application/json", payload)
		assert.Equal(t, http.StatusNoContent, statusCode, "response: %s", body)
	})

	t.Run("Upload_AASX_JSON_Real_Example_File", func(t *testing.T) {
		resetDatabase(t)
		payload := mustReadFixtureFile(t, "ProductionPlanSFKL.aasx")
		statusCode, body := uploadEnvironmentPayload(t, "application/aasx+json", payload)
		assert.Equal(t, http.StatusNoContent, statusCode, "response: %s", body)
	})

	t.Run("Upload_Fails_When_File_Field_Missing", func(t *testing.T) {
		resetDatabase(t)
		statusCode, body := uploadWithoutFileField(t)
		assert.Equal(t, http.StatusBadRequest, statusCode, "response: %s", body)
	})

	t.Run("Upload_Fails_For_Unsupported_Media_Type", func(t *testing.T) {
		resetDatabase(t)
		statusCode, body := uploadEnvironmentPayload(t, "text/plain", []byte("unsupported"))
		assert.Equal(t, http.StatusBadRequest, statusCode, "response: %s", body)
	})

	t.Run("Upload_Fails_For_Invalid_JSON", func(t *testing.T) {
		resetDatabase(t)
		statusCode, body := uploadEnvironmentPayload(t, "application/json", []byte("{invalid json"))
		assert.Equal(t, http.StatusBadRequest, statusCode, "response: %s", body)
	})

	t.Run("Upload_Fails_For_Invalid_AASX", func(t *testing.T) {
		resetDatabase(t)
		statusCode, body := uploadEnvironmentPayload(t, "application/aasx+xml", []byte("not a zip"))
		assert.Equal(t, http.StatusBadRequest, statusCode, "response: %s", body)
	})
}

func TestSerializationEndpointMediaTypes(t *testing.T) {
	jsonPayload, _, _, _, _ := buildSampleEnvironmentPayloads(t)
	resetDatabase(t)
	statusCode, body := uploadEnvironmentPayload(t, "application/json", jsonPayload)
	require.Equal(t, http.StatusNoContent, statusCode, "upload failed before serialization tests: %s", body)

	t.Run("Serialization_JSON", func(t *testing.T) {
		respStatus, contentType, payload := getSerializationPayload(t, "application/json", "")
		assert.Equal(t, http.StatusOK, respStatus)
		assert.Contains(t, contentType, "application/json")
		assertEnvironmentJSONPayload(t, payload, true)
	})

	t.Run("Serialization_XML", func(t *testing.T) {
		respStatus, contentType, payload := getSerializationPayload(t, "application/xml", "")
		assert.Equal(t, http.StatusOK, respStatus)
		assert.Contains(t, contentType, "application/xml")
		assertEnvironmentXMLPayload(t, payload, true)
	})

	t.Run("Serialization_AASX_XML_Media_Type", func(t *testing.T) {
		respStatus, contentType, payload := getSerializationPayload(t, "application/aasx+xml", "")
		assert.Equal(t, http.StatusOK, respStatus)
		assert.Contains(t, contentType, "application/aasx+xml")
		assertAASXPayloadContainsEnvironment(t, payload, "xml", true)
	})

	t.Run("Serialization_AASX_XML_Alias_Media_Type", func(t *testing.T) {
		respStatus, contentType, payload := getSerializationPayload(t, "application/asset-administration-shell+xml", "")
		assert.Equal(t, http.StatusOK, respStatus)
		assert.Contains(t, contentType, "application/asset-administration-shell+xml")
		assertAASXPayloadContainsEnvironment(t, payload, "xml", true)
	})

	t.Run("Serialization_AASX_JSON_Media_Type", func(t *testing.T) {
		respStatus, contentType, payload := getSerializationPayload(t, "application/aasx+json", "")
		assert.Equal(t, http.StatusOK, respStatus)
		assert.Contains(t, contentType, "application/aasx+json")
		assertAASXPayloadContainsEnvironment(t, payload, "json", true)
	})

	t.Run("Serialization_AASX_JSON_Alias_Media_Type", func(t *testing.T) {
		respStatus, contentType, payload := getSerializationPayload(t, "application/asset-administration-shell+json", "")
		assert.Equal(t, http.StatusOK, respStatus)
		assert.Contains(t, contentType, "application/asset-administration-shell+json")
		assertAASXPayloadContainsEnvironment(t, payload, "json", true)
	})

	t.Run("Serialization_Without_ConceptDescriptions", func(t *testing.T) {
		query := "?includeConceptDescriptions=false"
		respStatus, contentType, payload := getSerializationPayload(t, "application/json", query)
		assert.Equal(t, http.StatusOK, respStatus)
		assert.Contains(t, contentType, "application/json")
		assertEnvironmentJSONPayload(t, payload, false)
	})

	t.Run("Serialization_Fails_For_Unsupported_Accept", func(t *testing.T) {
		respStatus, _, payload := getSerializationPayload(t, "text/plain", "")
		assert.Equal(t, http.StatusBadRequest, respStatus, "response: %s", string(payload))
	})

	t.Run("Serialization_Fails_For_Invalid_AAS_Identifier", func(t *testing.T) {
		respStatus, _, payload := getSerializationPayload(t, "application/json", "?aasIds=not-base64")
		assert.Equal(t, http.StatusBadRequest, respStatus, "response: %s", string(payload))
	})

	t.Run("Serialization_Filters_By_AAS_Identifier", func(t *testing.T) {
		encodedAASID := base64.RawURLEncoding.EncodeToString([]byte(sampleAASID))
		query := "?aasIds=" + url.QueryEscape(encodedAASID)
		respStatus, _, payload := getSerializationPayload(t, "application/json", query)
		assert.Equal(t, http.StatusOK, respStatus)
		assertEnvironmentJSONPayload(t, payload, true)
	})
}

func TestUploadAndSerializationRoundTripWithRealFixtureFiles(t *testing.T) {
	t.Run("RoundTrip_JSON_Real_Example_File", func(t *testing.T) {
		resetDatabase(t)

		uploadedPayload := mustReadFixtureFile(t, "environment.json")
		uploadStatusCode, uploadResponse := uploadEnvironmentPayload(t, "application/json", uploadedPayload)
		require.Equal(t, http.StatusNoContent, uploadStatusCode, "upload failed: %s", uploadResponse)

		serializationStatusCode, contentType, serializedPayload := getSerializationPayload(t, "application/json", "")
		require.Equal(t, http.StatusOK, serializationStatusCode, "serialization failed: %s", string(serializedPayload))
		assert.Contains(t, contentType, "application/json")

		expectedEnvironment := parseEnvironmentFromJSONPayload(t, uploadedPayload)
		actualEnvironment := parseEnvironmentFromJSONPayload(t, serializedPayload)
		assertEnvironmentEquivalent(t, expectedEnvironment, actualEnvironment)
	})

	t.Run("RoundTrip_AASX_Real_Example_File", func(t *testing.T) {
		resetDatabase(t)

		uploadedPayload := mustReadFixtureFile(t, "ProductionPlanSFKL.aasx")
		uploadStatusCode, uploadResponse := uploadEnvironmentPayload(t, "application/aasx+json", uploadedPayload)
		require.Equal(t, http.StatusNoContent, uploadStatusCode, "upload failed: %s", uploadResponse)

		serializationStatusCode, contentType, serializedPayload := getSerializationPayload(t, "application/aasx+json", "")
		require.Equal(t, http.StatusOK, serializationStatusCode, "serialization failed: %s", string(serializedPayload))
		assert.Contains(t, contentType, "application/aasx+json")

		expectedEnvironment := parseEnvironmentFromAASXPayload(t, uploadedPayload, "json")
		actualEnvironment := parseEnvironmentFromAASXPayload(t, serializedPayload, "json")
		assertEnvironmentEquivalent(t, expectedEnvironment, actualEnvironment)
	})
}

func buildSampleEnvironmentPayloads(t *testing.T) ([]byte, []byte, []byte, []byte, []byte) {
	t.Helper()

	environment := parseEnvironmentFromJSONPayload(t, sampleEnvironmentJSON)

	xmlPayload, err := serializeEnvironmentAsXML(environment)
	require.NoError(t, err)

	aasxXMLPayload, err := createAASXPayload(t, "application/xml", "environment.aas.xml", xmlPayload, false)
	require.NoError(t, err)

	aasxXMLWithSupplementaryPayload, err := createAASXPayload(t, "application/xml", "environment.aas.xml", xmlPayload, true)
	require.NoError(t, err)

	aasxJSONPayload, err := createAASXPayload(t, "application/json", "environment.aas.json", sampleEnvironmentJSON, false)
	require.NoError(t, err)

	return sampleEnvironmentJSON, xmlPayload, aasxXMLPayload, aasxXMLWithSupplementaryPayload, aasxJSONPayload
}

func parseEnvironmentFromJSONPayload(t *testing.T, payload []byte) types.IEnvironment {
	t.Helper()

	var raw map[string]any
	require.NoError(t, json.Unmarshal(payload, &raw))
	environment, err := jsonization.EnvironmentFromJsonable(raw)
	require.NoError(t, err)
	return environment
}

func parseEnvironmentFromXMLPayload(t *testing.T, payload []byte) types.IEnvironment {
	t.Helper()

	decoder := xml.NewDecoder(bytes.NewReader(sanitizeXMLPayload(payload)))
	instance, err := xmlization.Unmarshal(decoder)
	require.NoError(t, err)

	environment, ok := instance.(types.IEnvironment)
	require.True(t, ok)
	return environment
}

func parseEnvironmentFromAASXPayload(t *testing.T, payload []byte, preferredKind string) types.IEnvironment {
	t.Helper()

	specPayload, detectedKind, err := extractEnvironmentSpecFromAASX(payload, preferredKind)
	require.NoError(t, err)

	if detectedKind == "json" {
		return parseEnvironmentFromJSONPayload(t, specPayload)
	}

	return parseEnvironmentFromXMLPayload(t, specPayload)
}

func serializeEnvironmentAsXML(environment types.IEnvironment) ([]byte, error) {
	buffer := &bytes.Buffer{}
	if _, err := buffer.WriteString(xml.Header); err != nil {
		return nil, err
	}

	encoder := xml.NewEncoder(buffer)
	if err := xmlization.Marshal(encoder, environment, true); err != nil {
		return nil, err
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func createAASXPayload(
	t *testing.T,
	specContentType string,
	specFileName string,
	specPayload []byte,
	includeSupplementary bool,
) ([]byte, error) {
	t.Helper()

	specPartPath := path.Join("aasx", strings.TrimPrefix(specFileName, "/"))
	originPartPath := path.Join("aasx", "aasx-origin")
	supplementaryPartPath := path.Join("aasx", "files", "manual.txt")

	rootRelationshipsPayload, err := marshalRelationships([]relationshipXML{
		{
			ID:         "R00000001",
			Type:       aasxRelationTypeOrigin,
			Target:     "/" + originPartPath,
			TargetMode: "Internal",
		},
	})
	if err != nil {
		return nil, err
	}

	originRelationshipsPayload, err := marshalRelationships([]relationshipXML{
		{
			ID:         "R00000002",
			Type:       aasxRelationTypeSpec,
			Target:     "/" + specPartPath,
			TargetMode: "Internal",
		},
	})
	if err != nil {
		return nil, err
	}

	specRelationshipsPayload := []byte{}
	if includeSupplementary {
		specRelationshipsPayload, err = marshalRelationships([]relationshipXML{
			{
				ID:         "R00000003",
				Type:       aasxRelationTypeSupplementary,
				Target:     "/" + supplementaryPartPath,
				TargetMode: "Internal",
			},
		})
		if err != nil {
			return nil, err
		}
	}

	contentTypesPayload, err := marshalContentTypes(specPartPath, specContentType, includeSupplementary, supplementaryPartPath)
	if err != nil {
		return nil, err
	}

	var zipped bytes.Buffer
	zipWriter := zip.NewWriter(&zipped)

	if err = writeZipEntry(zipWriter, "[Content_Types].xml", contentTypesPayload); err != nil {
		return nil, err
	}
	if err = writeZipEntry(zipWriter, "_rels/.rels", rootRelationshipsPayload); err != nil {
		return nil, err
	}
	if err = writeZipEntry(zipWriter, relsPathForSource(originPartPath), originRelationshipsPayload); err != nil {
		return nil, err
	}
	if includeSupplementary {
		if err = writeZipEntry(zipWriter, relsPathForSource(specPartPath), specRelationshipsPayload); err != nil {
			return nil, err
		}
	}

	if err = writeZipEntry(zipWriter, originPartPath, []byte("Intentionally empty.")); err != nil {
		return nil, err
	}
	if err = writeZipEntry(zipWriter, specPartPath, specPayload); err != nil {
		return nil, err
	}
	if includeSupplementary {
		if err = writeZipEntry(zipWriter, supplementaryPartPath, []byte("manual")); err != nil {
			return nil, err
		}
	}

	if err = zipWriter.Close(); err != nil {
		return nil, err
	}

	return zipped.Bytes(), nil
}

func uploadEnvironmentPayload(t *testing.T, mediaType string, payload []byte) (int, string) {
	t.Helper()

	requestBody := &bytes.Buffer{}
	writer := multipart.NewWriter(requestBody)

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="environment.bin"`)
	header.Set("Content-Type", mediaType)

	part, err := writer.CreatePart(header)
	require.NoError(t, err)

	_, err = part.Write(payload)
	require.NoError(t, err)

	require.NoError(t, writer.Close())

	request, err := http.NewRequest(http.MethodPost, aasEnvironmentBaseURL+"/upload", requestBody)
	require.NoError(t, err)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	require.NoError(t, err)
	defer func() {
		_ = response.Body.Close()
	}()

	responsePayload, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, string(responsePayload)
}

func uploadWithoutFileField(t *testing.T) (int, string) {
	t.Helper()

	requestBody := &bytes.Buffer{}
	writer := multipart.NewWriter(requestBody)
	require.NoError(t, writer.Close())

	request, err := http.NewRequest(http.MethodPost, aasEnvironmentBaseURL+"/upload", requestBody)
	require.NoError(t, err)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	require.NoError(t, err)
	defer func() {
		_ = response.Body.Close()
	}()

	responsePayload, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, string(responsePayload)
}

func getSerializationPayload(t *testing.T, accept string, query string) (int, string, []byte) {
	t.Helper()

	request, err := http.NewRequest(http.MethodGet, aasEnvironmentBaseURL+"/serialization"+query, nil)
	require.NoError(t, err)
	request.Header.Set("Accept", accept)

	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	require.NoError(t, err)
	defer func() {
		_ = response.Body.Close()
	}()

	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, response.Header.Get("Content-Type"), payload
}

func assertEnvironmentJSONPayload(t *testing.T, payload []byte, expectConceptDescriptions bool) {
	t.Helper()

	var environment map[string]any
	require.NoError(t, json.Unmarshal(payload, &environment))

	assert.Equal(t, 1, len(anySlice(environment["assetAdministrationShells"])))
	assert.Equal(t, 1, len(anySlice(environment["submodels"])))

	conceptDescriptions := anySlice(environment["conceptDescriptions"])
	if expectConceptDescriptions {
		assert.Equal(t, 1, len(conceptDescriptions))
	} else {
		assert.Equal(t, 0, len(conceptDescriptions))
	}
}

func assertEnvironmentXMLPayload(t *testing.T, payload []byte, expectConceptDescriptions bool) {
	t.Helper()

	environment := parseEnvironmentFromXMLPayload(t, payload)

	assert.Equal(t, 1, len(environment.AssetAdministrationShells()))
	assert.Equal(t, 1, len(environment.Submodels()))
	if expectConceptDescriptions {
		assert.Equal(t, 1, len(environment.ConceptDescriptions()))
	} else {
		assert.Equal(t, 0, len(environment.ConceptDescriptions()))
	}
}

func assertAASXPayloadContainsEnvironment(t *testing.T, payload []byte, preferredKind string, expectConceptDescriptions bool) {
	t.Helper()

	specPayload, detectedKind, err := extractEnvironmentSpecFromAASX(payload, preferredKind)
	require.NoError(t, err)

	if detectedKind == "json" {
		assertEnvironmentJSONPayload(t, specPayload, expectConceptDescriptions)
		return
	}

	assertEnvironmentXMLPayload(t, specPayload, expectConceptDescriptions)
}

func assertEnvironmentEquivalent(t *testing.T, expected types.IEnvironment, actual types.IEnvironment) {
	t.Helper()

	assertAssetAdministrationShellsEquivalent(t, expected.AssetAdministrationShells(), actual.AssetAdministrationShells())
	assertSubmodelsEquivalent(t, expected.Submodels(), actual.Submodels())
	assertConceptDescriptionsEquivalent(t, expected.ConceptDescriptions(), actual.ConceptDescriptions())
}

func assertAssetAdministrationShellsEquivalent(t *testing.T, expected []types.IAssetAdministrationShell, actual []types.IAssetAdministrationShell) {
	t.Helper()

	expectedByID := map[string]types.IAssetAdministrationShell{}
	for _, shell := range expected {
		if shell != nil {
			expectedByID[shell.ID()] = shell
		}
	}

	actualByID := map[string]types.IAssetAdministrationShell{}
	for _, shell := range actual {
		if shell != nil {
			actualByID[shell.ID()] = shell
		}
	}

	require.Equal(t, len(expectedByID), len(actualByID), "assetAdministrationShells count mismatch")
	for id, expectedShell := range expectedByID {
		actualShell, ok := actualByID[id]
		require.Truef(t, ok, "assetAdministrationShell with id %q is missing in serialized payload", id)
		assertClassJSONEquivalent(t, expectedShell, actualShell)
	}
}

func assertSubmodelsEquivalent(t *testing.T, expected []types.ISubmodel, actual []types.ISubmodel) {
	t.Helper()

	expectedByID := map[string]types.ISubmodel{}
	for _, submodel := range expected {
		if submodel != nil {
			expectedByID[submodel.ID()] = submodel
		}
	}

	actualByID := map[string]types.ISubmodel{}
	for _, submodel := range actual {
		if submodel != nil {
			actualByID[submodel.ID()] = submodel
		}
	}

	require.Equal(t, len(expectedByID), len(actualByID), "submodels count mismatch")
	for id, expectedSubmodel := range expectedByID {
		actualSubmodel, ok := actualByID[id]
		require.Truef(t, ok, "submodel with id %q is missing in serialized payload", id)
		assertClassJSONEquivalent(t, expectedSubmodel, actualSubmodel)
	}
}

func assertConceptDescriptionsEquivalent(t *testing.T, expected []types.IConceptDescription, actual []types.IConceptDescription) {
	t.Helper()

	expectedByID := map[string]types.IConceptDescription{}
	for _, conceptDescription := range expected {
		if conceptDescription != nil {
			expectedByID[conceptDescription.ID()] = conceptDescription
		}
	}

	actualByID := map[string]types.IConceptDescription{}
	for _, conceptDescription := range actual {
		if conceptDescription != nil {
			actualByID[conceptDescription.ID()] = conceptDescription
		}
	}

	require.Equal(t, len(expectedByID), len(actualByID), "conceptDescriptions count mismatch")
	for id, expectedConceptDescription := range expectedByID {
		actualConceptDescription, ok := actualByID[id]
		require.Truef(t, ok, "conceptDescription with id %q is missing in serialized payload", id)
		assertClassJSONEquivalent(t, expectedConceptDescription, actualConceptDescription)
	}
}

func assertClassJSONEquivalent(t *testing.T, expected types.IClass, actual types.IClass) {
	t.Helper()

	expectedJSON := classToJSON(t, expected)
	actualJSON := classToJSON(t, actual)
	assert.JSONEq(t, string(expectedJSON), string(actualJSON))
}

func classToJSON(t *testing.T, value types.IClass) []byte {
	t.Helper()

	jsonable, err := jsonization.ToJsonable(value)
	require.NoError(t, err)

	payload, err := json.Marshal(normalizeSemanticJSON(jsonable))
	require.NoError(t, err)
	return payload
}

func normalizeSemanticJSON(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, entry := range typed {
			normalizedEntry := normalizeSemanticJSON(entry)
			if entryString, ok := normalizedEntry.(string); ok && entryString == "" {
				continue
			}
			normalized[key] = normalizedEntry
		}
		return normalized
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, entry := range typed {
			normalized = append(normalized, normalizeSemanticJSON(entry))
		}
		return normalized
	default:
		return value
	}
}

func extractEnvironmentSpecFromAASX(payload []byte, preferredKind string) ([]byte, string, error) {
	archiveReader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, "", err
	}

	preferredKind = strings.ToLower(strings.TrimSpace(preferredKind))
	candidatesByKind := map[string][][]byte{
		"json": {},
		"xml":  {},
	}

	for _, file := range archiveReader.File {
		if file == nil || file.FileInfo().IsDir() {
			continue
		}

		name := strings.ToLower(file.Name)
		if name == "[content_types].xml" || strings.Contains(name, "_rels/") || strings.HasSuffix(name, "/aasx-origin") {
			continue
		}

		content, readErr := readZipFileBytes(file)
		if readErr != nil {
			return nil, "", readErr
		}

		if isEnvironmentJSONPayload(content) {
			candidatesByKind["json"] = append(candidatesByKind["json"], content)
		}
		if isEnvironmentXMLPayload(content) {
			candidatesByKind["xml"] = append(candidatesByKind["xml"], content)
		}
	}

	if preferredKind == "json" && len(candidatesByKind["json"]) > 0 {
		return candidatesByKind["json"][0], "json", nil
	}
	if preferredKind == "xml" && len(candidatesByKind["xml"]) > 0 {
		return candidatesByKind["xml"][0], "xml", nil
	}
	if len(candidatesByKind["json"]) > 0 {
		return candidatesByKind["json"][0], "json", nil
	}
	if len(candidatesByKind["xml"]) > 0 {
		return candidatesByKind["xml"][0], "xml", nil
	}

	return nil, "", io.EOF
}

func isEnvironmentJSONPayload(payload []byte) bool {
	var content map[string]any
	if err := json.Unmarshal(payload, &content); err != nil {
		return false
	}

	_, hasAAS := content["assetAdministrationShells"]
	_, hasSubmodels := content["submodels"]
	return hasAAS || hasSubmodels
}

func isEnvironmentXMLPayload(payload []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(sanitizeXMLPayload(payload)))
	instance, err := xmlization.Unmarshal(decoder)
	if err != nil {
		return false
	}

	_, ok := instance.(types.IEnvironment)
	return ok
}

func sanitizeXMLPayload(payload []byte) []byte {
	sanitized := bytes.TrimSpace(payload)
	if bytes.HasPrefix(sanitized, []byte("\uFEFF")) {
		sanitized = bytes.TrimPrefix(sanitized, []byte("\uFEFF"))
	}
	if bytes.HasPrefix(sanitized, []byte("<?xml")) {
		if declarationEnd := bytes.Index(sanitized, []byte("?>")); declarationEnd >= 0 {
			sanitized = sanitized[declarationEnd+2:]
		}
	}
	return bytes.TrimSpace(sanitized)
}

func writeZipEntry(zipWriter *zip.Writer, fileName string, payload []byte) error {
	writer, err := zipWriter.Create(fileName)
	if err != nil {
		return err
	}

	_, err = writer.Write(payload)
	return err
}

func readZipFileBytes(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()

	return io.ReadAll(reader)
}

func relsPathForSource(source string) string {
	source = strings.TrimPrefix(source, "/")
	if source == "" {
		return "_rels/.rels"
	}

	sourceDirectory := path.Dir(source)
	sourceFile := path.Base(source)
	if sourceDirectory == "." {
		return path.Join("_rels", sourceFile+".rels")
	}

	return path.Join(sourceDirectory, "_rels", sourceFile+".rels")
}

type relationshipsXML struct {
	XMLName       xml.Name          `xml:"Relationships"`
	Xmlns         string            `xml:"xmlns,attr"`
	Relationships []relationshipXML `xml:"Relationship"`
}

type relationshipXML struct {
	ID         string `xml:"Id,attr"`
	Type       string `xml:"Type,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr,omitempty"`
}

func marshalRelationships(relationships []relationshipXML) ([]byte, error) {
	document := relationshipsXML{
		Xmlns:         opcRelationshipsNamespace,
		Relationships: relationships,
	}

	payload, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), payload...), nil
}

type contentTypesXML struct {
	XMLName   xml.Name       `xml:"Types"`
	Xmlns     string         `xml:"xmlns,attr"`
	Defaults  []defaultType  `xml:"Default"`
	Overrides []overrideType `xml:"Override"`
}

type defaultType struct {
	Extension   string `xml:"Extension,attr"`
	ContentType string `xml:"ContentType,attr"`
}

type overrideType struct {
	PartName    string `xml:"PartName,attr"`
	ContentType string `xml:"ContentType,attr"`
}

func marshalContentTypes(specPartPath string, specContentType string, includeSupplementary bool, supplementaryPartPath string) ([]byte, error) {
	contentTypes := contentTypesXML{
		Xmlns: opcContentTypesNamespace,
		Defaults: []defaultType{
			{
				Extension:   "rels",
				ContentType: opcRelationshipsContentType,
			},
		},
		Overrides: []overrideType{
			{
				PartName:    "/aasx/aasx-origin",
				ContentType: "text/plain",
			},
			{
				PartName:    "/" + strings.TrimPrefix(specPartPath, "/"),
				ContentType: specContentType,
			},
		},
	}

	if includeSupplementary {
		contentTypes.Overrides = append(contentTypes.Overrides, overrideType{
			PartName:    "/" + strings.TrimPrefix(supplementaryPartPath, "/"),
			ContentType: "text/plain",
		})
	}

	payload, err := xml.MarshalIndent(contentTypes, "", "  ")
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), payload...), nil
}

func anySlice(value any) []any {
	slice, ok := value.([]any)
	if !ok {
		return []any{}
	}
	return slice
}

func mustReadFixtureFile(t *testing.T, fileName string) []byte {
	t.Helper()

	fixturePathByName := map[string]string{
		"environment.json":        "aas/environment.json",
		"ProductionPlanSFKL.aasx": "aas/ProductionPlanSFKL.aasx",
	}

	fixturePath, ok := fixturePathByName[fileName]
	require.Truef(t, ok, "unsupported fixture file: %s", fileName)

	payload, err := fixtureFiles.ReadFile(fixturePath)
	require.NoError(t, err, "failed reading fixture file: %s", fixturePath)
	require.NotEmpty(t, payload, "fixture file is empty: %s", fixturePath)

	return payload
}
