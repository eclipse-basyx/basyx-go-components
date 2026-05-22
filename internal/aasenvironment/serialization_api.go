package aasenvironment

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

// RegisterSerializationAPI registers the serialization endpoint to the router.
func RegisterSerializationAPI(r chi.Router, service SerializationService) {
	api := &serializationAPI{service: service}
	r.Get("/serialization", api.GenerateSerializationByIDs)
}

// SerializationService defines serialization business logic without HTTP dependencies.
type SerializationService interface {
	GenerateSerializationByIds(ctx context.Context, aasIDs []string, submodelIDs []string, includeConceptDescriptions bool) (commonmodel.ImplResponse, error)
}

type serializationAPI struct {
	service SerializationService
}

func (a *serializationAPI) GenerateSerializationByIDs(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.service == nil {
		writeSerializationError(w, http.StatusInternalServerError, errors.New("serialization service is required"), "AASENV-SERIALIZATIONAPI-NILSERVICE")
		return
	}

	query := r.URL.Query()
	aasIDsParam := parseSerializationStringArrayQueryParam(query["aasIds"])
	submodelIDsParam := parseSerializationStringArrayQueryParam(query["submodelIds"])

	includeConceptDescriptionsParam := true
	if query.Has("includeConceptDescriptions") {
		parsedIncludeConceptDescriptions, parseErr := parseSerializationBoolQueryParam(query.Get("includeConceptDescriptions"))
		if parseErr != nil {
			writeSerializationError(w, http.StatusBadRequest, parseErr, "AASENV-SERIALIZATIONAPI-PARSEINCLUDECDS")
			return
		}
		includeConceptDescriptionsParam = parsedIncludeConceptDescriptions
	}

	requestContext := common.WithAcceptHeader(r.Context(), r.Header.Get("Accept"))
	result, err := a.service.GenerateSerializationByIds(requestContext, aasIDsParam, submodelIDsParam, includeConceptDescriptionsParam)
	if err != nil {
		writeSerializationError(w, http.StatusInternalServerError, err, "AASENV-SERIALIZATIONAPI-HANDLER")
		return
	}

	switch fileDownload := result.Body.(type) {
	case SerializationFileDownload:
		writeSerializationFileDownload(w, result.Code, fileDownload)
		return
	case *SerializationFileDownload:
		if fileDownload != nil {
			writeSerializationFileDownload(w, result.Code, *fileDownload)
			return
		}
	}

	if encodeErr := commonmodel.EncodeJSONResponse(result.Body, &result.Code, w); encodeErr != nil {
		writeSerializationError(w, http.StatusInternalServerError, encodeErr, "AASENV-SERIALIZATIONAPI-ENCODERESPONSE")
	}
}

func writeSerializationFileDownload(w http.ResponseWriter, status int, payload SerializationFileDownload) {
	commonmodel.SetSafeDownloadHeaders(w.Header(), payload.Filename, payload.ContentType)
	if status <= 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	// #nosec G705 -- writing binary attachment payload with Content-Disposition attachment and nosniff header
	_, _ = w.Write(payload.Content)
}

func writeSerializationError(w http.ResponseWriter, status int, err error, info string) {
	resp := common.NewErrorResponse(err, status, "AASENV", "SerializationAPI", info)
	if encodeErr := commonmodel.EncodeJSONResponse(resp.Body, &resp.Code, w); encodeErr != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func parseSerializationBoolQueryParam(value string) (bool, error) {
	if value == "" {
		return false, nil
	}

	return strconv.ParseBool(value)
}

func parseSerializationStringArrayQueryParam(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}
