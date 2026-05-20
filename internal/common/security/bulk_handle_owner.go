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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

var ownerClaimKeys = []string{"iss", "sub", "azp", "client_id", "Edc-Bpn"}

// OwnerKeyFromContext builds a stable owner key from available JWT claims.
func OwnerKeyFromContext(ctx context.Context) string {
	return OwnerKeyFromClaims(ClaimsFromContext(ctx))
}

// OwnerKeyFromClaims builds a stable owner key from relevant claim fields.
func OwnerKeyFromClaims(claims Claims) string {
	if len(claims) == 0 {
		return "anonymous"
	}

	parts := make([]string, 0, len(ownerClaimKeys))
	for _, claimKey := range ownerClaimKeys {
		rawValue, found := claims[claimKey]
		if !found {
			continue
		}

		value := stringifyOwnerClaim(rawValue)
		if value == "" {
			continue
		}

		parts = append(parts, fmt.Sprintf("%s=%s", claimKey, value))
	}

	if len(parts) == 0 {
		return "anonymous"
	}

	return strings.Join(parts, "|")
}

func stringifyOwnerClaim(value any) string {
	switch castValue := value.(type) {
	case string:
		return strings.TrimSpace(castValue)
	case json.Number:
		return castValue.String()
	case fmt.Stringer:
		return strings.TrimSpace(castValue.String())
	case []string:
		if len(castValue) == 0 {
			return ""
		}
		return strings.Join(castValue, ",")
	case []any:
		if len(castValue) == 0 {
			return ""
		}
		parts := make([]string, 0, len(castValue))
		for _, entry := range castValue {
			parsedEntry := stringifyOwnerClaim(entry)
			if parsedEntry == "" {
				continue
			}
			parts = append(parts, parsedEntry)
		}
		return strings.Join(parts, ",")
	default:
		return strings.TrimSpace(fmt.Sprint(castValue))
	}
}
