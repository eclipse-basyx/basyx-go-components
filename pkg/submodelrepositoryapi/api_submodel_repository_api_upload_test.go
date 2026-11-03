package openapi

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"path/filepath"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

type captureSubmodelFileUploadService struct {
	SubmodelRepositoryAPIAPIServicer
	fileName    string
	contentType string
	content     []byte
	readErr     error
	status      int
}

func (s *captureSubmodelFileUploadService) PutFileByPathSubmodelRepo(
	_ context.Context,
	_ string,
	_ string,
	fileName string,
	contentType string,
	file io.Reader,
) (commonmodel.ImplResponse, error) {
	content, err := io.ReadAll(file)
	s.readErr = err
	if err != nil {
		return commonmodel.Response(http.StatusInternalServerError, map[string]string{"error": "read failed"}), nil
	}

	s.fileName = fileName
	s.contentType = contentType
	s.content = content

	return commonmodel.Response(http.StatusNoContent, nil), nil
}

func TestPutFileByPathSubmodelRepoDoesNotRequireTempDirectory(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	payload := []byte("submodel attachment payload")
	request := newMultipartUploadRequest(t, "/submodels/sm/submodel-elements/file/attachment", "attachment.txt", payload)
	addRouteParam(request, "submodelIdentifier", "sm")
	addRouteParam(request, "idShortPath", "file")

	service := &captureSubmodelFileUploadService{}
	controller := NewSubmodelRepositoryAPIAPIController(service, "", "")
	response := httptest.NewRecorder()

	controller.PutFileByPathSubmodelRepo(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected upload to succeed without TMPDIR, got status %d body %s", response.Code, response.Body.String())
	}
	if service.fileName != "attachment.txt" {
		t.Fatalf("expected fileName attachment.txt, got %q", service.fileName)
	}
	if service.contentType != "application/x-step" {
		t.Fatalf("expected content type application/x-step, got %q", service.contentType)
	}
	if !bytes.Equal(service.content, payload) {
		t.Fatalf("expected uploaded payload %q, got %q", string(payload), string(service.content))
	}
}

func TestPutFileByPathSubmodelRepoAcceptsFileAtConfiguredLimit(t *testing.T) {
	const uploadLimit int64 = 1024
	payload := bytes.Repeat([]byte("x"), int(uploadLimit))
	request := newMultipartUploadRequest(t, "/submodels/sm/submodel-elements/file/attachment", "maximum.txt", payload)
	applyUploadLimit(request, uploadLimit)
	addRouteParam(request, "submodelIdentifier", "sm")
	addRouteParam(request, "idShortPath", "file")

	service := &captureSubmodelFileUploadService{}
	controller := NewSubmodelRepositoryAPIAPIController(service, "", "")
	response := httptest.NewRecorder()

	controller.PutFileByPathSubmodelRepo(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected file at configured limit to succeed, got status %d body %s", response.Code, response.Body.String())
	}
	if !bytes.Equal(service.content, payload) {
		t.Fatalf("expected %d uploaded bytes, got %d", len(payload), len(service.content))
	}
}

func TestPutFileByPathSubmodelRepoReturnsPayloadTooLargeForOversizedStream(t *testing.T) {
	const uploadLimit int64 = 1024
	payload := bytes.Repeat([]byte("x"), int(common.MultipartRequestSizeLimit(uploadLimit)))
	request := newMultipartUploadRequest(t, "/submodels/sm/submodel-elements/file/attachment", "oversized.txt", payload)
	applyUploadLimit(request, uploadLimit)
	addRouteParam(request, "submodelIdentifier", "sm")
	addRouteParam(request, "idShortPath", "file")

	service := &captureSubmodelFileUploadService{}
	controller := NewSubmodelRepositoryAPIAPIController(service, "", "")
	response := httptest.NewRecorder()

	controller.PutFileByPathSubmodelRepo(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected oversized upload to return 413, got status %d body %s", response.Code, response.Body.String())
	}
	if service.readErr == nil {
		t.Fatal("expected service to observe a read error from the limited request body")
	}
}

