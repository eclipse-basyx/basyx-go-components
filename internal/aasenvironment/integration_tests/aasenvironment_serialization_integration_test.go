package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aasx "github.com/aas-core-works/aas-package3-golang"
	"github.com/stretchr/testify/require"
)

const (
	fixtureAasxFilePathThreeAasXml                          = "testdata/threeAasDuplicateFilesSerializationTestXml.aasx"
	fixtureAasxFilePathThreeAasJson                         = "testdata/threeAasDuplicateFilesSerializationTestJson.aasx"
	serializationThreeAasxXmlDownloadPath                   = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_xml_from_xml_aasx_upload.aasx"
	serializationThreeAasxXmlAltDownloadPath                = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_xml_alt_from_xml_aasx_upload.aasx"
	serializationThreeAasxXmlDownloadFromJsonUploadPath     = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_xml_from_json_aasx_upload.aasx"
	serializationThreeAasxXmlAltDownloadFromJsonUploadPath  = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_xml_alt_from_json_aasx_upload.aasx"
	serializationThreeAasxJsonDownloadPath                  = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_json_from_xml_aasx_upload.aasx"
	serializationThreeAasxJsonAltDownloadPath               = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_json_alt_from_xml_aasx_upload.aasx"
	serializationThreeAasxJsonDownloadFromJsonUploadPath    = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_json_from_json_aasx_upload.aasx"
	serializationThreeAasxJsonAltDownloadFromJsonUploadPath = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_json_alt_from_json_aasx_upload.aasx"
	serializationThreeAasXmlDownloadFromXmlUploadPath       = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_xml_from_xml_aasx_upload.xml"
	serializationThreeAasXmlDownloadFromJsonUploadPath      = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_xml_from_json_aasx_upload.xml"
	serializationThreeAasJsonDownloadFromXmlUploadPath      = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_json_from_xml_aasx_upload.json"
	serializationThreeAasJsonDownloadFromJsonUploadPath     = "testdata_results/threeAASDuplicateFilesSerializationTest_downloaded_json_from_json_aasx_upload.json"
	serializationBaseURL                                    = "http://127.0.0.1:6004"

	serializationIntegrationDSN = "host=127.0.0.1 port=6432 user=admin password=admin123 dbname=basyxTestDB sslmode=disable"
)

func TestSerializationDownloadAasXmlAfterThreeAasUpload(t *testing.T) {
	skipSerializationTestsInCI(t)

	testCases := []struct {
		name       string
		uploadPath string
		uploadType string
		accept     string
		outputPath string
	}{
		{
			name:       "AASXXMLFromAASXXMLUpload",
			uploadPath: fixtureAasxFilePathThreeAasXml,
			uploadType: "application/aasx+xml",
			accept:     "application/aasx+xml",
			outputPath: serializationThreeAasxXmlDownloadPath,
		},
		{
			name:       "AssetAdministrationShellXMLFromAASXXMLUpload",
			uploadPath: fixtureAasxFilePathThreeAasXml,
			uploadType: "application/aasx+xml",
			accept:     "application/asset-administration-shell+xml",
			outputPath: serializationThreeAasxXmlAltDownloadPath,
		},
		{
			name:       "AASXXMLFromAASXJSONUpload",
			uploadPath: fixtureAasxFilePathThreeAasJson,
			uploadType: "application/aasx+json",
			accept:     "application/aasx+xml",
			outputPath: serializationThreeAasxXmlDownloadFromJsonUploadPath,
		},
		{
			name:       "AssetAdministrationShellXMLFromAASXJSONUpload",
			uploadPath: fixtureAasxFilePathThreeAasJson,
			uploadType: "application/aasx+json",
			accept:     "application/asset-administration-shell+xml",
			outputPath: serializationThreeAasxXmlAltDownloadFromJsonUploadPath,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)
			uploadFixture(t, testCase.uploadPath, testCase.uploadType)

			payload := downloadAASXSerializationFullEnvironment(t, testCase.accept)
			require.NotEmpty(t, payload)
			writeSerializationOutput(t, testCase.outputPath, payload)

			t.Logf("downloaded AASX XML serialization for Accept %q to %s", testCase.accept, testCase.outputPath)
		})
	}
}

func TestSerializationDownloadAasxJsonAfterThreeAASUpload(t *testing.T) {
	skipSerializationTestsInCI(t)

	testCases := []struct {
		name       string
		uploadPath string
		uploadType string
		accept     string
		outputPath string
	}{
		{
			name:       "AASXJSONFromAASXXMLUpload",
			uploadPath: fixtureAasxFilePathThreeAasXml,
			uploadType: "application/aasx+xml",
			accept:     "application/aasx+json",
			outputPath: serializationThreeAasxJsonDownloadPath,
		},
		{
			name:       "AssetAdministrationShellJSONFromAASXXMLUpload",
			uploadPath: fixtureAasxFilePathThreeAasXml,
			uploadType: "application/aasx+xml",
			accept:     "application/asset-administration-shell+json",
			outputPath: serializationThreeAasxJsonAltDownloadPath,
		},
		{
			name:       "AASXJSONFromAASXJSONUpload",
			uploadPath: fixtureAasxFilePathThreeAasJson,
			uploadType: "application/aasx+json",
			accept:     "application/aasx+json",
			outputPath: serializationThreeAasxJsonDownloadFromJsonUploadPath,
		},
		{
			name:       "AssetAdministrationShellJSONFromAASXJSONUpload",
			uploadPath: fixtureAasxFilePathThreeAasJson,
			uploadType: "application/aasx+json",
			accept:     "application/asset-administration-shell+json",
			outputPath: serializationThreeAasxJsonAltDownloadFromJsonUploadPath,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)
			uploadFixture(t, testCase.uploadPath, testCase.uploadType)

			payload := downloadAASXSerializationFullEnvironment(t, testCase.accept)
			require.NotEmpty(t, payload)
			writeSerializationOutput(t, testCase.outputPath, payload)

			t.Logf("downloaded AASX JSON serialization for Accept %q to %s", testCase.accept, testCase.outputPath)
		})
	}
}

