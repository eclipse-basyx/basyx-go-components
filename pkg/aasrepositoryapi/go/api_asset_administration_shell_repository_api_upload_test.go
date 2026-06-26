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

type captureAASUploadService struct {
	AssetAdministrationShellRepositoryAPIAPIServicer
	fileName string
	content  []byte
}

func (s *captureAASUploadService) PutThumbnailAasRepository(_ context.Context, _ string, fileName string, file io.ReadSeeker) (commonmodel.ImplResponse, error) {
	return s.capture(fileName, file)
}

func (s *captureAASUploadService) PutFileByPathAasRepository(_ context.Context, _ string, _ string, _ string, fileName string, file io.ReadSeeker) (commonmodel.ImplResponse, error) {
	return s.capture(fileName, file)
}

func (s *captureAASUploadService) capture(fileName string, file io.ReadSeeker) (commonmodel.ImplResponse, error) {
	content, err := io.ReadAll(file)
	if err != nil {
		return commonmodel.ImplResponse{}, err
	}

	s.fileName = fileName
	s.content = content

	return commonmodel.Response(http.StatusNoContent, nil), nil
}

func TestPutThumbnailAasRepositoryDoesNotRequireTempDirectory(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	payload := []byte("thumbnail payload")
	request := newMultipartUploadRequest(t, "/shells/aas/asset-information/thumbnail", "thumbnail.bin", payload)
	addRouteParam(request, "aasIdentifier", "aas")

	service := &captureAASUploadService{}
	controller := NewAssetAdministrationShellRepositoryAPIAPIController(service, "", "")
	response := httptest.NewRecorder()

	controller.PutThumbnailAasRepository(response, request)

	assertCapturedUpload(t, response, service, "thumbnail.bin", payload)
}

func TestPutFileByPathAasRepositoryDoesNotRequireTempDirectory(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	payload := []byte("aas-scoped attachment payload")
	request := newMultipartUploadRequest(t, "/shells/aas/submodels/sm/submodel-elements/file/attachment", "attachment.txt", payload)
	addRouteParam(request, "aasIdentifier", "aas")
	addRouteParam(request, "submodelIdentifier", "sm")
	addRouteParam(request, "idShortPath", "file")

	service := &captureAASUploadService{}
	controller := NewAssetAdministrationShellRepositoryAPIAPIController(service, "", "")
	response := httptest.NewRecorder()

	controller.PutFileByPathAasRepository(response, request)

	assertCapturedUpload(t, response, service, "attachment.txt", payload)
}

func assertCapturedUpload(t *testing.T, response *httptest.ResponseRecorder, service *captureAASUploadService, expectedFileName string, expectedPayload []byte) {
	t.Helper()

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected upload to succeed without TMPDIR, got status %d body %s", response.Code, response.Body.String())
	}
	if service.fileName != expectedFileName {
		t.Fatalf("expected fileName %s, got %q", expectedFileName, service.fileName)
	}
	if !bytes.Equal(service.content, expectedPayload) {
		t.Fatalf("expected uploaded payload %q, got %q", string(expectedPayload), string(service.content))
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
