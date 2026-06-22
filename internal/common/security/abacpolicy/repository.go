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

package abacpolicy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type dbQueryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type policyEvent struct {
	VersionID                    int64
	PolicyID                     string
	Operation                    string
	SourceType                   string
	SourceRef                    string
	BeforePolicyHash             string
	AfterPolicyHash              string
	BeforeMaterializedPolicyHash string
	AfterMaterializedPolicyHash  string
	Details                      map[string]any
	Actor                        auditActor
}

// Repository stores service-scoped ABAC policy versions in PostgreSQL.
//
// A repository owns one service scope, such as "aasenvironmentservice". It
// persists immutable policy versions and materialized rule rows for that scope,
// and keeps the currently active policy compiled in memory for request-time
// authorization. Draft operations update only staged versions; authorization
// changes are published from committed activations, while explicit reload
// failures clear the cache so middleware fails closed.
type Repository struct {
	db           *sql.DB
	serviceScope string
	apiRouter    *chi.Mux
	basePath     string
	activeModel  atomic.Value
}

// NewRepository creates a service-local PostgreSQL ABAC policy repository.
//
// Parameters:
//   - db: PostgreSQL handle used for policy versions, rules, events, and
//     evidence catalog rows.
//   - serviceScope: Stable service identifier used to isolate active policies
//     when multiple BaSyx services share one database.
//   - apiRouter: Service router whose routes are used while materializing ABAC
//     objects and route-to-rights mappings.
//   - basePath: Optional server context path stripped before ABAC route matching.
//
// Returns:
//   - *Repository: Repository with an empty runtime cache.
//   - error: Bad request error when required dependencies are missing.
func NewRepository(db *sql.DB, serviceScope string, apiRouter *chi.Mux, basePath string) (*Repository, error) {
	if db == nil {
		return nil, common.NewErrBadRequest("ABACPOLICY-NEW-NILDB database handle must not be nil")
	}
	serviceScope = strings.TrimSpace(serviceScope)
	if serviceScope == "" {
		return nil, common.NewErrBadRequest("ABACPOLICY-NEW-EMPTYSCOPE service scope must not be empty")
	}
	if apiRouter == nil {
		return nil, common.NewErrBadRequest("ABACPOLICY-NEW-NILROUTER router must not be nil")
	}
	return &Repository{
		db:           db,
		serviceScope: serviceScope,
		apiRouter:    apiRouter,
		basePath:     basePath,
	}, nil
}

// ActiveAccessModel returns the compiled policy used by request middleware.
//
// The value is read from an atomic cache and is safe for concurrent requests.
// Nil means the repository has not loaded an active policy; ABAC middleware
// treats that as fail-closed.
func (r *Repository) ActiveAccessModel() *auth.AccessModel {
	if r == nil {
		return nil
	}
	value := r.activeModel.Load()
	if value == nil {
		return nil
	}
	model, _ := value.(*auth.AccessModel)
	return model
}

// RefreshActiveModel reloads the active materialized policy from PostgreSQL.
//
// The method rebuilds the in-memory AccessModel from durable rule rows and
// atomically publishes it to request middleware. It returns a service-unavailable
// error when no active policy exists so callers can fail closed during startup.
// If the reload fails, any previously cached model is cleared before returning.
func (r *Repository) RefreshActiveModel(ctx context.Context) error {
	active, err := r.loadActivePolicy(ctx, r.db)
	if err != nil {
		r.clearActiveModel()
		return err
	}
	r.publishActivePolicy(active)
	return nil
}

func (r *Repository) publishActivePolicy(active activePolicy) {
	if r == nil {
		return
	}
	r.activeModel.Store(active.model)
}

func (r *Repository) clearActiveModel() {
	if r == nil {
		return
	}
	r.activeModel.Store((*auth.AccessModel)(nil))
}

// HasActivePolicy reports whether this service scope has an active DB policy.
//
// It is used by startup import mode "if_missing" to decide whether the
// configured file should be imported or the existing database policy should be
// loaded unchanged.
func (r *Repository) HasActivePolicy(ctx context.Context) (bool, error) {
	_, found, err := r.loadActivePolicyVersion(ctx, r.db)
	return found, err
}

