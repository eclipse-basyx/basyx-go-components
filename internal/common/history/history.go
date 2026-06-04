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

// Package history stores append-only snapshots for v3.2 history and recent-change endpoints.
package history

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
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
	// ImmutabilityPostgresGuarded is reserved for DB-level history guards.
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

	// DefaultRecentChangesLimit is the page size used when no limit is provided.
	DefaultRecentChangesLimit int32 = 100
	// MaxRecentChangesLimit bounds snapshot loading for one recent-change request.
	MaxRecentChangesLimit int32 = 1000
	// DefaultFullSnapshotInterval stores every history row as a full snapshot.
	DefaultFullSnapshotInterval = 1
	// PayloadTypeSnapshot marks a directly readable stored snapshot payload.
	PayloadTypeSnapshot = "snapshot"
	// PayloadTypeDiff marks an RFC 6902 JSON Patch payload row.
	PayloadTypeDiff = "diff"
)

var (
	configMu     sync.RWMutex
	activeConfig = Config{
		Mode:                 ModeOff,
		RetentionDays:        0,
		FullSnapshotInterval: DefaultFullSnapshotInterval,
		Immutability:         ImmutabilityNone,
		AuditIdentityMode:    AuditIdentityNone,
	}
	payloadTables = map[string]string{
		TableAAS:        "aas_history_payload",
		TableSubmodel:   "submodel_history_payload",
		TableConcept:    "concept_description_history_payload",
		TableDescriptor: "descriptor_history_payload",
	}
)

// Row is a normalized history entry loaded from one of the history tables.
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

// ChangeEvent is the internal event representation shared by history, audit,
// anchoring, and future eventing sinks.
type ChangeEvent struct {
	EntityType    string
	Identifier    string
	ChangeType    string
	Timestamp     time.Time
	Snapshot      map[string]any
	Deleted       bool
	RequestID     string
	CorrelationID string
	ActorSubject  string
	ActorIssuer   string
	ClientID      string
	Endpoint      string
	HTTPMethod    string
	PayloadType   string
	ContentHash   string
	PayloadHash   string
	PreviousHash  string
	RowHash       string
}

// AuditContext carries vendor-neutral request and identity metadata.
type AuditContext struct {
	ActorSubject        string
	ActorIssuer         string
	ClientID            string
	AuthorizationResult string
	PolicyID            string
	MatchedRuleID       string
	RequestID           string
	CorrelationID       string
	SourceIP            string
	UserAgent           string
	Operation           string
	Endpoint            string
	HTTPMethod          string
}

type auditContextKey struct{}

// ContextWithAudit stores audit metadata in a context.
func ContextWithAudit(ctx context.Context, audit AuditContext) context.Context {
	return context.WithValue(ctx, auditContextKey{}, audit)
}

// FromContext returns audit metadata stored in ctx.
func FromContext(ctx context.Context) AuditContext {
	if ctx == nil {
		return AuditContext{}
	}
	audit, _ := ctx.Value(auditContextKey{}).(AuditContext)
	return audit
}

// Writer consumes change events for history storage.
type Writer interface {
	Append(ctx context.Context, event ChangeEvent) error
}

// AnchorClient is the extension point for external hash anchoring.
type AnchorClient interface {
	Anchor(ctx context.Context, batch AnchorBatch) (*AnchorResult, error)
}

// EventPublisher is the extension point for future CloudEvents-compatible publishing.
type EventPublisher interface {
	Publish(ctx context.Context, event ChangeEvent) error
}

// AnchorBatch groups hash-chain rows for external anchoring.
type AnchorBatch struct {
	Source string
	Rows   []ChangeEvent
}

// AnchorResult captures external anchor metadata.
type AnchorResult struct {
	AnchorID   string
	AnchorTime time.Time
}

// NoopAnchorClient is the default anchor client.
type NoopAnchorClient struct{}

// Anchor intentionally performs no external write.
func (NoopAnchorClient) Anchor(_ context.Context, _ AnchorBatch) (*AnchorResult, error) {
	return nil, nil
}

// Config controls the lightweight history/audit behavior.
type Config struct {
	Mode                 string
	RetentionDays        int
	FullSnapshotInterval int
	Immutability         string
	AuditIdentityMode    string
}

// SnapshotMutator applies a scoped change to an existing history snapshot.
type SnapshotMutator func(snapshot map[string]any) error

// RecentRowsFetcher loads one raw history page.
type RecentRowsFetcher func(limit int32, cursor string) ([]Row, string, error)

// RecentRowPredicate decides whether a history row belongs in a filtered response.
type RecentRowPredicate func(row Row) (bool, error)

// Configure replaces the process-local history configuration.
func Configure(cfg Config) {
	configMu.Lock()
	defer configMu.Unlock()
	activeConfig = normalizeConfig(cfg)
}

// ActiveConfig returns the normalized process-local history configuration.
func ActiveConfig() Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return activeConfig
}

