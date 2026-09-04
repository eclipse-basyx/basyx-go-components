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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/createprecheck"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelqueries "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/queries"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	"github.com/jackc/pgx/v5/pgconn"
)

// CreateSubmodel creates a new submodel and performs an ABAC re-check before commit when ABAC is enabled.
func (s *SubmodelDatabase) CreateSubmodel(ctx context.Context, submodel types.ISubmodel) (err error) {
	if err := s.verifySubmodel(submodel, "SMREPO-NEWSM-VERIFY"); err != nil {
		return err
	}

	tx, cu, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-STARTTX " + err.Error())
	}
	defer cu(&err)

	if err = s.createSubmodelInTransactionValidated(ctx, tx, submodel); err != nil {
		return err
	}

	if err = s.appendCreatedSubmodelHistoryTx(ctx, tx, submodel); err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-COMMIT " + err.Error())
	}

	return nil
}

// CreateSubmodelInTransaction creates a new submodel inside an existing transaction.
func (s *SubmodelDatabase) CreateSubmodelInTransaction(ctx context.Context, tx *sql.Tx, submodel types.ISubmodel) error {
	if tx == nil {
		return common.NewInternalServerError("SMREPO-NEWSM-NILTX transaction must not be nil")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-NEWSM-VERIFY"); err != nil {
		return err
	}

	if err := s.createSubmodelInTransactionValidated(ctx, tx, submodel); err != nil {
		return err
	}
	return s.appendCreatedSubmodelHistoryTx(ctx, tx, submodel)
}

func (s *SubmodelDatabase) createSubmodelInTransactionValidated(ctx context.Context, tx *sql.Tx, submodel types.ISubmodel) error {
	if err := history.LockMutationTx(ctx, tx, history.TableSubmodel, submodel.ID()); err != nil {
		return err
	}
	if err := s.ensureVisibleSubmodelCreateDoesNotExist(ctx, tx, submodel.ID()); err != nil {
		return err
	}

	err := s.createSubmodelInTransaction(ctx, tx, submodel)
	if err != nil {
		return err
	}

	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-NEWSM-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldEnforce {
		exists, visible, visErr := s.checkSubmodelVisibilityInTx(ctx, tx, submodel.ID())
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewInternalServerError("SMREPO-NEWSM-ABACCHECKMISSING created submodel not found before commit")
		}
		if !visible {
			return common.NewErrDenied("SMREPO-NEWSM-ABACDENIED Created submodel is not accessible under ABAC constraints")
		}
	}
	return nil
}

func (s *SubmodelDatabase) ensureVisibleSubmodelCreateDoesNotExist(ctx context.Context, tx *sql.Tx, submodelID string) error {
	if createprecheck.CanSkipExistenceCheck(ctx) {
		return nil
	}
	return createprecheck.EnsureVisibleCreate(
		ctx,
		func(context.Context) (bool, error) {
			_, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
			if err == nil {
				return true, nil
			}
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			return false, common.NewInternalServerError("SMREPO-NEWSM-CHKDUP-GETSMDATABASEID " + err.Error())
		},
		func(readCtx context.Context) error {
			exists, visible, err := s.checkSubmodelVisibilityInTx(readCtx, tx, submodelID)
			if err != nil {
				return err
			}
			if !exists {
				return common.NewErrNotFound("SMREPO-NEWSM-CHKDUP-NOTFOUND existing submodel not found")
			}
			if !visible {
				return common.NewErrDenied("SMREPO-NEWSM-CHKDUP-ABACDENIED existing submodel is not accessible under ABAC constraints")
			}
			return nil
		},
		"SMREPO-NEWSM-CREATE-CONFLICT submodel identifier already exists",
		"SMREPO-NEWSM-CHKDUP-ABACDENIED existing submodel is not accessible under ABAC constraints",
	)
}

func (s *SubmodelDatabase) createSubmodelInTransaction(ctx context.Context, tx *sql.Tx, submodel types.ISubmodel) error {
	ids, args, err := submodelqueries.BuildInsertSubmodelSQL(submodel)
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-INSERTSQL " + err.Error())
	}

	var submodelDBID int64
	if err := tx.QueryRow(ids, args...).Scan(&submodelDBID); err != nil {
		if mappedErr := mapCreateSubmodelInsertError(err); mappedErr != nil {
			return mappedErr
		}
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-EXECSQL " + err.Error())
	}

	jsonizedPayload, err := jsonizeSubmodelPayload(submodel)
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-JSON " + err.Error())
	}

	ids, args, err = submodelqueries.BuildInsertSubmodelPayloadSQL(
		submodelDBID,
		jsonizedPayload.description,
		jsonizedPayload.displayName,
		jsonizedPayload.administrativeInformation,
		jsonizedPayload.embeddedDataSpecification,
		jsonizedPayload.supplementalSemanticIDs,
		jsonizedPayload.extensions,
		jsonizedPayload.qualifiers,
	)
	if err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-PAYLOADSQL " + err.Error())
	}
	metadataBatch := &common.PostgreSQLBatch{}
	metadataBatch.AppendStatement(ids, args...)
	if err = metadataBatch.AppendContextReferences(
		submodelDBID,
		submodel.SupplementalSemanticIDs(),
		common.TblSubmodelSuppSemantic,
		common.ColSubmodelID,
	); err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-SUPPSEM " + err.Error())
	}
	if err = appendSubmodelSemanticIDCreate(metadataBatch, submodelDBID, submodel.SemanticID()); err != nil {
		return err
	}
	if len(submodel.SubmodelElements()) > 0 {
		submodelDatabaseID, conversionErr := submodelDatabaseIDAsInt(submodelDBID)
		if conversionErr != nil {
			return conversionErr
		}

		_, err = submodelelements.InsertSubmodelElementsForSubmodelDatabaseIDContext(ctx, s.db, submodelDatabaseID, submodel.SubmodelElements(), tx, nil, metadataBatch)
		if err != nil {
			return err
		}
		return nil
	}

	if err = common.ExecutePostgreSQLBatchInTransaction(ctx, tx, metadataBatch.Statements()); err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-EXECMETADATABATCH " + err.Error())
	}

	return nil
}

