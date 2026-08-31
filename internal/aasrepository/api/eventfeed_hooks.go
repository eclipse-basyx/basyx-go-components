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
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
)

func publishAASEvent(ctx context.Context, module *eventfeed.Module, created bool, aas types.IAssetAdministrationShell) {
	if module == nil || !module.Enabled() || aas == nil {
		return
	}
	aasID, globalAssetID, submodelIDs := aasFeedFields(aas)
	if created {
		module.PublishAASCreated(ctx, aasID, globalAssetID, submodelIDs)
		return
	}
	module.PublishAASUpdated(ctx, aasID, globalAssetID, submodelIDs)
}

func publishAASDeletedEvent(ctx context.Context, module *eventfeed.Module, aas types.IAssetAdministrationShell) {
	if module == nil || !module.Enabled() || aas == nil {
		return
	}
	aasID, globalAssetID, submodelIDs := aasFeedFields(aas)
	module.PublishAASDeleted(ctx, aasID, globalAssetID, submodelIDs)
}

func publishSubmodelEvent(ctx context.Context, module *eventfeed.Module, created bool, submodel types.ISubmodel, globalAssetIDs []string) {
	if module == nil || !module.Enabled() || submodel == nil {
		return
	}
	semanticID := eventfeed.SemanticIDFromSubmodel(submodel)
	if created {
		module.PublishSubmodelCreated(ctx, submodel.ID(), semanticID, globalAssetIDs)
	} else {
		module.PublishSubmodelUpdated(ctx, submodel.ID(), semanticID, globalAssetIDs)
	}
	if eventfeed.IsPCNSemanticID(semanticID) {
		module.PublishPCN(ctx, submodel.ID(), semanticID, eventfeed.PCNRecordIDShortsFromSubmodel(submodel))
	}
}

func publishSubmodelDeletedEvent(ctx context.Context, module *eventfeed.Module, submodel types.ISubmodel, globalAssetIDs []string) {
	if module == nil || !module.Enabled() || submodel == nil {
		return
	}
	module.PublishSubmodelDeleted(ctx, submodel.ID(), eventfeed.SemanticIDFromSubmodel(submodel), globalAssetIDs)
}

// globalAssetIDsForSubmodel looks up the global asset IDs of every Asset Administration Shell
// that currently references the given submodel, for embedding in submodel feed events.
func globalAssetIDsForSubmodel(ctx context.Context, backend *persistencepostgresql.AssetAdministrationShellDatabase, submodelID string) []string {
	if backend == nil || submodelID == "" {
		return nil
	}
	aasIDs, err := backend.ListAASIdentifiersBySubmodelID(ctx, submodelID)
	if err != nil || len(aasIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(aasIDs))
	globalAssetIDs := make([]string, 0, len(aasIDs))
	for _, aasID := range aasIDs {
		aas, getErr := backend.GetAssetAdministrationShellByID(ctx, aasID)
		if getErr != nil || aas == nil {
			continue
		}
		info := aas.AssetInformation()
		if info == nil {
			continue
		}
		globalAssetID := info.GlobalAssetID()
		if globalAssetID == nil || *globalAssetID == "" {
			continue
		}
		if _, exists := seen[*globalAssetID]; exists {
			continue
		}
		seen[*globalAssetID] = struct{}{}
		globalAssetIDs = append(globalAssetIDs, *globalAssetID)
	}
	return globalAssetIDs
}

func aasFeedFields(aas types.IAssetAdministrationShell) (aasID, globalAssetID string, submodelIDs []string) {
	aasID = aas.ID()
	if info := aas.AssetInformation(); info != nil {
		if gid := info.GlobalAssetID(); gid != nil {
			globalAssetID = *gid
		}
	}
	submodelIDs = referenceValues(aas.Submodels())
	return aasID, globalAssetID, submodelIDs
}

func referenceValues(refs []types.IReference) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		keys := ref.Keys()
		if len(keys) == 0 || keys[len(keys)-1] == nil {
			continue
		}
		value := keys[len(keys)-1].Value()
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
