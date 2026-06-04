package openapi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseTimeAcceptsISO8601TimezoneOffset(t *testing.T) {
	parsed, err := parseTime("2026-05-28T14:30:00+02:00")

	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 5, 28, 12, 30, 0, 0, time.UTC), parsed.UTC())
}

func TestParseTimeAcceptsQueryDecodedPositiveTimezoneOffset(t *testing.T) {
	parsed, err := parseTime("2026-05-28T14:30:00 02:00")

	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 5, 28, 12, 30, 0, 0, time.UTC), parsed.UTC())
}

func TestParseTimeAcceptsRFC3339Nano(t *testing.T) {
	parsed, err := parseTime("2026-05-28T12:30:00.123456789Z")

	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 5, 28, 12, 30, 0, 123456789, time.UTC), parsed.UTC())
}

func TestParseTimeAcceptsUTCSuffix(t *testing.T) {
	parsed, err := parseTime("2026-05-28T12:30:00.123456789 UTC")

	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 5, 28, 12, 30, 0, 123456789, time.UTC), parsed.UTC())
}
