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
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func TestSpecificAssetIDsWithGlobalAssetIDAddsPublicReadableGlobalAssetIDForDTRContext(t *testing.T) {
	t.Parallel()

	descriptor := model.AssetAdministrationShellDescriptor{
		GlobalAssetId: "global-asset",
		SpecificAssetIds: []types.ISpecificAssetID{
			specificAssetIDWithExternalSubjects("customerPartId", "4711", "BPN_COMPANY_001"),
			specificAssetIDWithExternalSubjects("manufacturerId", "0815", "BPN_COMPANY_001", "BPN_COMPANY_002", "PUBLIC_READABLE", "OTHER"),
		},
	}

	assetIDs := specificAssetIDsWithGlobalAssetID(dtrDiscoveryContext(), descriptor)
	if len(assetIDs) != 3 {
		t.Fatalf("expected original asset IDs plus generated globalAssetId, got %d", len(assetIDs))
	}

	globalAssetID := assetIDs[2]
	if globalAssetID.Name() != globalAssetIDSpecificAssetIDName || globalAssetID.Value() != "global-asset" {
		t.Fatalf("unexpected generated globalAssetId asset link: name=%q value=%q", globalAssetID.Name(), globalAssetID.Value())
	}
	externalSubjectID := globalAssetID.ExternalSubjectID()
	if externalSubjectID == nil {
		t.Fatalf("expected generated globalAssetId to include externalSubjectId")
	}
	if externalSubjectID.Type() != types.ReferenceTypesExternalReference {
		t.Fatalf("expected externalSubjectId type ExternalReference, got %v", externalSubjectID.Type())
	}
	keys := externalSubjectID.Keys()
	if len(keys) != 1 {
		t.Fatalf("expected one externalSubjectId key, got %d", len(keys))
	}
	if keys[0].Type() != types.KeyTypesGlobalReference || keys[0].Value() != globalAssetIDExternalSubjectIDValue {
		t.Fatalf("unexpected externalSubjectId key: type=%v value=%q", keys[0].Type(), keys[0].Value())
	}
}

func TestSpecificAssetIDsWithGlobalAssetIDKeepsExternalSubjectEmptyWithoutDTRContext(t *testing.T) {
	t.Parallel()

	descriptor := model.AssetAdministrationShellDescriptor{
		GlobalAssetId:    "global-asset",
		SpecificAssetIds: []types.ISpecificAssetID{specificAssetIDWithExternalSubjects("manufacturerId", "0815", "BPN_COMPANY_001")},
	}

	assetIDs := specificAssetIDsWithGlobalAssetID(discoveryContext(), descriptor)
	if len(assetIDs) != 2 {
		t.Fatalf("expected original asset ID plus generated globalAssetId, got %d", len(assetIDs))
	}
	if assetIDs[1].ExternalSubjectID() != nil {
		t.Fatalf("expected generated globalAssetId to keep empty externalSubjectId without DTR context")
	}
}

func TestSpecificAssetIDsWithGlobalAssetIDDoesNotAddGlobalAssetIDWhenDiscoveryIntegrationIsDisabled(t *testing.T) {
	t.Parallel()

	descriptor := model.AssetAdministrationShellDescriptor{
		GlobalAssetId:       "global-asset",
		SpecificAssetIds:    []types.ISpecificAssetID{specificAssetIDWithExternalSubjects("manufacturerId", "0815", "BPN_COMPANY_001")},
		SubmodelDescriptors: nil,
	}

	cfg := &common.Config{}
	cfg.General.DiscoveryIntegration = false
	ctx := common.ContextWithConfig(context.Background(), cfg)

	assetIDs := specificAssetIDsWithGlobalAssetID(ctx, descriptor)
	if len(assetIDs) != 1 {
		t.Fatalf("expected no generated globalAssetId when discovery integration is disabled, got %d", len(assetIDs))
	}
}

func discoveryContext() context.Context {
	cfg := &common.Config{}
	cfg.General.DiscoveryIntegration = true
	return common.ContextWithConfig(context.Background(), cfg)
}

func dtrDiscoveryContext() context.Context {
	return WithPublicReadableGlobalAssetIDExternalSubjectID(discoveryContext())
}

func specificAssetIDWithExternalSubjects(name string, value string, subjects ...string) types.ISpecificAssetID {
	assetID := types.NewSpecificAssetID(name, value)
	keys := make([]types.IKey, 0, len(subjects))
	for _, subject := range subjects {
		keys = append(keys, types.NewKey(types.KeyTypesGlobalReference, subject))
	}
	assetID.SetExternalSubjectID(types.NewReference(types.ReferenceTypesExternalReference, keys))
	return assetID
}
