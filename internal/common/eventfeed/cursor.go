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

package eventfeed

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func encodeCursor(afterID string, afterTime time.Time) (string, error) {
	payload, err := json.Marshal(cursorData{
		AfterID:   afterID,
		AfterTime: afterTime.UTC(),
	})
	if err != nil {
		return "", fmt.Errorf("EVENTFEED-CURSOR-ENCODE: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(cursor string) (cursorData, error) {
	raw := strings.TrimSpace(cursor)
	if raw == "" {
		return cursorData{}, fmt.Errorf("EVENTFEED-CURSOR-BLANK cursor must not be blank")
	}
	bytes, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		// Accept standard encoding with padding as a convenience.
		bytes, err = base64.URLEncoding.DecodeString(raw)
		if err != nil {
			return cursorData{}, fmt.Errorf("EVENTFEED-CURSOR-BASE64 Cursor is not valid Base64: %s", cursor)
		}
	}
	var data cursorData
	if err = json.Unmarshal(bytes, &data); err != nil {
		return cursorData{}, fmt.Errorf("EVENTFEED-CURSOR-JSON Cursor contains invalid content: %s", cursor)
	}
	if strings.TrimSpace(data.AfterID) == "" || data.AfterTime.IsZero() {
		return cursorData{}, fmt.Errorf("EVENTFEED-CURSOR-FIELDS Cursor contains invalid content: %s", cursor)
	}
	return data, nil
}
