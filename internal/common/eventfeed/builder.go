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
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"time"
)

// Builder constructs shared CloudEvents.
type Builder = events.Builder

// NewBuilder constructs a builder using feed-compatible URL settings.
func NewBuilder(cfg Config) *Builder {
	return events.NewBuilder(events.Config{SourceBaseURL: cfg.SourceBaseURL, SchemaBaseURL: cfg.SchemaBaseURL})
}

// IsPCNSemanticID identifies Product Change Notification Submodels.
func IsPCNSemanticID(id string) bool { return events.IsPCNSemanticID(id) }

// SemanticIDFromSubmodel reads the Submodel semantic identifier.
func SemanticIDFromSubmodel(sm types.ISubmodel) string { return events.SemanticIDFromSubmodel(sm) }

// PCNNewRecordValuesFromSubmodel returns newly added notification records.
func PCNNewRecordValuesFromSubmodel(previous, current types.ISubmodel) []model.SubmodelElementValue {
	return events.PCNNewRecordValuesFromSubmodel(previous, current)
}
func schemaPairForType(kind, base string) (string, string) {
	return events.SchemaPairForType(kind, base)
}
func feedDocumentID(now time.Time) string { return events.DocumentID(now) }

func allEventTypes() []string { return events.AllEventTypes() }
