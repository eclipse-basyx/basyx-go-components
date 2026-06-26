package openapi

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

type captureSubmodelFileUploadService struct {
	SubmodelRepositoryAPIAPIServicer
	fileName string
	content  []byte
	readErr  error
}

func (s *captureSubmodelFileUploadService) PutFileByPathSubmodelRepo(_ context.Context, _ string, _ string, fileName string, file io.Reader) (commonmodel.ImplResponse, error) {
	content, err := io.ReadAll(file)
	s.readErr = err
	if err != nil {
		return commonmodel.Response(http.StatusInternalServerError, map[string]string{"error": "read failed"}), nil
	}

	s.fileName = fileName
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
	if !bytes.Equal(service.content, payload) {
		t.Fatalf("expected uploaded payload %q, got %q", string(payload), string(service.content))
	}
}

func TestPutFileByPathSubmodelRepoReturnsPayloadTooLargeForOversizedStream(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 1024)
	request := newMultipartUploadRequest(t, "/submodels/sm/submodel-elements/file/attachment", "oversized.txt", payload)
	applyUploadLimit(request, request.ContentLength-int64(len(payload)/2))
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

func TestPutFileByPathSubmodelRepoUsesMultipartFilenameWhenFileNameFieldFollowsFile(t *testing.T) {
	payload := []byte("submodel attachment payload")
	request := newMultipartUploadRequestWithFileNameOrder(
		t,
		"/submodels/sm/submodel-elements/file/attachment",
		"metadata-name.txt",
		"part-name.txt",
		payload,
		false,
	)
	addRouteParam(request, "submodelIdentifier", "sm")
	addRouteParam(request, "idShortPath", "file")

	service := &captureSubmodelFileUploadService{}
	controller := NewSubmodelRepositoryAPIAPIController(service, "", "")
	response := httptest.NewRecorder()

	controller.PutFileByPathSubmodelRepo(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected upload to succeed, got status %d body %s", response.Code, response.Body.String())
	}
	if service.fileName != "part-name.txt" {
		t.Fatalf("expected multipart part filename part-name.txt, got %q", service.fileName)
	}
	if !bytes.Equal(service.content, payload) {
		t.Fatalf("expected uploaded payload %q, got %q", string(payload), string(service.content))
	}
}

func newMultipartUploadRequest(t *testing.T, target string, fileName string, payload []byte) *http.Request {
	return newMultipartUploadRequestWithFileNameOrder(t, target, fileName, fileName, payload, true)
}

func newMultipartUploadRequestWithFileNameOrder(
	t *testing.T,
	target string,
	fieldFileName string,
	partFileName string,
	payload []byte,
	fileNameBeforeFile bool,
) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if fileNameBeforeFile {
		writeFileNameField(t, writer, fieldFileName)
	}
	part, err := writer.CreateFormFile("file", partFileName)
	if err != nil {
		t.Fatalf("failed to create multipart file: %v", err)
	}
	if _, err = part.Write(payload); err != nil {
		t.Fatalf("failed to write multipart payload: %v", err)
	}
	if !fileNameBeforeFile {
		writeFileNameField(t, writer, fieldFileName)
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
