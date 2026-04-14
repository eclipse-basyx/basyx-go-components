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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq" // PostgreSQL Treiber

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	xsdTimezonePattern = `(?:Z|[+-](?:0\d|1[0-4]):[0-5]\d)?`
	xsdDurationPattern = regexp.MustCompile(
		`^-?P(?:(?:\d+Y(?:\d+M)?(?:\d+D)?|\d+M(?:\d+D)?|\d+D)(?:T(?:\d+H(?:\d+M)?(?:\d+(?:\.\d+)?S)?|\d+M(?:\d+(?:\.\d+)?S)?|\d+(?:\.\d+)?S))?|T(?:\d+H(?:\d+M)?(?:\d+(?:\.\d+)?S)?|\d+M(?:\d+(?:\.\d+)?S)?|\d+(?:\.\d+)?S))$`,
	)
	xsdGYearPattern      = regexp.MustCompile(`^-?\d{4,}` + xsdTimezonePattern + `$`)
	xsdGMonthPattern     = regexp.MustCompile(`^--(0[1-9]|1[0-2])(?:--)?` + xsdTimezonePattern + `$`)
	xsdGDayPattern       = regexp.MustCompile(`^---(0[1-9]|[12]\d|3[01])` + xsdTimezonePattern + `$`)
	xsdGYearMonthPattern = regexp.MustCompile(`^-?\d{4,}-(0[1-9]|1[0-2])` + xsdTimezonePattern + `$`)
	xsdGMonthDayPattern  = regexp.MustCompile(`^--(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])` + xsdTimezonePattern + `$`)
)

// uploadFileAttachment uploads a file to the attachment endpoint
func uploadFileAttachment(endpoint string, filePath string, fileName string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %v", err)
	}
	defer func() { _ = file.Close() }()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add the file field
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return 0, fmt.Errorf("failed to create form file: %v", err)
	}
	_, err = io.Copy(part, file)
	if err != nil {
		return 0, fmt.Errorf("failed to copy file: %v", err)
	}

	// Add the fileName field if provided
	if fileName != "" {
		if err := writer.WriteField("fileName", fileName); err != nil {
			return 0, fmt.Errorf("failed to write fileName field: %v", err)
		}
	}

	err = writer.Close()
	if err != nil {
		return 0, fmt.Errorf("failed to close writer: %v", err)
	}

	req, err := http.NewRequest("PUT", endpoint, body)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode, nil
}

func createTemporaryBinaryTestFile(t *testing.T, fileName string, payload []byte) string {
	t.Helper()

	filePath := filepath.Join(t.TempDir(), fileName)
	err := os.WriteFile(filePath, payload, 0o600)
	require.NoError(t, err, "Failed to create temporary test file")

	return filePath
}

// downloadFileAttachment downloads a file from the attachment endpoint and returns content and content-type
func downloadFileAttachment(endpoint string) ([]byte, string, int, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to create request: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", resp.StatusCode, fmt.Errorf("failed to read response: %v", err)
	}

	contentType := resp.Header.Get("Content-Type")
	return content, contentType, resp.StatusCode, nil
}

func getStatusWithoutRedirect(endpoint string) (int, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}

func requestJSON(method string, endpoint string, payload any) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to marshal payload: %v", err)
		}
		body = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %v", err)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return resp.StatusCode, respBody, nil
}

func getPropertyValueByIDShort(t *testing.T, submodel map[string]any, idShort string) string {
	t.Helper()

	rawElements, ok := submodel["submodelElements"].([]any)
	require.True(t, ok, "submodelElements must be an array")

	for _, rawElement := range rawElements {
		elementMap, ok := rawElement.(map[string]any)
		require.True(t, ok, "submodel element must be an object")

		if elementMap["idShort"] == idShort {
			value, ok := elementMap["value"].(string)
			require.True(t, ok, "property value must be a string for idShort=%s", idShort)
			return value
		}
	}

	t.Fatalf("property with idShort=%s not found", idShort)
	return ""
}

func getRangeValuesByIDShort(t *testing.T, submodel map[string]any, idShort string) (string, string) {
	t.Helper()

	rawElements, ok := submodel["submodelElements"].([]any)
	require.True(t, ok, "submodelElements must be an array")

	for _, rawElement := range rawElements {
		elementMap, ok := rawElement.(map[string]any)
		require.True(t, ok, "submodel element must be an object")

		if elementMap["idShort"] == idShort {
			minValue, ok := elementMap["min"].(string)
			require.True(t, ok, "range min must be a string for idShort=%s", idShort)
			maxValue, ok := elementMap["max"].(string)
			require.True(t, ok, "range max must be a string for idShort=%s", idShort)
			return minValue, maxValue
		}
	}

	t.Fatalf("range with idShort=%s not found", idShort)
	return "", ""
}

