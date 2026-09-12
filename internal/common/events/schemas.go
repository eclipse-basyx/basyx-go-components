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
	"embed"
	"fmt"
)

// SchemaPath is the existing API-relative schema location.
const SchemaPath = "/.well-known/event-feed/schemas"

//go:embed schemas/*.json
var schemaFiles embed.FS

// ReadSchema loads an embedded, versioned event payload schema.
//
// Parameters:
//   - name: Schema filename, for example metamodel-aasChangeEvent.v1.schema.json.
//
// Returns:
//   - []byte: JSON schema document.
//   - error: Coded read error for a missing name; the HTTP handler maps it to a not-found response.
func ReadSchema(name string) ([]byte, error) {
	document, err := schemaFiles.ReadFile("schemas/" + name)
	if err != nil {
		return nil, fmt.Errorf("EVENTS-SCHEMA-READ: %w", err)
	}
	return document, nil
}
