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

var (
	fractionBeyondMicrosecondsRegex = regexp.MustCompile(fractionBeyondMicrosecondsPattern)
	castDateTimeRegex               = regexp.MustCompile(`^([0-9]{4,})-([0-9]{2})-([0-9]{2})(?:[T ]([0-9]{2}):([0-9]{2})(?::([0-9]{2})(?:\.([0-9]+))?)?)?(Z|[+-][0-9]{2}:[0-9]{2})?$`)
	timeOfDayRegex                  = regexp.MustCompile(`^([0-9]{2}):([0-9]{2}):([0-9]{2})(?:\.([0-9]+))?(Z|[+-][0-9]{2}:[0-9]{2})?$`)
	maxPostgreSQLTimestamp          = time.Date(294276, time.December, 31, 23, 59, 59, 999999000, time.UTC)
)

// truncateToMicroseconds drops sub-microsecond precision the same way SQL generation does.
func truncateToMicroseconds(t time.Time) time.Time {
	return t.Add(-time.Duration(t.Nanosecond() % int(time.Microsecond)))
}

// truncateFractionDigits removes fractional-second digits beyond microsecond precision.
func truncateFractionDigits(text string) string {
	return fractionBeyondMicrosecondsRegex.ReplaceAllString(text, "$1")
}

// postgreSQLYear numbers years like PostgreSQL EXTRACT(YEAR), which has no year 0.
func postgreSQLYear(t time.Time) int {
	if year := t.Year(); year > 0 {
		return year
	}
	return t.Year() - 1
}

// parseCastDateTime parses the date and date-time forms that SQL generation accepts for
// $dateTimeCast. It rejects every input whose PostgreSQL result it cannot reproduce, so
// those comparisons are left to the database instead of being folded differently.
func parseCastDateTime(value string) (time.Time, bool) {
	parts := castDateTimeRegex.FindStringSubmatch(value)
	if parts == nil {
		return time.Time{}, false
	}
	year, _ := strconv.Atoi(parts[1])
	month, _ := strconv.Atoi(parts[2])
	day, _ := strconv.Atoi(parts[3])
	clock, clockOK := parseClock(parts[4], parts[5], parts[6], parts[7])
	zone, zoneOK := parseTimezoneOffset(parts[8])
	if year < 1 || !clockOK || !zoneOK {
		return time.Time{}, false
	}
	midnight := time.Date(year, time.Month(month), day, 0, 0, 0, 0, zone)
	if midnight.Year() != year || int(midnight.Month()) != month || midnight.Day() != day {
		return time.Time{}, false
	}
	instant := midnight.Add(clock)
	if instant.After(maxPostgreSQLTimestamp) {
		return time.Time{}, false
	}
	return instant, true
}

// toTimeOfDayMicros returns the clock time of an xs:time value in microseconds since
// midnight. Like a PostgreSQL time cast, it ignores the timezone offset.
func toTimeOfDayMicros(value interface{}) (int, bool) {
	parts := timeOfDayRegex.FindStringSubmatch(strings.TrimSpace(fmt.Sprint(value)))
	if parts == nil {
		return 0, false
	}
	clock, clockOK := parseClock(parts[1], parts[2], parts[3], parts[4])
	_, zoneOK := parseTimezoneOffset(parts[5])
	if !clockOK || !zoneOK {
		return 0, false
	}
	return int(clock / time.Microsecond), true
}

// parseClock converts clock components to the duration since midnight, truncated to
// microseconds. Empty components count as zero, and 24:00:00 denotes the end of the day.
func parseClock(hourText, minuteText, secondText, fractionText string) (time.Duration, bool) {
	hour, _ := strconv.Atoi(hourText)
	minute, _ := strconv.Atoi(minuteText)
	second, _ := strconv.Atoi(secondText)
	micros, _ := strconv.Atoi((fractionText + "000000")[:6])
	if minute > 59 || second > 59 || hour > 24 || hour == 24 && minute+second+micros > 0 {
		return 0, false
	}
	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute +
		time.Duration(second)*time.Second + time.Duration(micros)*time.Microsecond, true
}

// parseTimezoneOffset accepts an empty designator as UTC, "Z", or an XSD offset of at most 14 hours.
func parseTimezoneOffset(designator string) (*time.Location, bool) {
	if designator == "" || designator == "Z" {
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
