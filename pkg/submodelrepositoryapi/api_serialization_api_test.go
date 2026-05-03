package openapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

type serializationServiceStub struct {
	response model.ImplResponse
	err      error
}

func (s serializationServiceStub) GenerateSerializationByIDs(_ context.Context, _ []string, _ []string, _ bool) (model.ImplResponse, error) {
	return s.response, s.err
}

func TestSerializationRoutesIncludeContextPath(t *testing.T) {
	t.Parallel()

	ctrl := NewSerializationAPIAPIController(serializationServiceStub{}, "/api/v3")
	routes := ctrl.Routes()

	require.Equal(t, "/api/v3/serialization", routes["GenerateSerializationByIDs"].Pattern)
}

func TestGenerateSerializationByIDsReturnsNotImplemented(t *testing.T) {
	t.Parallel()

	ctrl := NewSerializationAPIAPIController(serializationServiceStub{
		response: model.Response(http.StatusNotImplemented, nil),
		err:      errors.New("not-implemented"),
	}, "")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/serialization", nil)

	ctrl.GenerateSerializationByIDs(rr, req)

	require.Equal(t, http.StatusNotImplemented, rr.Code)
}
