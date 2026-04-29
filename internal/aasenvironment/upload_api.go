package aasenvironment

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"

	"github.com/go-chi/chi/v5"
)

// RegisterUploadAPI registers the upload endpoint to the router
func RegisterUploadAPI(r chi.Router, service UploadService) {
	api := &uploadAPI{service: service}
	r.Post("/upload", api.HandleUpload)
}

// UploadService defines upload business logic without HTTP dependencies.
type UploadService interface {
	HandleUpload(ctx context.Context, fileName string, contentType string, file *os.File) (commonmodel.ImplResponse, error)
}

type uploadAPI struct {
	service UploadService
}

func (a *uploadAPI) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeUploadError(w, http.StatusBadRequest, err, "AASENV-UPLOAD-PARSEMULTIPART")
		return
	}

	_, fileHeader, err := r.FormFile("file")
	if err != nil {
		writeUploadError(w, http.StatusBadRequest, err, "AASENV-UPLOAD-READFILEHEADER")
		return
	}

	contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	if contentType == "" {
		writeUploadError(w, http.StatusUnsupportedMediaType, errors.New("missing content type for multipart file part"), "AASENV-UPLOAD-MISSINGCONTENTTYPE")
		return
	}

	fileName := strings.TrimSpace(r.FormValue("fileName"))
	if fileName == "" {
		fileName = fileHeader.Filename
	}

	file, err := commonmodel.ReadFormFileToTempFile(r, "file")
	if err != nil {
		writeUploadError(w, http.StatusBadRequest, err, "AASENV-UPLOAD-READFILE")
		return
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
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