func appendSubmodelSemanticIDCreate(batch *common.PostgreSQLBatch, submodelDBID int64, semanticID types.IReference) error {
	if semanticID == nil {
		return nil
	}
	builders := []struct {
		code  string
		build func() (string, []any, error)
	}{
		{"SMREPO-NEWSM-CREATE-SEMIDREFSQL", func() (string, []any, error) {
			return submodelqueries.BuildInsertSubmodelSemanticIDReferenceSQL(submodelDBID, semanticID)
		}},
		{"SMREPO-NEWSM-CREATE-SEMIDKEYSQL", func() (string, []any, error) {
			return submodelqueries.BuildInsertSubmodelSemanticIDReferenceKeysSQL(submodelDBID, semanticID)
		}},
		{"SMREPO-NEWSM-CREATE-SEMIDPAYLOADSQL", func() (string, []any, error) {
			return submodelqueries.BuildInsertSubmodelSemanticIDReferencePayloadSQL(submodelDBID, semanticID)
		}},
	}
	for _, builder := range builders {
		query, args, err := builder.build()
		if err != nil {
			return common.NewInternalServerError(builder.code + " " + err.Error())
		}
		if query != "" {
			batch.AppendStatement(query, args...)
		}
	}
	return nil
}

func submodelDatabaseIDAsInt(submodelDBID int64) (int, error) {
	if submodelDBID <= 0 || submodelDBID > int64(int(^uint(0)>>1)) {
		return 0, common.NewInternalServerError("SMREPO-NEWSM-CREATESM-SMDATABASEIDRANGE Submodel database ID is outside the supported integer range")
	}
	return int(submodelDBID), nil
}

func (s *SubmodelDatabase) verifySubmodel(submodel types.ISubmodel, errorPrefix string) error {
	return gen.ValidateWithMode(
		s.verificationMode,
		errorPrefix,
		func(collector func(*verification.VerificationError) bool) {
			verification.VerifySubmodel(submodel, collector)
		},
		func(message string) error {
			return common.NewErrBadRequest(errorPrefix + " " + message)
		},
	)
}

// PatchSubmodel updates an existing submodel in the database with the provided submodel data
// while preserving ABAC visibility checks from ctx.
func (s *SubmodelDatabase) PatchSubmodel(ctx context.Context, submodelID string, submodel types.ISubmodel) error {
	if submodelID != submodel.ID() {
		return common.NewErrBadRequest("SMREPO-PATCHSM-IDMISMATCH Submodel ID in path and body do not match")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-PATCHSM-VERIFY"); err != nil {
		return err
	}

	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSM-STARTTX " + err.Error())
	}
	defer cleanup(&err)
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	if err = s.patchSubmodelInTransactionValidated(ctx, submodelID, tx, submodel); err != nil {
		return err
	}

	if err = s.appendCurrentSubmodelHistoryTx(ctx, tx, submodelID, previousSnapshot, history.ChangeUpdated); err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSM-COMMIT " + err.Error())
	}

	return nil
}

// PatchSubmodelInTransaction replaces an existing submodel and appends history in an existing transaction.
func (s *SubmodelDatabase) PatchSubmodelInTransaction(ctx context.Context, submodelID string, tx *sql.Tx, submodel types.ISubmodel) error {
	if tx == nil {
		return common.NewInternalServerError("SMREPO-PATCHSM-NILTX transaction must not be nil")
	}
	if submodelID != submodel.ID() {
		return common.NewErrBadRequest("SMREPO-PATCHSM-IDMISMATCH Submodel ID in path and body do not match")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-PATCHSM-VERIFY"); err != nil {
		return err
	}
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	if err = s.patchSubmodelInTransactionValidated(ctx, submodelID, tx, submodel); err != nil {
		return err
	}
	return s.appendCurrentSubmodelHistoryTx(ctx, tx, submodelID, previousSnapshot, history.ChangeUpdated)
}

func (s *SubmodelDatabase) patchSubmodelInTransactionValidated(ctx context.Context, submodelID string, tx *sql.Tx, submodel types.ISubmodel) error {
	_, err := s.replaceSubmodelInTransaction(ctx, tx, submodelID, submodel, true)
	if err != nil {
		return err
	}
	return nil
}