// ImportStartupPolicy imports and activates the configured startup policy file.
//
// The caller should pass a system audit context, because startup imports are
// preconfiguration events rather than end-user HTTP requests. If the active
// policy already matches the canonical configured hash or legacy raw file hash,
// the existing version is reused and only a reuse event is recorded.
func (r *Repository) ImportStartupPolicy(ctx context.Context, raw []byte, sourceRef string) (*PolicyVersion, error) {
	materialized, err := auth.MaterializeABACPolicy(raw, r.apiRouter, r.basePath)
	if err != nil {
		return nil, common.NewErrBadRequest("ABACPOLICY-STARTUP-MATERIALIZE " + err.Error())
	}

	actor := actorFromContext(ctx, "ABACPreconfiguration", "startup:abac-preconfiguration")
	var activated PolicyVersion
	var activatedPolicy activePolicy
	err = common.ExecuteInTransaction(r.db, "ABACPOLICY-STARTUP-BEGINTX", "ABACPOLICY-STARTUP-COMMIT", func(tx *sql.Tx) error {
		active, found, activeErr := r.loadActivePolicyVersion(ctx, tx)
		if activeErr != nil {
			return activeErr
		}
		if found && sameStartupPolicy(active, materialized) {
			activated = active
			loadedActive, loadErr := r.loadActivePolicy(ctx, tx)
			if loadErr != nil {
				return loadErr
			}
			activatedPolicy = loadedActive
			return r.insertPolicyEventTx(ctx, tx, policyEvent{
				VersionID:                    active.VersionID,
				PolicyID:                     active.PolicyID,
				Operation:                    "StartupReuse",
				SourceType:                   SourceTypeFile,
				SourceRef:                    sourceRef,
				BeforePolicyHash:             active.ConfiguredPolicyHash,
				AfterPolicyHash:              active.ConfiguredPolicyHash,
				BeforeMaterializedPolicyHash: active.MaterializedPolicyHash,
				AfterMaterializedPolicyHash:  active.MaterializedPolicyHash,
				Details: map[string]any{
					"reason": "configured startup policy already active",
				},
				Actor: actor,
			})
		}
		version, createErr := r.createPolicyVersionTx(ctx, tx, materialized, StatusStaged, SourceTypeFile, sourceRef, actor)
		if createErr != nil {
			return createErr
		}
		activated, activatedPolicy, createErr = r.activateVersionTx(ctx, tx, version.VersionID, actor)
		return createErr
	})
	if err != nil {
		return nil, err
	}
	r.publishActivePolicy(activatedPolicy)
	return &activated, nil
}

// ImportPolicy creates a staged version from API-provided policy JSON.
//
// The policy is validated and materialized before storage so administrators can
// inspect the same rule rows that activation would use. The returned version is
// not used by authorization until ActivatePolicy promotes it.
func (r *Repository) ImportPolicy(ctx context.Context, raw []byte, sourceRef string) (*PolicyVersion, error) {
	materialized, err := auth.MaterializeABACPolicy(raw, r.apiRouter, r.basePath)
	if err != nil {
		return nil, common.NewErrBadRequest("ABACPOLICY-IMPORT-MATERIALIZE " + err.Error())
	}
	actor := actorFromContext(ctx, "ImportPolicy", managementBasePath)
	var version PolicyVersion
	err = common.ExecuteInTransaction(r.db, "ABACPOLICY-IMPORT-BEGINTX", "ABACPOLICY-IMPORT-COMMIT", func(tx *sql.Tx) error {
		created, createErr := r.createPolicyVersionTx(ctx, tx, materialized, StatusStaged, SourceTypeAPI, sourceRef, actor)
		version = created
		return createErr
	})
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// ImportPolicyAndActivate imports and activates a policy in one transaction.
//
// The policy is first stored as a staged version inside the transaction, then
// validated, evidenced when configured, and activated by the same transaction.
// If activation fails, the imported staged version is rolled back as well so
// callers do not receive a partial import for activate=true requests.
func (r *Repository) ImportPolicyAndActivate(ctx context.Context, raw []byte, sourceRef string) (*PolicyVersion, error) {
	materialized, err := auth.MaterializeABACPolicy(raw, r.apiRouter, r.basePath)
	if err != nil {
		return nil, common.NewErrBadRequest("ABACPOLICY-IMPORTACTIVATE-MATERIALIZE " + err.Error())
	}
	actor := actorFromContext(ctx, "ImportPolicyAndActivate", managementBasePath)
	var activated PolicyVersion
	var activatedPolicy activePolicy
	err = common.ExecuteInTransaction(r.db, "ABACPOLICY-IMPORTACTIVATE-BEGINTX", "ABACPOLICY-IMPORTACTIVATE-COMMIT", func(tx *sql.Tx) error {
		version, createErr := r.createPolicyVersionTx(ctx, tx, materialized, StatusStaged, SourceTypeAPI, sourceRef, actor)
		if createErr != nil {
			return createErr
		}
		var activateErr error
		activated, activatedPolicy, activateErr = r.activateVersionTx(ctx, tx, version.VersionID, actor)
		return activateErr
	})
	if err != nil {
		return nil, err
	}
	r.publishActivePolicy(activatedPolicy)
	return &activated, nil
}

// ListPolicyVersions returns policy versions for this service scope.
//
// Results are ordered newest first and include configured and materialized JSON
// so the protected management API can present a complete administrative view.
func (r *Repository) ListPolicyVersions(ctx context.Context) ([]PolicyVersion, error) {
	query, args, err := goqu.From(tablePolicyVersions).
		Select(policyVersionColumns()...).
		Where(goqu.C("service_scope").Eq(r.serviceScope)).
		Order(goqu.C("created_at").Desc(), goqu.C("version_id").Desc()).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-LIST-BUILD " + err.Error())
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-LIST-QUERY " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()
	versions := make([]PolicyVersion, 0)
	for rows.Next() {
		version, scanErr := scanPolicyVersion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		versions = append(versions, version)
	}
	if err = rows.Err(); err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-LIST-ROWS " + err.Error())
	}
	return versions, nil
}