func assertXSDDateTimeLexical(t *testing.T, value string) {
	t.Helper()

	assert.NotContains(t, value, " ", "xs:dateTime must not contain a space separator")
	assert.Contains(t, value, "T", "xs:dateTime must contain T separator")

	layouts := []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-07:00", "2006-01-02T15:04:05-07:00"}
	for _, layout := range layouts {
		if _, err := time.Parse(layout, value); err == nil {
			return
		}
	}

	t.Fatalf("xs:dateTime value is not parseable with expected lexical forms: %s", value)
}

func assertXSDDateLexical(t *testing.T, value string) {
	t.Helper()
	_, err := time.Parse("2006-01-02", value)
	require.NoError(t, err, "xs:date must match YYYY-MM-DD lexical form")
}

func assertXSDTimeLexical(t *testing.T, value string) {
	t.Helper()
	layouts := []string{"15:04:05", "15:04:05.999999999"}
	for _, layout := range layouts {
		if _, err := time.Parse(layout, value); err == nil {
			return
		}
	}
	t.Fatalf("xs:time value is not parseable with expected lexical forms: %s", value)
}

func TestTemporalXSDRoundTripFormatting(t *testing.T) {
	baseURL := "http://localhost:6004"
	submodelID := "urn:basyx:integration:temporal-format"
	submodelIDEncoded := common.EncodeString(submodelID)
	t.Run("Duration regex guards invalid lexicals", func(t *testing.T) {
		assert.NotRegexp(t, xsdDurationPattern, "P")
		assert.NotRegexp(t, xsdDurationPattern, "PT")
		assert.Regexp(t, xsdDurationPattern, "P1D")
		assert.Regexp(t, xsdDurationPattern, "PT1S")
	})
	t.Cleanup(func() {
		statusCode, body, err := requestJSON(http.MethodDelete, fmt.Sprintf("%s/submodels/%s", baseURL, submodelIDEncoded), nil)
		if err != nil {
			t.Logf("cleanup delete failed for temporal test submodel: %v", err)
			return
		}
		if statusCode != http.StatusNoContent && statusCode != http.StatusNotFound {
			t.Logf("cleanup delete returned unexpected status=%d body=%s", statusCode, string(body))
		}
	})

	payload := map[string]any{
		"id":        submodelID,
		"idShort":   "TemporalFormatSubmodel",
		"kind":      "Instance",
		"modelType": "Submodel",
		"submodelElements": []map[string]any{
			{
				"idShort":   "DateTimeProperty",
				"valueType": "xs:dateTime",
				"value":     "2026-03-21T13:13:35.485Z",
				"modelType": "Property",
			},
			{
				"idShort":   "DateProperty",
				"valueType": "xs:date",
				"value":     "2026-03-21",
				"modelType": "Property",
			},
			{
				"idShort":   "TimeProperty",
				"valueType": "xs:time",
				"value":     "13:13:35.485",
				"modelType": "Property",
			},
			{
				"idShort":   "DurationProperty",
				"valueType": "xs:duration",
				"value":     "P1DT2H3M4.5S",
				"modelType": "Property",
			},
			{
				"idShort":   "GYearProperty",
				"valueType": "xs:gYear",
				"value":     "2026",
				"modelType": "Property",
			},
			{
				"idShort":   "GMonthProperty",
				"valueType": "xs:gMonth",
				"value":     "--03",
				"modelType": "Property",
			},
			{
				"idShort":   "GDayProperty",
				"valueType": "xs:gDay",
				"value":     "---21",
				"modelType": "Property",
			},
			{
				"idShort":   "GYearMonthProperty",
				"valueType": "xs:gYearMonth",
				"value":     "2026-03",
				"modelType": "Property",
			},
			{
				"idShort":   "GMonthDayProperty",
				"valueType": "xs:gMonthDay",
				"value":     "--03-21",
				"modelType": "Property",
			},
			{
				"idShort":   "DateTimeRange",
				"modelType": "Range",
				"valueType": "xs:dateTime",
				"min":       "2026-03-21T12:00:00.123Z",
				"max":       "2026-03-21T14:00:00.987Z",
			},
		},
	}

	t.Run("Post temporal submodel", func(t *testing.T) {
		statusCode, body, err := requestJSON(http.MethodPost, fmt.Sprintf("%s/submodels", baseURL), payload)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))
	})

	t.Run("Get full submodel validates temporal lexical forms", func(t *testing.T) {
		statusCode, body, err := requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s", baseURL, submodelIDEncoded), nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))

		var submodel map[string]any
		require.NoError(t, json.Unmarshal(body, &submodel))

		assertXSDDateTimeLexical(t, getPropertyValueByIDShort(t, submodel, "DateTimeProperty"))
		assertXSDDateLexical(t, getPropertyValueByIDShort(t, submodel, "DateProperty"))
		assertXSDTimeLexical(t, getPropertyValueByIDShort(t, submodel, "TimeProperty"))
		assert.Regexp(t, xsdDurationPattern, getPropertyValueByIDShort(t, submodel, "DurationProperty"))
		assert.Regexp(t, xsdGYearPattern, getPropertyValueByIDShort(t, submodel, "GYearProperty"))
		assert.Regexp(t, xsdGMonthPattern, getPropertyValueByIDShort(t, submodel, "GMonthProperty"))
		assert.Regexp(t, xsdGDayPattern, getPropertyValueByIDShort(t, submodel, "GDayProperty"))
		assert.Regexp(t, xsdGYearMonthPattern, getPropertyValueByIDShort(t, submodel, "GYearMonthProperty"))
		assert.Regexp(t, xsdGMonthDayPattern, getPropertyValueByIDShort(t, submodel, "GMonthDayProperty"))

		rangeMin, rangeMax := getRangeValuesByIDShort(t, submodel, "DateTimeRange")
		assertXSDDateTimeLexical(t, rangeMin)
		assertXSDDateTimeLexical(t, rangeMax)
	})

	t.Run("Get single property endpoint validates xs:dateTime lexical form", func(t *testing.T) {
		statusCode, body, err := requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/submodel-elements/DateTimeProperty", baseURL, submodelIDEncoded), nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))

		var property map[string]any
		require.NoError(t, json.Unmarshal(body, &property))
		value, ok := property["value"].(string)
		require.True(t, ok, "Property value must be string")
		assertXSDDateTimeLexical(t, value)
	})

	t.Run("Get single range endpoint validates xs:dateTime lexical form", func(t *testing.T) {
		statusCode, body, err := requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/submodel-elements/DateTimeRange", baseURL, submodelIDEncoded), nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))

		var rangeElement map[string]any
		require.NoError(t, json.Unmarshal(body, &rangeElement))

		minValue, ok := rangeElement["min"].(string)
		require.True(t, ok, "Range min value must be string")
		maxValue, ok := rangeElement["max"].(string)
		require.True(t, ok, "Range max value must be string")
		assertXSDDateTimeLexical(t, minValue)
		assertXSDDateTimeLexical(t, maxValue)
	})
}