// PatchSubmodelMetadata updates a submodel without rewriting submodel elements
// while preserving ABAC visibility checks from ctx.
func (s *SubmodelDatabase) PatchSubmodelMetadata(ctx context.Context, submodelID string, submodel types.ISubmodel) error {
	if submodelID != submodel.ID() {
		return common.NewErrBadRequest("SMREPO-PATCHSMMETA-IDMISMATCH Submodel ID in path and body do not match")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-PATCHSMMETA-VERIFY"); err != nil {
		return err
	}

	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-STARTTX " + err.Error())
	}
	defer cleanup(&err)
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	if err = s.patchSubmodelMetadataInTransactionValidated(ctx, submodelID, tx, submodel); err != nil {
		return err
	}

	if err = s.appendSubmodelMetadataHistoryTx(ctx, tx, submodelID, previousSnapshot, submodel); err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-COMMIT " + err.Error())
	}

	return nil
}

// PatchSubmodelMetadataInTransaction updates submodel metadata and appends history in an existing transaction.
func (s *SubmodelDatabase) PatchSubmodelMetadataInTransaction(ctx context.Context, submodelID string, tx *sql.Tx, submodel types.ISubmodel) error {
	if tx == nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-NILTX transaction must not be nil")
	}
	if submodelID != submodel.ID() {
		return common.NewErrBadRequest("SMREPO-PATCHSMMETA-IDMISMATCH Submodel ID in path and body do not match")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-PATCHSMMETA-VERIFY"); err != nil {
		return err
	}
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	if err = s.patchSubmodelMetadataInTransactionValidated(ctx, submodelID, tx, submodel); err != nil {
		return err
	}
	return s.appendSubmodelMetadataHistoryTx(ctx, tx, submodelID, previousSnapshot, submodel)
}

func (s *SubmodelDatabase) patchSubmodelMetadataInTransactionValidated(_ context.Context, submodelID string, tx *sql.Tx, submodel types.ISubmodel) error {
	return s.patchSubmodelMetadataInTransaction(tx, submodelID, submodel)
}

