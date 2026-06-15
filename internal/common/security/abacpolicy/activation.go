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
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// ValidatePolicy materializes a staged policy and persists refreshed rule rows.
//
// Validation never changes the active evaluator cache. A successful validation
// updates the staged version's canonical policy hashes and materialized rule
// rows. A failed validation records an audit event and returns Valid=false so
// callers can show rule errors without activating the draft.
func (r *Repository) ValidatePolicy(ctx context.Context, versionID int64) (ValidationResult, error) {
	actor := actorFromContext(ctx, "ValidatePolicy", managementBasePath)
	var result ValidationResult
	err := common.ExecuteInTransaction(r.db, "ABACPOLICY-VALIDATE-BEGINTX", "ABACPOLICY-VALIDATE-COMMIT", func(tx *sql.Tx) error {
		version, loadErr := r.loadPolicyVersion(ctx, tx, versionID)
		if loadErr != nil {
			return loadErr
		}
		if version.Status != StatusStaged {
			return common.NewErrConflict("ABACPOLICY-VALIDATE-IMMUTABLE only staged policy versions can be validated")
		}
		materialized, materializeErr := auth.MaterializeABACPolicy(version.ConfiguredPolicyJSON, r.apiRouter, r.basePath)
		if materializeErr != nil {
			result = ValidationResult{Valid: false, Error: materializeErr.Error()}
			return r.insertPolicyEventTx(ctx, tx, policyEvent{
				VersionID:                    version.VersionID,
				PolicyID:                     version.PolicyID,
				Operation:                    "ValidatePolicyFailed",
				SourceType:                   version.SourceType,
				SourceRef:                    version.SourceRef,
				BeforePolicyHash:             version.ConfiguredPolicyHash,
				AfterPolicyHash:              version.ConfiguredPolicyHash,
				BeforeMaterializedPolicyHash: version.MaterializedPolicyHash,
				AfterMaterializedPolicyHash:  version.MaterializedPolicyHash,
				Details: map[string]any{
					"valid": false,
					"error": materializeErr.Error(),
				},
				Actor: actor,
			})
		}
		updated, replaceErr := r.replaceMaterializationTx(ctx, tx, versionID, materialized, actor)
		if replaceErr != nil {
			return replaceErr
		}
		result = ValidationResult{
			Valid:                  true,
			PolicyID:               updated.PolicyID,
			MaterializedPolicyHash: updated.MaterializedPolicyHash,
		}
		return nil
	})
	if err != nil {
		return ValidationResult{}, err
	}
	return result, nil
}

// ActivatePolicy atomically promotes one staged policy version to active.
//
// The database transaction validates the staged policy, writes required WORM
// evidence, supersedes the previous active version, marks the selected version
// active, and records policy events. After commit, the repository publishes the
// already materialized active model from that transaction so activation does not
// depend on a second database read.
func (r *Repository) ActivatePolicy(ctx context.Context, versionID int64) (*PolicyVersion, error) {
	actor := actorFromContext(ctx, "ActivatePolicy", managementBasePath)
	var activated PolicyVersion
	var activatedPolicy activePolicy
	err := common.ExecuteInTransaction(r.db, "ABACPOLICY-ACTIVATE-BEGINTX", "ABACPOLICY-ACTIVATE-COMMIT", func(tx *sql.Tx) error {
		var activateErr error
		activated, activatedPolicy, activateErr = r.activateVersionTx(ctx, tx, versionID, actor)
		return activateErr
	})
	if err != nil {
		return nil, err
	}
	r.publishActivePolicy(activatedPolicy)
	return &activated, nil
}

