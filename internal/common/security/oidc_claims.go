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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	canonicalClaimPrefix = "basyx."
	canonicalScopesClaim = canonicalClaimPrefix + "scopes"

	claimMappingModeList   = "list"
	claimMappingModeScalar = "scalar"
)

var defaultScopeClaimPointers = []string{"/scope", "/scp"}

type claimMapping struct {
	target  string
	mode    string
	sources []string
}

func normalizeScopeClaimPointers(pointers []string) ([]string, error) {
	if len(pointers) == 0 {
		return append([]string(nil), defaultScopeClaimPointers...), nil
	}

	return normalizeJSONPointers(pointers, "scope claim")
}

func normalizeClaimMappings(settings []OIDCClaimMappingSettings) ([]claimMapping, error) {
	mappings := make([]claimMapping, 0, len(settings))
	targets := make(map[string]struct{}, len(settings))
	for _, setting := range settings {
		target := strings.TrimSpace(setting.Target)
		if target == "" {
			return nil, fmt.Errorf("COMMON-OIDC-VALIDATECLAIMMAPPING claim mapping target must not be empty")
		}
		if strings.HasPrefix(target, canonicalClaimPrefix) || target == "scopes" {
			return nil, fmt.Errorf("COMMON-OIDC-VALIDATECLAIMMAPPING claim mapping target %q is reserved", target)
		}
		canonicalTarget := canonicalClaimPrefix + target
		if _, ok := targets[canonicalTarget]; ok {
			return nil, fmt.Errorf("COMMON-OIDC-VALIDATECLAIMMAPPING duplicate claim mapping target %q", target)
		}

		mode := strings.ToLower(strings.TrimSpace(setting.Mode))
		if mode != claimMappingModeList && mode != claimMappingModeScalar {
			return nil, fmt.Errorf("COMMON-OIDC-VALIDATECLAIMMAPPING claim mapping %q has unsupported mode %q", target, setting.Mode)
		}

		sources, err := normalizeJSONPointers(setting.Sources, "claim mapping source")
		if err != nil {
			return nil, err
		}
		if len(sources) == 0 {
			return nil, fmt.Errorf("COMMON-OIDC-VALIDATECLAIMMAPPING claim mapping %q requires at least one source", target)
		}

		targets[canonicalTarget] = struct{}{}
		mappings = append(mappings, claimMapping{
			target:  canonicalTarget,
			mode:    mode,
			sources: sources,
		})
	}
	return mappings, nil
}

func normalizeVerifiedClaims(claims Claims, scopeClaimPointers []string, mappings []claimMapping) error {
	for key := range claims {
		if strings.HasPrefix(key, canonicalClaimPrefix) {
			return fmt.Errorf("COMMON-OIDC-VALIDATECLAIMS token contains reserved claim %q", key)
		}
	}

	scopes, err := collectScopes(claims, scopeClaimPointers)
	if err != nil {
		return err
	}
	claims[canonicalScopesClaim] = scopes

	for _, mapping := range mappings {
		if err := applyClaimMapping(claims, mapping); err != nil {
			return err
		}
	}
	return nil
}

func applyClaimMapping(claims Claims, mapping claimMapping) error {
	switch mapping.mode {
	case claimMappingModeList:
		values, found, err := collectMappedList(claims, mapping.sources)
		if err != nil {
			return fmt.Errorf("COMMON-OIDC-NORMALIZECLAIMMAPPING normalize %q: %w", mapping.target, err)
		}
		if found {
			claims[mapping.target] = values
		}
	case claimMappingModeScalar:
		value, found, err := firstMappedScalar(claims, mapping.sources)
		if err != nil {
			return fmt.Errorf("COMMON-OIDC-NORMALIZECLAIMMAPPING normalize %q: %w", mapping.target, err)
		}
		if found {
			claims[mapping.target] = value
		}
	}
	return nil
}

func collectScopes(claims Claims, pointers []string) ([]string, error) {
	values := make([]string, 0)
	seen := make(map[string]struct{})
	for _, pointer := range pointers {
		raw, found, err := jsonPointerValue(claims, pointer)
		if err != nil {
			return nil, fmt.Errorf("COMMON-OIDC-READSCOPECLAIM read scope claim %q: %w", pointer, err)
		}
		if !found {
			continue
		}
		scopes, err := scopeValues(raw)
		if err != nil {
			return nil, fmt.Errorf("COMMON-OIDC-NORMALIZESCOPECLAIM normalize scope claim %q: %w", pointer, err)
		}
		appendUniqueStrings(&values, seen, scopes)
	}
	return values, nil
}

func scopeValues(raw any) ([]string, error) {
	switch value := raw.(type) {
	case string:
		return strings.Fields(value), nil
	case []string:
		return fieldsFromStrings(value), nil
	case []any:
		values := make([]string, 0, len(value))
		for _, item := range value {
			stringItem, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("scope array contains %T, want string", item)
			}
			values = append(values, strings.Fields(stringItem)...)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("scope claim has type %T, want string or string array", raw)
	}
}