// PutSubmodel creates or replaces a submodel and checks ABAC access on old/new state before commit when ABAC is enabled.
func (s *SubmodelDatabase) PutSubmodel(ctx context.Context, submodelID string, submodel types.ISubmodel) (bool, error) {
	if submodelID != submodel.ID() {
		return false, common.NewErrBadRequest("SMREPO-PUTSM-IDMISMATCH Submodel ID in path and body do not match")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-PUTSM-VERIFY"); err != nil {
		return false, err
	}

	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return false, common.NewInternalServerError("SMREPO-PUTSM-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	result, err := s.putSubmodelInTransaction(ctx, tx, submodelID, submodel)
	if err != nil {
		return false, err
	}

	err = tx.Commit()
	if err != nil {
		return false, common.NewInternalServerError("SMREPO-PUTSM-COMMIT " + err.Error())
	}

	return result.IsUpdate, nil
}

// PutSubmodelWithResult creates or replaces a submodel and reports whether persisted
// content changed, including the previous submodel state for diffing.
func (s *SubmodelDatabase) PutSubmodelWithResult(ctx context.Context, submodelID string, submodel types.ISubmodel) (PutSubmodelResult, error) {
	if submodelID != submodel.ID() {
		return PutSubmodelResult{}, common.NewErrBadRequest("SMREPO-PUTSM-IDMISMATCH Submodel ID in path and body do not match")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-PUTSM-VERIFY"); err != nil {
		return PutSubmodelResult{}, err
	}

	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return PutSubmodelResult{}, common.NewInternalServerError("SMREPO-PUTSM-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	result, err := s.putSubmodelInTransaction(ctx, tx, submodelID, submodel)
	if err != nil {
		return PutSubmodelResult{}, err
	}

	err = tx.Commit()
	if err != nil {
		return PutSubmodelResult{}, common.NewInternalServerError("SMREPO-PUTSM-COMMIT " + err.Error())
	}

	return result, nil
}

// PutSubmodelResult describes the repository mutation performed by a PUT.
type PutSubmodelResult struct {
	IsUpdate bool
	Changed  bool
	Previous types.ISubmodel
}

// PutSubmodelInTransaction creates or replaces a submodel within an existing transaction.
func (s *SubmodelDatabase) PutSubmodelInTransaction(ctx context.Context, tx *sql.Tx, submodelID string, submodel types.ISubmodel) (bool, error) {
	result, err := s.PutSubmodelInTransactionWithResult(ctx, tx, submodelID, submodel)
	return result.IsUpdate, err
}

// PutSubmodelInTransactionWithResult creates or replaces a submodel and reports whether persisted content changed.
func (s *SubmodelDatabase) PutSubmodelInTransactionWithResult(ctx context.Context, tx *sql.Tx, submodelID string, submodel types.ISubmodel) (PutSubmodelResult, error) {
	if tx == nil {
		return PutSubmodelResult{}, common.NewInternalServerError("SMREPO-PUTSM-NILTX transaction must not be nil")
	}
	if submodelID != submodel.ID() {
		return PutSubmodelResult{}, common.NewErrBadRequest("SMREPO-PUTSM-IDMISMATCH Submodel ID in path and body do not match")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-PUTSM-VERIFY"); err != nil {
		return PutSubmodelResult{}, err
	}

	return s.putSubmodelInTransaction(ctx, tx, submodelID, submodel)
}

func (s *SubmodelDatabase) putSubmodelInTransaction(ctx context.Context, tx *sql.Tx, submodelID string, submodel types.ISubmodel) (PutSubmodelResult, error) {
	if err := history.LockMutationTx(ctx, tx, history.TableSubmodel, submodelID); err != nil {
		return PutSubmodelResult{}, err
	}

	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseIDForUpdate(tx, submodelID)
	if errors.Is(err, sql.ErrNoRows) {
		if _, createErr := s.createSubmodelForPutTx(ctx, tx, submodel); createErr != nil {
			return PutSubmodelResult{}, createErr
		}
		return PutSubmodelResult{Changed: true}, nil
	}
	if err != nil {
		return PutSubmodelResult{}, common.NewInternalServerError("SMREPO-PUTSM-LOCKSUBMODEL " + err.Error())
	}
	return s.reconcileExistingSubmodelForPutTx(ctx, tx, submodelDatabaseID, submodel)
}

func (s *SubmodelDatabase) createSubmodelForPutTx(ctx context.Context, tx *sql.Tx, submodel types.ISubmodel) (bool, error) {
	readCtx, shouldEnforce, err := selectPutFormulaContext(ctx, false)
	if err != nil {
		return false, err
	}
	if err = s.createSubmodelInTransaction(ctx, tx, submodel); err != nil {
		return false, err
	}
	recordHistory := history.MutationRecordingEnabled()
	persisted := submodel
	if shouldEnforce || recordHistory {
		submodelDatabaseID, resolveErr := persistenceutils.GetSubmodelDatabaseID(tx, submodel.ID())
		if resolveErr != nil {
			return false, common.NewInternalServerError("SMREPO-PUTSM-RESOLVECREATED " + resolveErr.Error())
		}
		persisted, err = s.readPutSubmodelStateTx(readCtx, tx, submodelDatabaseID, submodel.ID())
		if err != nil {
			return false, mapPutReadbackError(err, shouldEnforce, false)
		}
	}
	if recordHistory {
		if err = s.appendSubmodelHistoryTx(ctx, tx, persisted, nil, history.ChangeCreated, false); err != nil {
			return false, err
		}
	}
	return false, nil
}

func (s *SubmodelDatabase) reconcileExistingSubmodelForPutTx(ctx context.Context, tx *sql.Tx, submodelDatabaseID int, submitted types.ISubmodel) (PutSubmodelResult, error) {
	readCtx, shouldEnforce, err := selectPutFormulaContext(ctx, true)
	if err != nil {
		return PutSubmodelResult{}, err
	}
	previous, err := s.readExistingPutSubmodelStateTx(readCtx, tx, submodelDatabaseID, submitted.ID())
	if err != nil {
		return PutSubmodelResult{}, mapPutReadbackError(err, shouldEnforce, true)
	}
	plan, err := s.buildSubmodelReconciliationPlan(previous, submitted)
	if err != nil {
		return PutSubmodelResult{}, err
	}
	if !plan.hasLiveMutation() {
		persisted := previous
		if shouldEnforce {
			persisted, err = s.readExistingPutSubmodelStateTx(readCtx, tx, submodelDatabaseID, submitted.ID())
			if err != nil {
				return PutSubmodelResult{}, mapPutReadbackError(err, true, false)
			}
		}
		if err = s.appendAcknowledgedSubmodelPutHistoryTx(ctx, tx, previous, persisted); err != nil {
			return PutSubmodelResult{}, err
		}
		return PutSubmodelResult{IsUpdate: true, Previous: previous}, nil
	}
	if err = s.executeSubmodelReconciliationTx(ctx, tx, submitted.ID(), plan); err != nil {
		return PutSubmodelResult{}, err
	}
	recordHistory := history.MutationRecordingEnabled()
	if !shouldEnforce && !recordHistory {
		return PutSubmodelResult{IsUpdate: true, Changed: true, Previous: previous}, nil
	}
	persisted, err := s.readExistingPutSubmodelStateTx(readCtx, tx, submodelDatabaseID, submitted.ID())
	if err != nil {
		return PutSubmodelResult{}, mapPutReadbackError(err, shouldEnforce, false)
	}
	if err = s.appendAcknowledgedSubmodelPutHistoryTx(ctx, tx, previous, persisted); err != nil {
		return PutSubmodelResult{}, err
	}
	return PutSubmodelResult{IsUpdate: true, Changed: true, Previous: previous}, nil
}

func forceGenericPlanForSubmodelPutTx(ctx context.Context, tx *sql.Tx) (func() error, error) {
	previousMode, err := currentPostgreSQLPlanCacheModeTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	const genericPlanMode = "force_generic_plan"
	if previousMode == genericPlanMode {
		return func() error { return nil }, nil
	}
	if err = setPostgreSQLPlanCacheModeTx(ctx, tx, genericPlanMode); err != nil {
		return nil, err
	}
	return func() error {
		return setPostgreSQLPlanCacheModeTx(ctx, tx, previousMode)
	}, nil
}

func currentPostgreSQLPlanCacheModeTx(ctx context.Context, tx *sql.Tx) (string, error) {
	query := goqu.Dialect("postgres").Select(
		goqu.Func("current_setting", common.PostgreSQLTextLiteral("plan_cache_mode")),
	)
	sqlQuery, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return "", common.NewInternalServerError("SMREPO-PUTSM-GETPLANMODE-BUILDQ " + err.Error())
	}
	var mode string
	if err = tx.QueryRowContext(ctx, sqlQuery, args...).Scan(&mode); err != nil {
		return "", common.NewInternalServerError("SMREPO-PUTSM-GETPLANMODE-EXECQ " + err.Error())
	}
	return mode, nil
}

func setPostgreSQLPlanCacheModeTx(ctx context.Context, tx *sql.Tx, mode string) error {
	query := goqu.Dialect("postgres").Select(
		goqu.Func(
			"set_config",
			common.PostgreSQLTextLiteral("plan_cache_mode"),
			common.PostgreSQLTextLiteral(mode),
			goqu.L("TRUE"),
		),
	)
	sqlQuery, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-PUTSM-SETPLANMODE-BUILDQ " + err.Error())
	}
	var configuredMode string
	if err = tx.QueryRowContext(ctx, sqlQuery, args...).Scan(&configuredMode); err != nil {
		return common.NewInternalServerError("SMREPO-PUTSM-SETPLANMODE-EXECQ " + err.Error())
	}
	if configuredMode != mode {
		return common.NewInternalServerError("SMREPO-PUTSM-SETPLANMODE-MISMATCH expected " + mode + " but PostgreSQL returned " + configuredMode)
	}
	return nil
}