// GetPolicyVersion loads one policy version by internal version id.
//
// The lookup is scoped to the repository service scope. Active, superseded,
// staged, and rejected versions are all inspectable through this method.
func (r *Repository) GetPolicyVersion(ctx context.Context, versionID int64) (PolicyVersion, error) {
	return r.loadPolicyVersion(ctx, r.db, versionID)
}

// GetActivePolicyVersion loads the active policy version for this service.
//
// The lookup reads PostgreSQL rather than the in-memory evaluator cache so
// operators always inspect the durable active policy state. A missing active
// version is reported as service unavailable because ABAC-enabled services fail
// closed without one.
func (r *Repository) GetActivePolicyVersion(ctx context.Context) (PolicyVersion, error) {
	return r.loadRequiredActivePolicyVersion(ctx, r.db)
}

// ListRules loads materialized rules for one policy version in configured order.
//
// The stable 1-based rule order is security relevant because it participates in
// matched_rule_id generation and is preserved by the evaluator.
func (r *Repository) ListRules(ctx context.Context, versionID int64) ([]PolicyRule, error) {
	return r.loadPolicyRules(ctx, r.db, versionID)
}

// ListActiveRules loads materialized rules for the active policy version.
//
// Rules are returned in configured order and are suitable for administrative
// inspection of the currently effective evaluator inputs.
func (r *Repository) ListActiveRules(ctx context.Context) ([]PolicyRule, error) {
	version, err := r.loadRequiredActivePolicyVersion(ctx, r.db)
	if err != nil {
		return nil, err
	}
	return r.loadPolicyRules(ctx, r.db, version.VersionID)
}

// LookupRuleByPolicyAndMatchedID resolves history audit identifiers to a rule.
//
// policyID may be the canonical configured policy hash used by DB-backed
// versions or a legacy raw file hash alias stored on file imports. The
// matchedRuleID must be the deterministic value recorded in mutation history.
func (r *Repository) LookupRuleByPolicyAndMatchedID(ctx context.Context, policyID string, matchedRuleID string) (PolicyRule, error) {
	versionID, err := r.resolvePolicyVersionID(ctx, r.db, policyID)
	if err != nil {
		return PolicyRule{}, err
	}
	query, args, err := goqu.From(tablePolicyRules).
		Select(policyRuleColumns()...).
		Where(
			goqu.C("service_scope").Eq(r.serviceScope),
			goqu.C("version_id").Eq(versionID),
			goqu.C("matched_rule_id").Eq(strings.TrimSpace(matchedRuleID)),
		).
		Limit(1).
		ToSQL()
	if err != nil {
		return PolicyRule{}, common.NewInternalServerError("ABACPOLICY-LOOKUPRULE-BUILD " + err.Error())
	}
	rule, err := scanPolicyRule(r.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return PolicyRule{}, common.NewErrNotFound("ABACPOLICY-LOOKUPRULE-NOTFOUND rule not found")
	}
	return rule, err
}

