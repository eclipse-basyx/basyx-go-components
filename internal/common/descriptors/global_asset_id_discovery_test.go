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

func TestSpecificAssetIDsWithGlobalAssetIDAddsGlobalAssetIDWithoutExternalSubjects(t *testing.T) {
	t.Parallel()

	descriptor := model.AssetAdministrationShellDescriptor{
		GlobalAssetId: "global-asset",
		SpecificAssetIds: []types.ISpecificAssetID{
			specificAssetIDWithExternalSubjects("customerPartId", "4711", "BPN_COMPANY_001"),
			specificAssetIDWithExternalSubjects("manufacturerId", "0815", "BPN_COMPANY_001", "BPN_COMPANY_002", "PUBLIC_READABLE", "OTHER"),
		},
	}

	assetIDs := specificAssetIDsWithGlobalAssetID(discoveryContext(), descriptor)
	if len(assetIDs) != 3 {
		t.Fatalf("expected original asset IDs plus generated globalAssetId, got %d", len(assetIDs))
	}

	globalAssetID := assetIDs[2]
	if globalAssetID.Name() != globalAssetIDSpecificAssetIDName || globalAssetID.Value() != "global-asset" {
		t.Fatalf("unexpected generated globalAssetId asset link: name=%q value=%q", globalAssetID.Name(), globalAssetID.Value())
	}
	if globalAssetID.ExternalSubjectID() != nil {
		t.Fatalf("expected generated globalAssetId to keep empty externalSubjectId")
	}
}

func TestSpecificAssetIDsWithGlobalAssetIDKeepsNonDTRBehavior(t *testing.T) {
	t.Parallel()

	descriptor := model.AssetAdministrationShellDescriptor{
		GlobalAssetId:       "global-asset",
		SpecificAssetIds:    []types.ISpecificAssetID{specificAssetIDWithExternalSubjects("manufacturerId", "0815", "BPN_COMPANY_001")},
		SubmodelDescriptors: nil,
	}

	assetIDs := specificAssetIDsWithGlobalAssetID(discoveryContext(), descriptor)
	if len(assetIDs) != 2 {
		t.Fatalf("expected original asset ID plus generated globalAssetId, got %d", len(assetIDs))
	}
	if assetIDs[1].ExternalSubjectID() != nil {
		t.Fatalf("expected non-DTR generated globalAssetId to keep empty externalSubjectId")
	}
}

func discoveryContext() context.Context {
	cfg := &common.Config{}
	cfg.General.DiscoveryIntegration = true
	return common.ContextWithConfig(context.Background(), cfg)
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