func TestContractSubmodelRepository(t *testing.T) {
	baseURL := "http://localhost:6004"

	createSubmodel := func(t *testing.T, submodelID string, submodelIDShort string) string {
		t.Helper()

		statusCode, body, err := requestJSON(http.MethodPost, fmt.Sprintf("%s/submodels", baseURL), map[string]any{
			"id":        submodelID,
			"idShort":   submodelIDShort,
			"kind":      "Instance",
			"modelType": "Submodel",
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))

		encodedSubmodelID := common.EncodeString(submodelID)
		t.Cleanup(func() {
			deleteStatusCode, deleteBody, deleteErr := requestJSON(http.MethodDelete, fmt.Sprintf("%s/submodels/%s", baseURL, encodedSubmodelID), nil)
			if deleteErr != nil {
				t.Logf("cleanup delete failed for submodel %s: %v", submodelID, deleteErr)
				return
			}

			if deleteStatusCode != http.StatusNoContent && deleteStatusCode != http.StatusNotFound {
				t.Logf("cleanup delete returned unexpected status=%d for submodel %s, response=%s", deleteStatusCode, submodelID, string(deleteBody))
			}
		})

		return encodedSubmodelID
	}

	t.Run("GetSubmodelByIDMetadataReturnsMetadataPayload", func(t *testing.T) {
		submodelID := fmt.Sprintf("https://example.com/ids/sm/contract-metadata-%d", time.Now().UnixNano())
		submodelIDShort := fmt.Sprintf("contractMetadata%d", time.Now().UnixNano())
		encodedSubmodelID := createSubmodel(t, submodelID, submodelIDShort)

		statusCode, body, err := requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s/$metadata", baseURL, encodedSubmodelID), nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))

		var metadata map[string]any
		require.NoError(t, json.Unmarshal(body, &metadata), "response=%s", string(body))
		assert.NotEmpty(t, metadata, "metadata response should not be empty")
	})

	t.Run("PostSubmodelElementReturnsCreatedPayload", func(t *testing.T) {
		submodelID := fmt.Sprintf("https://example.com/ids/sm/contract-post-element-%d", time.Now().UnixNano())
		submodelIDShort := fmt.Sprintf("contractPostElement%d", time.Now().UnixNano())
		encodedSubmodelID := createSubmodel(t, submodelID, submodelIDShort)

		statusCode, body, err := requestJSON(http.MethodPost, fmt.Sprintf("%s/submodels/%s/submodel-elements", baseURL, encodedSubmodelID), map[string]any{
			"idShort":   "testProperty",
			"valueType": "xs:integer",
			"value":     "1984",
			"modelType": "Property",
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, statusCode, "response=%s", string(body))

		var createdElement map[string]any
		require.NoError(t, json.Unmarshal(body, &createdElement), "response=%s", string(body))
		assert.NotEmpty(t, createdElement, "created submodel element payload should not be empty")
		assert.Equal(t, "testProperty", createdElement["idShort"], "created payload should contain idShort")
		assert.Equal(t, "Property", createdElement["modelType"], "created payload should contain modelType")
	})

	t.Run("GetSubmodelByIDDoesNotExposeEmptySubmodelElementsArray", func(t *testing.T) {
		submodelID := fmt.Sprintf("https://example.com/ids/sm/contract-empty-elements-%d", time.Now().UnixNano())
		submodelIDShort := fmt.Sprintf("contractEmptyElements%d", time.Now().UnixNano())
		encodedSubmodelID := createSubmodel(t, submodelID, submodelIDShort)

		statusCode, body, err := requestJSON(http.MethodGet, fmt.Sprintf("%s/submodels/%s", baseURL, encodedSubmodelID), nil)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, statusCode, "response=%s", string(body))

		var submodel map[string]any
		require.NoError(t, json.Unmarshal(body, &submodel), "response=%s", string(body))

		rawSubmodelElements, hasSubmodelElements := submodel["submodelElements"]
		if !hasSubmodelElements {
			return
		}

		submodelElements, ok := rawSubmodelElements.([]any)
		require.True(t, ok, "submodelElements should be a JSON array when present")
		assert.NotEmpty(t, submodelElements, "submodelElements must be omitted when empty or contain at least one element")
	})
}

