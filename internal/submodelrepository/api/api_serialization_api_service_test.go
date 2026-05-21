package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateSerializationByIdsReturnsNotImplemented(t *testing.T) {
	t.Parallel()

	sut := NewSerializationAPIAPIService()
	response, err := sut.GenerateSerializationByIds(contextWithABACDisabled(t), []string{"aas-1"}, []string{"sm-1"}, true)

	require.NoError(t, err)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotImplemented, response.Code)
}
