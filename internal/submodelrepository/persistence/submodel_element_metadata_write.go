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

package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelpath "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/path"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
	persistenceutils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
)

// PatchSubmodelElementMetadata applies only the supplied metadata fields and
// verifies authorization, stored values, and history in one writer transaction.
func (s *SubmodelDatabase) PatchSubmodelElementMetadata(ctx context.Context, submodelID, path string, patch map[string]any) (err error) {
	if submodelID == "" || path == "" {
		return common.NewErrBadRequest("SMREPO-PATCHSMEMETA-IDENTIFIER Submodel ID and element path are required")
	}
	if _, err = submodelpath.ParseIDShortPathSegments(path); err != nil {
		return common.NewErrBadRequest("SMREPO-PATCHSMEMETA-PATH " + err.Error())
	}
	metadata, err := common.ParseSubmodelElementMetadataPayload(patch)
	if err != nil {
		return common.NewErrBadRequest("SMREPO-PATCHSMEMETA-PAYLOAD " + err.Error())
	}
	if err = validateMetadataPatchIDShort(patch); err != nil {
		return err
	}
	tx, cleanup, err := common.StartTransaction(s.db)
	if err != nil {
		return common.NewInternalServerError("SMREPO-PATCHSMEMETA-STARTTX " + err.Error())
	}
	defer cleanup(&err)
	shouldEnforce, err := shouldEnforceFormula(ctx, "SMREPO-PATCHSMEMETA-SHOULDENFORCE")
	if err != nil {
		return err
	}
	if shouldEnforce {
		ctx, err = s.ensureSubmodelElementCanBeUpdated(ctx, tx, submodelID, path)
		if err != nil {
			return err
		}
	}
	previousSnapshot, err := s.beginSubmodelMutationTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}
	if err = s.patchSubmodelElementMetadataTx(ctx, tx, submodelID, path, patch, metadata, shouldEnforce); err != nil {
		return err
	}
	if err = s.appendChangedSubmodelElementHistoryTx(ctx, tx, submodelID, previousSnapshot, submodelElementRootMutation{
		previousPath: path,
		currentPath:  path,
	}); err != nil {
		return err
	}
	return common.CommitTransaction(tx)
}

// validateMetadataPatchIDShort rejects an invalid idShort in a metadata patch.
// A metadata PATCH never renames the element.
func validateMetadataPatchIDShort(patch map[string]any) error {
	idShort, present := patch["idShort"].(string)
	if !present {
		return nil
	}
	return common.ValidateOptionalIDShort("SMREPO-PATCHSMEMETA", "Submodel element in the request body", &idShort)
}

func (s *SubmodelDatabase) patchSubmodelElementMetadataTx(ctx context.Context, tx *sql.Tx, submodelID, path string, patch map[string]any, metadata gen.SubmodelElementMetadata, shouldEnforce bool) error {
	target, err := lockMetadataTargetTx(ctx, tx, submodelID, path, metadata.ModelType)
	if err != nil {
		return err
	}
	if shouldEnforce {
		if err = s.ensureUpdatedSubmodelElementIsVisible(ctx, tx, submodelID, path); err != nil {
			return err
		}
	}
	current, err := readMetadataValueTypeStateTx(ctx, tx, submodelID, path, patch)
	if err != nil {
		return err
	}
	if err = submodelelements.ApplyMetadataPatchTx(ctx, tx, target, patch, current); err != nil {
		return err
	}
	if shouldEnforce {
		if err = s.ensureUpdatedSubmodelElementIsVisible(ctx, tx, submodelID, path); err != nil {
			return err
		}
	}
	return s.verifyWrittenSubmodelElementTx(ctx, tx, submodelID, path, target.ModelType == types.ModelTypeSubmodelElementList, "SMREPO-PATCHSMEMETA-VERIFY")
}

func lockMetadataTargetTx(ctx context.Context, tx *sql.Tx, submodelID, path string, modelType types.ModelType) (submodelelements.MetadataTarget, error) {
	submodelDatabaseID, err := persistenceutils.GetSubmodelDatabaseIDForNoKeyUpdateContext(ctx, tx, submodelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return submodelelements.MetadataTarget{}, common.NewErrNotFound("SMREPO-PATCHSMEMETA-SMNOTFOUND " + submodelID)
		}
		return submodelelements.MetadataTarget{}, common.NewInternalServerError("SMREPO-PATCHSMEMETA-LOCKSUBMODEL " + err.Error())
	}
	target, err := submodelelements.LockMetadataTargetTx(ctx, tx, submodelDatabaseID, path)
	if err != nil {
		return target, err
	}
	if target.ModelType != modelType {
		return target, common.NewErrBadRequest("SMREPO-PATCHSMEMETA-MODELTYPEMISMATCH metadata modelType must match existing submodel element modelType")
	}
	return target, nil
}

func readMetadataValueTypeStateTx(ctx context.Context, tx *sql.Tx, submodelID, path string, patch map[string]any) (types.ISubmodelElement, error) {
	if _, present := patch["valueType"]; !present {
		return nil, nil
	}
	return submodelelements.GetSubmodelElementWithoutChildrenByPathTx(auth.ContextWithoutQueryFilter(ctx), tx, submodelID, path, false)
}
