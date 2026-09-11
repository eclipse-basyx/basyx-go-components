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

import "time"

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

	// SemanticIDPCN is the IDTA Product Change Notifications submodel semantic id.
	SemanticIDPCN = "0173-1#01-AHE582#003"
)

// SubmodelRef identifies a submodel referenced from an AAS or asset change event.
type SubmodelRef struct {
	SubmodelID string
	SemanticID string
}

// FeedEvent is a captured model event with REGULAR and COMPACT payloads.
// Seq and PublishSeq are populated only by HTTP feed storage.
type FeedEvent struct {
	// Seq is the internal write-order id (assigned before commit). It is
	// never used for client-facing ordering or cursors.
	Seq int64
	// PublishSeq is the client-facing cursor/order key, assigned after the
	// row becomes visible (see database/patches/1_2_0.sql). Zero means not
	// yet assigned.
	PublishSeq        int64
	ID                string
	Type              string
	Subject           string
	Source            string
	Time              time.Time
	DataSchemaFull    string
	DataSchemaCompact string
	DataFull          string
	DataCompact       string
	// AuthorizationAASIDs records the owning AASs of payload data at capture time.
	// A nil slice means provenance is unknown; an empty slice means none is needed.
	AuthorizationAASIDs []string
}

// FeedRecord is a structured CloudEvents JSON envelope used by the event transports.
type FeedRecord struct {
	DataContentType string         `json:"datacontenttype"`
	SpecVersion     string         `json:"specversion"`
	ID              string         `json:"id"`
	Time            time.Time      `json:"time"`
	Subject         string         `json:"subject"`
	Type            string         `json:"type"`
	Source          string         `json:"source"`
	DataSchema      string         `json:"dataschema"`
	Data            map[string]any `json:"data,omitempty"`
}
