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
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
)

// ReBACIdentity is the bounded identity derived from an OIDC-validated context.
// Principal is nil when the request is anonymous and therefore has no owner.
type ReBACIdentity struct {
	Principal        *rebac.Principal
	ContextualTuples []rebac.Tuple
	Administrator    bool
	Issuer           string
	Subject          string
	Groups           []string
	ExpiresAt        time.Time
}

// ReBACIdentityFromContext derives ReBAC identity data exclusively from claims
// accepted by the OIDC middleware. Unauthenticated contexts represent an
// anonymous identity without an owner.
func ReBACIdentityFromContext(ctx context.Context, cfg common.ReBACConfig) (ReBACIdentity, error) {
	if !IsAuthenticated(ctx) {
		return ReBACIdentity{}, nil
	}

	claims := ClaimsFromContext(ctx)
	issuer, subject, err := rebacIssuerSubject(claims)
	if err != nil {
		return ReBACIdentity{}, err
	}
	expiresAt, err := rebacExpiry(claims, time.Now())
	if err != nil {
		return ReBACIdentity{}, err
	}
	groups, err := rebacGroups(claims, issuer, cfg.GroupClaimMappings)
	if err != nil {
		return ReBACIdentity{}, err
	}

	principal := rebac.Principal{Kind: "user", Issuer: issuer, ID: subject}
	tuples, err := rebac.MembershipTuples(principal, groups)
	if err != nil {
		return ReBACIdentity{}, fmt.Errorf("SECURITY-REBACIDENTITY-TUPLES build contextual tuples: %w", err)
	}

	return ReBACIdentity{
		Principal:        &principal,
		ContextualTuples: tuples,
		Administrator:    rebacAdministrator(issuer, subject, groups, cfg.Administrators),
		Issuer:           issuer,
		Subject:          subject,
		Groups:           groups,
		ExpiresAt:        expiresAt,
	}, nil
}

func rebacIssuerSubject(claims Claims) (string, string, error) {
	if claims == nil {
		return "", "", fmt.Errorf("SECURITY-REBACIDENTITY-CLAIMS authenticated context has no claims")
	}
	issuer, err := rebacStringClaim(claims, "iss")
	if err != nil {
		return "", "", fmt.Errorf("SECURITY-REBACIDENTITY-ISSUER %w", err)
	}
	subject, err := rebacStringClaim(claims, "sub")
	if err != nil {
		return "", "", fmt.Errorf("SECURITY-REBACIDENTITY-SUBJECT %w", err)
	}
	return issuer, subject, nil
}

func rebacStringClaim(claims Claims, name string) (string, error) {
	value, ok := claims[name].(string)
	if !ok || value == "" || value != strings.TrimSpace(value) {
		return "", fmt.Errorf("claim %q must be a non-empty string", name)
	}
	return value, nil
}

func rebacExpiry(claims Claims, now time.Time) (time.Time, error) {
	raw, ok := claims["exp"].(json.Number)
	if !ok {
		return time.Time{}, fmt.Errorf("SECURITY-REBACIDENTITY-EXPIRY claim \"exp\" must be a JSON number")
	}
	seconds, err := strconv.ParseFloat(raw.String(), 64)
	const maxUnixSeconds = float64(1<<63 - 1)
	if err != nil || math.IsNaN(seconds) || seconds > maxUnixSeconds || seconds < -maxUnixSeconds {
		return time.Time{}, fmt.Errorf("SECURITY-REBACIDENTITY-EXPIRY claim \"exp\" must be a valid Unix timestamp")
	}
	wholeSeconds := int64(seconds)
	nanoseconds := int64((seconds - float64(wholeSeconds)) * float64(time.Second))
	expiresAt := time.Unix(wholeSeconds, nanoseconds)
	if !expiresAt.After(now) {
		return time.Time{}, fmt.Errorf("SECURITY-REBACIDENTITY-EXPIRED validated token has expired")
	}
	return expiresAt, nil
}

func rebacGroups(claims Claims, issuer string, mappings []common.ReBACGroupClaimMappingConfig) ([]string, error) {
	for _, mapping := range mappings {
		if mapping.Issuer != issuer {
			continue
		}
		return rebacGroupClaim(claims, mapping.Claim)
	}
	return nil, nil
}

func rebacGroupClaim(claims Claims, claim string) ([]string, error) {
	raw, found := claims[claim]
	if !found {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("SECURITY-REBACIDENTITY-GROUPS claim %q must be a string array", claim)
	}
	groups := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, rawGroup := range values {
		group, ok := rawGroup.(string)
		if !ok || group == "" || group != strings.TrimSpace(group) {
			return nil, fmt.Errorf("SECURITY-REBACIDENTITY-GROUPS claim %q contains an invalid group", claim)
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	return groups, nil
}

func rebacAdministrator(issuer, subject string, groups []string, administrators []common.ReBACAdministratorConfig) bool {
	for _, administrator := range administrators {
		if administrator.Issuer != issuer {
			continue
		}
		if administrator.Type == "user" && administrator.Subject == subject {
			return true
		}
		if administrator.Type == "group" && containsReBACGroup(groups, administrator.Subject) {
			return true
		}
	}
	return false
}

func containsReBACGroup(groups []string, subject string) bool {
	for _, group := range groups {
		if group == subject {
			return true
		}
	}
	return false
}
