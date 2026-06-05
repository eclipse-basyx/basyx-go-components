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

package history

import (
	"context"
	"time"
)

const (
	// ModeOff disables history writes.
	ModeOff = "off"
	// ModeAPI enables functional API history.
	ModeAPI = "api"
	// ModeAudit enables audit-oriented append-only history.
	ModeAudit = "audit"

	// ImmutabilityNone leaves history under normal PostgreSQL permissions.
	ImmutabilityNone = "none"
	// ImmutabilityPostgresGuarded enables DB-level history guards.
	ImmutabilityPostgresGuarded = "postgres_guarded"
	// ImmutabilityExternalAnchor is reserved for anchored hash chains.
	ImmutabilityExternalAnchor = "external_anchor"

	// AuditIdentityNone stores no identity metadata.
	AuditIdentityNone = "none"
	// AuditIdentityMinimal stores stable technical identity metadata.
	AuditIdentityMinimal = "minimal"
	// AuditIdentityExtended stores optional extended metadata.
	AuditIdentityExtended = "extended"

	// TableAAS stores Asset Administration Shell history snapshots.
	TableAAS = "aas_history"
	// TableSubmodel stores Submodel history snapshots.
	TableSubmodel = "submodel_history"
	// TableConcept stores Concept Description history snapshots.
	TableConcept = "concept_description_history"
	// TableDescriptor stores AAS descriptor history snapshots.
	TableDescriptor = "descriptor_history"
	// TableGuardConfig stores the runtime switch for PostgreSQL history guards.
	TableGuardConfig = "history_guard_config"

	// ChangeCreated marks a created entity version.
	ChangeCreated = "Created"
	// ChangeUpdated marks an updated entity version.
	ChangeUpdated = "Updated"
	// ChangeDeleted marks a deleted entity version.
	ChangeDeleted = "Deleted"

	// DefaultRecentChangesLimit is the recent-change page size used when no limit is provided.
	DefaultRecentChangesLimit int32 = 100
	// MaxRecentChangesLimit bounds restored snapshot loading for one recent-change request.
	MaxRecentChangesLimit int32 = 1000
	// DefaultFullSnapshotInterval stores every history row as a full snapshot.
	DefaultFullSnapshotInterval = 1
	// PayloadTypeSnapshot marks a directly readable stored snapshot payload.
	PayloadTypeSnapshot = "snapshot"
	// PayloadTypeDiff marks an RFC 6902 JSON Patch payload row.
	PayloadTypeDiff = "diff"

	historyRowHashContract = "basyx-history-row-v2"
)

var payloadTables = map[string]string{
	TableAAS:        "aas_history_payload",
	TableSubmodel:   "submodel_history_payload",
	TableConcept:    "concept_description_history_payload",
	TableDescriptor: "descriptor_history_payload",
}

// Row is a normalized history entry loaded from one of the history tables.
//
// Recent-change APIs use Row as their internal transport shape: metadata comes
// from the history table, while Snapshot is restored from the payload table so
// service-specific mappers can derive response fields from a complete entity
// version.
type Row struct {
	HistoryID   int64
	Identifier  string
	ChangeType  string
	Snapshot    map[string]any
	Deleted     bool
	CreatedAt   string
	UpdatedAt   string
	OperationAt time.Time
}

// ChangeEvent is the internal event representation shared by history, audit, and anchoring.
//
// AppendVersionTx builds ChangeEvent values before calculating the row hash.
// The hash includes payload hashes, the previous row hash, and audit metadata so
// each persisted row can be verified as part of an append-only chain.
type ChangeEvent struct {
	EntityType          string
	Identifier          string
	ChangeType          string
	Timestamp           time.Time
	Snapshot            map[string]any
	Deleted             bool
	RequestID           string
	CorrelationID       string
	ActorSubject        string
	ActorIssuer         string
	ClientID            string
	AuthorizationResult string
	PolicyID            string
	MatchedRuleID       string
	SourceIP            string
	UserAgent           string
	Operation           string
	Endpoint            string
	HTTPMethod          string
	PayloadType         string
	ContentHash         string
	PayloadHash         string
	PreviousHash        string
	RowHash             string
}

// Writer consumes change events for history storage.
//
// The interface is reserved for storage adapters that accept already-normalized
// events instead of repository-specific snapshots.
type Writer interface {
	Append(ctx context.Context, event ChangeEvent) error
}

// AnchorClient is the extension point for external hash anchoring.
//
// Implementations receive batches of hash-chain events and return provider
// metadata that can later be used to prove the chain existed at a given time.
type AnchorClient interface {
	Anchor(ctx context.Context, batch AnchorBatch) (*AnchorResult, error)
}

// EventPublisher is the extension point for future CloudEvents-compatible publishing.
//
// The history package defines the interface before eventing is wired so service
// code can keep event publication separate from history persistence.
type EventPublisher interface {
	Publish(ctx context.Context, event ChangeEvent) error
}

// AnchorBatch groups hash-chain rows for external anchoring.
//
// Source identifies the producing service or table group and Rows contains the
// ordered events being anchored.
type AnchorBatch struct {
	Source string
	Rows   []ChangeEvent
}

// AnchorResult captures external anchor metadata.
//
// AnchorID is provider-defined, while AnchorTime records when the provider
// accepted the batch.
type AnchorResult struct {
	AnchorID   string
	AnchorTime time.Time
}

// NoopAnchorClient is the default anchor client used when external anchoring is disabled.
type NoopAnchorClient struct{}

// Anchor intentionally performs no external write and returns no anchor metadata.
//
// Parameters:
//   - context.Context: Ignored request context.
//   - AnchorBatch: Ignored batch of events.
//
// Returns:
//   - *AnchorResult: Always nil.
//   - error: Always nil.
//
// Example:
//
//	result, err := NoopAnchorClient{}.Anchor(ctx, batch)
//	if err != nil {
//		return err
//	}
//	_ = result
func (NoopAnchorClient) Anchor(_ context.Context, _ AnchorBatch) (*AnchorResult, error) {
	return nil, nil
}

// SnapshotMutator applies a scoped change to an existing history snapshot.
//
// AppendMutatedVersionTx passes a mutable restored snapshot to the mutator and
// then persists the mutated value as the next version.
type SnapshotMutator func(snapshot map[string]any) error

// RecentRowsFetcher loads one raw history page.
//
// FilterRecentRows uses this callback to page through unfiltered rows while
// applying service-specific predicates.
type RecentRowsFetcher func(limit int32, cursor string) ([]Row, string, error)

// RecentRowPredicate decides whether a history row belongs in a filtered response.
//
// Predicates can inspect the restored Snapshot as well as normalized row
// metadata, and may return an error when snapshot content is malformed.
type RecentRowPredicate func(row Row) (bool, error)
