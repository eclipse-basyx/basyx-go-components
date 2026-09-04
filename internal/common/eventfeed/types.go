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

import "time"

// Presentation selects which stored payload variant is returned to consumers.
type Presentation string

const (
	// PresentationRegular returns the regular event data representation.
	PresentationRegular Presentation = "REGULAR"
	// PresentationFull is a deprecated alias for PresentationRegular.
	PresentationFull Presentation = "FULL"
	// PresentationCompact returns the compact event data representation.
	PresentationCompact Presentation = "COMPACT"
)

// CloudEvent type values from the Event Feed specification.
const (
	TypeAssetCreated       = "io.admin-shell.asset.created.v1"
	TypeAssetUpdated       = "io.admin-shell.asset.updated.v1"
	TypeAssetDeleted       = "io.admin-shell.asset.deleted.v1"
	TypeAASCreated         = "io.admin-shell.aas.created.v1"
	TypeAASUpdated         = "io.admin-shell.aas.updated.v1"
	TypeAASDeleted         = "io.admin-shell.aas.deleted.v1"
	TypeSubmodelCreated    = "io.admin-shell.submodel.created.v1"
	TypeSubmodelUpdated    = "io.admin-shell.submodel.updated.v1"
	TypeSubmodelDeleted    = "io.admin-shell.submodel.deleted.v1"
	TypePCN                = "io.admin-shell.pcn.v1"
	CloudEventsSpecVersion = "1.0"
	APIVersion             = "1.0"

	// SemanticIDPCN is the IDTA Product Change Notifications submodel semantic id.
	SemanticIDPCN = "0173-1#01-AHE582#003"
)

// SubmodelRef identifies a submodel referenced from an AAS or asset change event.
type SubmodelRef struct {
	SubmodelID string
	SemanticID string
}

// FeedEvent is a persisted CloudEvents feed record with both presentation variants.
type FeedEvent struct {
	Seq               int64
	ID                string
	Type              string
	Subject           string
	Source            string
	Time              time.Time
	DataSchemaFull    string
	DataSchemaCompact string
	DataFull          string
	DataCompact       string
}

// FeedQuery is the consumer-facing read request.
type FeedQuery struct {
	LastEventID  string
	Since        *time.Time
	Cursor       string
	Filter       string
	Presentation Presentation
	Limit        int
}

// FeedRecord is one CloudEvents record in a feed response page.
type FeedRecord struct {
	SpecVersion string         `json:"specversion"`
	ID          string         `json:"id"`
	Time        time.Time      `json:"time"`
	Subject     string         `json:"subject"`
	Type        string         `json:"type"`
	Source      string         `json:"source"`
	DataSchema  string         `json:"dataschema"`
	Data        map[string]any `json:"data,omitempty"`
}

// FeedResponse is the page document returned by GET /events.
type FeedResponse struct {
	ID      string       `json:"id"`
	Updated time.Time    `json:"updated"`
	Records []FeedRecord `json:"records"`
	Cursor  string       `json:"cursor,omitempty"`
}

// CapabilitiesResponse describes feed capabilities for discovery.
type CapabilitiesResponse struct {
	APIVersion   string                           `json:"apiVersion"`
	EventTypes   map[string]EventTypeCapabilities `json:"eventTypes"`
	Filter       FilterCapabilities               `json:"filter"`
	Presentation PresentationCapabilities         `json:"presentation"`
	MaxAge       string                           `json:"maxAge"`
	MaxPageSize  int                              `json:"maxPageSize"`
	Auth         AuthCapabilities                 `json:"auth"`
}

// EventTypeCapabilities describes one advertised event type.
type EventTypeCapabilities struct {
	SupportsCompact      bool              `json:"supportsCompact"`
	Schemas              map[string]string `json:"schemas"`
	FilterableDataFields []string          `json:"filterableDataFields"`
}

// FilterCapabilities describes supported filter languages and fields.
type FilterCapabilities struct {
	FilterableFields  []string         `json:"filterableFields"`
	SupportedPrefixes []string         `json:"supportedPrefixes"`
	RSQL              RSQLCapabilities `json:"rsql"`
}

// RSQLCapabilities lists supported RSQL operators.
type RSQLCapabilities struct {
	Operators []string `json:"operators"`
}

// PresentationCapabilities lists supported presentation modes.
type PresentationCapabilities struct {
	Supported []string `json:"supported"`
	Default   string   `json:"default"`
}

// AuthCapabilities reports that feed HTTP security is inherited from the service.
type AuthCapabilities struct {
	Inherited bool `json:"inherited"`
}

// cursorData is the decoded keyset pagination cursor payload.
type cursorData struct {
	AfterSeq int64 `json:"afterSeq"`
}

// domainQuery is the internal repository query after validation/cursor resolution.
type domainQuery struct {
	AfterSeq int64
	Since    *time.Time
	Filter   *parsedFilter
	Limit    int
}
