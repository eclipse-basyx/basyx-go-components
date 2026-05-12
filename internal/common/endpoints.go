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
package common

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	aasjsonization "github.com/aas-core-works/aas-core3.1-golang/jsonization"
	aastypes "github.com/aas-core-works/aas-core3.1-golang/types"
	"github.com/aas-core-works/aas-core3.1-golang/verification"
	aasxmlization "github.com/aas-core-works/aas-core3.1-golang/xmlization"
	aasx "github.com/aas-core-works/aas-package3-golang"
	"github.com/go-chi/chi/v5"
)

const verifyMaxPayloadBytes = 128 << 20

// HealthProbe reports if the service is healthy and optionally returns detail text.
type HealthProbe func() (bool, string)

// AddHealthEndpoint registers a health check endpoint on the provided router.
//
// The health endpoint provides a simple way to verify that the service is running
// and responsive. It's commonly used by load balancers, monitoring systems,
// and container orchestrators to determine service health.
//
// Endpoint details:
//   - Method: GET
//   - Path: {contextPath}/health
//   - Response: HTTP 200 with JSON body {"status":"UP"}
//   - Content-Type: application/json (implicit)
//
// Parameters:
//   - r: Chi router to register the health endpoint on
//   - config: Configuration containing the server context path
//
// Example:
//
//	router := chi.NewRouter()
//	config := &Config{Server: ServerConfig{ContextPath: "/api/v1"}}
//	AddHealthEndpoint(router, config)
//	// Health check available at: GET /api/v1/health
//
// Response format:
//
//	{
//	  "status": "UP"
//	}
func AddHealthEndpoint(r *chi.Mux, config *Config) {
	AddHealthEndpointWithProbe(r, config, nil)
}

// AddHealthEndpointWithProbe registers a health endpoint with optional readiness probing.
func AddHealthEndpointWithProbe(r *chi.Mux, config *Config, probe HealthProbe) {
	r.Get(config.Server.ContextPath+"/health", func(w http.ResponseWriter, _ *http.Request) {
		if probe != nil {
			healthy, details := probe()
			if !healthy {
				response := map[string]string{"status": "DOWN"}
				if strings.TrimSpace(details) != "" {
					response["details"] = details
				}
				writeHealthResponse(w, http.StatusServiceUnavailable, response)
				return
			}
		}

		writeHealthResponse(w, http.StatusOK, map[string]string{"status": "UP"})
	})
}

func writeHealthResponse(w http.ResponseWriter, statusCode int, body map[string]string) {
	responsePayload, err := json.Marshal(body)
	if err != nil {
		log.Printf("COMMON-WRITEHEALTH-MARSHAL response marshal failed: %v", err)
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, err = w.Write(responsePayload); err != nil {
		log.Printf("COMMON-WRITEHEALTH-WRITE response write failed: %v", err)
	}
}

// AddVerificationEndpoint registers the POST /verify Endpoint that accepts a JSON, XML or AASX payload and verifies it against the AAS meta model regardless of the service and verification mode.
func AddVerificationEndpoint(r *chi.Mux, config *Config) {
	r.Post(config.Server.ContextPath+"/verify", func(w http.ResponseWriter, r *http.Request) {
		verificationResult, err := VerifyPayload(r)
		if err != nil {
			log.Printf("COMMON-VERIFY-PAYLOAD failed to verify payload: %v", err)
			http.Error(w, "Failed to verify payload: "+err.Error(), http.StatusBadRequest)
			return
		}

		responsePayload, err := json.Marshal(verificationResult)
		if err != nil {
			log.Printf("COMMON-VERIFY-PAYLOAD-MARSHAL failed to marshal verification result: %v", err)
			http.Error(w, "Failed to write response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err = w.Write(responsePayload); err != nil {
			log.Printf("COMMON-VERIFY-PAYLOAD-WRITE response write failed: %v", err)
		}
	})
}

// VerifyPayload verifies the request payload and returns a structured result. It supports JSON, XML, and AASX formats.
func VerifyPayload(r *http.Request) (map[string]interface{}, error) {
	payload, fileName, contentType, err := readVerificationPayload(r)
	if err != nil {
		return nil, fmt.Errorf("COMMON-VERIFYPAYLOAD-READPAYLOAD %w", err)
	}

	format, err := detectVerificationFormat(fileName, contentType, payload)
	if err != nil {
		return nil, fmt.Errorf("COMMON-VERIFYPAYLOAD-DETECTFORMAT %w", err)
	}

	environment, err := parseVerificationEnvironment(format, payload)
	if err != nil {
		return nil, fmt.Errorf("COMMON-VERIFYPAYLOAD-PARSE %w", err)
	}

	messages := collectVerificationMessages(environment)

	return map[string]interface{}{
		"valid":                         len(messages) == 0,
		"format":                        format,
		"assetAdministrationShellCount": len(environment.AssetAdministrationShells()),
		"submodelCount":                 len(environment.Submodels()),
		"conceptDescriptionCount":       len(environment.ConceptDescriptions()),
		"messages":                      messages,
	}, nil

}

func readVerificationPayload(r *http.Request) ([]byte, string, string, error) {
	if r == nil {
		return nil, "", "", fmt.Errorf("request is nil")
	}

	headerContentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if strings.HasPrefix(strings.ToLower(headerContentType), "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return nil, "", "", fmt.Errorf("failed to parse multipart form: %w", err)
		}

		file, fileHeader, err := r.FormFile("file")
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to read multipart file field 'file': %w", err)
		}
		defer func() {
			_ = file.Close()
		}()

		payload, err := io.ReadAll(io.LimitReader(file, verifyMaxPayloadBytes+1))
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to read multipart file payload: %w", err)
		}
		if int64(len(payload)) > verifyMaxPayloadBytes {
			return nil, "", "", fmt.Errorf("payload exceeds max size of %d bytes", verifyMaxPayloadBytes)
		}
		if len(bytes.TrimSpace(payload)) == 0 {
			return nil, "", "", fmt.Errorf("payload is empty")
		}

		fileContentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
		if fileContentType == "" {
			fileContentType = headerContentType
		}

		return payload, fileHeader.Filename, fileContentType, nil
	}

	if r.Body == nil {
		return nil, "", "", fmt.Errorf("request body is empty")
	}
	defer func() {
		_ = r.Body.Close()
	}()

	payload, err := io.ReadAll(io.LimitReader(r.Body, verifyMaxPayloadBytes+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read request body: %w", err)
	}
	if int64(len(payload)) > verifyMaxPayloadBytes {
		return nil, "", "", fmt.Errorf("payload exceeds max size of %d bytes", verifyMaxPayloadBytes)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, "", "", fmt.Errorf("payload is empty")
	}

	return payload, "", headerContentType, nil
}

