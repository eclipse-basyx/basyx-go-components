/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	acm "github.com/eclipse-basyx/basyx-go-components/internal/common/security/model"
)

func resolveGlobalToken(name string, now time.Time) (string, bool) {
	switch strings.ToUpper(name) {
	case "UTCNOW":
		return now.UTC().Format(time.RFC3339), true
	case "LOCALNOW":
		return now.In(time.Local).Format(time.RFC3339), true
	case "CLIENTNOW":
		return now.Format(time.RFC3339), true
	case "ANONYMOUS":
		return "ANONYMOUS", true
	default:
		return "", false
	}
}

func evalLE(le acm.LogicalExpression, claims Claims, now time.Time) bool {
	if le.Boolean != nil {
		return *le.Boolean
	}

	if len(le.Gt) == 2 {
		return numCmp(resolveValue(le.Gt[0], claims, now), resolveValue(le.Gt[1], claims, now), "gt")
	}
	if len(le.Ge) == 2 {
		return numCmp(resolveValue(le.Ge[0], claims, now), resolveValue(le.Ge[1], claims, now), "ge")
	}
	if len(le.Lt) == 2 {
		return numCmp(resolveValue(le.Lt[0], claims, now), resolveValue(le.Lt[1], claims, now), "lt")
	}
	if len(le.Le) == 2 {
		return numCmp(resolveValue(le.Le[0], claims, now), resolveValue(le.Le[1], claims, now), "le")
	}

	if len(le.Eq) == 2 {
		return fmt.Sprint(resolveValue(le.Eq[0], claims, now)) == fmt.Sprint(resolveValue(le.Eq[1], claims, now))
	}
	if len(le.Ne) == 2 {
		return fmt.Sprint(resolveValue(le.Ne[0], claims, now)) != fmt.Sprint(resolveValue(le.Ne[1], claims, now))
	}

	if len(le.Regex) == 2 {
		hay := asString(resolveStringItem(le.Regex[0], claims, now))
		pat := asString(resolveStringItem(le.Regex[1], claims, now))
		re, err := regexp.Compile(pat)
		if err != nil {
			return false
		}
		return re.MatchString(hay)
	}
	if len(le.Contains) == 2 {
		hay := asString(resolveStringItem(le.Contains[0], claims, now))
		needle := asString(resolveStringItem(le.Contains[1], claims, now))
		return strings.Contains(hay, needle)
	}
	if len(le.StartsWith) == 2 {
		hay := asString(resolveStringItem(le.StartsWith[0], claims, now))
		prefix := asString(resolveStringItem(le.StartsWith[1], claims, now))
		return strings.HasPrefix(hay, prefix)
	}
	if len(le.EndsWith) == 2 {
		hay := asString(resolveStringItem(le.EndsWith[0], claims, now))
		suffix := asString(resolveStringItem(le.EndsWith[1], claims, now))
		return strings.HasSuffix(hay, suffix)
	}

	if len(le.And) >= 2 {
		for _, sub := range le.And {
			if !evalLE(sub, claims, now) {
				return false
			}
		}
		return true
	}
	if len(le.Or) >= 2 {
		for _, sub := range le.Or {
			if evalLE(sub, claims, now) {
				return true
			}
		}
		return false
	}
	if le.Not != nil {
		return !evalLE(*le.Not, claims, now)
	}

	if len(le.Match) > 0 {
		for _, m := range le.Match {
			if !evalMatch(m, claims, now) {
				return false
			}
		}
		return true
	}
	return false
}

func evalMatch(me acm.MatchExpression, claims Claims, now time.Time) bool {
	if me.Boolean != nil {
		return *me.Boolean
	}
	if len(me.Gt) == 2 {
		return numCmp(resolveValue(me.Gt[0], claims, now), resolveValue(me.Gt[1], claims, now), "gt")
	}
	if len(me.Ge) == 2 {
		return numCmp(resolveValue(me.Ge[0], claims, now), resolveValue(me.Ge[1], claims, now), "ge")
	}
	if len(me.Lt) == 2 {
		return numCmp(resolveValue(me.Lt[0], claims, now), resolveValue(me.Lt[1], claims, now), "lt")
	}
	if len(me.Le) == 2 {
		return numCmp(resolveValue(me.Le[0], claims, now), resolveValue(me.Le[1], claims, now), "le")
	}
	if len(me.Eq) == 2 {
		return fmt.Sprint(resolveValue(me.Eq[0], claims, now)) == fmt.Sprint(resolveValue(me.Eq[1], claims, now))
	}
	if len(me.Ne) == 2 {
		return fmt.Sprint(resolveValue(me.Ne[0], claims, now)) != fmt.Sprint(resolveValue(me.Ne[1], claims, now))
	}
	if len(me.Regex) == 2 {
		hay := asString(resolveStringItem(me.Regex[0], claims, now))
		pat := asString(resolveStringItem(me.Regex[1], claims, now))
		re, err := regexp.Compile(pat)
		if err != nil {
			return false
		}
		return re.MatchString(hay)
	}
	if len(me.Contains) == 2 {
		hay := asString(resolveStringItem(me.Contains[0], claims, now))
		needle := asString(resolveStringItem(me.Contains[1], claims, now))
		return strings.Contains(hay, needle)
	}
	if len(me.StartsWith) == 2 {
		hay := asString(resolveStringItem(me.StartsWith[0], claims, now))
		prefix := asString(resolveStringItem(me.StartsWith[1], claims, now))
		return strings.HasPrefix(hay, prefix)
	}
	if len(me.EndsWith) == 2 {
		hay := asString(resolveStringItem(me.EndsWith[0], claims, now))
		suffix := asString(resolveStringItem(me.EndsWith[1], claims, now))
		return strings.HasSuffix(hay, suffix)
	}
	if len(me.Match) > 0 {
		for _, sub := range me.Match {
			if !evalMatch(sub, claims, now) {
				return false
			}
		}
		return true
	}
	return false
}