func (r *Repository) resolvePolicyVersionID(ctx context.Context, queryer dbQueryer, policyID string) (int64, error) {
	policyID = strings.TrimSpace(policyID)
	query, args, err := goqu.From(tablePolicyVersions).
		Select(goqu.C("version_id")).
		Where(
			goqu.C("service_scope").Eq(r.serviceScope),
			goqu.Or(
				goqu.C("policy_id").Eq(policyID),
				goqu.C("raw_policy_hash").Eq(policyID),
			),
		).
		Order(goqu.C("version_id").Desc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return 0, common.NewInternalServerError("ABACPOLICY-RESOLVEPOLICY-BUILD " + err.Error())
	}
	var versionID int64
	err = queryer.QueryRowContext(ctx, query, args...).Scan(&versionID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, common.NewErrNotFound("ABACPOLICY-RESOLVEPOLICY-NOTFOUND policy version not found")
	}
	if err != nil {
		return 0, common.NewInternalServerError("ABACPOLICY-RESOLVEPOLICY-QUERY " + err.Error())
	}
	return versionID, nil
}

func (r *Repository) loadActivePolicy(ctx context.Context, queryer dbQueryer) (activePolicy, error) {
	version, err := r.loadRequiredActivePolicyVersion(ctx, queryer)
	if err != nil {
		return activePolicy{}, err
	}
	rules, err := r.loadPolicyRules(ctx, queryer, version.VersionID)
	if err != nil {
		return activePolicy{}, err
	}
	model, err := r.accessModelFromPolicyRules(version.PolicyID, rules)
	if err != nil {
		return activePolicy{}, common.NewInternalServerError("ABACPOLICY-ACTIVE-BUILDMODEL " + err.Error())
	}
	return activePolicy{version: version, rules: rules, model: model}, nil
}

func (r *Repository) accessModelFromPolicyRules(policyID string, rules []PolicyRule) (*auth.AccessModel, error) {
	return auth.AccessModelFromMaterializedRules(policyID, materializedRulesFromPolicyRules(rules), r.apiRouter, r.basePath)
}

func (r *Repository) loadRequiredActivePolicyVersion(ctx context.Context, queryer dbQueryer) (PolicyVersion, error) {
	version, found, err := r.loadActivePolicyVersion(ctx, queryer)
	if err != nil {
		return PolicyVersion{}, err
	}
	if !found {
		return PolicyVersion{}, common.NewErrServiceUnavailable("ABACPOLICY-ACTIVE-NOTFOUND no active ABAC policy exists")
	}
	return version, nil
}

func (r *Repository) loadActivePolicyVersion(ctx context.Context, queryer dbQueryer) (PolicyVersion, bool, error) {
	query, args, err := goqu.From(tablePolicyVersions).
		Select(policyVersionColumns()...).
		Where(
			goqu.C("service_scope").Eq(r.serviceScope),
			goqu.C("status").Eq(StatusActive),
		).
		Limit(1).
		ToSQL()
	if err != nil {
		return PolicyVersion{}, false, common.NewInternalServerError("ABACPOLICY-ACTIVE-BUILD " + err.Error())
	}
	version, err := scanPolicyVersion(queryer.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return PolicyVersion{}, false, nil
	}
	if err != nil {
		return PolicyVersion{}, false, err
	}
	return version, true, nil
}

func (r *Repository) loadPolicyVersion(ctx context.Context, queryer dbQueryer, versionID int64) (PolicyVersion, error) {
	query, args, err := goqu.From(tablePolicyVersions).
		Select(policyVersionColumns()...).
		Where(
			goqu.C("service_scope").Eq(r.serviceScope),
			goqu.C("version_id").Eq(versionID),
		).
		Limit(1).
		ToSQL()
	if err != nil {
		return PolicyVersion{}, common.NewInternalServerError("ABACPOLICY-GET-BUILD " + err.Error())
	}
	version, err := scanPolicyVersion(queryer.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return PolicyVersion{}, common.NewErrNotFound("ABACPOLICY-GET-NOTFOUND policy version not found")
	}
	return version, err
}

func (r *Repository) loadPolicyRules(ctx context.Context, queryer dbQueryer, versionID int64) ([]PolicyRule, error) {
	query, args, err := goqu.From(tablePolicyRules).
		Select(policyRuleColumns()...).
		Where(
			goqu.C("service_scope").Eq(r.serviceScope),
			goqu.C("version_id").Eq(versionID),
		).
		Order(goqu.C("rule_index").Asc()).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-RULES-BUILD " + err.Error())
	}
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-RULES-QUERY " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()
	rules := make([]PolicyRule, 0)
	for rows.Next() {
		rule, scanErr := scanPolicyRule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		rules = append(rules, rule)
	}
	if err = rows.Err(); err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-RULES-ROWS " + err.Error())
	}
	return rules, nil
}