// ApplyPostgresGuardConfig enables database-side history mutation guards without
// allowing ordinary service startup to downgrade an enabled database guard.
func ApplyPostgresGuardConfig(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return common.NewInternalServerError("HISTORY-GUARD-NILDB database handle must not be nil")
	}

	enabled := postgresGuardEnabled(ActiveConfig())
	query, args, err := goqu.Insert(TableGuardConfig).
		Rows(goqu.Record{
			"id":         true,
			"enabled":    enabled,
			"updated_at": goqu.L("NOW()"),
		}).
		OnConflict(goqu.DoUpdate("id", goqu.Record{
			"enabled":    goqu.L("history_guard_config.enabled OR EXCLUDED.enabled"),
			"updated_at": goqu.L("NOW()"),
		})).
		Returning(goqu.C("enabled")).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-GUARD-BUILDUPSERT " + err.Error())
	}
	var databaseGuardEnabled bool
	if err = db.QueryRowContext(ctx, query, args...).Scan(&databaseGuardEnabled); err != nil {
		return common.NewInternalServerError("HISTORY-GUARD-EXECUPSERT " + err.Error())
	}
	if !enabled && databaseGuardEnabled {
		return common.NewInternalServerError("HISTORY-GUARD-CONFLICT database history guard is enabled but this service is configured without postgres_guarded immutability")
	}
	return nil
}

// AppendVersionTx appends an immutable snapshot event for identifier.
func AppendVersionTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, snapshot map[string]any, deleted bool) error {
	cfg := ActiveConfig()
	if cfg.Mode == ModeOff {
		return nil
	}

	identifier, err := validateAppendInputs(tx, identifier)
	if err != nil {
		return err
	}
	if err = lockIdentifierTx(ctx, tx, table, identifier); err != nil {
		return err
	}
	if cfg.FullSnapshotInterval == DefaultFullSnapshotInterval {
		previousHash, hashErr := latestRowHashTx(ctx, tx, table, identifier)
		if hashErr != nil {
			return hashErr
		}
		return appendSnapshotVersionWithPreviousHashTx(ctx, tx, table, identifier, changeType, snapshot, deleted, previousHash)
	}
	latest, err := latestVersionTx(ctx, tx, table, identifier)
	if err != nil && !common.IsErrNotFound(err) {
		return err
	}
	if common.IsErrNotFound(err) {
		return appendVersionWithLatestTx(ctx, tx, table, identifier, changeType, snapshot, deleted, nil)
	}
	return appendVersionWithLatestTx(ctx, tx, table, identifier, changeType, snapshot, deleted, &latest)
}

// AppendMutatedVersionTx derives and appends a version from the latest snapshot.
func AppendMutatedVersionTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, mutate SnapshotMutator) error {
	if ActiveConfig().Mode == ModeOff {
		return nil
	}

	identifier, err := validateAppendInputs(tx, identifier)
	if err != nil {
		return err
	}
	if mutate == nil {
		return common.NewInternalServerError("HISTORY-MUTATE-NILMUTATOR snapshot mutator must not be nil")
	}
	if err = lockIdentifierTx(ctx, tx, table, identifier); err != nil {
		return err
	}

	latest, err := latestVersionTx(ctx, tx, table, identifier)
	if err != nil {
		return err
	}
	if latest.deleted {
		return common.NewErrNotFound("HISTORY-MUTATE-DELETED latest historical version is deleted")
	}
	currentSnapshot := latest.snapshot
	previousSnapshot, err := cloneSnapshotMap(latest.snapshot)
	if err != nil {
		return err
	}
	if err = mutate(currentSnapshot); err != nil {
		return err
	}
	previousVersion := latest
	previousVersion.snapshot = previousSnapshot
	return appendVersionWithLatestTx(ctx, tx, table, identifier, changeType, currentSnapshot, false, &previousVersion)
}

func validateAppendInputs(tx *sql.Tx, identifier string) (string, error) {
	if tx == nil {
		return "", common.NewInternalServerError("HISTORY-APPEND-NILTX transaction must not be nil")
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", common.NewErrBadRequest("HISTORY-APPEND-EMPTYID identifier must not be empty")
	}
	return identifier, nil
}

func appendVersionWithLatestTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, snapshot map[string]any, deleted bool, latest *latestVersion) error {
	payload, err := buildHistoryPayload(snapshot, latest, ActiveConfig())
	if err != nil {
		return err
	}
	previousHash := ""
	if latest != nil {
		previousHash = latest.rowHash
	}
	return insertHistoryVersionTx(ctx, tx, table, identifier, changeType, snapshot, deleted, payload, previousHash)
}

func appendSnapshotVersionWithPreviousHashTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, snapshot map[string]any, deleted bool, previousHash string) error {
	payload, err := buildSnapshotPayload(snapshot)
	if err != nil {
		return err
	}
	return insertHistoryVersionTx(ctx, tx, table, identifier, changeType, snapshot, deleted, payload, previousHash)
}

func insertHistoryVersionTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, snapshot map[string]any, deleted bool, payload historyPayload, previousHash string) error {
	payloadTable, err := historyPayloadTable(table)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	contentHash, err := CanonicalJSONHash(snapshot)
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-CONTENTHASH " + err.Error())
	}
	audit := FromContext(ctx)
	event := ChangeEvent{
		EntityType:    table,
		Identifier:    identifier,
		ChangeType:    changeType,
		Timestamp:     now,
		Snapshot:      snapshot,
		Deleted:       deleted,
		PayloadType:   payload.payloadType,
		ContentHash:   contentHash,
		PayloadHash:   payload.hash,
		PreviousHash:  previousHash,
		RequestID:     audit.RequestID,
		CorrelationID: audit.CorrelationID,
		ActorSubject:  audit.ActorSubject,
		ActorIssuer:   audit.ActorIssuer,
		ClientID:      audit.ClientID,
		Endpoint:      audit.Endpoint,
		HTTPMethod:    audit.HTTPMethod,
	}
	rowHash, err := ComputeHistoryRowHash(event)
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-ROWHASH " + err.Error())
	}
	createdAt, updatedAt := administrationTimestamps(snapshot)
	insertQuery, insertArgs, err := goqu.Insert(table).Rows(goqu.Record{
		"identifier":                     identifier,
		"change_type":                    changeType,
		"deleted":                        deleted,
		"valid_from":                     now,
		"operation_time":                 now,
		"administration_created_at_text": createdAt,
		"administration_updated_at_text": updatedAt,
		"administration_created_at":      nullableTimestamp(createdAt),
		"administration_updated_at":      nullableTimestamp(updatedAt),
		"payload_type":                   payload.payloadType,
		"content_hash":                   contentHash,
		"payload_hash":                   payload.hash,
		"previous_hash":                  previousHash,
		"row_hash":                       rowHash,
		"actor_subject":                  audit.ActorSubject,
		"actor_issuer":                   audit.ActorIssuer,
		"client_id":                      audit.ClientID,
		"authorization_result":           audit.AuthorizationResult,
		"policy_id":                      audit.PolicyID,
		"matched_rule_id":                audit.MatchedRuleID,
		"request_id":                     audit.RequestID,
		"correlation_id":                 audit.CorrelationID,
		"source_ip":                      nullableINET(audit.SourceIP),
		"user_agent":                     audit.UserAgent,
		"operation":                      audit.Operation,
		"endpoint":                       audit.Endpoint,
		"http_method":                    audit.HTTPMethod,
	}).Returning(goqu.C("history_id")).ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-BUILDINSERT " + err.Error())
	}
	var historyID int64
	if err = tx.QueryRowContext(ctx, insertQuery, insertArgs...).Scan(&historyID); err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-EXECINSERT " + err.Error())
	}
	payloadRecord := goqu.Record{
		"history_id": historyID,
	}
	if payload.payloadType == PayloadTypeSnapshot {
		payloadRecord["snapshot"] = goqu.L("?::jsonb", string(payload.json))
	} else {
		payloadRecord["diff"] = goqu.L("?::jsonb", string(payload.json))
	}
	payloadQuery, payloadArgs, err := goqu.Insert(payloadTable).Rows(payloadRecord).ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-BUILDPAYLOADINSERT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, payloadQuery, payloadArgs...); err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-EXECPAYLOADINSERT " + err.Error())
	}
	return nil
}

func lockIdentifierTx(ctx context.Context, tx *sql.Tx, table string, identifier string) error {
	query, args, err := buildLockIdentifierQuery(table, identifier)
	if err != nil {
		return common.NewInternalServerError("HISTORY-LOCK-BUILDSQL " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("HISTORY-LOCK-EXECSQL " + err.Error())
	}
	return nil
}

func buildLockIdentifierQuery(table string, identifier string) (string, []any, error) {
	lockKey := table + ":" + identifier
	return goqu.
		Dialect("postgres").
		Select(goqu.Func("pg_advisory_xact_lock", goqu.Func("hashtextextended", lockKey, 0))).
		Prepared(true).
		ToSQL()
}

type latestVersion struct {
	historyID         int64
	snapshot          map[string]any
	deleted           bool
	rowHash           string
	rowsSinceSnapshot int
}

type historyQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type historyPayload struct {
	payloadType string
	json        []byte
	hash        string
}

type storedHistoryRow struct {
	HistoryID   int64
	Identifier  string
	ChangeType  string
	PayloadType string
	Snapshot    sql.NullString
	Diff        sql.NullString
	Deleted     bool
	CreatedAt   sql.NullString
	UpdatedAt   sql.NullString
	OperationAt time.Time
	ContentHash sql.NullString
	PayloadHash sql.NullString
	RowHash     sql.NullString
}

func latestVersionTx(ctx context.Context, tx *sql.Tx, table string, identifier string) (latestVersion, error) {
	query, args, err := goqu.From(table).
		Select(goqu.C("history_id")).
		Where(goqu.C("identifier").Eq(identifier)).
		Order(goqu.C("history_id").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return latestVersion{}, common.NewInternalServerError("HISTORY-MUTATE-BUILDLATESTID " + err.Error())
	}

	var historyID int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&historyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return latestVersion{}, common.NewErrNotFound("HISTORY-MUTATE-NOTFOUND no historical version found")
		}
		return latestVersion{}, common.NewInternalServerError("HISTORY-MUTATE-READLATESTID " + err.Error())
	}

	return restoreVersionByHistoryID(ctx, tx, table, identifier, historyID)
}