func (s *SubmodelDatabase) appendAcknowledgedSubmodelPutHistoryTx(
	ctx context.Context,
	tx *sql.Tx,
	previous types.ISubmodel,
	persisted types.ISubmodel,
) error {
	if !history.MutationRecordingEnabled() {
		return nil
	}
	previousSnapshot, err := submodelToHistorySnapshot(previous)
	if err != nil {
		return err
	}
	return s.appendSubmodelHistoryTx(ctx, tx, persisted, previousSnapshot, history.ChangeUpdated, false)
}

func (s *SubmodelDatabase) executeSubmodelReconciliationTx(
	ctx context.Context,
	tx *sql.Tx,
	submodelID string,
	plan submodelReconciliationPlan,
) error {
	if !plan.hasLiveMutation() {
		return nil
	}
	deferSiblingConstraints := plan.requiresDeferredSiblingConstraints()
	if deferSiblingConstraints {
		if err := deferSubmodelElementReconciliationConstraints(ctx, tx); err != nil {
			return err
		}
	}
	if _, err := executeSubmodelReconciliationStatement(ctx, tx, submodelID, plan); err != nil {
		return err
	}
	if deferSiblingConstraints {
		return enforceSubmodelElementReconciliationConstraints(ctx, tx)
	}
	return nil
}

func selectPutFormulaContext(ctx context.Context, exists bool) (context.Context, bool, error) {
	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-PUTSM-SHOULDENFORCE")
	if enforceErr != nil {
		return nil, false, enforceErr
	}
	if !shouldEnforce {
		return auth.ContextWithoutQueryFilter(ctx), false, nil
	}
	return auth.SelectPutFormulaByExistence(ctx, exists), true, nil
}

func (s *SubmodelDatabase) readPutSubmodelStateTx(ctx context.Context, tx *sql.Tx, submodelDatabaseID int, submodelID string) (types.ISubmodel, error) {
	return s.readPutSubmodelStateWithElementsTx(
		ctx, tx, submodelDatabaseID, submodelID, readPutSubmodelElementsTx,
	)
}

func (s *SubmodelDatabase) readExistingPutSubmodelStateTx(ctx context.Context, tx *sql.Tx, submodelDatabaseID int, submodelID string) (types.ISubmodel, error) {
	return s.readPutSubmodelStateWithElementsTx(
		ctx, tx, submodelDatabaseID, submodelID, readPutSubmodelElementsWithGenericPlanTx,
	)
}

func (s *SubmodelDatabase) readPutSubmodelStateWithElementsTx(
	ctx context.Context,
	tx *sql.Tx,
	submodelDatabaseID int,
	submodelID string,
	readElements func(context.Context, *sql.Tx, int) ([]types.ISubmodelElement, error),
) (types.ISubmodel, error) {
	metadataCtx, err := contextWithoutFragmentFilters(ctx)
	if err != nil {
		return nil, err
	}
	submodel, err := s.getSubmodelMetadataByIDInTransaction(metadataCtx, tx, submodelID)
	if err != nil {
		return nil, err
	}
	elements, err := readElements(ctx, tx, submodelDatabaseID)
	if err != nil {
		return nil, err
	}
	submodel.SetSubmodelElements(elements)
	return submodel, nil
}

func readPutSubmodelElementsTx(ctx context.Context, tx *sql.Tx, submodelDatabaseID int) ([]types.ISubmodelElement, error) {
	return submodelelements.GetSubmodelElementsByDatabaseIDTxInPersistenceOrder(
		auth.ContextWithoutQueryFilter(ctx), tx, submodelDatabaseID, true,
	)
}

func readPutSubmodelElementsWithGenericPlanTx(ctx context.Context, tx *sql.Tx, submodelDatabaseID int) (elements []types.ISubmodelElement, err error) {
	restorePlanCacheMode, err := forceGenericPlanForSubmodelPutTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if restoreErr := restorePlanCacheMode(); err == nil && restoreErr != nil {
			elements = nil
			err = restoreErr
		}
	}()

	return readPutSubmodelElementsTx(ctx, tx, submodelDatabaseID)
}

func contextWithoutFragmentFilters(ctx context.Context) (context.Context, error) {
	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter == nil {
		return ctx, nil
	}
	cloned, err := auth.CloneQueryFilter(queryFilter)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-PUTSM-CLONEFILTER " + err.Error())
	}
	cloned.Filters = nil
	return auth.WithQueryFilter(ctx, cloned), nil
}