// RejectPolicy marks a staged version as rejected without deleting it.
//
// Rejected versions remain inspectable for audit and change-control purposes.
// Only staged versions can be rejected; active and superseded policy versions
// stay immutable.
func (r *Repository) RejectPolicy(ctx context.Context, versionID int64) (*PolicyVersion, error) {
	actor := actorFromContext(ctx, "RejectPolicy", managementBasePath)
	var rejected PolicyVersion
	err := common.ExecuteInTransaction(r.db, "ABACPOLICY-REJECT-BEGINTX", "ABACPOLICY-REJECT-COMMIT", func(tx *sql.Tx) error {
		version, loadErr := r.loadPolicyVersion(ctx, tx, versionID)
		if loadErr != nil {
			return loadErr
		}
		if version.Status != StatusStaged {
			return common.NewErrConflict("ABACPOLICY-REJECT-IMMUTABLE only staged policy versions can be rejected")
		}
		query, args, buildErr := goqu.Update(tablePolicyVersions).
			Set(goqu.Record{"status": StatusRejected, "updated_at": time.Now().UTC()}).
			Where(goqu.C("service_scope").Eq(r.serviceScope), goqu.C("version_id").Eq(versionID)).
			ToSQL()
		if buildErr != nil {
			return common.NewInternalServerError("ABACPOLICY-REJECT-BUILD " + buildErr.Error())
		}
		if _, execErr := tx.ExecContext(ctx, query, args...); execErr != nil {
			return common.NewInternalServerError("ABACPOLICY-REJECT-UPDATE " + execErr.Error())
		}
		if eventErr := r.insertPolicyEventTx(ctx, tx, policyEvent{
			VersionID:                    versionID,
			PolicyID:                     version.PolicyID,
			Operation:                    "RejectPolicy",
			SourceType:                   version.SourceType,
			SourceRef:                    version.SourceRef,
			BeforePolicyHash:             version.ConfiguredPolicyHash,
			AfterPolicyHash:              version.ConfiguredPolicyHash,
			BeforeMaterializedPolicyHash: version.MaterializedPolicyHash,
			AfterMaterializedPolicyHash:  version.MaterializedPolicyHash,
			Details: map[string]any{
				"status": StatusRejected,
			},
			Actor: actor,
		}); eventErr != nil {
			return eventErr
		}
		var readErr error
		rejected, readErr = r.loadPolicyVersion(ctx, tx, versionID)
		return readErr
	})
	if err != nil {
		return nil, err
	}
	return &rejected, nil
}

func (r *Repository) activateVersionTx(ctx context.Context, tx *sql.Tx, versionID int64, actor auditActor) (PolicyVersion, activePolicy, error) {
	version, err := r.loadPolicyVersion(ctx, tx, versionID)
	if err != nil {
		return PolicyVersion{}, activePolicy{}, err
	}
	if version.Status != StatusStaged {
		return PolicyVersion{}, activePolicy{}, common.NewErrConflict("ABACPOLICY-ACTIVATE-IMMUTABLE only staged policy versions can be activated")
	}
	materialized, err := auth.MaterializeABACPolicy(version.ConfiguredPolicyJSON, r.apiRouter, r.basePath)
	if err != nil {
		return PolicyVersion{}, activePolicy{}, common.NewErrBadRequest("ABACPOLICY-ACTIVATE-MATERIALIZE " + err.Error())
	}
	version, err = r.replaceMaterializationTx(ctx, tx, versionID, materialized, actor)
	if err != nil {
		return PolicyVersion{}, activePolicy{}, err
	}
	rules, err := r.loadPolicyRules(ctx, tx, versionID)
	if err != nil {
		return PolicyVersion{}, activePolicy{}, err
	}
	model, err := r.accessModelFromPolicyRules(version.PolicyID, rules)
	if err != nil {
		return PolicyVersion{}, activePolicy{}, common.NewInternalServerError("ABACPOLICY-ACTIVATE-BUILDMODEL " + err.Error())
	}
	activatedAt := time.Now().UTC()
	evidenceVersion := version
	evidenceVersion.Status = StatusActive
	evidenceVersion.ActivatedAt = &activatedAt
	evidenceVersion.ActivatedBySubject = actor.Subject
	evidenceVersion.ActivatedByIssuer = actor.Issuer
	evidenceVersion.ActivatedByClientID = actor.ClientID
	artifactRef, err := writeActivationEvidenceTx(ctx, tx, evidenceVersion, rules, actor)
	if err != nil {
		return PolicyVersion{}, activePolicy{}, err
	}
	if err = r.supersedeActiveTx(ctx, tx, versionID, actor); err != nil {
		return PolicyVersion{}, activePolicy{}, err
	}
	if err = r.markActiveTx(ctx, tx, versionID, artifactRef, actor, activatedAt); err != nil {
		return PolicyVersion{}, activePolicy{}, err
	}
	if err = r.insertPolicyEventTx(ctx, tx, policyEvent{
		VersionID:                    versionID,
		PolicyID:                     version.PolicyID,
		Operation:                    "ActivatePolicy",
		SourceType:                   version.SourceType,
		SourceRef:                    version.SourceRef,
		BeforePolicyHash:             version.ConfiguredPolicyHash,
		AfterPolicyHash:              version.ConfiguredPolicyHash,
		BeforeMaterializedPolicyHash: version.MaterializedPolicyHash,
		AfterMaterializedPolicyHash:  version.MaterializedPolicyHash,
		Details: map[string]any{
			"activated_at": activatedAt.Format(time.RFC3339Nano),
			"status":       StatusActive,
		},
		Actor: actor,
	}); err != nil {
		return PolicyVersion{}, activePolicy{}, err
	}
	activeVersion, err := r.loadPolicyVersion(ctx, tx, versionID)
	if err != nil {
		return PolicyVersion{}, activePolicy{}, err
	}
	return activeVersion, activePolicy{version: activeVersion, rules: rules, model: model}, nil
}

