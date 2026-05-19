package api

import (
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestGenerateSerializationByIDsReturnsNotImplemented(t *testing.T) {
	t.Parallel()

	sut := NewSerializationAPIAPIService()
	response, err := sut.GenerateSerializationByIDs(contextWithABACDisabled(t), []string{"aas-1"}, []string{"sm-1"}, true)

	require.NoError(t, err)
	require.Equal(t, http.StatusNotImplemented, response.Code)
	require.IsType(t, []common.ErrorHandler{}, response.Body)

	messages := response.Body.([]common.ErrorHandler)
	require.Len(t, messages, 1)
	require.Equal(t, errGenerateSerializationByIDsNotImplemented, messages[0].Text)
}