func latestRowHashTx(ctx context.Context, tx *sql.Tx, table string, identifier string) (string, error) {
	query, args, err := goqu.From(table).
		Select(goqu.C("row_hash")).
		Where(goqu.C("identifier").Eq(identifier)).
		Order(goqu.C("history_id").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return "", common.NewInternalServerError("HISTORY-HASH-BUILDPREVIOUS " + err.Error())
	}
	var previousHash sql.NullString
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&previousHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", common.NewInternalServerError("HISTORY-HASH-READPREVIOUS " + err.Error())
	}
	return previousHash.String, nil
}

func buildHistoryPayload(snapshot map[string]any, latest *latestVersion, cfg Config) (historyPayload, error) {
	if latest == nil || cfg.FullSnapshotInterval == DefaultFullSnapshotInterval || latest.rowsSinceSnapshot >= cfg.FullSnapshotInterval {
		return buildSnapshotPayload(snapshot)
	}
	patch, err := BuildJSONPatch(latest.snapshot, snapshot)
	if err != nil {
		return historyPayload{}, common.NewInternalServerError("HISTORY-APPEND-BUILDDIFF " + err.Error())
	}
	payloadJSON, err := json.Marshal(patch)
	if err != nil {
		return historyPayload{}, common.NewInternalServerError("HISTORY-APPEND-MARSHALDIFF " + err.Error())
	}
	payloadHash, err := CanonicalJSONHash(patch)
	if err != nil {
		return historyPayload{}, common.NewInternalServerError("HISTORY-APPEND-DIFFHASH " + err.Error())
	}
	return historyPayload{payloadType: PayloadTypeDiff, json: payloadJSON, hash: payloadHash}, nil
}

func buildSnapshotPayload(snapshot map[string]any) (historyPayload, error) {
	payloadJSON, err := json.Marshal(snapshot)
	if err != nil {
		return historyPayload{}, common.NewInternalServerError("HISTORY-APPEND-MARSHALSNAPSHOT " + err.Error())
	}
	payloadHash, err := CanonicalJSONHash(snapshot)
	if err != nil {
		return historyPayload{}, common.NewInternalServerError("HISTORY-APPEND-SNAPSHOTHASH " + err.Error())
	}
	return historyPayload{payloadType: PayloadTypeSnapshot, json: payloadJSON, hash: payloadHash}, nil
}

func cloneSnapshotMap(snapshot map[string]any) (map[string]any, error) {
	cloned, err := cloneJSONValue(snapshot)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-MUTATE-CLONESNAPSHOT " + err.Error())
	}
	result, ok := cloned.(map[string]any)
	if !ok {
		return nil, common.NewInternalServerError("HISTORY-MUTATE-CLONESNAPSHOTTYPE cloned snapshot must be an object")
	}
	return result, nil
}

func normalizeConfig(cfg Config) Config {
	cfg.Mode = normalizeHistoryMode(cfg.Mode)
	cfg.Immutability = normalizeImmutability(cfg.Immutability)
	cfg.AuditIdentityMode = normalizeAuditIdentityMode(cfg.AuditIdentityMode)
	if cfg.RetentionDays < 0 {
		cfg.RetentionDays = 0
	}
	if cfg.FullSnapshotInterval < 1 {
		cfg.FullSnapshotInterval = DefaultFullSnapshotInterval
	}
	return cfg
}

func postgresGuardEnabled(cfg Config) bool {
	if cfg.Mode == ModeOff {
		return false
	}
	return cfg.Immutability == ImmutabilityPostgresGuarded || cfg.Immutability == ImmutabilityExternalAnchor
}

func normalizeHistoryMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ModeOff:
		return ModeOff
	case ModeAPI:
		return ModeAPI
	case ModeAudit:
		return ModeAudit
	default:
		return ModeAPI
	}
}

func normalizeImmutability(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ImmutabilityNone:
		return ImmutabilityNone
	case ImmutabilityPostgresGuarded:
		return ImmutabilityPostgresGuarded
	case ImmutabilityExternalAnchor:
		return ImmutabilityExternalAnchor
	default:
		return ImmutabilityNone
	}
}

func normalizeAuditIdentityMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", AuditIdentityNone:
		return AuditIdentityNone
	case AuditIdentityMinimal:
		return AuditIdentityMinimal
	case AuditIdentityExtended:
		return AuditIdentityExtended
	default:
		return AuditIdentityMinimal
	}
}

