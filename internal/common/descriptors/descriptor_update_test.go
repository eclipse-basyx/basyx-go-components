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

package descriptors

import (
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestBuildAdministrationShellDescriptorUpdatePlan(t *testing.T) {
	base := testAdministrationShellDescriptor()

	t.Run("unchanged", func(t *testing.T) {
		plan, err := buildAdministrationShellDescriptorUpdatePlan(t.Context(), base, base)
		require.NoError(t, err)
		require.False(t, plan.changed())
	})

	t.Run("payload only", func(t *testing.T) {
		updated := base
		updated.Description = []types.ILangStringTextType{types.NewLangStringTextType("en", "updated")}
		plan, err := buildAdministrationShellDescriptorUpdatePlan(t.Context(), base, updated)
		require.NoError(t, err)
		require.True(t, plan.payload.description)
		require.False(t, plan.payload.displayName)
		require.False(t, plan.payload.administration)
		require.False(t, plan.payload.extensions)
		require.False(t, plan.root)
		require.False(t, plan.endpoints)
		require.False(t, plan.specificAssetIDs)
		require.False(t, plan.submodelDescriptors)
	})

	t.Run("endpoint only", func(t *testing.T) {
		updated := base
		updated.Endpoints = []model.Endpoint{testDescriptorEndpoint("https://example.com/updated")}
		plan, err := buildAdministrationShellDescriptorUpdatePlan(t.Context(), base, updated)
		require.NoError(t, err)
		require.True(t, plan.endpoints)
		require.False(t, plan.root)
		require.False(t, plan.payload.changed())
		require.False(t, plan.specificAssetIDs)
		require.False(t, plan.submodelDescriptors)
	})

	t.Run("embedded descriptor only", func(t *testing.T) {
		updated := base
		updated.SubmodelDescriptors = []model.SubmodelDescriptor{testSubmodelDescriptor("sm-1", "updated")}
		plan, err := buildAdministrationShellDescriptorUpdatePlan(t.Context(), base, updated)
		require.NoError(t, err)
		require.True(t, plan.submodelDescriptors)
		require.False(t, plan.root)
		require.False(t, plan.payload.changed())
		require.False(t, plan.endpoints)
		require.False(t, plan.specificAssetIDs)
	})
}

func TestBuildSubmodelDescriptorUpdatePlan(t *testing.T) {
	base := testSubmodelDescriptor("sm-1", "original")

	t.Run("unchanged", func(t *testing.T) {
		plan, err := buildSubmodelDescriptorUpdatePlan(base, base)
		require.NoError(t, err)
		require.False(t, plan.changed())
	})

	t.Run("payload only", func(t *testing.T) {
		updated := base
		updated.Description = []types.ILangStringTextType{types.NewLangStringTextType("en", "updated")}
		plan, err := buildSubmodelDescriptorUpdatePlan(base, updated)
		require.NoError(t, err)
		require.True(t, plan.payload.description)
		require.False(t, plan.payload.displayName)
		require.False(t, plan.payload.administration)
		require.False(t, plan.payload.extensions)
		require.False(t, plan.root)
		require.False(t, plan.endpoints)
		require.False(t, plan.semanticID)
		require.False(t, plan.supplementalSemantic)
	})

	t.Run("endpoint only", func(t *testing.T) {
		updated := base
		updated.Endpoints = []model.Endpoint{testDescriptorEndpoint("https://example.com/updated")}
		plan, err := buildSubmodelDescriptorUpdatePlan(base, updated)
		require.NoError(t, err)
		require.True(t, plan.endpoints)
		require.False(t, plan.root)
		require.False(t, plan.payload.changed())
		require.False(t, plan.semanticID)
		require.False(t, plan.supplementalSemantic)
	})
}

func testAdministrationShellDescriptor() model.AssetAdministrationShellDescriptor {
	return model.AssetAdministrationShellDescriptor{
		Id:                  "aas-1",
		IdShort:             "AASOne",
		Description:         []types.ILangStringTextType{types.NewLangStringTextType("en", "original")},
		Endpoints:           []model.Endpoint{testDescriptorEndpoint("https://example.com/aas")},
		SubmodelDescriptors: []model.SubmodelDescriptor{testSubmodelDescriptor("sm-1", "original")},
	}
}

func testSubmodelDescriptor(id string, description string) model.SubmodelDescriptor {
	return model.SubmodelDescriptor{
		Id:          id,
		IdShort:     "SubmodelOne",
		Description: []types.ILangStringTextType{types.NewLangStringTextType("en", description)},
		Endpoints:   []model.Endpoint{testDescriptorEndpoint("https://example.com/submodel")},
	}
}

func testDescriptorEndpoint(href string) model.Endpoint {
	return model.Endpoint{
		Interface: "SUBMODEL-3.0",
		ProtocolInformation: model.ProtocolInformation{
			Href:             href,
			EndpointProtocol: "https",
		},
	}
}
