package auth

import (
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// joinBasePath prefixes a route pattern with the configured base path.
// It preserves wildcards like "*" or "/*".
func joinBasePath(basePath, route string) string {
	base := common.NormalizeBasePath(basePath)
	if base == "/" {
		return ensureLeadingSlash(route)
	}
	if route == "" || route == "/" {
		return base
	}
	if route == "*" || route == "/*" {
		return base + "/*"
	}

	trimmed := strings.TrimPrefix(route, "/")
	return base + "/" + trimmed
}

// stripBasePath removes the configured base path from a request path so
// the router can match against its unmounted patterns.
func stripBasePath(basePath, reqPath string) string {
	base := common.NormalizeBasePath(basePath)
	if base == "/" {
		return ensureLeadingSlash(reqPath)
	}
	if strings.HasPrefix(reqPath, base) {
		trimmed := strings.TrimPrefix(reqPath, base)
		if trimmed == "" {
			return "/"
		}
		return ensureLeadingSlash(trimmed)
	}
	return ensureLeadingSlash(reqPath)
}

func ensureLeadingSlash(p string) string {
	if p == "" {
		return "/"
	}
	if strings.HasPrefix(p, "/") || p == "*" {
		return p
	}
	return "/" + p
}
