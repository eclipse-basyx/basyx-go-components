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

	// ChangeCreated marks a created entity version.
	ChangeCreated = "Created"
	// ChangeUpdated marks an updated entity version.
	ChangeUpdated = "Updated"
	// ChangeDeleted marks a deleted entity version.
	ChangeDeleted = "Deleted"
)

var (
	configMu     sync.RWMutex
	activeConfig = Config{
		Mode:              ModeAPI,
		RetentionDays:     0,
		Immutability:      ImmutabilityNone,
		AuditIdentityMode: AuditIdentityMinimal,
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
	ContentHash   string
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

// HistoryWriter consumes change events for history storage.
type HistoryWriter interface {
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
	Mode              string
	RetentionDays     int
	Immutability      string
	AuditIdentityMode string
}

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

// AppendVersionTx appends an immutable snapshot event for identifier.
func AppendVersionTx(ctx context.Context, tx *sql.Tx, table string, identifier string, changeType string, snapshot map[string]any, deleted bool) error {
	if ActiveConfig().Mode == ModeOff {
		return nil
	}
	if tx == nil {
		return common.NewInternalServerError("HISTORY-APPEND-NILTX transaction must not be nil")
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return common.NewErrBadRequest("HISTORY-APPEND-EMPTYID identifier must not be empty")
	}

	now := time.Now().UTC()
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-MARSHAL " + err.Error())
	}
	contentHash, err := CanonicalJSONHash(snapshot)
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-CONTENTHASH " + err.Error())
	}
	previousHash, err := latestRowHashTx(ctx, tx, table, identifier)
	if err != nil {
		return err
	}
	audit := FromContext(ctx)
	event := ChangeEvent{
		EntityType:    table,
		Identifier:    identifier,
		ChangeType:    changeType,
		Timestamp:     now,
		Snapshot:      snapshot,
		Deleted:       deleted,
		ContentHash:   contentHash,
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
		"snapshot":                       goqu.L("?::jsonb", string(snapshotJSON)),
		"deleted":                        deleted,
		"valid_from":                     now,
		"operation_time":                 now,
		"administration_created_at_text": createdAt,
		"administration_updated_at_text": updatedAt,
		"content_hash":                   contentHash,
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
	}).ToSQL()
	if err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-BUILDINSERT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, insertQuery, insertArgs...); err != nil {
		return common.NewInternalServerError("HISTORY-APPEND-EXECINSERT " + err.Error())
	}
	return nil
}

func normalizeConfig(cfg Config) Config {
	cfg.Mode = normalizeHistoryMode(cfg.Mode)
	cfg.Immutability = normalizeImmutability(cfg.Immutability)
	cfg.AuditIdentityMode = normalizeAuditIdentityMode(cfg.AuditIdentityMode)
	if cfg.RetentionDays < 0 {
		cfg.RetentionDays = 0
	}
	return cfg
}

func normalizeHistoryMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ModeAPI:
		return ModeAPI
	case ModeOff:
		return ModeOff
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
	case "", AuditIdentityMinimal:
		return AuditIdentityMinimal
	case AuditIdentityNone:
		return AuditIdentityNone
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
	query, args, err := goqu.From(table).
		Select(goqu.L("snapshot::text"), goqu.C("deleted")).
		Where(
			goqu.C("identifier").Eq(identifier),
			goqu.C("valid_from").Lte(at.UTC()),
		).
		Order(goqu.C("valid_from").Desc(), goqu.C("history_id").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("HISTORY-GET-BUILDSQL " + err.Error())
	}
	var snapshotText string
	var deleted bool
	if err = db.QueryRowContext(ctx, query, args...).Scan(&snapshotText, &deleted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.NewErrNotFound("HISTORY-GET-NOTFOUND no historical version found")
		}
		return nil, common.NewInternalServerError("HISTORY-GET-EXECSQL " + err.Error())
	}
	if deleted {
		return nil, common.NewErrNotFound("HISTORY-GET-DELETED historical version is deleted at the requested date")
	}
	var snapshot map[string]any
	if err = json.Unmarshal([]byte(snapshotText), &snapshot); err != nil {
		return nil, common.NewInternalServerError("HISTORY-GET-UNMARSHAL " + err.Error())
	}
	return snapshot, nil
}

