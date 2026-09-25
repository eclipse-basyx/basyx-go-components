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

package aasenvironment

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func generatedAASDescriptorContext(ctx context.Context, descriptor commonmodel.AssetAdministrationShellDescriptor) context.Context {
	ctx = auth.ContextWithReBACDerivedTarget(ctx, "aas", descriptor.Id, "aas_descriptor", descriptor.Id)
	for _, embedded := range descriptor.SubmodelDescriptors {
		ctx = auth.ContextWithReBACEmbeddedSource(ctx, descriptor.Id, embedded.Id)
	}
	return ctx
}

func (s *CustomAASRepositoryService) ensureAuthorizedSubmodelReference(ctx context.Context, tx *sql.Tx, aasID, submodelID string) (bool, error) {
	if cfg, ok := common.ConfigFromContext(ctx); ok && cfg.ReBAC.Enabled {
		err := s.persistence.AASRepository.CheckIfSubmodelReferenceExistsInAssetAdministrationShellInTransaction(tx, aasID, submodelID)
		if err == nil {
			return false, nil
		}
		if !common.IsErrNotFound(err) {
			return false, err
		}
	}
	authorized, err := auth.AuthorizeReBACResource(ctx, tx, "aas", aasID, http.MethodPatch)
	if err != nil {
		return false, err
	}
	reference := types.NewReference(types.ReferenceTypesModelReference, []types.IKey{types.NewKey(types.KeyTypesSubmodel, submodelID)})
	err = s.persistence.AASRepository.CreateSubmodelReferenceInAssetAdministrationShellInTransaction(authorized, tx, aasID, reference)
	if common.IsErrConflict(err) {
		return false, nil
	}
	return err == nil, err
}
