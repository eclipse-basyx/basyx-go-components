package common

import (
	"context"
	"net/url"
	"strings"
)

// ExternalBaseURLFromContext resolves the configured external base URL from request context.
// Returns an empty string if no configuration is present or the configured URL is invalid.
func ExternalBaseURLFromContext(ctx context.Context) string {
	cfg, ok := ConfigFromContext(ctx)
	if !ok || cfg == nil {
		return ""
	}

	return NormalizePrimaryExternalBaseURL(cfg.General.ExternalURL)
}

// NormalizePrimaryExternalBaseURL parses and normalizes the first configured external base URL.
// A non-empty result always has http/https scheme, host, and no query/fragment.
func NormalizePrimaryExternalBaseURL(rawExternalURL string) string {
	trimmed := strings.TrimSpace(rawExternalURL)
	if trimmed == "" {
		return ""
	}

	firstEntry, _, _ := strings.Cut(trimmed, ",")
	entry := strings.TrimSpace(firstEntry)
	if entry == "" {
		return ""
	}

	parsed, err := url.Parse(entry)
	if err != nil {
		return ""
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return ""
	}
	if strings.TrimSpace(parsed.Host) == "" || parsed.User != nil {
		return ""
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}

	parsed.Scheme = scheme
	parsed.Host = strings.TrimSpace(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return strings.TrimRight(parsed.String(), "/")
}