// RecentRows returns history rows after cursor, ordered by history id with one look-ahead row for pagination.
func RecentRows(ctx context.Context, db *sql.DB, table string, limit int32, cursor string, createdFrom time.Time, updatedFrom time.Time) ([]Row, string, error) {
	if db == nil {
		return nil, "", common.NewErrBadRequest("HISTORY-RECENT-NILDB database handle must not be nil")
	}
	if limit <= 0 {
		limit = 100
	}
	limitInt := int(limit)
	cursorID, err := parseCursor(cursor)
	if err != nil {
		return nil, "", err
	}

	query := goqu.From(table).
		Select(
			goqu.C("history_id"),
			goqu.C("identifier"),
			goqu.C("change_type"),
			goqu.L("snapshot::text"),
			goqu.C("deleted"),
			goqu.C("administration_created_at_text"),
			goqu.C("administration_updated_at_text"),
			goqu.C("operation_time"),
		).
		Order(goqu.C("history_id").Asc()).
		Limit(uint(limitInt + 1)) //nolint:gosec // limit is positive int32 and therefore safe on supported platforms.
	if cursorID > 0 {
		query = query.Where(goqu.C("history_id").Gt(cursorID))
	}
	if !createdFrom.IsZero() {
		query = query.Where(goqu.Or(
			goqu.C("operation_time").Gte(createdFrom.UTC()),
			goqu.C("administration_created_at_text").Gte(createdFrom.Format(time.RFC3339Nano)),
		))
	}
	if !updatedFrom.IsZero() {
		query = query.Where(goqu.Or(
			goqu.C("operation_time").Gte(updatedFrom.UTC()),
			goqu.C("administration_updated_at_text").Gte(updatedFrom.Format(time.RFC3339Nano)),
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
	for rows.Next() {
		var row Row
		var snapshotText string
		var created sql.NullString
		var updated sql.NullString
		if err = rows.Scan(&row.HistoryID, &row.Identifier, &row.ChangeType, &snapshotText, &row.Deleted, &created, &updated, &row.OperationAt); err != nil {
			return nil, "", common.NewInternalServerError("HISTORY-RECENT-SCAN " + err.Error())
		}
		if len(result) == limitInt {
			nextCursor = strconv.FormatInt(row.HistoryID, 10)
			break
		}
		if err = json.Unmarshal([]byte(snapshotText), &row.Snapshot); err != nil {
			return nil, "", common.NewInternalServerError("HISTORY-RECENT-UNMARSHAL " + err.Error())
		}
		row.CreatedAt = created.String
		row.UpdatedAt = updated.String
		result = append(result, row)
	}
	if err = rows.Err(); err != nil {
		return nil, "", common.NewInternalServerError("HISTORY-RECENT-ROWS " + err.Error())
	}
	return result, nextCursor, nil
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
		"contentHash":   event.ContentHash,
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
		var normalized any
		if err := json.Unmarshal(typed, &normalized); err != nil {
			return nil, err
		}
		return normalized, nil
	case []byte:
		var normalized any
		if err := json.Unmarshal(typed, &normalized); err != nil {
			return nil, err
		}
		return normalized, nil
	default:
		return value, nil
	}
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
		out.Write(encoded)
		return nil
	}
}

func writeCanonicalObject(out *bytes.Buffer, value map[string]any) error {
	out.WriteByte('{')
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for index, key := range keys {
		if index > 0 {
			out.WriteByte(',')
		}
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return fmt.Errorf("marshal object key: %w", err)
		}
		out.Write(keyJSON)
		out.WriteByte(':')
		if err = writeCanonicalJSON(out, value[key]); err != nil {
			return err
		}
	}
	out.WriteByte('}')
	return nil
}

func writeCanonicalArray(out *bytes.Buffer, value []any) error {
	out.WriteByte('[')
	for index, item := range value {
		if index > 0 {
			out.WriteByte(',')
		}
		if err := writeCanonicalJSON(out, item); err != nil {
			return err
		}
	}
	out.WriteByte(']')
	return nil
}
