/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.2.0
 * Contact: info@idtwin.org
 */

package openapi

import (
	"log/slog"
	"net/http"
	"time"
)

// Logger wraps an HTTP handler to log request details including method, URI, handler name, and duration.
// It logs the HTTP method, request URI, the name of the handler, and the time taken to process the request.
func Logger(inner http.Handler, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		inner.ServeHTTP(w, r)

		slog.InfoContext(
			r.Context(),
			"HTTP request completed",
			"http.request.method", r.Method,
			"url.path", r.URL.Path,
			"handler.name", name,
			"duration", time.Since(start),
		)
	})
}
