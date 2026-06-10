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

// ValidatePolicy materializes a staged policy and persists its refreshed rule rows.
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
			return nil
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
func (r *Repository) ActivatePolicy(ctx context.Context, versionID int64) (*PolicyVersion, error) {
	actor := actorFromContext(ctx, "ActivatePolicy", managementBasePath)
	var activated PolicyVersion
	err := common.ExecuteInTransaction(r.db, "ABACPOLICY-ACTIVATE-BEGINTX", "ABACPOLICY-ACTIVATE-COMMIT", func(tx *sql.Tx) error {
		var activateErr error
		activated, activateErr = r.activateVersionTx(ctx, tx, versionID, actor)
		return activateErr
	})
	if err != nil {
		return nil, err
	}
	if refreshErr := r.RefreshActiveModel(ctx); refreshErr != nil {
		return nil, refreshErr
	}
	return &activated, nil
}

// RejectPolicy marks a staged version as rejected without deleting its audit trail.
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
		if eventErr := r.insertPolicyEventTx(ctx, tx, versionID, version.PolicyID, "RejectPolicy", version.SourceType, version.SourceRef, version.ConfiguredPolicyHash, version.MaterializedPolicyHash, actor); eventErr != nil {
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

func (r *Repository) activateVersionTx(ctx context.Context, tx *sql.Tx, versionID int64, actor auditActor) (PolicyVersion, error) {
	version, err := r.loadPolicyVersion(ctx, tx, versionID)
	if err != nil {
		return PolicyVersion{}, err
	}
	if version.Status != StatusStaged {
		return PolicyVersion{}, common.NewErrConflict("ABACPOLICY-ACTIVATE-IMMUTABLE only staged policy versions can be activated")
	}
	materialized, err := auth.MaterializeABACPolicy(version.ConfiguredPolicyJSON, r.apiRouter, r.basePath)
	if err != nil {
		return PolicyVersion{}, common.NewErrBadRequest("ABACPOLICY-ACTIVATE-MATERIALIZE " + err.Error())
	}
	version, err = r.replaceMaterializationTx(ctx, tx, versionID, materialized, actor)
	if err != nil {
		return PolicyVersion{}, err
	}
	rules, err := r.loadPolicyRules(ctx, tx, versionID)
	if err != nil {
		return PolicyVersion{}, err
	}
	artifactRef, err := writeActivationEvidenceTx(ctx, tx, version, rules, actor)
	if err != nil {
		return PolicyVersion{}, err
	}
	if err = r.supersedeActiveTx(ctx, tx, versionID, actor); err != nil {
		return PolicyVersion{}, err
	}
	if err = r.markActiveTx(ctx, tx, versionID, artifactRef, actor); err != nil {
		return PolicyVersion{}, err
	}
	if err = r.insertPolicyEventTx(ctx, tx, versionID, version.PolicyID, "ActivatePolicy", version.SourceType, version.SourceRef, version.ConfiguredPolicyHash, version.MaterializedPolicyHash, actor); err != nil {
		return PolicyVersion{}, err
	}
	return r.loadPolicyVersion(ctx, tx, versionID)
}

func (r *Repository) supersedeActiveTx(ctx context.Context, tx *sql.Tx, activatingVersionID int64, actor auditActor) error {
	query, args, err := goqu.Update(tablePolicyVersions).
		Set(goqu.Record{
			"status":        StatusSuperseded,
			"superseded_at": time.Now().UTC(),
			"updated_at":    time.Now().UTC(),
		}).
		Where(
			goqu.C("service_scope").Eq(r.serviceScope),
			goqu.C("status").Eq(StatusActive),
			goqu.C("version_id").Neq(activatingVersionID),
		).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError("ABACPOLICY-ACTIVATE-BUILDSUPERSEDE " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("ABACPOLICY-ACTIVATE-SUPERSEDE " + err.Error())
	}
	return r.insertPolicyEventTx(ctx, tx, activatingVersionID, "", "SupersedePreviousActivePolicy", SourceTypeAPI, "", "", "", actor)
}

func (r *Repository) markActiveTx(ctx context.Context, tx *sql.Tx, versionID int64, artifactRef []byte, actor auditActor) error {
	now := time.Now().UTC()
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