// SnapshotByDate returns the snapshot that was valid for identifier at the requested instant.
func SnapshotByDate(ctx context.Context, db *sql.DB, table string, identifier string, at time.Time) (map[string]any, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("HISTORY-GET-NILDB database handle must not be nil")
	}
	historyAlias := goqu.T(table).As("history")
	query, args, err := goqu.From(historyAlias).
		Select(historyAlias.Col("history_id")).
		Where(
			historyAlias.Col("identifier").Eq(identifier),
			historyAlias.Col("valid_from").Lte(at.UTC()),
		).
		Order(historyAlias.Col("valid_from").Desc(), historyAlias.Col("history_id").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-GET-BUILDSQL " + err.Error())
	}
	var historyID int64
	if err = db.QueryRowContext(ctx, query, args...).Scan(&historyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("HISTORY-GET-NOTFOUND no historical version found")
		}
		return nil, common.NewInternalServerError("HISTORY-GET-EXECSQL " + err.Error())
	}

	version, err := restoreVersionByHistoryID(ctx, db, table, identifier, historyID)
	if err != nil {
		return nil, err
	}
	if version.deleted {
		return nil, common.NewErrNotFound("HISTORY-GET-DELETED historical version is deleted at the requested date")
	}
	return version.snapshot, nil
}

// RecentRows returns history rows before cursor, ordered from newest to oldest with one look-ahead row for pagination.
func RecentRows(ctx context.Context, db *sql.DB, table string, limit int32, cursor string, createdFrom time.Time, updatedFrom time.Time) ([]Row, string, error) {
	if db == nil {
		return nil, "", common.NewErrBadRequest("HISTORY-RECENT-NILDB database handle must not be nil")
	}
	limit, err := normalizeRecentChangesLimit(limit)
	if err != nil {
		return nil, "", err
	}
	limitInt := int(limit)
	cursorID, err := parseCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	historyAlias := goqu.T(table).As("history")
	query := goqu.From(historyAlias).
		Select(
			historyAlias.Col("history_id"),
			historyAlias.Col("identifier"),
			historyAlias.Col("change_type"),
			historyAlias.Col("deleted"),
			historyAlias.Col("administration_created_at_text"),
			historyAlias.Col("administration_updated_at_text"),
			historyAlias.Col("operation_time"),
		).
		Order(historyAlias.Col("history_id").Desc()).
		Limit(uint(limitInt + 1)) //nolint:gosec // limit is positive int32 and therefore safe on supported platforms.
	if cursorID > 0 {
		query = query.Where(historyAlias.Col("history_id").Lt(cursorID))
	}
	if !createdFrom.IsZero() {
		query = query.Where(goqu.Or(
			historyAlias.Col("operation_time").Gte(createdFrom.UTC()),
			historyAlias.Col("administration_created_at").Gte(createdFrom.UTC()),
		))
	}
	if !updatedFrom.IsZero() {
		query = query.Where(goqu.Or(
			historyAlias.Col("operation_time").Gte(updatedFrom.UTC()),
			historyAlias.Col("administration_updated_at").Gte(updatedFrom.UTC()),
		))
	}

	sqlQuery, args, err := query.ToSQL()
	if err != nil {
		return nil, "", common.NewInternalServerError("HISTORY-RECENT-BUILDSQL " + err.Error())
	}
	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, "", common.NewInternalServerError("HISTORY-RECENT-EXECSQL " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()

	result := make([]Row, 0, limitInt)
	nextCursor := ""
	restoreCache := make(map[string]map[int64]latestVersion)
	for rows.Next() {
		var row Row
		var created sql.NullString
		var updated sql.NullString
		if err = rows.Scan(&row.HistoryID, &row.Identifier, &row.ChangeType, &row.Deleted, &created, &updated, &row.OperationAt); err != nil {
			return nil, "", common.NewInternalServerError("HISTORY-RECENT-SCAN " + err.Error())
		}
		if len(result) == limitInt {
			nextCursor = strconv.FormatInt(result[len(result)-1].HistoryID, 10)
			break
		}
		version, restoreErr := cachedRestoreVersionByHistoryID(ctx, db, table, row.Identifier, row.HistoryID, restoreCache)
		if restoreErr != nil {
			return nil, "", restoreErr
		}
		row.Snapshot = version.snapshot
		row.CreatedAt = timestampOrOperationTime(created.String, row.OperationAt)
		row.UpdatedAt = timestampOrOperationTime(updated.String, row.OperationAt)
		result = append(result, row)
	}
	if err = rows.Err(); err != nil {
		return nil, "", common.NewInternalServerError("HISTORY-RECENT-ROWS " + err.Error())
	}
	return result, nextCursor, nil
}

func restoreVersionByHistoryID(ctx context.Context, queryer historyQueryer, table string, identifier string, historyID int64) (latestVersion, error) {
	versions, err := restoreVersionsThroughHistoryID(ctx, queryer, table, identifier, historyID)
	if err != nil {
		return latestVersion{}, err
	}
	version, ok := versions[historyID]
	if !ok {
		return latestVersion{}, common.NewInternalServerError("HISTORY-RESTORE-MISSINGTARGET restored chain does not contain requested history row")
	}
	return version, nil
}

func cachedRestoreVersionByHistoryID(
	ctx context.Context,
	queryer historyQueryer,
	table string,
	identifier string,
	historyID int64,
	cache map[string]map[int64]latestVersion,
) (latestVersion, error) {
	if versions := cache[identifier]; versions != nil {
		if version, ok := versions[historyID]; ok {
			return version, nil
		}
	}
	versions, err := restoreVersionsThroughHistoryID(ctx, queryer, table, identifier, historyID)
	if err != nil {
		return latestVersion{}, err
	}
	identifierCache := cache[identifier]
	if identifierCache == nil {
		identifierCache = make(map[int64]latestVersion, len(versions))
		cache[identifier] = identifierCache
	}
	for restoredHistoryID, version := range versions {
		identifierCache[restoredHistoryID] = version
	}
	version, ok := identifierCache[historyID]
	if !ok {
		return latestVersion{}, common.NewInternalServerError("HISTORY-RESTORE-MISSINGTARGET restored chain does not contain requested history row")
	}
	return version, nil
}

func restoreVersionsThroughHistoryID(ctx context.Context, queryer historyQueryer, table string, identifier string, historyID int64) (map[int64]latestVersion, error) {
	payloadTable, err := historyPayloadTable(table)
	if err != nil {
		return nil, err
	}
	checkpointID, err := nearestSnapshotHistoryID(ctx, queryer, table, identifier, historyID)
	if err != nil {
		return nil, err
	}
	rows, err := loadVersionChain(ctx, queryer, table, payloadTable, identifier, checkpointID, historyID)
	if err != nil {
		return nil, err
	}
	return restoreVersionChainRows(rows)
}

func nearestSnapshotHistoryID(ctx context.Context, queryer historyQueryer, table string, identifier string, historyID int64) (int64, error) {
	query, args, err := goqu.From(table).
		Select(goqu.C("history_id")).
		Where(
			goqu.C("identifier").Eq(identifier),
			goqu.C("history_id").Lte(historyID),
			goqu.C("payload_type").Eq(PayloadTypeSnapshot),
		).
		Order(goqu.C("history_id").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("HISTORY-RESTORE-BUILDCHECKPOINT " + err.Error())
	}
	var checkpointID int64
	if err = queryer.QueryRowContext(ctx, query, args...).Scan(&checkpointID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, common.NewInternalServerError("HISTORY-RESTORE-NOCHECKPOINT no full snapshot checkpoint found")
		}
		return 0, common.NewInternalServerError("HISTORY-RESTORE-READCHECKPOINT " + err.Error())
	}
	return checkpointID, nil
}