func TestSerializationDownloadXmlAfterThreeAasUpload(t *testing.T) {
	skipSerializationTestsInCI(t)

	testCases := []struct {
		name       string
		uploadPath string
		uploadType string
		outputPath string
	}{
		{
			name:       "XMLFromAASXXMLUpload",
			uploadPath: fixtureAasxFilePathThreeAasXml,
			uploadType: "application/aasx+xml",
			outputPath: serializationThreeAasXmlDownloadFromXmlUploadPath,
		},
		{
			name:       "XMLFromAASXJSONUpload",
			uploadPath: fixtureAasxFilePathThreeAasJson,
			uploadType: "application/aasx+json",
			outputPath: serializationThreeAasXmlDownloadFromJsonUploadPath,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)
			uploadFixture(t, testCase.uploadPath, testCase.uploadType)

			payload := downloadAASXSerializationFullEnvironment(t, "application/xml")
			require.NotEmpty(t, payload)
			writeSerializationOutput(t, testCase.outputPath, payload)

			t.Logf("downloaded XML serialization to %s", testCase.outputPath)
		})
	}
}

func TestSerializationDownloadJsonAfterThreeAasUpload(t *testing.T) {
	skipSerializationTestsInCI(t)

	testCases := []struct {
		name       string
		uploadPath string
		uploadType string
		outputPath string
	}{
		{
			name:       "JSONFromAASXXMLUpload",
			uploadPath: fixtureAasxFilePathThreeAasXml,
			uploadType: "application/aasx+xml",
			outputPath: serializationThreeAasJsonDownloadFromXmlUploadPath,
		},
		{
			name:       "JSONFromAASXJSONUpload",
			uploadPath: fixtureAasxFilePathThreeAasJson,
			uploadType: "application/aasx+json",
			outputPath: serializationThreeAasJsonDownloadFromJsonUploadPath,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resetDatabaseForUploadIT(t, serializationIntegrationDSN)
			uploadFixture(t, testCase.uploadPath, testCase.uploadType)

			payload := downloadAASXSerializationFullEnvironment(t, "application/json")
			require.NotEmpty(t, payload)
			writeSerializationOutput(t, testCase.outputPath, payload)

			t.Logf("downloaded JSON serialization to %s", testCase.outputPath)
		})
	}
}

func TestSerializationDownloadAASXXMLAfterThreeAASUploadMatchesUploadedSets(t *testing.T) {
	skipSerializationTestsInCI(t)
	packaging := aasx.NewPackaging()
	packageReader, err := packaging.OpenRead(filepath.Clean(fixtureAasxFilePathThreeAasXml))
	require.NoErrorf(t, err, "failed to open fixture package %q", fixtureAasxFilePathThreeAasXml)
	defer func() { _ = packageReader.Close() }()

	specParts, err := packageReader.Specs()
	require.NoError(t, err)
	require.NotEmptyf(t, specParts, "fixture %q does not contain spec parts", fixtureAasxFilePathThreeAasXml)
}

func writeSerializationOutput(t *testing.T, outputPath string, payload []byte) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(outputPath), 0o750))
	require.NoError(t, os.WriteFile(outputPath, payload, 0o600))
}

func skipSerializationTestsInCI(t *testing.T) {
	t.Helper()

	if isTruthyEnv("CI") || isTruthyEnv("GITHUB_ACTIONS") {
		t.Skip("serialization download integration tests are local-only and skipped in CI")
	}
}

func isTruthyEnv(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func uploadFixture(t *testing.T, fixturePath string, partContentType string) {
	t.Helper()

	file, err := os.Open(filepath.Clean(fixturePath))
	require.NoError(t, err)
	defer func() { _ = file.Close() }()

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", fmt.Sprintf("form-data; name=%q; filename=%q", "file", filepath.Base(fixturePath)))
	partHeader.Set("Content-Type", partContentType)

	part, err := writer.CreatePart(partHeader)
	require.NoError(t, err)

	_, err = io.Copy(part, file)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPost, serializationBaseURL+"/upload", payload)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp := doHTTPIntegrationRequest(t, client, req)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "upload failed: %s", string(body))
}

func downloadAASXSerializationFullEnvironment(t *testing.T, acceptHeader string) []byte {
	t.Helper()

	query := url.Values{}
	query.Set("includeConceptDescriptions", "true")

	req, err := http.NewRequest(http.MethodGet, serializationBaseURL+"/serialization?"+query.Encode(), nil)
	require.NoError(t, err)
	req.Header.Set("Accept", acceptHeader)

	client := &http.Client{Timeout: 60 * time.Second}
	resp := doHTTPIntegrationRequest(t, client, req)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "serialization full environment request failed for Accept %q: %s", acceptHeader, string(body))

	return body
}
