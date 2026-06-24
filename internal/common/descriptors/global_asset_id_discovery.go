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
*******************************************************************************/

package descriptors

import (
	"context"
	"fmt"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const publicReadableExternalSubjectValue = "PUBLIC_READABLE"

func specificAssetIDsWithGlobalAssetID(
	ctx context.Context,
	descriptor model.AssetAdministrationShellDescriptor,
) []types.ISpecificAssetID {
	if !discoveryIntegrationEnabled(ctx) || strings.TrimSpace(descriptor.GlobalAssetId) == "" {
		return descriptor.SpecificAssetIds
	}

	specificAssetIDs := append([]types.ISpecificAssetID(nil), descriptor.SpecificAssetIds...)
	return append(specificAssetIDs, globalAssetIDSpecificAssetID(ctx, descriptor))
}

func globalAssetIDSpecificAssetID(
	ctx context.Context,
	descriptor model.AssetAdministrationShellDescriptor,
) types.ISpecificAssetID {
	assetID := types.NewSpecificAssetID(globalAssetIDSpecificAssetIDName, descriptor.GlobalAssetId)
	if !digitalTwinRegistryDiscoveryFromContext(ctx) {
		return assetID
	}

	externalSubjectID := externalSubjectIDForGlobalAssetID(descriptor.SpecificAssetIds)
	if externalSubjectID != nil {
		assetID.SetExternalSubjectID(externalSubjectID)
	}
	return assetID
}

func externalSubjectIDForGlobalAssetID(specificAssetIDs []types.ISpecificAssetID) types.IReference {
	keys := make([]types.IKey, 0)
	seen := make(map[string]struct{})
	for _, specificAssetID := range specificAssetIDs {
		if specificAssetID == nil || specificAssetID.ExternalSubjectID() == nil {
			continue
		}
		for _, key := range specificAssetID.ExternalSubjectID().Keys() {
			if key == nil {
				continue
			}
			value := strings.TrimSpace(key.Value())
			if !isGlobalAssetIDExternalSubjectValue(value) {
				continue
			}
			signature := fmt.Sprint(key.Type()) + "\x00" + value
			if _, ok := seen[signature]; ok {
				continue
			}
			seen[signature] = struct{}{}
			keys = append(keys, types.NewKey(key.Type(), value))
		}
	}
	if len(keys) == 0 {
		return nil
	}
	return types.NewReference(types.ReferenceTypesExternalReference, keys)
}

func isGlobalAssetIDExternalSubjectValue(value string) bool {
	return value == publicReadableExternalSubjectValue || strings.HasPrefix(value, "BPN")
}

func discoveryIntegrationEnabled(ctx context.Context) bool {
	cfg, ok := common.ConfigFromContext(ctx)
	return ok && cfg.General.DiscoveryIntegration
}
