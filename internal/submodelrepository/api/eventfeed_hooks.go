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

package api

import (
	"context"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
)

func publishSubmodelEvent(ctx context.Context, module *eventfeed.Module, backend persistencepostgresql.SubmodelDatabase, created bool, submodel types.ISubmodel, previous types.ISubmodel) {
	if module == nil || !module.Enabled() || submodel == nil {
		return
	}
	semanticID := eventfeed.SemanticIDFromSubmodel(submodel)
	globalAssetIDs := globalAssetIDsForSubmodel(ctx, backend, submodel.ID())
	if created {
		module.PublishSubmodelCreated(ctx, submodel.ID(), semanticID, globalAssetIDs)
	} else {
		module.PublishSubmodelUpdated(ctx, submodel.ID(), semanticID, globalAssetIDs)
	}
	if eventfeed.IsPCNSemanticID(semanticID) {
		module.PublishPCN(ctx, previous, submodel, globalAssetIDs)
	}
}

func publishSubmodelDeletedEvent(ctx context.Context, module *eventfeed.Module, backend persistencepostgresql.SubmodelDatabase, submodel types.ISubmodel) {
	if module == nil || !module.Enabled() || submodel == nil {
		return
	}
	globalAssetIDs := globalAssetIDsForSubmodel(ctx, backend, submodel.ID())
	module.PublishSubmodelDeleted(ctx, submodel.ID(), eventfeed.SemanticIDFromSubmodel(submodel), globalAssetIDs)
}

// globalAssetIDsForSubmodel looks up the global asset IDs of every Asset Administration Shell
// that currently references the given submodel, for embedding in submodel feed events.
func globalAssetIDsForSubmodel(ctx context.Context, backend persistencepostgresql.SubmodelDatabase, submodelID string) []string {
	if submodelID == "" {
		return nil
	}
	globalAssetIDs, err := backend.ListGlobalAssetIDsBySubmodelID(ctx, submodelID)
	if err != nil {
		return nil
	}
	return globalAssetIDs
}
