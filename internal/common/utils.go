package common

import (
	"strings"
	"time"
)

func GetCurrentTimestamp() string {
	timestamp := time.Now().Format(time.RFC3339)
	return timestamp
}

func NormalizeBasePath(p string) string {
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}
