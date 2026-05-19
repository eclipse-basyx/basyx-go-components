package aasregistryapi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestAASBulkCreateErrorStatusCodeMappings(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.StatusBadRequest, aasBulkCreateErrorStatusCode(common.NewErrBadRequest("bad request")))
	require.Equal(t, http.StatusConflict, aasBulkCreateErrorStatusCode(common.NewErrConflict("conflict")))
	require.Equal(t, http.StatusForbidden, aasBulkCreateErrorStatusCode(common.NewErrDenied("denied")))
	require.Equal(t, http.StatusNotFound, aasBulkCreateErrorStatusCode(common.NewErrNotFound("missing")))
	require.Equal(t, http.StatusInternalServerError, aasBulkCreateErrorStatusCode(errors.New("unknown")))
}

func TestAASBulkPutErrorStatusCodeMappings(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.StatusBadRequest, aasBulkPutErrorStatusCode(common.NewErrBadRequest("bad request")))
	require.Equal(t, http.StatusConflict, aasBulkPutErrorStatusCode(common.NewErrConflict("conflict")))
	require.Equal(t, http.StatusForbidden, aasBulkPutErrorStatusCode(common.NewErrDenied("denied")))
	require.Equal(t, http.StatusNotFound, aasBulkPutErrorStatusCode(common.NewErrNotFound("missing")))
	require.Equal(t, http.StatusInternalServerError, aasBulkPutErrorStatusCode(errors.New("unknown")))
}

func TestAASBulkDeleteErrorStatusCodeMappings(t *testing.T) {
	t.Parallel()

	require.Equal(t, http.StatusBadRequest, aasBulkDeleteErrorStatusCode(common.NewErrBadRequest("bad request")))
	require.Equal(t, http.StatusForbidden, aasBulkDeleteErrorStatusCode(common.NewErrDenied("denied")))
	require.Equal(t, http.StatusNotFound, aasBulkDeleteErrorStatusCode(common.NewErrNotFound("missing")))
	require.Equal(t, http.StatusInternalServerError, aasBulkDeleteErrorStatusCode(errors.New("unknown")))
}