func mapPutReadbackError(err error, shouldEnforce bool, existingState bool) error {
	if !common.IsErrNotFound(err) {
		return err
	}
	if shouldEnforce {
		state := "Written"
		if existingState {
			state = "Existing"
		}
		return common.NewErrDenied("SMREPO-PUTSM-ABACDENIED " + state + " submodel is not accessible under ABAC constraints")
	}
	return common.NewInternalServerError("SMREPO-PUTSM-READBACKMISSING written submodel not found inside transaction")
}

// DeleteSubmodel deletes a submodel and checks ABAC access on the existing submodel before delete when ABAC is enabled.
func (s *SubmodelDatabase) DeleteSubmodel(ctx context.Context, submodelID string) (err error) {
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-STARTTX " + err.Error())
	}
	defer cleanup(&err)

	err = s.deleteSubmodelInTransaction(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-COMMIT " + err.Error())
	}

	return nil
}

// DeleteSubmodelInTransaction deletes a submodel within an existing transaction.
func (s *SubmodelDatabase) DeleteSubmodelInTransaction(ctx context.Context, tx *sql.Tx, submodelID string) error {
	if tx == nil {
		return common.NewInternalServerError("SMREPO-DELSM-NILTX transaction must not be nil")
	}

	return s.deleteSubmodelInTransaction(ctx, tx, submodelID)
}

func (s *SubmodelDatabase) deleteSubmodelInTransaction(ctx context.Context, tx *sql.Tx, submodelID string) error {
	if err := history.LockMutationTx(ctx, tx, history.TableSubmodel, submodelID); err != nil {
		return err
	}
	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-DELSM-SHOULDENFORCE")
	if enforceErr != nil {
		return enforceErr
	}
	if shouldEnforce {
		exists, visible, visErr := s.checkSubmodelVisibilityInTx(ctx, tx, submodelID)
		if visErr != nil {
			return visErr
		}
		if !exists {
			return common.NewErrNotFound("SMREPO-DELSM-NOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		if !visible {
			return common.NewErrDenied("SMREPO-DELSM-ABACDENIED Deleting this submodel is not allowed")
		}
	}

	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseIDForUpdate(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("SMREPO-DELSM-NOTFOUND Submodel with ID '" + submodelID + "' not found")
		}
		return common.NewInternalServerError("SMREPO-DELSM-GETSMDATABASEID " + err.Error())
	}
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	if err := history.AppendVersionTx(ctx, tx, history.TableSubmodel, submodelID, history.ChangeDeleted, previousSnapshot, map[string]any{"id": submodelID}, true); err != nil {
		return err
	}

	err = cleanupAndDeleteSubmodelByDatabaseID(ctx, tx, int64(submodelDatabaseID))
	if err != nil {
		return err
	}

	return nil
}

func cleanupAndDeleteSubmodelByDatabaseID(ctx context.Context, tx *sql.Tx, submodelDatabaseID int64) error {
	cleanupQuery, cleanupArgs, err := submodelqueries.BuildCleanupSubmodelLargeObjectsSQL(submodelDatabaseID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-BUILDUNLINKQUERY " + err.Error())
	}
	deleteQuery, deleteArgs, err := submodelqueries.BuildDeleteSubmodelByDatabaseIDSQL(submodelDatabaseID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-BUILDDELETESM " + err.Error())
	}
	batch := &common.PostgreSQLBatch{}
	batch.AppendStatement(cleanupQuery, cleanupArgs...)
	batch.AppendStatement(deleteQuery, deleteArgs...)
	if err = common.ExecutePostgreSQLBatchInTransaction(ctx, tx, batch.Statements()); err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-EXECBATCH " + err.Error())
	}
	return nil
}

func (s *SubmodelDatabase) replaceSubmodelInTransaction(ctx context.Context, tx *sql.Tx, submodelID string, submodel types.ISubmodel, requireExisting bool) (bool, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseIDForUpdate(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			if requireExisting {
				return false, common.NewErrNotFound("SMREPO-UPDSM-NOTFOUND Submodel with ID '" + submodelID + "' not found")
			}

			if createErr := s.createSubmodelInTransaction(ctx, tx, submodel); createErr != nil {
				return false, createErr
			}
			return false, nil
		}

		return false, common.NewInternalServerError("SMREPO-UPDSM-GETSMDATABASEID " + err.Error())
	}
	managedReferences, err := loadManagedFileReferencesForReplacementTx(tx, int64(submodelDatabaseID))
	if err != nil {
		return false, err
	}

	err = cleanupSubmodelLargeObjects(tx, int64(submodelDatabaseID))
	if err != nil {
		return false, err
	}

	err = deleteSubmodelByDatabaseID(tx, int64(submodelDatabaseID))
	if err != nil {
		return false, err
	}

	err = s.createSubmodelInTransaction(ctx, tx, submodel)
	if err != nil {
		return false, err
	}
	if err = restoreManagedFileReferencesAfterReplacementTx(tx, submodelID, managedReferences); err != nil {
		return false, err
	}

	return true, nil
}

