/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// Response return a ImplResponse struct filled
func Response(code int, body interface{}) ImplResponse {
	return ImplResponse{
		Code: code,
		Body: body,
	}
}

// encodeIdentifierForPath encodes an identifier as base64url without padding for path usage.
func encodeIdentifierForPath(identifier string) string {
	if identifier == "" {
		return ""
	}

	return base64.RawURLEncoding.EncodeToString([]byte(identifier))
}

// parseForwardedHeaderValue extracts a key value from the first RFC7239 Forwarded entry.
func parseForwardedHeaderValue(forwarded string, key string) string {
	firstEntry, _, _ := strings.Cut(forwarded, ",")

	for _, token := range strings.Split(firstEntry, ";") {
		pair := strings.SplitN(strings.TrimSpace(token), "=", 2)
		if len(pair) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(pair[0]), key) {
			return strings.Trim(strings.TrimSpace(pair[1]), "\"")
		}
	}

	return ""
}

// firstForwardedValue returns the first comma-separated forwarded header value trimmed.
func firstForwardedValue(value string) string {
	if value == "" {
		return ""
	}

	first, _, _ := strings.Cut(value, ",")

	return strings.TrimSpace(first)
}

// requestScheme resolves the external scheme using forwarded headers with fallback to request TLS.
func requestScheme(r *http.Request) string {
	if forwardedProto := parseForwardedHeaderValue(r.Header.Get("Forwarded"), "proto"); forwardedProto != "" {
		return forwardedProto
	}

	if xForwardedProto := firstForwardedValue(r.Header.Get("X-Forwarded-Proto")); xForwardedProto != "" {
		return xForwardedProto
	}

	if r.TLS != nil {
		return "https"
	}

	return "http"
}

// requestHost resolves the external host using forwarded headers with fallback to request host.
func requestHost(r *http.Request) string {
	if forwardedHost := parseForwardedHeaderValue(r.Header.Get("Forwarded"), "host"); forwardedHost != "" {
		return forwardedHost
	}

	if xForwardedHost := firstForwardedValue(r.Header.Get("X-Forwarded-Host")); xForwardedHost != "" {
		return xForwardedHost
	}

	return r.Host
}

func normalizeContextPathForBaseLocation(contextPath string) string {
	trimmed := strings.TrimSpace(contextPath)
	if trimmed == "" || trimmed == "/" {
		return ""
	}

	return "/" + strings.Trim(trimmed, "/")
}

// buildBaseLocation builds an absolute base URL from scheme, host, and configured context path.
func (c *SubmodelRepositoryAPIAPIController) buildBaseLocation(r *http.Request) string {
	if externalBaseURL := common.ExternalBaseURLFromContext(r.Context()); externalBaseURL != "" {
		return externalBaseURL
	}

	host := requestHost(r)
	if host == "" {
		return ""
	}

	basePath := normalizeContextPathForBaseLocation(c.contextPath)

	return requestScheme(r) + "://" + host + basePath
}

// buildSubmodelLocationFromEncodedIdentifier builds the absolute location URL for a submodel resource
// when the identifier already follows the encoded path contract.
func (c *SubmodelRepositoryAPIAPIController) buildSubmodelLocationFromEncodedIdentifier(r *http.Request, encodedSubmodelIdentifier string) string {
	baseLocation := c.buildBaseLocation(r)
	if baseLocation == "" {
		return ""
	}

	escapedSubmodelID := url.PathEscape(encodedSubmodelIdentifier)

	return baseLocation + "/submodels/" + escapedSubmodelID
}

// buildSubmodelLocationFromRawId builds the absolute location URL for a submodel resource
// from a raw identifier by applying base64url path encoding once.
func (c *SubmodelRepositoryAPIAPIController) buildSubmodelLocationFromRawId(r *http.Request, rawSubmodelID string) string {
	return c.buildSubmodelLocationFromEncodedIdentifier(r, encodeIdentifierForPath(rawSubmodelID))
}

// buildSubmodelElementLocationFromEncodedIdentifier builds the absolute location URL for a submodel element resource
// when the submodel identifier already follows the encoded path contract.
func (c *SubmodelRepositoryAPIAPIController) buildSubmodelElementLocationFromEncodedIdentifier(r *http.Request, encodedSubmodelIdentifier string, idShortPath string) string {
	baseLocation := c.buildBaseLocation(r)
	if baseLocation == "" {
		return ""
	}

	escapedSubmodelID := url.PathEscape(encodedSubmodelIdentifier)
	escapedIDShortPath := url.PathEscape(idShortPath)

	return baseLocation + "/submodels/" + escapedSubmodelID + "/submodel-elements/" + escapedIDShortPath
}

// joinIDShortPath concatenates parent and child idShort segments using dot notation.
func joinIDShortPath(parentPath string, childIDShort string) string {
	if parentPath == "" {
		return childIDShort
	}

	if childIDShort == "" {
		return parentPath
	}

	return parentPath + "." + childIDShort
}

// IsZeroValue checks if the val is the zero-ed value.
func IsZeroValue(val interface{}) bool {
	return val == nil || reflect.DeepEqual(val, reflect.Zero(reflect.TypeOf(val)).Interface())
}

// AssertRecurseInterfaceRequired recursively checks each struct in a slice against the callback.
// This method traverse nested slices in a preorder fashion.
func AssertRecurseInterfaceRequired[T any](obj interface{}, callback func(T) error) error {
	return AssertRecurseValueRequired(reflect.ValueOf(obj), callback)
}

// AssertRecurseValueRequired checks each struct in the nested slice against the callback.
// This method traverse nested slices in a preorder fashion. ErrTypeAssertionError is thrown if
// the underlying struct does not match type T.
func AssertRecurseValueRequired[T any](value reflect.Value, callback func(T) error) error {
	switch value.Kind() {
	// If it is a struct we check using callback
	case reflect.Struct:
		obj, ok := value.Interface().(T)
		if !ok {
			return ErrTypeAssertionError
		}

		if err := callback(obj); err != nil {
			return err
		}

	// If it is a slice we continue recursion
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			if err := AssertRecurseValueRequired(value.Index(i), callback); err != nil {
				return err
			}
		}
	}
	return nil
}