func loadVersionChain(
	ctx context.Context,
	queryer historyQueryer,
	table string,
	payloadTable string,
	identifier string,
	checkpointID int64,
	historyID int64,
) ([]storedHistoryRow, error) {
	historyAlias := goqu.T(table).As("history")
	payloadAlias := goqu.T(payloadTable).As("payload")
	query, args, err := goqu.From(historyAlias).
		InnerJoin(payloadAlias, goqu.On(historyAlias.Col("history_id").Eq(payloadAlias.Col("history_id")))).
		Select(
			historyAlias.Col("history_id"),
			historyAlias.Col("identifier"),
			historyAlias.Col("change_type"),
			historyAlias.Col("payload_type"),
			goqu.L(`"payload"."snapshot"::text`),
			goqu.L(`"payload"."diff"::text`),
			historyAlias.Col("deleted"),
			historyAlias.Col("administration_created_at_text"),
			historyAlias.Col("administration_updated_at_text"),
			historyAlias.Col("operation_time"),
			historyAlias.Col("content_hash"),
			historyAlias.Col("payload_hash"),
			historyAlias.Col("row_hash"),
		).
		Where(
			historyAlias.Col("identifier").Eq(identifier),
			historyAlias.Col("history_id").Gte(checkpointID),
			historyAlias.Col("history_id").Lte(historyID),
		).
		Order(historyAlias.Col("history_id").Asc()).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-BUILDCHAIN " + err.Error())
	}
	sqlRows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-EXECCHAIN " + err.Error())
	}
	defer func() {
		_ = sqlRows.Close()
	}()

	rows := make([]storedHistoryRow, 0)
	for sqlRows.Next() {
		var row storedHistoryRow
		if err = sqlRows.Scan(
			&row.HistoryID,
			&row.Identifier,
			&row.ChangeType,
			&row.PayloadType,
			&row.Snapshot,
			&row.Diff,
			&row.Deleted,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.OperationAt,
			&row.ContentHash,
			&row.PayloadHash,
			&row.RowHash,
		); err != nil {
			return nil, common.NewInternalServerError("HISTORY-RESTORE-SCANCHAIN " + err.Error())
		}
		rows = append(rows, row)
	}
	if err = sqlRows.Err(); err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-ROWS " + err.Error())
	}
	if len(rows) == 0 {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-EMPTYCHAIN no history rows found for restore")
	}
	return rows, nil
}