// IntegrationTest runs the integration tests based on the config file
func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet, http.MethodPost),
		ShouldSkipStep: func(step testenv.JSONSuiteStep) bool {
			if strings.EqualFold(step.Method, http.MethodPut) && strings.Contains(step.Endpoint, "/attachment") {
				return true
			}
			if strings.EqualFold(step.Method, http.MethodGet) && strings.Contains(step.Endpoint, "/attachment") {
				return true
			}
			return false
		},
		StepName: func(step testenv.JSONSuiteStep, stepNumber int) string {
			context := "Not Provided"
			if step.Context != "" {
				context = step.Context
			}
			return fmt.Sprintf("Step_(%s)_%d_%s_%s", context, stepNumber, step.Method, step.Endpoint)
		},
	})
}

// TestFileAttachmentOperations tests file upload, download, and deletion for File SME
func TestFileAttachmentOperations(t *testing.T) {
	baseURL := "http://localhost:6004"
	submodelID := "aHR0cDovL2llc2UuZnJhdW5ob2Zlci5kZS9pZC9zbS9Pbmx5RmlsZVN1Ym1vZGVsX1Rlc3Q" // base64 encoded: http://iese.fraunhofer.de/id/sm/OnlyFileSubmodel_Test
	testFilePath := "testFiles/marcus.gif"
	weakFileContent := []byte{0x00, 0x01, 0x02, 0x03}

	// Read the test file content for later comparison
	originalFileContent, err := os.ReadFile(testFilePath)
	require.NoError(t, err, "Failed to read test file")

	t.Run("1_Upload_File_Attachment", func(t *testing.T) {
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		statusCode, err := uploadFileAttachment(endpoint, testFilePath, "marcus.gif")
		require.NoError(t, err, "File upload failed")
		assert.Equal(t, http.StatusNoContent, statusCode, "Expected 204 No Content for file upload")
	})

	t.Run("2_Download_File_Attachment_And_Verify", func(t *testing.T) {
		// Wait a moment to ensure the file is available
		time.Sleep(2 * time.Second)
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		content, contentType, statusCode, err := downloadFileAttachment(endpoint)
		require.NoError(t, err, "File download failed")
		assert.Equal(t, http.StatusOK, statusCode, "Expected 200 OK for file download")

		assert.Equal(t, "image/gif", contentType, "Content-Type should match detected MIME type for uploaded GIF")
		t.Logf("Downloaded file Content-Type: %s", contentType)

		// Verify file content matches uploaded file byte-by-byte
		assert.Equal(t, originalFileContent, content, "Downloaded file content should match uploaded file")
		t.Logf("File content verified: %d bytes", len(content))
	})

	t.Run("3_Update_File_Element_Value_Should_Delete_LargeObject", func(t *testing.T) {
		// Update the File SME value to an external URL (should trigger LO cleanup)
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile", baseURL, submodelID)
		updateData, err := os.ReadFile("bodies/updateFileElement.json")
		require.NoError(t, err, "Failed to read update data")

		req, err := http.NewRequest("PUT", endpoint, bytes.NewBuffer(updateData))
		require.NoError(t, err, "Failed to create PUT request")
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err, "PUT request failed")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode, "Expected 204 No Content for File SME update")
	})

	t.Run("4_Verify_File_Attachment_Removed_After_Value_Update", func(t *testing.T) {
		// Try to download - should fail since value is now an external URL
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		statusCode, err := getStatusWithoutRedirect(endpoint)
		require.NoError(t, err, "File attachment check failed")

		// Should return 404 or redirect to external URL (302)
		// Since value is now http://example.com/updated-file.png, it should redirect
		assert.Contains(t, []int{http.StatusFound, http.StatusNotFound}, statusCode,
			"Should redirect to external URL or return 404 after value update")
	})

	t.Run("5_Upload_Weak_File_Attachment_Uses_Declared_ContentType", func(t *testing.T) {
		weakFilePath := createTemporaryBinaryTestFile(t, "weak-attachment", weakFileContent)
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		statusCode, err := uploadFileAttachment(endpoint, weakFilePath, "")
		require.NoError(t, err, "Weak file upload failed")
		assert.Equal(t, http.StatusNoContent, statusCode, "Expected 204 No Content for weak file upload")
	})

	t.Run("6_Verify_Weak_File_Attachment_Uses_Declared_ContentType", func(t *testing.T) {
		time.Sleep(2 * time.Second)
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		content, contentType, statusCode, err := downloadFileAttachment(endpoint)
		require.NoError(t, err, "Weak file download failed")
		assert.Equal(t, http.StatusOK, statusCode, "Expected 200 OK for weak file download")
		assert.Equal(t, weakFileContent, content, "Weak file content should match uploaded payload")
		assert.Equal(t, "image/png", contentType, "Weak MIME detection should fall back to declared File contentType")
	})

	t.Run("7_Reupload_File_Attachment", func(t *testing.T) {
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		statusCode, err := uploadFileAttachment(endpoint, testFilePath, "test-image-reupload.png")
		require.NoError(t, err, "File reupload failed")
		assert.Equal(t, http.StatusNoContent, statusCode, "Expected 204 No Content for file reupload")
	})

	t.Run("8_Verify_Reuploaded_File", func(t *testing.T) {
		// Wait a moment to ensure the file is available
		time.Sleep(2 * time.Second)
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		content, contentType, statusCode, err := downloadFileAttachment(endpoint)
		require.NoError(t, err, "File download failed")
		assert.Equal(t, http.StatusOK, statusCode, "Expected 200 OK for file download")
		assert.Equal(t, "image/gif", contentType, "Content-Type should match detected MIME type for uploaded GIF")
		assert.Equal(t, originalFileContent, content, "Reuploaded file content should match original")
	})

	t.Run("9_Delete_File_Attachment", func(t *testing.T) {
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		req, err := http.NewRequest("DELETE", endpoint, nil)
		require.NoError(t, err, "Failed to create DELETE request")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err, "DELETE request failed")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK for file deletion")
	})

	t.Run("10_Verify_File_Deleted", func(t *testing.T) {
		endpoint := fmt.Sprintf("%s/submodels/%s/submodel-elements/DemoFile/attachment", baseURL, submodelID)
		_, _, statusCode, _ := downloadFileAttachment(endpoint)
		assert.Equal(t, http.StatusNotFound, statusCode, "Expected 404 Not Found after file deletion")
	})
}

// TestMain handles setup and teardown
func TestMain(m *testing.M) {
	if os.Getenv("BASYX_EXTERNAL_COMPOSE") == "1" {
		os.Exit(m.Run())
	}

	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     "docker_compose/docker_compose.yml",
		PreDownBeforeUp: true,
		HealthURL:       "http://localhost:6004/health",
		HealthTimeout:   150 * time.Second,
	}))
}