func TestPutFileByPathSubmodelRepoIgnoresFileNameField(t *testing.T) {
	payload := []byte("submodel attachment payload")
	request := newMultipartUploadRequestWithFileNameField(
		t, "/submodels/sm/submodel-elements/file/attachment", "field-name.txt", "part-name.txt", payload,
	)
	service := putSubmodelFileByPath(request)

	assertCapturedSubmodelUpload(t, service, "part-name.txt", payload)
}

func TestPutFileByPathSubmodelRepoAcceptsPartWithoutFileName(t *testing.T) {
	payload := []byte("submodel attachment payload")
	request := newMultipartUploadRequestWithFileNameField(t, "/submodels/sm/submodel-elements/file/attachment", "", "", payload)
	service := putSubmodelFileByPath(request)

	assertCapturedSubmodelUpload(t, service, "", payload)
}

func putSubmodelFileByPath(request *http.Request) *captureSubmodelFileUploadService {
	addRouteParam(request, "submodelIdentifier", "sm")
	addRouteParam(request, "idShortPath", "file")
	service := &captureSubmodelFileUploadService{}
	controller := NewSubmodelRepositoryAPIAPIController(service, "", "")
	response := httptest.NewRecorder()
	controller.PutFileByPathSubmodelRepo(response, request)
	service.status = response.Code
	return service
}

func assertCapturedSubmodelUpload(t *testing.T, service *captureSubmodelFileUploadService, expectedFileName string, expectedPayload []byte) {
	t.Helper()

	if service.status != http.StatusNoContent {
		t.Fatalf("expected upload to succeed, got status %d", service.status)
	}
	if service.fileName != expectedFileName {
		t.Fatalf("expected fileName %q, got %q", expectedFileName, service.fileName)
	}
	if service.contentType != "application/x-step" {
		t.Fatalf("expected content type application/x-step, got %q", service.contentType)
	}
	if !bytes.Equal(service.content, expectedPayload) {
		t.Fatalf("expected uploaded payload %q, got %q", string(expectedPayload), string(service.content))
	}
}

func newMultipartUploadRequest(t *testing.T, target string, fileName string, payload []byte) *http.Request {
	return newMultipartUploadRequestWithFileNameField(t, target, "", fileName, payload)
}

func newMultipartUploadRequestWithFileNameField(
	t *testing.T,
	target string,
	fieldFileName string,
	partFileName string,
	payload []byte,
) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if fieldFileName != "" {
		writeFileNameField(t, writer, fieldFileName)
	}
	contentDisposition := `form-data; name="file"`
	if partFileName != "" {
		contentDisposition += `; filename="` + partFileName + `"`
	}
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", contentDisposition)
	partHeader.Set("Content-Type", "application/x-step")
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		t.Fatalf("failed to create multipart file: %v", err)
	}
	if _, err = part.Write(payload); err != nil {
		t.Fatalf("failed to write multipart payload: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPut, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func writeFileNameField(t *testing.T, writer *multipart.Writer, fileName string) {
	t.Helper()

	if err := writer.WriteField("fileName", fileName); err != nil {
		t.Fatalf("failed to write fileName field: %v", err)
	}
}

func applyUploadLimit(request *http.Request, uploadMaxSizeBytes int64) {
	cfg := &common.Config{}
	cfg.General.UploadMaxSizeBytes = uploadMaxSizeBytes
	*request = *request.WithContext(common.ContextWithConfig(request.Context(), cfg))
}

func addRouteParam(request *http.Request, key string, value string) {
	routeContext := chi.RouteContext(request.Context())
	if routeContext == nil {
		routeContext = chi.NewRouteContext()
		*request = *request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	}
	routeContext.URLParams.Add(key, value)
}
