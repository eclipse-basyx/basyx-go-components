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

package events

import (
	"encoding/json"
	"fmt"
)

// Record converts the immutable event into its CloudEvents JSON representation.
func Record(event FeedEvent, compact bool) (FeedRecord, error) {
	dataJSON, schema := event.DataFull, event.DataSchemaFull
	if compact {
		dataJSON, schema = event.DataCompact, event.DataSchemaCompact
	}
	var data map[string]any
	if dataJSON != "" && dataJSON != "null" {
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			return FeedRecord{}, fmt.Errorf("EVENTS-RECORD-DATAJSON: %w", err)
		}
	}
	return FeedRecord{SpecVersion: CloudEventsSpecVersion, DataContentType: "application/json", ID: event.ID, Time: event.Time.UTC(), Subject: event.Subject, Type: event.Type, Source: event.Source, DataSchema: schema, Data: data}, nil
}
