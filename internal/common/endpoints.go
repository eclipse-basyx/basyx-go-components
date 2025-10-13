package common

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func AddHealthEndpoint(r *chi.Mux, config *Config) {
	r.Get(config.Server.ContextPath+"/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{\"status\":\"UP\"}"))
	})
}
