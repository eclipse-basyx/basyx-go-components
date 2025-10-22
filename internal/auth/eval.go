package auth

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/pkg/schemagen"
)

// evalLE evaluates a schemagen.LogicalExpression against claims & now.
func evalLE(le schemagen.LogicalExpression, claims Claims, now time.Time) bool {
	// literal boolean

	fmt.Println("sdknf")
	if le.Boolean != nil {
		return *le.Boolean
	}

	// numeric comparisons
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

	// equality
	if len(le.Eq) == 2 {
		return fmt.Sprint(resolveValue(le.Eq[0], claims, now)) == fmt.Sprint(resolveValue(le.Eq[1], claims, now))
	}
	if len(le.Ne) == 2 {
		return fmt.Sprint(resolveValue(le.Ne[0], claims, now)) != fmt.Sprint(resolveValue(le.Ne[1], claims, now))
	}

	// string ops
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

	// logical ops
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

	// $match: array of expressions treated as AND
	if len(le.Match) > 0 {
		for _, m := range le.Match {
			if !evalMatch(m, claims, now) {
				return false
			}
		}
		return true
	}

	// Unknown/empty expression -> false (defensive)
	return false
}

// evalMatch mirrors evalLE but for MatchExpression (no $not/$and/$or nesting except via its own $match).
func evalMatch(me schemagen.MatchExpression, claims Claims, now time.Time) bool {
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

// ---------- Value & StringValue resolution ----------

// resolveValue evaluates a schemagen.Value into a Go value (string/bool/float64/…).
func resolveValue(v schemagen.Value, claims Claims, now time.Time) any {
	// $attribute (claims/global like UTCNOW)
	if v.Attribute != nil {
		if m, ok := asStringMap(v.Attribute); ok {
			if c := m["CLAIM"]; c != "" {
				return claims[c]
			}
			if strings.EqualFold(m["GLOBAL"], "UTCNOW") {
				return now.Format(time.RFC3339)
			}
		}
	}

	// direct literals
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
		// If you need date parts, add proper handling; otherwise stringify defensively:
		return stringValueFromDate(v)
	}
	if v.HexVal != nil {
		return string(*v.HexVal)
	}

	// field lookups ($field) — if you support model-field access, implement it here.
	// For now we return "" to keep comparisons safe.
	if v.Field != nil {
		return "" // or lookupModelField(string(*v.Field))
	}

	// casts
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

	// other casts ($timeCast, $dateTimeCast, $hexCast) → implement as needed
	if v.TimeCast != nil {
		return fmt.Sprint(resolveValue(*v.TimeCast, claims, now))
	}
	if v.DateTimeCast != nil {
		return fmt.Sprint(resolveValue(*v.DateTimeCast, claims, now))
	}
	if v.HexCast != nil {
		return fmt.Sprint(resolveValue(*v.HexCast, claims, now))
	}

	return nil
}

// resolveStringItem evaluates StringValue to string (for regex/contains/...).
func resolveStringItem(s schemagen.StringValue, claims Claims, now time.Time) string {
	// $attribute
	if s.Attribute != nil {
		if m, ok := asStringMap(s.Attribute); ok {
			if c := m["CLAIM"]; c != "" {
				return asString(claims[c])
			}
			if strings.EqualFold(m["GLOBAL"], "UTCNOW") {
				return now.Format(time.RFC3339)
			}
		}
	}
	// $strVal
	if s.StrVal != nil {
		return string(*s.StrVal)
	}
	// $strCast
	if s.StrCast != nil {
		return fmt.Sprint(resolveValue(*s.StrCast, claims, now))
	}
	// $field
	if s.Field != nil {
		return "" // or lookupModelField(string(*s.Field))
	}
	return ""
}

// ---------- helpers (coercions & comparisons) ----------

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

// toFloat coerces many encodings to float64.
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

func numCmp(a, b any, op string) bool {
	af, aok := toFloat(a)
	bf, bok := toFloat(b)

	fmt.Println(af)
	fmt.Println(bf)
	if !aok || !bok {
		return false
	}
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

// Best-effort stringify for date parts in the schema.
// Expand if you start using these fields semantically.
func stringValueFromDate(v schemagen.Value) string {
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