func (r *Repository) supersedeActiveTx(ctx context.Context, tx *sql.Tx, activatingVersionID int64, actor auditActor) error {
	active, found, err := r.loadActivePolicyVersion(ctx, tx)
	if err != nil {
		return err
	}
	if !found || active.VersionID == activatingVersionID {
		return nil
	}
	now := time.Now().UTC()
	query, args, err := goqu.Update(tablePolicyVersions).
		Set(goqu.Record{
			"status":        StatusSuperseded,
			"superseded_at": now,
			"updated_at":    now,
		}).
		Where(
			goqu.C("service_scope").Eq(r.serviceScope),
			goqu.C("version_id").Eq(active.VersionID),
		).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("ABACPOLICY-ACTIVATE-BUILDSUPERSEDE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("ABACPOLICY-ACTIVATE-SUPERSEDE " + err.Error())
	}
	return r.insertPolicyEventTx(ctx, tx, policyEvent{
		VersionID:                    active.VersionID,
		PolicyID:                     active.PolicyID,
		Operation:                    "SupersedePolicyVersion",
		SourceType:                   active.SourceType,
		SourceRef:                    active.SourceRef,
		BeforePolicyHash:             active.ConfiguredPolicyHash,
		AfterPolicyHash:              active.ConfiguredPolicyHash,
		BeforeMaterializedPolicyHash: active.MaterializedPolicyHash,
		AfterMaterializedPolicyHash:  active.MaterializedPolicyHash,
		Details: map[string]any{
			"activating_version_id": activatingVersionID,
			"status":                StatusSuperseded,
		},
		Actor: actor,
	})
}

func (r *Repository) markActiveTx(ctx context.Context, tx *sql.Tx, versionID int64, artifactRef []byte, actor auditActor, activatedAt time.Time) error {
	now := activatedAt.UTC()
	update := goqu.Record{
		"status":                 StatusActive,
		"activated_at":           now,
		"activated_by_subject":   nullableString(actor.Subject),
		"activated_by_issuer":    nullableString(actor.Issuer),
		"activated_by_client_id": nullableString(actor.ClientID),
		"updated_at":             now,
	}
	if len(artifactRef) > 0 {
		update["artifact_ref"] = jsonbParam(artifactRef)
	}
	query, args, err := goqu.Update(tablePolicyVersions).
		Set(update).
		Where(goqu.C("service_scope").Eq(r.serviceScope), goqu.C("version_id").Eq(versionID)).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("ABACPOLICY-ACTIVATE-BUILDACTIVE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("ABACPOLICY-ACTIVATE-MARKACTIVE " + err.Error())
	}
	return nil
}
