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

	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

type captureSubmodelFileUploadService struct {
	SubmodelRepositoryAPIAPIServicer
	fileName string
	content  []byte
}

func (s *captureSubmodelFileUploadService) PutFileByPathSubmodelRepo(_ context.Context, _ string, _ string, fileName string, file io.ReadSeeker) (commonmodel.ImplResponse, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return commonmodel.ImplResponse{}, err
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

func newMultipartUploadRequest(t *testing.T, target string, fileName string, payload []byte) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("failed to create multipart file: %v", err)
	}
	if _, err = part.Write(payload); err != nil {
		t.Fatalf("failed to write multipart payload: %v", err)
	}
	if err = writer.WriteField("fileName", fileName); err != nil {
		t.Fatalf("failed to write fileName field: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPut, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func addRouteParam(request *http.Request, key string, value string) {
	routeContext := chi.RouteContext(request.Context())
	if routeContext == nil {
		routeContext = chi.NewRouteContext()
		*request = *request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	}
	routeContext.URLParams.Add(key, value)
}
