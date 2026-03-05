package common

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

// AddDefaultRouterErrorHandlers attaches standardized 404/405 responses to the router.
// The component name is used in correlation code generation.
func AddDefaultRouterErrorHandlers(r *chi.Mux, component string) {
	componentID := normalizeComponentID(component)

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeRouterError(w, component, componentID, http.StatusNotFound, "resource not found", "NOTFOUND")
	})

	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeRouterError(w, component, componentID, http.StatusMethodNotAllowed, "method not allowed", "METHODNOTALLOWED")
	})
}

func writeRouterError(w http.ResponseWriter, component, componentID string, status int, message, errorType string) {
	resp := NewErrorResponse(
		errors.New(message),
		status,
		component,
		"Router",
		fmt.Sprintf("%s-ROUTER-%s", componentID, errorType),
	)
	if err := model.EncodeJSONResponse(resp.Body, &resp.Code, w); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func normalizeComponentID(component string) string {
	var b strings.Builder
	b.Grow(len(component))

	for _, r := range component {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(unicode.ToUpper(r))
		}
	}

	if b.Len() == 0 {
		return "COMPONENT"
	}

	return b.String()
}