func loadManagedFileReferencesForReplacementTx(tx *sql.Tx, submodelDatabaseID int64) ([]gen.ManagedFileReferenceForReplacement, error) {
	query, args, err := goqu.From(goqu.T("submodel_element").As("sme")).
		Join(goqu.T("file_element").As("fe"), goqu.On(goqu.I("fe.id").Eq(goqu.I("sme.id")))).
		Join(goqu.T(binarycontent.TableFileReference).As("fr"), goqu.On(goqu.I("fr.file_element_id").Eq(goqu.I("sme.id")))).
		Select("sme.idshort_path", "fe.value", "fr.binary_content_id", "fr.path_token", "fr.safe_file_name").
		Where(goqu.I("sme.submodel_id").Eq(submodelDatabaseID)).
		Order(goqu.I("fr.binary_content_id").Asc(), goqu.I("sme.idshort_path").Asc()).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-UPDSM-BUILDMANAGEDFILES " + err.Error())
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-UPDSM-QUERYMANAGEDFILES " + err.Error())
	}
	defer func() { _ = rows.Close() }()
	references := make([]gen.ManagedFileReferenceForReplacement, 0)
	for rows.Next() {
		var reference gen.ManagedFileReferenceForReplacement
		if err = rows.Scan(&reference.IDShortPath, &reference.ManagedPath, &reference.ContentID, &reference.PathToken, &reference.SafeFileName); err != nil {
			return nil, common.NewInternalServerError("SMREPO-UPDSM-SCANMANAGEDFILES " + err.Error())
		}
		references = append(references, reference)
	}
	if err = rows.Err(); err != nil {
		return nil, common.NewInternalServerError("SMREPO-UPDSM-ITERATEMANAGEDFILES " + err.Error())
	}
	return references, nil
}

func restoreManagedFileReferencesAfterReplacementTx(tx *sql.Tx, submodelID string, references []gen.ManagedFileReferenceForReplacement) error {
	if len(references) == 0 {
		return nil
	}
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-UPDSM-GETNEWSMDATABASEID " + err.Error())
	}
	byPath := make(map[string]gen.ManagedFileReferenceForReplacement, len(references))
	ownedManagedPaths := make(map[string]struct{}, len(references))
	for _, reference := range references {
		byPath[reference.IDShortPath] = reference
		ownedManagedPaths[reference.ManagedPath] = struct{}{}
	}
	query, args, err := goqu.From(goqu.T("submodel_element").As("sme")).
		Join(goqu.T("file_element").As("fe"), goqu.On(goqu.I("fe.id").Eq(goqu.I("sme.id")))).
		Select("sme.id", "sme.idshort_path", "fe.value").
		Where(goqu.I("sme.submodel_id").Eq(submodelDatabaseID)).
		Order(goqu.I("sme.idshort_path").Asc()).ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-UPDSM-BUILDNEWFILES " + err.Error())
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return common.NewInternalServerError("SMREPO-UPDSM-QUERYNEWFILES " + err.Error())
	}
	type newFileElement struct {
		id          int64
		idShortPath string
		value       sql.NullString
	}
	files := make([]newFileElement, 0)
	for rows.Next() {
		var file newFileElement
		if err = rows.Scan(&file.id, &file.idShortPath, &file.value); err != nil {
			_ = rows.Close()
			return common.NewInternalServerError("SMREPO-UPDSM-SCANNEWFILES " + err.Error())
		}
		files = append(files, file)
	}
	if err = rows.Close(); err != nil {
		return common.NewInternalServerError("SMREPO-UPDSM-CLOSENEWFILES " + err.Error())
	}
	for _, file := range files {
		reference, sameOwner := byPath[file.idShortPath]
		if sameOwner && file.value.Valid && file.value.String == reference.ManagedPath {
			if err = insertRestoredManagedFileReferenceTx(tx, file.id, reference); err != nil {
				return err
			}
			continue
		}
		if file.value.Valid {
			if _, wasOwned := ownedManagedPaths[file.value.String]; wasOwned && strings.HasPrefix(file.value.String, "/aasx/files/") {
				if err = clearReassignedManagedFilePathTx(tx, file.id); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func insertRestoredManagedFileReferenceTx(tx *sql.Tx, fileElementID int64, reference gen.ManagedFileReferenceForReplacement) error {
	query, args, err := goqu.Insert(binarycontent.TableFileReference).Rows(goqu.Record{
		"file_element_id": fileElementID, "binary_content_id": reference.ContentID,
		"path_token": reference.PathToken, "safe_file_name": reference.SafeFileName,
	}).ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-UPDSM-BUILDRESTOREFILE " + err.Error())
	}
	if _, err = tx.Exec(query, args...); err != nil {
		return common.NewInternalServerError("SMREPO-UPDSM-EXECRESTOREFILE " + err.Error())
	}
	query, args, err = goqu.Update("file_element").Set(goqu.Record{"file_name": reference.SafeFileName}).
		Where(goqu.C("id").Eq(fileElementID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-UPDSM-BUILDRESTOREFILENAME " + err.Error())
	}
	if _, err = tx.Exec(query, args...); err != nil {
		return common.NewInternalServerError("SMREPO-UPDSM-EXECRESTOREFILENAME " + err.Error())
	}
	return nil
}

func clearReassignedManagedFilePathTx(tx *sql.Tx, fileElementID int64) error {
	query, args, err := goqu.Update("file_element").Set(goqu.Record{"value": nil, "file_name": nil}).
		Where(goqu.C("id").Eq(fileElementID)).ToSQL()
	if err != nil {
		return common.NewInternalServerError("SMREPO-UPDSM-BUILDCLEARFILEPATH " + err.Error())
	}
	if _, err = tx.Exec(query, args...); err != nil {
		return common.NewInternalServerError("SMREPO-UPDSM-EXECCLEARFILEPATH " + err.Error())
	}
	return nil
}

func (s *SubmodelDatabase) patchSubmodelMetadataInTransaction(tx *sql.Tx, submodelID string, submodel types.ISubmodel) error {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseID(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.NewErrNotFound("SMREPO-PATCHSMMETA-NOTFOUND Submodel with ID '" + submodelID + "' not found")
		}

		return common.NewInternalServerError("SMREPO-PATCHSMMETA-GETSMDATABASEID " + err.Error())
	}

	updateSubmodelQuery, updateSubmodelArgs, err := submodelqueries.BuildUpdateSubmodelMetadataSQL(submodelDatabaseID, submodel)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDUPDATESM " + err.Error())
	}

	if _, err = tx.Exec(updateSubmodelQuery, updateSubmodelArgs...); err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-UPDATESM " + err.Error())
	}

	jsonizedPayload, err := jsonizeSubmodelPayload(submodel)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-JSON " + err.Error())
	}

	upsertPayloadQuery, upsertPayloadArgs, err := submodelqueries.BuildUpsertSubmodelPayloadSQL(
		submodelDatabaseID,
		jsonizedPayload.description,
		jsonizedPayload.displayName,
		jsonizedPayload.administrativeInformation,
		jsonizedPayload.embeddedDataSpecification,
		jsonizedPayload.supplementalSemanticIDs,
		jsonizedPayload.extensions,
		jsonizedPayload.qualifiers,
	)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDUPSERTPAYLOAD " + err.Error())
	}

	if _, err = tx.Exec(upsertPayloadQuery, upsertPayloadArgs...); err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-UPSERTPAYLOAD " + err.Error())
	}

	if err = common.ReplaceContextReferences1ToMany(
		tx,
		int64(submodelDatabaseID),
		submodel.SupplementalSemanticIDs(),
		common.TblSubmodelSuppSemantic,
		common.ColSubmodelID,
	); err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-SUPPSEM " + err.Error())
	}

	deleteSemanticIDQuery, deleteSemanticIDArgs, err := submodelqueries.BuildDeleteSubmodelSemanticIDSQL(submodelDatabaseID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDDELSEMID " + err.Error())
	}

	if _, err = tx.Exec(deleteSemanticIDQuery, deleteSemanticIDArgs...); err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-DELSEMID " + err.Error())
	}

	semanticID := submodel.SemanticID()
	if semanticID == nil {
		return nil
	}

	insertSemanticIDQuery, insertSemanticIDArgs, err := submodelqueries.BuildInsertSubmodelSemanticIDReferenceSQL(int64(submodelDatabaseID), semanticID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDSEMIDREF " + err.Error())
	}

	if _, err = tx.Exec(insertSemanticIDQuery, insertSemanticIDArgs...); err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-INSERTSEMIDREF " + err.Error())
	}

	insertSemanticKeysQuery, insertSemanticKeysArgs, err := submodelqueries.BuildInsertSubmodelSemanticIDReferenceKeysSQL(int64(submodelDatabaseID), semanticID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDSEMIDKEYS " + err.Error())
	}

	if insertSemanticKeysQuery != "" {
		if _, err = tx.Exec(insertSemanticKeysQuery, insertSemanticKeysArgs...); err != nil {
			return common.NewInternalServerError("SMREPO-PATCHSMMETA-INSERTSEMIDKEYS " + err.Error())
		}
	}

	insertSemanticPayloadQuery, insertSemanticPayloadArgs, err := submodelqueries.BuildInsertSubmodelSemanticIDReferencePayloadSQL(int64(submodelDatabaseID), semanticID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-BUILDSEMIDPAYLOAD " + err.Error())
	}

	if _, err = tx.Exec(insertSemanticPayloadQuery, insertSemanticPayloadArgs...); err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMMETA-INSERTSEMIDPAYLOAD " + err.Error())
	}

	return nil
}

