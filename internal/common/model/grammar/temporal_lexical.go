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

package grammar

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// fractionBeyondMicrosecondsPattern matches fractional seconds with more digits than
// PostgreSQL stores. SQL generation and constant folding both truncate the excess digits.
const fractionBeyondMicrosecondsPattern = `(\.[0-9]{6})[0-9]+`

const maxXSDTimezoneOffsetHours = 14

// xsdWhitespace lists the characters the XSD whiteSpace facet "collapse" removes around temporal values.
const xsdWhitespace = " \t\n\r"

var (
	// xsdDateTimeRegex matches the xs:dateTime lexical space accepted by a cast from xs:string.
	xsdDateTimeRegex = regexp.MustCompile(`^((?:[1-9][0-9]{3,}|0[0-9]{3}))-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(?:\.([0-9]+))?(Z|[+-][0-9]{2}:[0-9]{2})?$`)
	// dateTimeLiteralRegex matches <DateTimeLiteral>: the "T", the seconds and the timezone are optional.
	dateTimeLiteralRegex = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})T?([0-9]{2}):([0-9]{2})(?::([0-9]{2}))?(?:\.([0-9]+))?(Z|[+-][0-9]{2}:[0-9]{2})?$`)
	// xsdTimeRegex matches the xs:time lexical space accepted by a cast from xs:string.
	xsdTimeRegex = regexp.MustCompile(`^([0-9]{2}):([0-9]{2}):([0-9]{2})(?:\.([0-9]+))?(Z|[+-][0-9]{2}:[0-9]{2})?$`)
	// timeLiteralRegex matches <TimeLiteral>: the seconds and the timezone are optional.
	timeLiteralRegex = regexp.MustCompile(`^([0-9]{2}):([0-9]{2})(?::([0-9]{2}))?(?:\.([0-9]+))?(Z|[+-][0-9]{2}:[0-9]{2})?$`)

	maxPostgreSQLTimestamp = time.Date(294276, time.December, 31, 23, 59, 59, 999999000, time.UTC)

	// implicitTimezone marks values written without a timezone. It is UTC, the implicit
	// timezone, but keeps such values zone-less when they are cast to strings.
	implicitTimezone = time.FixedZone("", 0)
)

// timeReferenceDate is the date XPath attaches to xs:time values to compare them as instants.
var timeReferenceDate = time.Date(1972, time.December, 31, 0, 0, 0, 0, time.UTC)

// truncateToMicroseconds drops sub-microsecond precision the same way SQL generation does.
func truncateToMicroseconds(t time.Time) time.Time {
	return t.Add(-time.Duration(t.Nanosecond() % int(time.Microsecond)))
}

// parseXSDDateTime casts a string to xs:dateTime. The result keeps the submitted timezone
// offset, and a value without one is placed in UTC, the implicit timezone. It rejects every
// input that is not an xs:dateTime or that PostgreSQL cannot parse, such as year 0000.
func parseXSDDateTime(value string) (time.Time, bool) {
	parts := xsdDateTimeRegex.FindStringSubmatch(strings.Trim(value, xsdWhitespace))
	if parts != nil && parts[1] == "0000" {
		return time.Time{}, false
	}
	return dateTimeFromParts(parts)
}

// parseDateTimeLiteral parses a <DateTimeLiteral> like parseXSDDateTime, but also accepts
// the forms without "T" or seconds that the query language allows for literals.
func parseDateTimeLiteral(value string) (time.Time, bool) {
	return dateTimeFromParts(dateTimeLiteralRegex.FindStringSubmatch(value))
}

// parseXSDTime casts a string to xs:time and returns it as an instant on the XPath reference date.
func parseXSDTime(value string) (time.Time, bool) {
	return timeFromParts(xsdTimeRegex.FindStringSubmatch(strings.Trim(value, xsdWhitespace)))
}

// parseTimeLiteral parses a <TimeLiteral> like parseXSDTime, but also accepts the form without seconds.
func parseTimeLiteral(value string) (time.Time, bool) {
	return timeFromParts(timeLiteralRegex.FindStringSubmatch(value))
}

// timeOfDateTime returns the xs:time of an xs:dateTime: its local clock time and timezone on the reference date.
func timeOfDateTime(t time.Time) time.Time {
	hour, minute, second := t.Clock()
	return time.Date(timeReferenceDate.Year(), timeReferenceDate.Month(), timeReferenceDate.Day(),
		hour, minute, second, t.Nanosecond(), t.Location())
}

func dateTimeFromParts(parts []string) (time.Time, bool) {
	if parts == nil {
		return time.Time{}, false
	}
	year, _ := strconv.Atoi(parts[1])
	month, _ := strconv.Atoi(parts[2])
	day, _ := strconv.Atoi(parts[3])
	clock, clockOK := parseClock(parts[4], parts[5], parts[6], parts[7])
	zone, zoneOK := parseTimezoneOffset(parts[8])
	if !clockOK || !zoneOK {
		return time.Time{}, false
	}
	midnight := time.Date(year, time.Month(month), day, 0, 0, 0, 0, zone)
	if midnight.Year() != year || int(midnight.Month()) != month || midnight.Day() != day {
		return time.Time{}, false
	}
	instant := midnight.Add(clock)
	if truncateToMicroseconds(instant).After(maxPostgreSQLTimestamp) {
		return time.Time{}, false
	}
	return instant, true
}

// timeFromParts maps 24:00:00 to 00:00:00, as xs:time does.
func timeFromParts(parts []string) (time.Time, bool) {
	if parts == nil {
		return time.Time{}, false
	}
	clock, clockOK := parseClock(parts[1], parts[2], parts[3], parts[4])
	zone, zoneOK := parseTimezoneOffset(parts[5])
	if !clockOK || !zoneOK {
		return time.Time{}, false
	}
	referenceMidnight := time.Date(timeReferenceDate.Year(), timeReferenceDate.Month(), timeReferenceDate.Day(), 0, 0, 0, 0, zone)
	return referenceMidnight.Add(clock % (24 * time.Hour)), true
}

// parseClock converts clock components to the duration since midnight. An empty second
// counts as zero, and 24:00:00 with a zero fraction denotes the end of the day.
func parseClock(hourText, minuteText, secondText, fractionText string) (time.Duration, bool) {
	hour, _ := strconv.Atoi(hourText)
	minute, _ := strconv.Atoi(minuteText)
	second, _ := strconv.Atoi(secondText)
	nanos, _ := strconv.Atoi((fractionText + "000000000")[:9])
	if minute > 59 || second > 59 || hour > 24 || hour == 24 && (minute > 0 || second > 0 || strings.Trim(fractionText, "0") != "") {
		return 0, false
	}
	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute +
		time.Duration(second)*time.Second + time.Duration(nanos), true
}

// parseTimezoneOffset accepts an empty designator as the implicit timezone, "Z", or an offset of at most 14 hours.
func parseTimezoneOffset(designator string) (*time.Location, bool) {
	switch designator {
	case "":
		return implicitTimezone, true
	case "Z":
		return time.UTC, true
	}
	hours, _ := strconv.Atoi(designator[1:3])
	minutes, _ := strconv.Atoi(designator[4:6])
	if minutes > 59 || hours > maxXSDTimezoneOffsetHours || hours == maxXSDTimezoneOffsetHours && minutes > 0 {
		return nil, false
	}
	offset := hours*3600 + minutes*60
	if designator[0] == '-' {
		offset = -offset
	}
	return time.FixedZone(designator, offset), true
}

// extractDatePart returns a calendar part of an xs:dateTime in its own timezone, the way
// PostgreSQL EXTRACT reports it: part is "YEAR", "MONTH", "DAY", or "DOW" (0 for Sunday).
// It fails when the local date and time exceed the PostgreSQL timestamp range.
func extractDatePart(t time.Time, part string) (float64, bool) {
	local, ok := localDateTime(t)
	if !ok {
		return 0, false
	}
	switch part {
	case "YEAR":
		return float64(local.Year()), true
	case "MONTH":
		return float64(local.Month()), true
	case "DAY":
		return float64(local.Day()), true
	case "DOW":
		return float64(local.Weekday()), true
	default:
		return 0, false
	}
}

// localDateTime returns the local date and time of an xs:dateTime, truncated to microseconds.
func localDateTime(t time.Time) (time.Time, bool) {
	truncated := truncateToMicroseconds(t)
	year, month, day := truncated.Date()
	hour, minute, second := truncated.Clock()
	local := time.Date(year, month, day, hour, minute, second, truncated.Nanosecond(), time.UTC)
	return local, !local.After(maxPostgreSQLTimestamp)
}

// canonicalDateTimeString casts an xs:dateTime to xs:string the way XPath does: its local date
// and time with redundant fractional zeros removed, followed by its timezone, "Z" for a zero
// offset, or nothing for a value without a timezone.
func canonicalDateTimeString(t time.Time) (string, bool) {
	local, ok := localDateTime(t)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%04d-%s%s", local.Year(), local.Format("01-02T15:04:05.999999"), canonicalTimezone(t.Location())), true
}

// canonicalTimeString casts an xs:time on the reference date to xs:string the way XPath does.
func canonicalTimeString(t time.Time) string {
	return truncateToMicroseconds(t).Format("15:04:05.999999") + canonicalTimezone(t.Location())
}

func canonicalTimezone(location *time.Location) string {
	if location == implicitTimezone {
		return ""
	}
	_, offset := time.Date(timeReferenceDate.Year(), 1, 1, 0, 0, 0, 0, location).Zone()
	if offset == 0 {
		return "Z"
	}
	sign := '+'
	if offset < 0 {
		sign, offset = '-', -offset
	}
	return fmt.Sprintf("%c%02d:%02d", sign, offset/3600, offset%3600/60)
}
