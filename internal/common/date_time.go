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

package common

import (
	"strings"
	"time"
)

var iso8601DateTimeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05Z0700",
	"2006-01-02T15:04:05.999999999Z0700",
}

// ParseISO8601DateTime parses API date-time parameters including RFC3339Nano
// values and ISO 8601 offsets without a colon.
func ParseISO8601DateTime(param string) (time.Time, error) {
	trimmed := strings.TrimSpace(param)
	if trimmed == "" {
		return time.Time{}, nil
	}

	for _, candidate := range iso8601DateTimeCandidates(trimmed) {
		for _, layout := range iso8601DateTimeLayouts {
			parsed, err := time.Parse(layout, candidate)
			if err == nil {
				return parsed, nil
			}
		}
	}

	return time.Parse(time.RFC3339Nano, trimmed)
}

func iso8601DateTimeCandidates(value string) []string {
	candidates := []string{value}
	if utcValue, ok := normalizeUTCSuffix(value); ok {
		candidates = append(candidates, utcValue)
	}
	decodedOffset, ok := restoreDecodedQueryPlusOffset(value)
	if ok {
		candidates = append(candidates, decodedOffset)
	}
	return candidates
}

func normalizeUTCSuffix(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	upper := strings.ToUpper(trimmed)
	if !strings.HasSuffix(upper, "UTC") {
		return "", false
	}
	prefix := strings.TrimSpace(trimmed[:len(trimmed)-len("UTC")])
	if !strings.Contains(prefix, "T") {
		return "", false
	}
	return prefix + "Z", true
}

func restoreDecodedQueryPlusOffset(value string) (string, bool) {
	offsetStart := strings.LastIndex(value, " ")
	if offsetStart < 0 || !strings.Contains(value[:offsetStart], "T") {
		return "", false
	}
	offset := value[offsetStart+1:]
	if !isPositiveOffsetSuffix(offset) {
		return "", false
	}
	return value[:offsetStart] + "+" + offset, true
}

func isPositiveOffsetSuffix(value string) bool {
	switch len(value) {
	case len("02:00"):
		return isTwoDigits(value[0:2]) && value[2] == ':' && isTwoDigits(value[3:5])
	case len("0200"):
		return isTwoDigits(value[0:2]) && isTwoDigits(value[2:4])
	default:
		return false
	}
}

func isTwoDigits(value string) bool {
	return len(value) == 2 &&
		value[0] >= '0' && value[0] <= '9' &&
		value[1] >= '0' && value[1] <= '9'
}
