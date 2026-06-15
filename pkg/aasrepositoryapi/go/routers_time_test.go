/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

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
