package common

import (
	"fmt"
	"net/url"
	"strings"
)

// BuildPostgresDSN creates a PostgreSQL DSN with URL-encoded credentials.
func BuildPostgresDSN(cfg PostgresConfig) string {
	postgresURL := &url.URL{
		Scheme:   "postgres",
		Host:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:     cfg.DBName,
		RawQuery: "sslmode=disable",
	}
	postgresURL.User = url.UserPassword(cfg.User, cfg.Password)

	return postgresURL.String()
}

// NormalizePostgresDSN ensures URL-based PostgreSQL DSNs are encoded correctly.
func NormalizePostgresDSN(dsn string) string {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return trimmed
	}

	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "postgres://") && !strings.HasPrefix(lower, "postgresql://") {
		return trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.User == nil {
		return trimmed
	}

	username := parsed.User.Username()
	password, hasPassword := parsed.User.Password()
	if hasPassword {
		parsed.User = url.UserPassword(username, password)
	} else {
		parsed.User = url.User(username)
	}

	return parsed.String()
}
