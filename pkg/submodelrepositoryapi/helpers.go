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

// requestScheme resolves the external scheme using trusted proxy headers and request fallback.
func requestScheme(r *http.Request) string {
	return common.RequestScheme(r)
}

// requestHost resolves the external host using trusted proxy headers and request fallback.
func requestHost(r *http.Request) string {
	return common.RequestHost(r)
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
