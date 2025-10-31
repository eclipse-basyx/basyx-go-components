package common

import (
	"encoding/json"
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

// isArrayNotEmpty checks if a JSON array contains data.
//
// This utility function determines whether a JSON RawMessage contains an actual
// array with data, as opposed to being empty or containing a null value.
//
// Parameters:
//   - data: JSON RawMessage to check
//
// Returns:
//   - bool: true if the data is not empty and not "null", false otherwise
func IsArrayNotEmpty(data json.RawMessage) bool {
	return len(data) > 0 && string(data) != "null"
}