func restoreVersionChainRows(rows []storedHistoryRow) (map[int64]latestVersion, error) {
	var snapshot map[string]any
	rowsSinceSnapshot := 0
	versions := make(map[int64]latestVersion, len(rows))
	for _, row := range rows {
		var err error
		switch row.PayloadType {
		case PayloadTypeSnapshot:
			snapshot, err = restoreSnapshotPayload(row)
			rowsSinceSnapshot = 1
		case PayloadTypeDiff:
			if snapshot == nil {
				return nil, common.NewInternalServerError("HISTORY-RESTORE-DIFFWITHOUTBASE diff row has no preceding snapshot")
			}
			snapshot, err = restoreDiffPayload(snapshot, row)
			rowsSinceSnapshot++
		default:
			return nil, common.NewInternalServerError("HISTORY-RESTORE-PAYLOADTYPE unsupported payload type '" + row.PayloadType + "'")
		}
		if err != nil {
			return nil, err
		}
		if err = verifyCanonicalHash(snapshot, row.ContentHash, "HISTORY-RESTORE-CONTENTHASH"); err != nil {
			return nil, err
		}
		versionSnapshot, cloneErr := cloneSnapshotMap(snapshot)
		if cloneErr != nil {
			return nil, cloneErr
		}
		versions[row.HistoryID] = latestVersion{
			historyID:         row.HistoryID,
			snapshot:          versionSnapshot,
			deleted:           row.Deleted,
			rowHash:           row.RowHash.String,
			rowsSinceSnapshot: rowsSinceSnapshot,
		}
	}
	if len(versions) == 0 {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-NOLATEST no latest row found after restore")
	}
	return versions, nil
}

