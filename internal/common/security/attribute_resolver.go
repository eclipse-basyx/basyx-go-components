/********************************************************************************
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
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	jsoniter "github.com/json-iterator/go"
)

func resolveGlobalToken(name string, claims Claims) (any, bool) {
	switch strings.ToUpper(name) {
	case "UTCNOW":
		if val, ok := claims["UTCNOW"]; ok {
			return normalizeClaimScalar(val), true
		}
		return "", false
	case "LOCALNOW":
		if val, ok := claims["LOCALNOW"]; ok {
			return normalizeClaimScalar(val), true
		}
		return "", false
	case "CLIENTNOW":
		if val, ok := claims["CLIENTNOW"]; ok {
			return normalizeClaimScalar(val), true
		}
		return "", false
	case "ANONYMOUS":
		return "ANONYMOUS", true
	default:
		return "", false
	}
}

// resolveAttributeValue resolves a grammar.AttributeValue to a concrete literal using claims/globals.
// It also normalizes common claim container shapes (e.g., single-element arrays from Keycloak).
func resolveAttributeValue(attr grammar.AttributeValue, claims Claims) any {
	m, ok := asStringMap(attr)
	if !ok {
		return nil
	}
	if c := m["CLAIM"]; c != "" {
		return fmt.Sprint(normalizeClaimScalar(claims[c]))
	}
	if g := m["GLOBAL"]; g != "" {
		if val, ok := resolveGlobalToken(g, claims); ok {
			return val
		}
	}
	return nil
}

// normalizeClaimScalar unwraps common container formats so operators see a scalar.
func normalizeClaimScalar(v any) any {
	switch val := v.(type) {
	case []any:
		if len(val) == 0 {
			return ""
		}
		return normalizeClaimScalar(val[0])
	case []string:
		if len(val) == 0 {
			return ""
		}
		return val[0]
	default:
		return v
	}
}

// asStringMap attempts to normalize arbitrary map-like values into a map[string]string.
func asStringMap(v any) (map[string]string, bool) {
	switch vv := v.(type) {
	case map[string]string:
		return vv, true
	case map[string]any:
		out := make(map[string]string, len(vv))
		for k, val := range vv {
			out[k] = fmt.Sprint(val)
		}
		return out, true
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var m map[string]any
		var jsonMarshaller = jsoniter.ConfigCompatibleWithStandardLibrary
		if err := jsonMarshaller.Unmarshal(b, &m); err != nil {
			return nil, false
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			out[k] = fmt.Sprint(val)
		}
		return out, true
	}
}