func (r *Repository) createPolicyVersionTx(
	ctx context.Context,
	tx *sql.Tx,
	materialized auth.MaterializedABACPolicy,
	status string,
	sourceType string,
	sourceRef string,
	actor auditActor,
) (PolicyVersion, error) {
	query, args, err := goqu.Insert(tablePolicyVersions).
		Rows(policyVersionInsertRecord(r.serviceScope, materialized, status, sourceType, sourceRef, actor)).
		Returning(goqu.C("version_id")).
		ToSQL()
	if err != nil {
		return PolicyVersion{}, common.NewInternalServerError("ABACPOLICY-CREATE-BUILD " + err.Error())
	}
	var versionID int64
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&versionID); err != nil {
		return PolicyVersion{}, common.NewInternalServerError("ABACPOLICY-CREATE-INSERT " + err.Error())
	}
	if err = r.insertRulesTx(ctx, tx, versionID, materialized, actor); err != nil {
		return PolicyVersion{}, err
	}
	if err = r.insertPolicyEventTx(ctx, tx, policyEvent{
		VersionID:                   versionID,
		PolicyID:                    materialized.PolicyID,
		Operation:                   "CreatePolicyVersion",
		SourceType:                  sourceType,
		SourceRef:                   sourceRef,
		AfterPolicyHash:             materialized.ConfiguredPolicyHash,
		AfterMaterializedPolicyHash: materialized.MaterializedPolicyHash,
		Details: map[string]any{
			"status": status,
		},
		Actor: actor,
	}); err != nil {
		return PolicyVersion{}, err
	}
	return r.loadPolicyVersion(ctx, tx, versionID)
}