func fieldsFromStrings(values []string) []string {
	fields := make([]string, 0, len(values))
	for _, value := range values {
		fields = append(fields, strings.Fields(value)...)
	}
	return fields
}

func collectMappedList(claims Claims, pointers []string) ([]string, bool, error) {
	values := make([]string, 0)
	seen := make(map[string]struct{})
	foundAny := false
	for _, pointer := range pointers {
		raw, found, err := jsonPointerValue(claims, pointer)
		if err != nil {
			return nil, false, err
		}
		if !found {
			continue
		}
		foundAny = true
		items, err := stringList(raw)
		if err != nil {
			return nil, false, fmt.Errorf("source %q: %w", pointer, err)
		}
		appendUniqueStrings(&values, seen, items)
	}
	return values, foundAny, nil
}

func firstMappedScalar(claims Claims, pointers []string) (any, bool, error) {
	for _, pointer := range pointers {
		raw, found, err := jsonPointerValue(claims, pointer)
		if err != nil {
			return nil, false, err
		}
		if !found {
			continue
		}
		value, err := scalarValue(raw)
		if err != nil {
			return nil, false, fmt.Errorf("source %q: %w", pointer, err)
		}
		return value, true, nil
	}
	return nil, false, nil
}

func stringList(raw any) ([]string, error) {
	switch value := raw.(type) {
	case string:
		return []string{value}, nil
	case []string:
		return value, nil
	case []any:
		values := make([]string, 0, len(value))
		for _, item := range value {
			stringItem, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("list contains %T, want string", item)
			}
			values = append(values, stringItem)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("claim has type %T, want string or string array", raw)
	}
}

func scalarValue(raw any) (any, error) {
	if values, ok := raw.([]any); ok {
		if len(values) != 1 {
			return nil, fmt.Errorf("scalar array has %d items, want exactly one", len(values))
		}
		return scalarValue(values[0])
	}
	if values, ok := raw.([]string); ok {
		if len(values) != 1 {
			return nil, fmt.Errorf("scalar array has %d items, want exactly one", len(values))
		}
		return values[0], nil
	}

	switch raw.(type) {
	case string, bool, json.Number, float64, float32, int, int32, int64:
		return raw, nil
	default:
		return nil, fmt.Errorf("claim has type %T, want primitive or single-item primitive array", raw)
	}
}

func appendUniqueStrings(values *[]string, seen map[string]struct{}, candidates []string) {
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		*values = append(*values, candidate)
	}
}

func normalizeJSONPointers(pointers []string, description string) ([]string, error) {
	normalized := make([]string, 0, len(pointers))
	seen := make(map[string]struct{}, len(pointers))
	for _, pointer := range pointers {
		if _, err := decodeJSONPointer(pointer); err != nil {
			return nil, fmt.Errorf("COMMON-OIDC-VALIDATEJSONPOINTER invalid %s %q: %w", description, pointer, err)
		}
		if _, ok := seen[pointer]; ok {
			continue
		}
		seen[pointer] = struct{}{}
		normalized = append(normalized, pointer)
	}
	return normalized, nil
}

func jsonPointerValue(root any, pointer string) (any, bool, error) {
	tokens, err := decodeJSONPointer(pointer)
	if err != nil {
		return nil, false, err
	}

	current := root
	for _, token := range tokens {
		switch value := current.(type) {
		case Claims:
			next, found := value[token]
			if !found {
				return nil, false, nil
			}
			current = next
		case map[string]any:
			next, found := value[token]
			if !found {
				return nil, false, nil
			}
			current = next
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(value) {
				return nil, false, nil
			}
			current = value[index]
		default:
			return nil, false, nil
		}
	}
	return current, true, nil
}

func decodeJSONPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("JSON pointer must start with '/'")
	}
	rawTokens := strings.Split(pointer[1:], "/")
	tokens := make([]string, 0, len(rawTokens))
	for _, rawToken := range rawTokens {
		var token strings.Builder
		for index := 0; index < len(rawToken); index++ {
			if rawToken[index] != '~' {
				_ = token.WriteByte(rawToken[index])
				continue
			}
			if index+1 >= len(rawToken) {
				return nil, fmt.Errorf("JSON pointer contains incomplete escape")
			}
			index++
			switch rawToken[index] {
			case '0':
				_ = token.WriteByte('~')
			case '1':
				_ = token.WriteByte('/')
			default:
				return nil, fmt.Errorf("JSON pointer contains unsupported escape ~%c", rawToken[index])
			}
		}
		tokens = append(tokens, token.String())
	}
	return tokens, nil
}

// hasAllScopes reports whether all required scopes are present in normalized OAuth claims.
func hasAllScopes(claims Claims, required []string) bool {
	scopes, ok := claims[canonicalScopesClaim].([]string)
	if !ok {
		var err error
		scopes, err = collectScopes(claims, defaultScopeClaimPointers)
		if err != nil {
			return false
		}
	}

	have := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		have[scope] = struct{}{}
	}
	for _, scope := range required {
		if _, ok := have[scope]; !ok {
			return false
		}
	}
	return true
}
