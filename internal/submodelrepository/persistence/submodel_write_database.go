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

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/createprecheck"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelqueries "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/queries"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
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

	if err = s.appendSubmodelHistoryTx(ctx, tx, submodel, nil, history.ChangeCreated, false); err != nil {
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
	return s.appendSubmodelHistoryTx(ctx, tx, submodel, nil, history.ChangeCreated, false)
}

func (s *SubmodelDatabase) createSubmodelInTransactionValidated(ctx context.Context, tx *sql.Tx, submodel types.ISubmodel) error {
	if err := history.LockMutationTx(ctx, tx, history.TableSubmodel, submodel.ID()); err != nil {
		return err
	}
	if err := s.ensureVisibleSubmodelCreateDoesNotExist(ctx, tx, submodel.ID()); err != nil {
		return err
	}

	err := s.createSubmodelInTransaction(tx, submodel)
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

func (s *SubmodelDatabase) createSubmodelInTransaction(tx *sql.Tx, submodel types.ISubmodel) error {
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

	if _, err := tx.Exec(ids, args...); err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-EXECPAYLOADSQL " + err.Error())
	}

	if err := common.CreateContextReferences1ToMany(
		tx,
		submodelDBID,
		submodel.SupplementalSemanticIDs(),
		common.TblSubmodelSuppSemantic,
		common.ColSubmodelID,
	); err != nil {
		return common.NewInternalServerError("SMREPO-NEWSM-CREATE-SUPPSEM " + err.Error())
	}

	semanticID := submodel.SemanticID()
	if semanticID != nil {
		ids, args, err = submodelqueries.BuildInsertSubmodelSemanticIDReferenceSQL(submodelDBID, semanticID)
		if err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATE-SEMIDREFSQL " + err.Error())
		}

		if _, err := tx.Exec(ids, args...); err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATE-EXECSEMIDREFSQL " + err.Error())
		}

		ids, args, err = submodelqueries.BuildInsertSubmodelSemanticIDReferenceKeysSQL(submodelDBID, semanticID)
		if err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATE-SEMIDKEYSQL " + err.Error())
		}

		if ids != "" {
			if _, err := tx.Exec(ids, args...); err != nil {
				return common.NewInternalServerError("SMREPO-NEWSM-CREATE-EXECSEMIDKEYSQL " + err.Error())
			}
		}

		ids, args, err = submodelqueries.BuildInsertSubmodelSemanticIDReferencePayloadSQL(submodelDBID, semanticID)
		if err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATE-SEMIDPAYLOADSQL " + err.Error())
		}

		if _, err := tx.Exec(ids, args...); err != nil {
			return common.NewInternalServerError("SMREPO-NEWSM-CREATE-EXECSEMIDPAYLOADSQL " + err.Error())
		}
	}

	if len(submodel.SubmodelElements()) > 0 {
		submodelDatabaseID, conversionErr := submodelDatabaseIDAsInt(submodelDBID)
		if conversionErr != nil {
			return conversionErr
		}

		_, err = submodelelements.InsertSubmodelElementsForSubmodelDatabaseID(s.db, submodelDatabaseID, submodel.SubmodelElements(), tx, nil)
		if err != nil {
			return err
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

	if err = s.appendSubmodelHistoryTx(ctx, tx, submodel, previousSnapshot, history.ChangeUpdated, false); err != nil {
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
	return s.appendSubmodelHistoryTx(ctx, tx, submodel, previousSnapshot, history.ChangeUpdated, false)
}

func (s *SubmodelDatabase) patchSubmodelInTransactionValidated(_ context.Context, submodelID string, tx *sql.Tx, submodel types.ISubmodel) error {
	_, err := s.replaceSubmodelInTransaction(tx, submodelID, submodel, true)
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

	isUpdate, err := s.putSubmodelInTransaction(ctx, tx, submodelID, submodel)
	if err != nil {
		return false, err
	}

	err = tx.Commit()
	if err != nil {
		return false, common.NewInternalServerError("SMREPO-PUTSM-COMMIT " + err.Error())
	}

	return isUpdate, nil
}

// PutSubmodelInTransaction creates or replaces a submodel within an existing transaction.
func (s *SubmodelDatabase) PutSubmodelInTransaction(ctx context.Context, tx *sql.Tx, submodelID string, submodel types.ISubmodel) (bool, error) {
	if tx == nil {
		return false, common.NewInternalServerError("SMREPO-PUTSM-NILTX transaction must not be nil")
	}
	if submodelID != submodel.ID() {
		return false, common.NewErrBadRequest("SMREPO-PUTSM-IDMISMATCH Submodel ID in path and body do not match")
	}

	if err := s.verifySubmodel(submodel, "SMREPO-PUTSM-VERIFY"); err != nil {
		return false, err
	}

	return s.putSubmodelInTransaction(ctx, tx, submodelID, submodel)
}

func (s *SubmodelDatabase) putSubmodelInTransaction(ctx context.Context, tx *sql.Tx, submodelID string, submodel types.ISubmodel) (bool, error) {
	shouldEnforce, enforceErr := shouldEnforceFormula(ctx, "SMREPO-PUTSM-SHOULDENFORCE")
	if enforceErr != nil {
		return false, enforceErr
	}
	if shouldEnforce {
		exists, _, visErr := s.checkSubmodelVisibilityInTx(ctx, tx, submodelID)
		if visErr != nil {
			return false, visErr
		}
		ctx = auth.SelectPutFormulaByExistence(ctx, exists)
		exists, visible, visErr := s.checkSubmodelVisibilityInTx(ctx, tx, submodelID)
		if visErr != nil {
			return false, visErr
		}
		if exists && !visible {
			return false, common.NewErrDenied("SMREPO-PUTSM-ABACDENIED Existing submodel is not accessible under ABAC constraints")
		}
	}
	previousSnapshot, err := s.loadSubmodelHistorySnapshotBeforeMutationTx(ctx, tx, submodelID)
	if err != nil && !common.IsErrNotFound(err) {
		return false, err
	}

	isUpdate, err := s.replaceSubmodelInTransaction(tx, submodelID, submodel, false)
	if err != nil {
		return false, err
	}

	if shouldEnforce {
		exists, visible, visErr := s.checkSubmodelVisibilityInTx(ctx, tx, submodelID)
		if visErr != nil {
			return false, visErr
		}
		if !exists {
			return false, common.NewInternalServerError("SMREPO-PUTSM-ABACCHECKMISSING written submodel not found before commit")
		}
		if !visible {
			return false, common.NewErrDenied("SMREPO-PUTSM-ABACDENIED Written submodel is not accessible under ABAC constraints")
		}
	}

	changeType := history.ChangeCreated
	if isUpdate {
		changeType = history.ChangeUpdated
	}
	if err := s.appendSubmodelHistoryTx(ctx, tx, submodel, previousSnapshot, changeType, false); err != nil {
		return false, err
	}

	return isUpdate, nil
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

	err = cleanupSubmodelLargeObjects(tx, int64(submodelDatabaseID))
	if err != nil {
		return err
	}

	err = deleteSubmodelByDatabaseID(tx, int64(submodelDatabaseID))
	if err != nil {
		return err
	}

	return nil
}

func (s *SubmodelDatabase) replaceSubmodelInTransaction(tx *sql.Tx, submodelID string, submodel types.ISubmodel, requireExisting bool) (bool, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseIDForUpdate(tx, submodelID)
	if err != nil {
		if err == sql.ErrNoRows {
			if requireExisting {
				return false, common.NewErrNotFound("SMREPO-UPDSM-NOTFOUND Submodel with ID '" + submodelID + "' not found")
			}

			if createErr := s.createSubmodelInTransaction(tx, submodel); createErr != nil {
				return false, createErr
			}
			return false, nil
		}

		return false, common.NewInternalServerError("SMREPO-UPDSM-GETSMDATABASEID " + err.Error())
	}

	err = cleanupSubmodelLargeObjects(tx, int64(submodelDatabaseID))
	if err != nil {
		return false, err
	}

	err = deleteSubmodelByDatabaseID(tx, int64(submodelDatabaseID))
	if err != nil {
		return false, err
	}

	err = s.createSubmodelInTransaction(tx, submodel)
	if err != nil {
		return false, err
	}

	return true, nil
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
	if err == nil {
		return nil
	}

	if common.IsPostgresUniqueViolation(err) {
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
