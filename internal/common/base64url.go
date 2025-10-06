package common

import (
	"encoding/base64"
	"strconv"
	"strings"
)

// Encode takes a byte slice and returns a base64 URL-encoded string
// This encoding is URL and filename safe as specified in RFC 4648
func Encode(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	// Replace standard Base64 characters with URL-safe variants
	encoded = strings.ReplaceAll(encoded, "+", "-")
	encoded = strings.ReplaceAll(encoded, "/", "_")
	// Remove padding characters
	encoded = strings.TrimRight(encoded, "=")
	return encoded
}

// Decode takes a base64 URL-encoded string and returns the decoded bytes
// Returns an error if the input is not properly encoded
func Decode(encoded string) ([]byte, error) {
	// Restore the standard Base64 format
	standardB64 := strings.ReplaceAll(encoded, "-", "+")
	standardB64 = strings.ReplaceAll(standardB64, "_", "/")

	// Add padding if needed
	switch len(standardB64) % 4 {
	case 2:
		standardB64 += "=="
	case 3:
		standardB64 += "="
	}

	// Decode
	return base64.StdEncoding.DecodeString(standardB64)
}

// EncodeString is a convenience function that takes a string,
// converts it to bytes, and returns a base64 URL-encoded string
func EncodeString(data string) string {
	return Encode([]byte(data))
}

// DecodeString is a convenience function that decodes a base64 URL-encoded
// string and returns the decoded string
// Returns an error if the input is not properly encoded
func DecodeString(encoded string) (string, error) {
	bytes, err := Decode(encoded)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func ParseCursorToID(c string) (int64, error) {
	c = strings.TrimSpace(c)
	if c == "" {
		return 0, nil
	}

	return strconv.ParseInt(string(c), 10, 64)
}