func (r *Repository) replaceMaterializationTx(ctx context.Context, tx *sql.Tx, versionID int64, materialized auth.MaterializedABACPolicy, actor auditActor) (PolicyVersion, error) {
	before, err := r.loadPolicyVersion(ctx, tx, versionID)
	if err != nil {
		return PolicyVersion{}, err
	}
	if before.Status != StatusStaged {
		return PolicyVersion{}, common.NewErrConflict("ABACPOLICY-REPLACE-IMMUTABLE only staged policy versions are editable")
	}
	deleteQuery, deleteArgs, err := goqu.Delete(tablePolicyRules).
		Where(goqu.C("version_id").Eq(versionID)).
		ToSQL()
	if err != nil {
		return PolicyVersion{}, common.NewInternalServerError("ABACPOLICY-REPLACE-BUILDDELETE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, deleteQuery, deleteArgs...); err != nil {
		return PolicyVersion{}, common.NewInternalServerError("ABACPOLICY-REPLACE-DELETERULES " + err.Error())
	}
	update := goqu.Record{
		"policy_id":                materialized.PolicyID,
		"configured_policy_json":   jsonbParam(materialized.ConfiguredPolicyJSON),
		"configured_policy_hash":   materialized.ConfiguredPolicyHash,
		"raw_policy_hash":          nullableString(materialized.RawPolicyHash),
		"materialized_policy_json": jsonbParam(materialized.MaterializedPolicyJSON),
		"materialized_policy_hash": materialized.MaterializedPolicyHash,
		"updated_at":               time.Now().UTC(),
		"updated_by_subject":       nullableString(actor.Subject),
		"updated_by_issuer":        nullableString(actor.Issuer),
		"updated_by_client_id":     nullableString(actor.ClientID),
	}
	query, args, err := goqu.Update(tablePolicyVersions).
		Set(update).
		Where(
			goqu.C("service_scope").Eq(r.serviceScope),
			goqu.C("version_id").Eq(versionID),
		).
		ToSQL()
	if err != nil {
		return PolicyVersion{}, common.NewInternalServerError("ABACPOLICY-REPLACE-BUILDUPDATE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return PolicyVersion{}, common.NewInternalServerError("ABACPOLICY-REPLACE-UPDATE " + err.Error())
	}
	if err = r.insertRulesTx(ctx, tx, versionID, materialized, actor); err != nil {
		return PolicyVersion{}, err
	}
	if err = r.insertPolicyEventTx(ctx, tx, policyEvent{
		VersionID:                    versionID,
		PolicyID:                     materialized.PolicyID,
		Operation:                    actor.Operation,
		SourceType:                   before.SourceType,
		SourceRef:                    before.SourceRef,
		BeforePolicyHash:             before.ConfiguredPolicyHash,
		AfterPolicyHash:              materialized.ConfiguredPolicyHash,
		BeforeMaterializedPolicyHash: before.MaterializedPolicyHash,
		AfterMaterializedPolicyHash:  materialized.MaterializedPolicyHash,
		Actor:                        actor,
	}); err != nil {
		return PolicyVersion{}, err
	}
	return r.loadPolicyVersion(ctx, tx, versionID)
}

func (r *Repository) insertRulesTx(ctx context.Context, tx *sql.Tx, versionID int64, materialized auth.MaterializedABACPolicy, actor auditActor) error {
	if len(materialized.Rules) == 0 {
		return common.NewErrBadRequest("ABACPOLICY-RULES-EMPTY policy must contain at least one rule")
	}
	rows := make([]goqu.Record, 0, len(materialized.Rules))
	for _, rule := range materialized.Rules {
		rows = append(rows, policyRuleInsertRecord(r.serviceScope, versionID, materialized.PolicyID, rule, actor))
	}
	query, args, err := goqu.Insert(tablePolicyRules).Rows(rows).ToSQL()
	if err != nil {
		return common.NewInternalServerError("ABACPOLICY-RULES-BUILDINSERT " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("ABACPOLICY-RULES-INSERT " + err.Error())
	}
	return nil
}

func (r *Repository) insertPolicyEventTx(ctx context.Context, tx *sql.Tx, event policyEvent) error {
	if strings.TrimSpace(event.Operation) == "" {
		event.Operation = "ABACPolicyChange"
	}
	detailsJSON, err := policyEventDetailsJSON(event.Details)
	if err != nil {
		return err
	}
	row := goqu.Record{
		"version_id":                      event.VersionID,
		"service_scope":                   r.serviceScope,
		"policy_id":                       nullableString(event.PolicyID),
		"operation":                       event.Operation,
		"endpoint":                        nullableString(event.Actor.Endpoint),
		"actor_subject":                   nullableString(event.Actor.Subject),
		"actor_issuer":                    nullableString(event.Actor.Issuer),
		"actor_client_id":                 nullableString(event.Actor.ClientID),
		"request_id":                      nullableString(event.Actor.RequestID),
		"correlation_id":                  nullableString(event.Actor.CorrelationID),
		"source_type":                     nullableString(event.SourceType),
		"source_ref":                      nullableString(event.SourceRef),
		"before_policy_hash":              nullableString(event.BeforePolicyHash),
		"after_policy_hash":               nullableString(event.AfterPolicyHash),
		"before_materialized_policy_hash": nullableString(event.BeforeMaterializedPolicyHash),
		"after_materialized_policy_hash":  nullableString(event.AfterMaterializedPolicyHash),
		"details_json":                    detailsJSON,
	}
	query, args, err := goqu.Insert(tablePolicyEvents).Rows(row).ToSQL()
	if err != nil {
		return common.NewInternalServerError("ABACPOLICY-EVENT-BUILD " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("ABACPOLICY-EVENT-INSERT " + err.Error())
	}
	return nil
}

func policyEventDetailsJSON(details map[string]any) (any, error) {
	if len(details) == 0 {
		return nil, nil
	}
	raw, err := common.CanonicalJSON(details)
	if err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-EVENT-DETAILS " + err.Error())
	}
	return jsonbParam(raw), nil
}

func sameStartupPolicy(active PolicyVersion, materialized auth.MaterializedABACPolicy) bool {
	if active.PolicyID == materialized.PolicyID {
		return true
	}
	return strings.TrimSpace(active.RawPolicyHash) != "" && active.RawPolicyHash == materialized.RawPolicyHash
}

func policyVersionInsertRecord(serviceScope string, materialized auth.MaterializedABACPolicy, status string, sourceType string, sourceRef string, actor auditActor) goqu.Record {
	return goqu.Record{
		"service_scope":            serviceScope,
		"policy_id":                materialized.PolicyID,
		"status":                   status,
		"source_type":              sourceType,
		"source_ref":               nullableString(sourceRef),
		"configured_policy_json":   jsonbParam(materialized.ConfiguredPolicyJSON),
		"configured_policy_hash":   materialized.ConfiguredPolicyHash,
		"raw_policy_hash":          nullableString(materialized.RawPolicyHash),
		"materialized_policy_json": jsonbParam(materialized.MaterializedPolicyJSON),
		"materialized_policy_hash": materialized.MaterializedPolicyHash,
		"created_by_subject":       nullableString(actor.Subject),
		"created_by_issuer":        nullableString(actor.Issuer),
		"created_by_client_id":     nullableString(actor.ClientID),
	}
}

func policyRuleInsertRecord(serviceScope string, versionID int64, policyID string, rule auth.MaterializedABACRule, actor auditActor) goqu.Record {
	return goqu.Record{
		"version_id":             versionID,
		"policy_id":              policyID,
		"service_scope":          serviceScope,
		"rule_index":             rule.RuleIndex,
		"matched_rule_id":        rule.MatchedRuleID,
		"configured_rule_json":   jsonbParam(rule.ConfiguredRuleJSON),
		"materialized_rule_json": jsonbParam(rule.MaterializedRuleJSON),
		"acl_json":               jsonbParam(rule.ACLJSON),
		"attributes_json":        jsonbParam(rule.AttributesJSON),
		"objects_json":           jsonbParam(rule.ObjectsJSON),
		"formula_json":           jsonbParam(rule.FormulaJSON),
		"filters_json":           jsonbParam(rule.FiltersJSON),
		"access":                 rule.Access,
		"rights":                 textArrayParam(rule.Rights),
		"rule_hash":              rule.ConfiguredRuleHash,
		"materialized_rule_hash": rule.MaterializedRuleHash,
		"created_by_subject":     nullableString(actor.Subject),
		"created_by_issuer":      nullableString(actor.Issuer),
		"created_by_client_id":   nullableString(actor.ClientID),
	}
}

func textArrayParam(values []string) any {
	encoded, err := json.Marshal(values)
	if err != nil {
		encoded = []byte("[]")
	}
	return goqu.L("ARRAY(SELECT jsonb_array_elements_text(?::jsonb))", string(encoded))
}

func materializedRulesFromPolicyRules(rules []PolicyRule) []auth.MaterializedABACRule {
	out := make([]auth.MaterializedABACRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, auth.MaterializedABACRule{
			RuleIndex:            rule.RuleIndex,
			MatchedRuleID:        rule.MatchedRuleID,
			ConfiguredRuleJSON:   []byte(rule.ConfiguredRuleJSON),
			MaterializedRuleJSON: []byte(rule.MaterializedRuleJSON),
			ACLJSON:              []byte(rule.ACLJSON),
			AttributesJSON:       []byte(rule.AttributesJSON),
			ObjectsJSON:          []byte(rule.ObjectsJSON),
			FormulaJSON:          []byte(rule.FormulaJSON),
			FiltersJSON:          []byte(rule.FiltersJSON),
			Access:               rule.Access,
			Rights:               append([]string(nil), rule.Rights...),
			ConfiguredRuleHash:   rule.RuleHash,
			MaterializedRuleHash: rule.MaterializedRuleHash,
		})
	}
	return out
}

func actorFromContext(ctx context.Context, operation string, endpoint string) auditActor {
	audit := history.FromContext(ctx)
	return auditActor{
		Subject:       audit.ActorSubject,
		Issuer:        audit.ActorIssuer,
		ClientID:      audit.ClientID,
		RequestID:     audit.RequestID,
		CorrelationID: audit.CorrelationID,
		Operation:     firstNonEmpty(operation, audit.Operation),
		Endpoint:      firstNonEmpty(endpoint, audit.Endpoint),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func jsonbParam(raw []byte) any {
	if len(raw) == 0 {
		return goqu.L("?::jsonb", "null")
	}
	return goqu.L("?::jsonb", string(raw))
}

func policyVersionColumns() []any {
	return []any{
		goqu.C("version_id"),
		goqu.C("service_scope"),
		goqu.C("policy_id"),
		goqu.C("status"),
		goqu.C("source_type"),
		goqu.C("source_ref"),
		goqu.C("configured_policy_json"),
		goqu.C("configured_policy_hash"),
		goqu.C("raw_policy_hash"),
		goqu.C("materialized_policy_json"),
		goqu.C("materialized_policy_hash"),
		goqu.C("created_at"),
		goqu.C("created_by_subject"),
		goqu.C("created_by_issuer"),
		goqu.C("created_by_client_id"),
		goqu.C("updated_at"),
		goqu.C("updated_by_subject"),
		goqu.C("updated_by_issuer"),
		goqu.C("updated_by_client_id"),
		goqu.C("activated_at"),
		goqu.C("activated_by_subject"),
		goqu.C("activated_by_issuer"),
		goqu.C("activated_by_client_id"),
		goqu.C("superseded_at"),
		goqu.L("artifact_ref::text"),
	}
}

func policyRuleColumns() []any {
	return []any{
		goqu.C("rule_id"),
		goqu.C("version_id"),
		goqu.C("policy_id"),
		goqu.C("service_scope"),
		goqu.C("rule_index"),
		goqu.C("matched_rule_id"),
		goqu.C("configured_rule_json"),
		goqu.C("materialized_rule_json"),
		goqu.C("acl_json"),
		goqu.C("attributes_json"),
		goqu.C("objects_json"),
		goqu.C("formula_json"),
		goqu.C("filters_json"),
		goqu.C("access"),
		goqu.C("rights"),
		goqu.C("rule_hash"),
		goqu.C("materialized_rule_hash"),
		goqu.C("created_at"),
		goqu.C("created_by_subject"),
		goqu.C("created_by_issuer"),
		goqu.C("created_by_client_id"),
	}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPolicyVersion(scanner rowScanner) (PolicyVersion, error) {
	var version PolicyVersion
	var configured []byte
	var materialized []byte
	var nullable nullableStrings
	if err := scanner.Scan(
		&version.VersionID,
		&version.ServiceScope,
		&version.PolicyID,
		&version.Status,
		&version.SourceType,
		&nullable.sourceRef,
		&configured,
		&version.ConfiguredPolicyHash,
		&nullable.rawPolicyHash,
		&materialized,
		&version.MaterializedPolicyHash,
		&version.CreatedAt,
		&nullable.createdBySubject,
		&nullable.createdByIssuer,
		&nullable.createdByClientID,
		&version.UpdatedAt,
		&nullable.updatedBySubject,
		&nullable.updatedByIssuer,
		&nullable.updatedByClientID,
		&version.ActivatedAt,
		&nullable.activatedBySubject,
		&nullable.activatedByIssuer,
		&nullable.activatedByClientID,
		&version.SupersededAt,
		&nullable.artifactRef,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PolicyVersion{}, err
		}
		return PolicyVersion{}, common.NewInternalServerError("ABACPOLICY-SCAN-VERSION " + err.Error())
	}
	version.SourceRef = nullable.sourceRef.String
	version.ConfiguredPolicyJSON = append(json.RawMessage(nil), configured...)
	version.RawPolicyHash = nullable.rawPolicyHash.String
	version.MaterializedPolicyJSON = append(json.RawMessage(nil), materialized...)
	version.CreatedBySubject = nullable.createdBySubject.String
	version.CreatedByIssuer = nullable.createdByIssuer.String
	version.CreatedByClientID = nullable.createdByClientID.String
	version.UpdatedBySubject = nullable.updatedBySubject.String
	version.UpdatedByIssuer = nullable.updatedByIssuer.String
	version.UpdatedByClientID = nullable.updatedByClientID.String
	version.ActivatedBySubject = nullable.activatedBySubject.String
	version.ActivatedByIssuer = nullable.activatedByIssuer.String
	version.ActivatedByClientID = nullable.activatedByClientID.String
	if nullable.artifactRef.Valid {
		version.ArtifactRef = json.RawMessage(nullable.artifactRef.String)
	}
	return version, nil
}

func scanPolicyRule(scanner rowScanner) (PolicyRule, error) {
	var rule PolicyRule
	var rights []string
	var createdBySubject sql.NullString
	var createdByIssuer sql.NullString
	var createdByClientID sql.NullString
	if err := scanner.Scan(
		&rule.RuleID,
		&rule.VersionID,
		&rule.PolicyID,
		&rule.ServiceScope,
		&rule.RuleIndex,
		&rule.MatchedRuleID,
		&rule.ConfiguredRuleJSON,
		&rule.MaterializedRuleJSON,
		&rule.ACLJSON,
		&rule.AttributesJSON,
		&rule.ObjectsJSON,
		&rule.FormulaJSON,
		&rule.FiltersJSON,
		&rule.Access,
		pgtype.NewMap().SQLScanner(&rights),
		&rule.RuleHash,
		&rule.MaterializedRuleHash,
		&rule.CreatedAt,
		&createdBySubject,
		&createdByIssuer,
		&createdByClientID,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PolicyRule{}, err
		}
		return PolicyRule{}, common.NewInternalServerError("ABACPOLICY-SCAN-RULE " + err.Error())
	}
	rule.Rights = rights
	rule.CreatedBySubject = createdBySubject.String
	rule.CreatedByIssuer = createdByIssuer.String
	rule.CreatedByClientID = createdByClientID.String
	return rule, nil
}
