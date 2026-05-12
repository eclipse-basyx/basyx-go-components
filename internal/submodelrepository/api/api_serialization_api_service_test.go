package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateSerializationByIDsReturnsNotImplemented(t *testing.T) {
	t.Parallel()

	sut := NewSerializationAPIAPIService()
	response, err := sut.GenerateSerializationByIDs(contextWithABACDisabled(t), []string{"aas-1"}, []string{"sm-1"}, true)

	require.Error(t, err)
	require.Equal(t, http.StatusNotImplemented, response.Code)
	require.Equal(t, errGenerateSerializationByIDsNotImplemented, err.Error())
}
