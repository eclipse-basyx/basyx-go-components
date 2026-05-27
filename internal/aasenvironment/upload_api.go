package aasenvironment

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"

	"github.com/go-chi/chi/v5"
)

const defaultUploadMaxSizeBytes int64 = 128 << 20

// RegisterUploadAPI registers the upload endpoint to the router
func RegisterUploadAPI(r chi.Router, service UploadService, maxUploadSizeBytes int64) {
	if maxUploadSizeBytes <= 0 {
		maxUploadSizeBytes = defaultUploadMaxSizeBytes
	}

	api := &uploadAPI{service: service, maxUploadSizeBytes: maxUploadSizeBytes}
	r.Post("/upload", api.HandleUpload)
}

// UploadService defines upload business logic without HTTP dependencies.
type UploadService interface {
	HandleUpload(ctx context.Context, fileName string, contentType string, file *os.File) (commonmodel.ImplResponse, error)
}

type uploadAPI struct {
	service            UploadService
	maxUploadSizeBytes int64
}

func (a *uploadAPI) HandleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, a.maxUploadSizeBytes)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeUploadError(
				w,
				http.StatusRequestEntityTooLarge,
				fmt.Errorf("request body exceeds upload limit of %d bytes", a.maxUploadSizeBytes),
				"AASENV-UPLOAD-MAXSIZEEXCEEDED",
			)
			return
		}

		writeUploadError(w, http.StatusBadRequest, err, "AASENV-UPLOAD-PARSEMULTIPART")
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	fileHeader, err := readMultipartFileHeader(r, "file")
	if err != nil {
		writeUploadError(w, http.StatusBadRequest, err, "AASENV-UPLOAD-READFILEHEADER")
		return
	}

	contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))

	fileName := strings.TrimSpace(r.FormValue("fileName"))
	if fileName == "" {
		fileName = fileHeader.Filename
	}
	fileName = sanitizeUploadFileName(fileName)

	file, err := commonmodel.ReadFileHeaderToTempFile(fileHeader)
	if err != nil {
		writeUploadError(w, http.StatusBadRequest, err, "AASENV-UPLOAD-READFILE")
		return
	}
	defer func() {
		closeAndRemoveTempFile(file)
	}()

	result, err := a.service.HandleUpload(r.Context(), fileName, contentType, file)
	if err != nil {
		writeUploadError(w, http.StatusInternalServerError, err, "AASENV-UPLOAD-HANDLER")
		return
	}

	if encErr := commonmodel.EncodeJSONResponse(result.Body, &result.Code, w); encErr != nil {
		writeUploadError(w, http.StatusInternalServerError, encErr, "AASENV-UPLOAD-ENCODERESPONSE")
	}
}

func writeUploadError(w http.ResponseWriter, status int, err error, info string) {
	resp := common.NewErrorResponse(err, status, "AASENV", "UploadAPI", info)
	if encErr := commonmodel.EncodeJSONResponse(resp.Body, &resp.Code, w); encErr != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func readMultipartFileHeader(r *http.Request, key string) (*multipart.FileHeader, error) {
	if r.MultipartForm == nil || r.MultipartForm.File == nil {
		return nil, errors.New("multipart form is missing")
	}

	fileHeaders, ok := r.MultipartForm.File[key]
	if !ok || len(fileHeaders) == 0 || fileHeaders[0] == nil {
		return nil, fmt.Errorf("multipart file field %q is required", key)
	}

	return fileHeaders[0], nil
}

func sanitizeUploadFileName(fileName string) string {
	baseName := strings.TrimSpace(fileName)
	if baseName == "" {
		return ""
	}

	baseName = strings.ReplaceAll(baseName, "\\", "/")
	if lastSlash := strings.LastIndex(baseName, "/"); lastSlash >= 0 {
		baseName = baseName[lastSlash+1:]
	}

	baseName = strings.TrimSpace(baseName)
	if baseName == "" || baseName == "." || baseName == "/" {
		return ""
	}

	return baseName
}
