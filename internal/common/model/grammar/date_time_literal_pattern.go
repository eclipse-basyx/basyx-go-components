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

//nolint:all
package grammar

import (
	"fmt"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// UnmarshalJSON parses a <DateTimeLiteral> into a DateTimeLiteralPattern.
func (d *DateTimeLiteralPattern) UnmarshalJSON(value []byte) error {
	var s string
	if err := common.UnmarshalAndDisallowUnknownFields(value, &s); err != nil {
		return err
	}
	t, ok := parseDateTimeLiteral(s)
	if !ok {
		return fmt.Errorf("GRAMMAR-DATETIMEVAL-PARSE: %q is not a valid $dateTimeVal", s)
	}
	*d = DateTimeLiteralPattern(t)
	return nil
}

// MarshalJSON renders the timestamp while preserving the optional timezone.
func (d DateTimeLiteralPattern) MarshalJSON() ([]byte, error) {
	t := time.Time(d)
	literal := t.Format(time.RFC3339Nano)
	if t.Year() == 10000 && t.Equal(time.Date(10000, time.January, 1, 0, 0, 0, 0, t.Location())) {
		// Use the equivalent end-of-day spelling to keep the year within four digits.
		literal = "9999-12-31T24:00:00" + t.Format("Z07:00")
	}
	if t.Location() == implicitTimezone {
		literal = strings.TrimSuffix(literal, "Z")
	}
	return []byte(fmt.Sprintf("%q", literal)), nil
}
