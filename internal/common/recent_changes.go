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

	"github.com/FriedJannik/aas-go-sdk/types"
)

const (
	// DefaultRecentChangesLimit is the recent-change page size used when no limit is provided.
	DefaultRecentChangesLimit int32 = 100
)

// NormalizeRecentChangesLimit applies the public recent-change endpoint limit contract.
func NormalizeRecentChangesLimit(limit int32) (int32, error) {
	if limit <= 0 {
		return DefaultRecentChangesLimit, nil
	}
	return limit, nil
}

// RecentChangeTimestamps extracts the data-bound timestamps used by recent-change responses.
func RecentChangeTimestamps(administration types.IAdministrativeInformation) (string, string, bool) {
	if administration == nil {
		return "", "", false
	}

	createdAt, ok := validAdministrationTimestamp(administration.CreatedAt())
	if !ok {
		return "", "", false
	}

	updatedAt, ok := validAdministrationTimestamp(administration.UpdatedAt())
	if !ok {
		return "", "", false
	}

	return createdAt, updatedAt, true
}

func validAdministrationTimestamp(value *string) (string, bool) {
	if value == nil {
		return "", false
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return "", false
	}
	if _, err := ParseISO8601DateTime(trimmed); err != nil {
		return "", false
	}
	return trimmed, true
}