func restoreSnapshotPayload(row storedHistoryRow) (map[string]any, error) {
	if !row.Snapshot.Valid {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-MISSINGSNAPSHOT snapshot payload is missing")
	}
	var snapshot map[string]any
	if err := decodeJSONPreservingNumbers([]byte(row.Snapshot.String), &snapshot); err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-UNMARSHALSNAPSHOT " + err.Error())
	}
	if err := verifyCanonicalHash(snapshot, row.PayloadHash, "HISTORY-RESTORE-SNAPSHOTPAYLOADHASH"); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func restoreDiffPayload(base map[string]any, row storedHistoryRow) (map[string]any, error) {
	if !row.Diff.Valid {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-MISSINGDIFF diff payload is missing")
	}
	var patch []map[string]any
	if err := decodeJSONPreservingNumbers([]byte(row.Diff.String), &patch); err != nil {
		return nil, common.NewInternalServerError("HISTORY-RESTORE-UNMARSHALDIFF " + err.Error())
	}
	if err := verifyCanonicalHash(patch, row.PayloadHash, "HISTORY-RESTORE-DIFFPAYLOADHASH"); err != nil {
		return nil, err
	}
	snapshot, err := ApplyJSONPatch(base, patch)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func verifyCanonicalHash(value any, expected sql.NullString, errorCode string) error {
	if !expected.Valid || strings.TrimSpace(expected.String) == "" {
		return common.NewInternalServerError(errorCode + " expected hash is missing")
	}
	actual, err := CanonicalJSONHash(value)
	if err != nil {
		return common.NewInternalServerError(errorCode + " " + err.Error())
	}
	if actual != expected.String {
		return common.NewInternalServerError(errorCode + " stored hash does not match reconstructed payload")
	}
	return nil
}

// FilterRecentRows fills a filtered page without exposing empty intermediary raw pages.
func FilterRecentRows(limit int32, cursor string, fetch RecentRowsFetcher, include RecentRowPredicate) ([]Row, string, error) {
	limit, err := normalizeRecentChangesLimit(limit)
	if err != nil {
		return nil, "", err
	}
	if fetch == nil {
		return nil, "", common.NewErrBadRequest("HISTORY-FILTERRECENT-NILFETCH fetch function must not be nil")
	}
	if include == nil {
		return nil, "", common.NewErrBadRequest("HISTORY-FILTERRECENT-NILPREDICATE predicate must not be nil")
	}

	result := make([]Row, 0, int(limit))
	scanCursor := cursor
	for {
		rows, nextCursor, fetchErr := fetch(limit, scanCursor)
		if fetchErr != nil {
			return nil, "", fetchErr
		}
		for index, row := range rows {
			included, includeErr := include(row)
			if includeErr != nil {
				return nil, "", includeErr
			}
			if !included {
				continue
			}
			result = append(result, row)
			if len(result) == int(limit) {
				if index < len(rows)-1 || nextCursor != "" {
					return result, strconv.FormatInt(row.HistoryID, 10), nil
				}
				return result, "", nil
			}
		}
		if nextCursor == "" {
			return result, "", nil
		}
		if nextCursor == scanCursor {
			return nil, "", common.NewInternalServerError("HISTORY-FILTERRECENT-CURSOR raw history cursor did not advance")
		}
		scanCursor = nextCursor
	}
}

func timestampOrOperationTime(timestamp string, operationTime time.Time) string {
	if strings.TrimSpace(timestamp) != "" {
		return timestamp
	}
	return operationTime.UTC().Format(time.RFC3339Nano)
}

func normalizeRecentChangesLimit(limit int32) (int32, error) {
	if limit <= 0 {
		return DefaultRecentChangesLimit, nil
	}
	if limit > MaxRecentChangesLimit {
		return 0, common.NewErrBadRequest("HISTORY-RECENT-LIMIT limit must not exceed " + strconv.FormatInt(int64(MaxRecentChangesLimit), 10))
	}
	return limit, nil
}

func parseCursor(cursor string) (int64, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil || value < 0 {
		return 0, common.NewErrBadRequest("HISTORY-CURSOR-INVALID cursor must be a non-negative history id")
	}
	return value, nil
}

func administrationTimestamps(snapshot map[string]any) (string, string) {
	administration, ok := snapshot["administration"].(map[string]any)
	if !ok {
		return "", ""
	}
	created, _ := administration["createdAt"].(string)
	updated, _ := administration["updatedAt"].(string)
	return created, updated
}

func nullableINET(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return goqu.L("?::inet", value)
}

func nullableTimestamp(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return parsed.UTC()
}

func historyPayloadTable(table string) (string, error) {
	payloadTable, ok := payloadTables[table]
	if !ok {
		return "", common.NewInternalServerError("HISTORY-PAYLOADTABLE-UNSUPPORTED unsupported history table '" + table + "'")
	}
	return payloadTable, nil
}

// CanonicalJSONHash returns a SHA-256 hash over deterministic JSON.
func CanonicalJSONHash(value any) (string, error) {
	canonical, err := CanonicalJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// ComputeHistoryRowHash returns the hash-chain row hash for a history event.
func ComputeHistoryRowHash(event ChangeEvent) (string, error) {
	return CanonicalJSONHash(map[string]any{
		"entityType":    event.EntityType,
		"identifier":    event.Identifier,
		"changeType":    event.ChangeType,
		"timestamp":     event.Timestamp.UTC().Format(time.RFC3339Nano),
		"deleted":       event.Deleted,
		"payloadType":   event.PayloadType,
		"contentHash":   event.ContentHash,
		"payloadHash":   event.PayloadHash,
		"previousHash":  event.PreviousHash,
		"requestId":     event.RequestID,
		"correlationId": event.CorrelationID,
		"actorSubject":  event.ActorSubject,
		"actorIssuer":   event.ActorIssuer,
		"clientId":      event.ClientID,
		"endpoint":      event.Endpoint,
		"httpMethod":    event.HTTPMethod,
	})
}

// CanonicalJSON encodes JSON values with stable object-key ordering.
func CanonicalJSON(value any) ([]byte, error) {
	normalized, err := normalizeJSONValue(value)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err = writeCanonicalJSON(&out, normalized); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func normalizeJSONValue(value any) (any, error) {
	switch typed := value.(type) {
	case json.RawMessage:
		return decodeNormalizedJSON(typed)
	case []byte:
		return decodeNormalizedJSON(typed)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return decodeNormalizedJSON(encoded)
	}
}

func decodeNormalizedJSON(raw []byte) (any, error) {
	var normalized any
	if err := decodeJSONPreservingNumbers(raw, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func decodeJSONPreservingNumbers(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func writeCanonicalJSON(out *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case map[string]any:
		return writeCanonicalObject(out, typed)
	case []any:
		return writeCanonicalArray(out, typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Errorf("marshal scalar: %w", err)
		}
		return writeCanonicalBytes(out, encoded)
	}
}

func writeCanonicalObject(out *bytes.Buffer, value map[string]any) error {
	if err := writeCanonicalByte(out, '{'); err != nil {
		return err
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for index, key := range keys {
		if index > 0 {
			if err := writeCanonicalByte(out, ','); err != nil {
				return err
			}
		}
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return fmt.Errorf("marshal object key: %w", err)
		}
		if err = writeCanonicalBytes(out, keyJSON); err != nil {
			return err
		}
		if err = writeCanonicalByte(out, ':'); err != nil {
			return err
		}
		if err = writeCanonicalJSON(out, value[key]); err != nil {
			return err
		}
	}
	return writeCanonicalByte(out, '}')
}

func writeCanonicalArray(out *bytes.Buffer, value []any) error {
	if err := writeCanonicalByte(out, '['); err != nil {
		return err
	}
	for index, item := range value {
		if index > 0 {
			if err := writeCanonicalByte(out, ','); err != nil {
				return err
			}
		}
		if err := writeCanonicalJSON(out, item); err != nil {
			return err
		}
	}
	return writeCanonicalByte(out, ']')
}

func writeCanonicalBytes(out *bytes.Buffer, value []byte) error {
	if _, err := out.Write(value); err != nil {
		return fmt.Errorf("write canonical json: %w", err)
	}
	return nil
}

func writeCanonicalByte(out *bytes.Buffer, value byte) error {
	if err := out.WriteByte(value); err != nil {
		return fmt.Errorf("write canonical json byte: %w", err)
	}
	return nil
}