func detectVerificationFormat(fileName string, contentType string, payload []byte) (string, error) {
	normalizedContentType := normalizeVerificationContentType(contentType)
	if normalizedContentType != "" {
		switch normalizedContentType {
		case "application/aasx+xml", "application/aasx+json", "application/asset-administration-shell+xml", "application/asset-administration-shell+json":
			return "aasx", nil
		case "application/json":
			return "json", nil
		case "application/xml", "text/xml":
			return "xml", nil
		default:
			return "", fmt.Errorf("unsupported content type %q", normalizedContentType)
		}
	}

	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	switch extension {
	case ".aasx":
		return "aasx", nil
	case ".json":
		return "json", nil
	case ".xml":
		return "xml", nil
	}

	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("unable to determine format from empty payload")
	}
	if len(payload) >= 2 && payload[0] == 0x50 && payload[1] == 0x4b {
		return "aasx", nil
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		return "json", nil
	}
	if trimmed[0] == '<' {
		return "xml", nil
	}

	return "", fmt.Errorf("unsupported payload format")
}

func normalizeVerificationContentType(contentType string) string {
	trimmed := strings.TrimSpace(strings.ToLower(contentType))
	if trimmed == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(trimmed)
	if err != nil {
		return ""
	}

	normalized := strings.TrimSpace(strings.ToLower(mediaType))
	if normalized == "application/octet-stream" {
		return ""
	}

	return normalized
}

func parseVerificationEnvironment(format string, payload []byte) (aastypes.IEnvironment, error) {
	switch format {
	case "json":
		return parseVerificationJSONEnvironment(payload)
	case "xml":
		return parseVerificationXMLEnvironment(payload)
	case "aasx":
		return parseVerificationAASXEnvironment(payload)
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func parseVerificationJSONEnvironment(payload []byte) (aastypes.IEnvironment, error) {
	var jsonable any
	if err := json.Unmarshal(payload, &jsonable); err != nil {
		return nil, err
	}

	environment, err := aasjsonization.EnvironmentFromJsonable(jsonable)
	if err != nil {
		return nil, err
	}

	return environment, nil
}

func parseVerificationXMLEnvironment(payload []byte) (aastypes.IEnvironment, error) {
	instance, err := parseVerificationXMLInstance(payload)
	if err != nil {
		return nil, err
	}

	environment, ok := instance.(aastypes.IEnvironment)
	if !ok {
		return nil, fmt.Errorf("XML root is %T, expected AAS Environment", instance)
	}

	return environment, nil
}

func parseVerificationXMLInstance(payload []byte) (aastypes.IClass, error) {
	instance, err := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(payload)))
	if err == nil {
		return instance, nil
	}

	normalized := normalizeVerificationXMLContent(payload)
	if len(normalized) == 0 || bytes.Equal(normalized, payload) {
		return nil, err
	}

	retried, retryErr := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(normalized)))
	if retryErr == nil {
		return retried, nil
	}

	sanitized, sanitizeErr := sanitizeVerificationXMLRootAttributes(normalized)
	if sanitizeErr == nil && len(sanitized) > 0 {
		sanitizedRetried, sanitizedRetryErr := aasxmlization.Unmarshal(xml.NewDecoder(bytes.NewReader(sanitized)))
		if sanitizedRetryErr == nil {
			return sanitizedRetried, nil
		}
		return nil, fmt.Errorf("%w (retry after normalization failed: %v; retry after root attribute sanitization failed: %v)", err, retryErr, sanitizedRetryErr)
	}

	if sanitizeErr != nil {
		return nil, fmt.Errorf("%w (retry after normalization failed: %v; root attribute sanitization failed: %v)", err, retryErr, sanitizeErr)
	}

	return nil, fmt.Errorf("%w (retry after normalization failed: %v)", err, retryErr)
}

