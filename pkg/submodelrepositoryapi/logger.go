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
	"net/http"
)

// Logger preserves the generated middleware hook for compatibility.
//
// Access logging is configured by the hosting application.
func Logger(inner http.Handler, _ string) http.Handler {
	return inner
}