func mapCreateSubmodelInsertError(err error) error {
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) && postgresErr.Code == "23505" && postgresErr.ConstraintName == "submodel_submodel_identifier_key" {
		return common.NewErrConflict("SMREPO-NEWSM-CREATE-CONFLICT submodel identifier already exists")
	}

	return nil
}

func cleanupSubmodelLargeObjects(tx *sql.Tx, submodelDatabaseID int64) error {
	unlinkQuery, unlinkArgs, err := submodelqueries.BuildCleanupSubmodelLargeObjectsSQL(submodelDatabaseID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-BUILDUNLINKQUERY " + err.Error())
	}

	var unlinkedCount int64
	if err = tx.QueryRow(unlinkQuery, unlinkArgs...).Scan(&unlinkedCount); err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-UNLINKLO " + err.Error())
	}

	return nil
}

func deleteSubmodelByDatabaseID(tx *sql.Tx, submodelDatabaseID int64) error {
	deleteSubmodelQuery, deleteSubmodelArgs, err := submodelqueries.BuildDeleteSubmodelByDatabaseIDSQL(submodelDatabaseID)
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-BUILDDELETESM " + err.Error())
	}

	deleteResult, err := tx.Exec(deleteSubmodelQuery, deleteSubmodelArgs...)
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-DELETESM " + err.Error())
	}

	rowsAffected, err := deleteResult.RowsAffected()
	if err != nil {
		return common.NewInternalServerError("SMREPO-DELSM-ROWSAFFECTED " + err.Error())
	}
	if rowsAffected == 0 {
		return common.NewErrNotFound("SMREPO-DELSM-NOTFOUND Submodel not found")
	}

	return nil
}
