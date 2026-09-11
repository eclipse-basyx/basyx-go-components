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

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const boundGroupClaimPrefix = "basyx.rebac.group."

func (repo *resourceBoundRepository) boundRequestPrincipals(ctx context.Context) []common.AccessPrincipal {
	return repo.boundPrincipals(ClaimsFromContext(ctx))
}

func (repo *resourceBoundRepository) boundPrincipals(claims Claims) []common.AccessPrincipal {
	issuer, issuerOK := claims.GetString("iss")
	subject, subjectOK := claims.GetString("sub")
	if !issuerOK || !subjectOK || strings.TrimSpace(issuer) == "" || strings.TrimSpace(subject) == "" {
		return nil
	}
	principals := []common.AccessPrincipal{{Type: common.AccessPrincipalUser, Issuer: issuer, Subject: subject}}
	seen := make(map[string]struct{})
	for _, group := range boundGroupValues(claims[repo.groupsClaim]) {
		if strings.TrimSpace(group) == "" {
			continue
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		principals = append(principals, common.AccessPrincipal{Type: common.AccessPrincipalGroup, Issuer: issuer, Subject: group})
	}
	return principals
}

func boundGroupValues(value any) []string {
	switch groups := value.(type) {
	case string:
		return []string{groups}
	case []string:
		return groups
	case []any:
		result := make([]string, 0, len(groups))
		for _, value := range groups {
			if group, ok := value.(string); ok {
				result = append(result, group)
			}
		}
		return result
	default:
		return nil
	}
}

func withBoundGroupClaims(claims Claims, principals []common.AccessPrincipal) Claims {
	result := make(Claims, len(claims)+len(principals))
	for name, value := range claims {
		result[name] = value
	}
	for _, principal := range principals {
		if principal.NormalizedType() == common.AccessPrincipalGroup {
			result[boundGroupClaimName(principal)] = "true"
		}
	}
	return result
}

func boundGroupClaimName(principal common.AccessPrincipal) string {
	digest := sha256.Sum256([]byte(principal.Issuer + "\x00" + principal.Subject))
	return boundGroupClaimPrefix + hex.EncodeToString(digest[:])
}
