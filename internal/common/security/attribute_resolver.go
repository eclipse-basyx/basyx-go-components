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
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func globalAttributesForEvaluation(configured GlobalAttributes, claims Claims, currentTime time.Time) GlobalAttributes {
	globals := make(GlobalAttributes, len(configured)+3)
	for name, value := range configured {
		globals[name] = value
	}

	if _, exists := globals["UTCNOW"]; !exists {
		globals["UTCNOW"] = currentTime.UTC().Format(time.RFC3339)
	}
	if _, exists := globals["LOCALNOW"]; !exists {
		globals["LOCALNOW"] = currentTime.In(time.Local).Format(time.RFC3339)
	}
	delete(globals, "CLIENTNOW")
	if clientNow, exists := claims["CLIENTNOW"]; exists {
		globals["CLIENTNOW"] = normalizeClaimScalar(clientNow)
	}

	return globals
}

func resolveGlobalToken(name string, globals GlobalAttributes) (any, bool) {
	normalizedName := strings.ToUpper(name)
	switch normalizedName {
	case "UTCNOW", "LOCALNOW", "CLIENTNOW":
		if val, ok := globals[normalizedName]; ok {
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
func resolveAttributeValue(attr grammar.AttributeValue, claims Claims, globals GlobalAttributes) any {
	m, ok := asStringMap(attr)
	if !ok {
		return nil
	}
	if c := m["CLAIM"]; c != "" {
		val, exists := claims[c]
		if !exists {
			return nil
		}
		normalized, ok := normalizeClaimValue(val)
		if !ok {
			return nil
		}
		return normalized
	}
	if pointer := m["CLAIMPATH"]; pointer != "" {
		val, exists, err := jsonPointerValue(claims, pointer)
		if err != nil || !exists {
			return nil
		}
		normalized, ok := normalizeClaimValue(val)
		if !ok {
			return nil
		}
		return normalized
	}
	if g := m["GLOBAL"]; g != "" {
		if val, ok := resolveGlobalToken(g, globals); ok {
			return val
		}
	}
	return nil
}

func normalizeClaimValue(value any) (any, bool) {
	switch claim := value.(type) {
	case nil:
		return nil, false
	case string:
		return claim, true
	case json.Number:
		return claim.String(), true
	case bool:
		return strconv.FormatBool(claim), true
	case float64:
		return strconv.FormatFloat(claim, 'g', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(claim), 'g', -1, 32), true
	case int:
		return strconv.Itoa(claim), true
	case int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(claim), true
	case []string:
		if len(claim) == 0 {
			return nil, false
		}
		return claim, true
	case []any:
		values := make([]string, len(claim))
		for index, item := range claim {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			values[index] = text
		}
		if len(values) == 0 {
			return nil, false
		}
		return values, true
	default:
		return nil, false
	}
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
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, false
		}
		out := make(map[string]string, len(m))
		for k, val := range m {
			out[k] = fmt.Sprint(val)
		}
		return out, true
	}
}