func resolveValue(v acm.Value, claims Claims, now time.Time) any {

	if v.Attribute != nil {
		if m, ok := asStringMap(v.Attribute); ok {
			if c := m["CLAIM"]; c != "" {
				return claims[c]
			}
			if g := m["GLOBAL"]; g != "" {
				if val, ok := resolveGlobalToken(g, now); ok {
					return val
				}
			}
		}
	}

	if v.StrVal != nil {
		return string(*v.StrVal)
	}
	if v.NumVal != nil {
		return *v.NumVal
	}
	if v.Boolean != nil {
		return *v.Boolean
	}
	if v.DateTimeVal != nil || v.TimeVal != nil || v.Year != nil || v.Month != nil || v.DayOfMonth != nil || v.DayOfWeek != nil {
		return stringValueFromDate(v)
	}
	if v.HexVal != nil {
		return string(*v.HexVal)
	}

	if v.Field != nil {
		return ""
	}

	if v.StrCast != nil {
		return fmt.Sprint(resolveValue(*v.StrCast, claims, now))
	}
	if v.NumCast != nil {
		x := resolveValue(*v.NumCast, claims, now)
		if f, ok := toFloat(x); ok {
			return f
		}
		return x
	}
	if v.BoolCast != nil {
		return castToBool(resolveValue(*v.BoolCast, claims, now))
	}

	if v.TimeCast != nil {
		inner := resolveValue(*v.TimeCast, claims, now)

		if t, ok := toDateTime(inner); ok {
			return t.Format("15:04:05")
		}

		if _, ok := toTimeOfDaySeconds(inner); ok {
			return fmt.Sprint(inner)
		}

		return fmt.Sprint(inner)
	}
	if v.DateTimeCast != nil {

		return fmt.Sprint(resolveValue(*v.DateTimeCast, claims, now))
	}
	if v.HexCast != nil {
		return fmt.Sprint(resolveValue(*v.HexCast, claims, now))
	}

	return nil
}

func resolveStringItem(s acm.StringValue, claims Claims, now time.Time) string {
	if s.Attribute != nil {
		if m, ok := asStringMap(s.Attribute); ok {
			if c := m["CLAIM"]; c != "" {
				return asString(claims[c])
			}
			if g := m["GLOBAL"]; g != "" {
				if val, ok := resolveGlobalToken(g, now); ok {
					return val
				}
			}
		}
	}

	if s.StrVal != nil {
		return string(*s.StrVal)
	}

	if s.StrCast != nil {
		return fmt.Sprint(resolveValue(*s.StrCast, claims, now))
	}

	if s.Field != nil {
		return ""
	}
	return ""
}

func asString(v any) string {
	return fmt.Sprint(v)
}

func castToBool(v any) bool {
	switch strings.ToLower(fmt.Sprint(v)) {
	case "true", "1", "yes", "y", "on":
		return true
	case "false", "0", "no", "n", "off", "":
		return false
	default:
		return false
	}
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func toTimeOfDaySeconds(v any) (int, bool) {
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" {
		return 0, false
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	h, errH := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	sec := 0
	var errS error
	if len(parts) == 3 {
		sec, errS = strconv.Atoi(parts[2])
	}
	if errH != nil || errM != nil || (len(parts) == 3 && errS != nil) {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 || sec < 0 || sec > 59 {
		return 0, false
	}
	return h*3600 + m*60 + sec, true
}

func toDateTime(v any) (time.Time, bool) {
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func numCmp(a, b any, op string) bool {
	if af, aok := toFloat(a); aok {
		if bf, bok := toFloat(b); bok {
			switch op {
			case "gt":
				return af > bf
			case "ge":
				return af >= bf
			case "lt":
				return af < bf
			case "le":
				return af <= bf
			default:
				return false
			}
		}
	}
	if as, aok := toTimeOfDaySeconds(a); aok {
		if bs, bok := toTimeOfDaySeconds(b); bok {
			switch op {
			case "gt":
				return as > bs
			case "ge":
				return as >= bs
			case "lt":
				return as < bs
			case "le":
				return as <= bs
			default:
				return false
			}
		}
	}

	if at, aok := toDateTime(a); aok {
		if bt, bok := toDateTime(b); bok {
			switch op {
			case "gt":
				return at.After(bt)
			case "ge":
				return at.After(bt) || at.Equal(bt)
			case "lt":
				return at.Before(bt)
			case "le":
				return at.Before(bt) || at.Equal(bt)
			default:
				return false
			}
		}
	}

	return false
}

func stringValueFromDate(v acm.Value) string {
	switch {
	case v.DateTimeVal != nil:
		return time.Time(*v.DateTimeVal).Format(time.RFC3339)
	case v.TimeVal != nil:
		return string(*v.TimeVal)
	case v.Year != nil:
		return time.Time(*v.Year).Format("2006")
	case v.Month != nil:
		return time.Time(*v.Month).Format("01")
	case v.DayOfMonth != nil:
		return time.Time(*v.DayOfMonth).Format("02")
	case v.DayOfWeek != nil:
		return time.Time(*v.DayOfWeek).Weekday().String()
	default:
		return ""
	}
}
