package apis

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func (c *AssetAdministrationShellRegistryAPIAPIController) buildBaseLocation(r *http.Request) string {
	host := requestHost(r)
	if host == "" {
		return ""
	}

	basePath := strings.TrimSuffix(c.contextPath, "/")

	return requestScheme(r) + "://" + host + basePath
}

func parseForwardedHeaderValue(forwarded string, key string) string {
	parts := strings.Split(forwarded, ",")
	if len(parts) == 0 {
		return ""
	}

	for _, token := range strings.Split(parts[0], ";") {
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

func firstForwardedValue(value string) string {
	if value == "" {
		return ""
	}

	parts := strings.Split(value, ",")
	if len(parts) == 0 {
		return ""
	}

	return strings.TrimSpace(parts[0])
}

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

func requestHost(r *http.Request) string {
	if forwardedHost := parseForwardedHeaderValue(r.Header.Get("Forwarded"), "host"); forwardedHost != "" {
		return forwardedHost
	}

	if xForwardedHost := firstForwardedValue(r.Header.Get("X-Forwarded-Host")); xForwardedHost != "" {
		return xForwardedHost
	}

	return r.Host
}

func (c *AssetAdministrationShellRegistryAPIAPIController) buildShellDescriptorLocation(r *http.Request, aasID string) string {
	baseLocation := c.buildBaseLocation(r)
	if baseLocation == "" {
		return ""
	}

	return baseLocation + "/shell-descriptors/" + url.PathEscape(common.EncodeString(aasID))
}