func normalizeVerificationXMLContent(payload []byte) []byte {
	content := bytes.TrimSpace(payload)
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})
	content = bytes.TrimSpace(content)

	start := firstVerificationXMLStartElementIndex(content)
	if start < 0 || start >= len(content) {
		return content
	}

	return bytes.TrimSpace(content[start:])
}

func firstVerificationXMLStartElementIndex(content []byte) int {
	index := 0
	for index < len(content) {
		lt := bytes.IndexByte(content[index:], '<')
		if lt < 0 {
			return -1
		}
		candidate := index + lt
		if candidate+1 >= len(content) {
			return -1
		}

		next := content[candidate+1]
		if next != '?' && next != '!' {
			return candidate
		}

		closing := bytes.IndexByte(content[candidate:], '>')
		if closing < 0 {
			return -1
		}
		index = candidate + closing + 1
	}

	return -1
}

func sanitizeVerificationXMLRootAttributes(content []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	rootProcessed := false

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		start, ok := token.(xml.StartElement)
		if !ok || rootProcessed {
			if encodeErr := encoder.EncodeToken(token); encodeErr != nil {
				return nil, encodeErr
			}
			continue
		}

		filtered := make([]xml.Attr, 0, len(start.Attr))
		for _, attribute := range start.Attr {
			if attribute.Name.Space == "xmlns" || (attribute.Name.Space == "" && attribute.Name.Local == "xmlns") {
				filtered = append(filtered, attribute)
			}
		}
		start.Attr = filtered
		if encodeErr := encoder.EncodeToken(start); encodeErr != nil {
			return nil, encodeErr
		}
		rootProcessed = true
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}

	return output.Bytes(), nil
}

func parseVerificationAASXEnvironment(payload []byte) (aastypes.IEnvironment, error) {
	packaging := aasx.NewPackaging()
	packageReader, err := packaging.OpenReadFromStream(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = packageReader.Close()
	}()

	specs, err := packageReader.Specs()
	if err != nil {
		return nil, err
	}

	supportedSpecs := make([]*aasx.Part, 0, len(specs))
	for _, spec := range specs {
		if spec == nil {
			continue
		}
		uri := ""
		if spec.URI != nil {
			uri = strings.ToLower(strings.TrimSpace(spec.URI.String()))
		}
		specContentType := strings.ToLower(strings.TrimSpace(spec.ContentType))
		if strings.HasSuffix(uri, ".json") || strings.Contains(specContentType, "json") || strings.HasSuffix(uri, ".xml") || strings.Contains(specContentType, "xml") {
			supportedSpecs = append(supportedSpecs, spec)
		}
	}

	if len(supportedSpecs) == 0 {
		return nil, fmt.Errorf("no supported AASX specs found")
	}
	if len(supportedSpecs) > 1 {
		return nil, fmt.Errorf("multiple supported AASX specs found")
	}

	specContent, err := supportedSpecs[0].ReadAllBytes()
	if err != nil {
		return nil, err
	}

	uri := ""
	if supportedSpecs[0].URI != nil {
		uri = strings.ToLower(strings.TrimSpace(supportedSpecs[0].URI.String()))
	}
	specContentType := strings.ToLower(strings.TrimSpace(supportedSpecs[0].ContentType))
	if strings.HasSuffix(uri, ".json") || strings.Contains(specContentType, "json") {
		return parseVerificationJSONEnvironment(specContent)
	}

	return parseVerificationXMLEnvironment(specContent)
}

func collectVerificationMessages(environment aastypes.IEnvironment) []string {
	messages := make([]string, 0)
	verification.Verify(environment, func(verErr *verification.VerificationError) bool {
		if verErr != nil {
			messages = append(messages, verErr.Error())
		}
		return true
	})
	return messages

}
