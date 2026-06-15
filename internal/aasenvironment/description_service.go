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
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// DescriptionService exposes a merged profile description for the AAS Environment Service.
type DescriptionService struct{}

// NewDescriptionService creates a new description service.
func NewDescriptionService() *DescriptionService {
	return &DescriptionService{}
}

// GetDescription returns merged service profile metadata for all bundled components.
func (s *DescriptionService) GetDescription(_ context.Context) (model.ImplResponse, error) {
	return model.Response(http.StatusOK, model.ServiceDescription{
		Profiles: mergedProfiles(),
	}), nil
}

func mergedProfiles() []string {
	profiles := []string{
		"https://basyx.org/aas/API/3/2/AssetAdministrationShellEnvironment/SSP-001",
		"https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-003",
		"https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-004",
		"https://basyx.org/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/SubmodelRegistryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/SubmodelRegistryServiceSpecification/SSP-003",
		"https://basyx.org/aas/API/3/2/SubmodelRegistryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRepositoryServiceSpecification/SSP-001",
		"https://basyx.org/aas/API/3/2/AssetAdministrationShellRepositoryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/SubmodelRepositoryServiceSpecification/SSP-001",
		"https://admin-shell.io/aas/API/3/2/SubmodelRepositoryServiceSpecification/SSP-005",
		"https://basyx.org/aas/API/3/2/SubmodelRepositoryService/1.0",
		"https://admin-shell.io/aas/API/3/2/ConceptDescriptionRepositoryServiceSpecification/SSP-001",
		"https://basyx.org/aas/API/3/2/ConceptDescriptionRepositoryService/1.0",
		"https://admin-shell.io/aas/API/3/2/DiscoveryServiceSpecification/SSP-001",
		"https://basyx.org/aas/API/3/2/DiscoveryServiceSpecification/SSP-001",
	}

	seen := make(map[string]struct{}, len(profiles))
	result := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		result = append(result, p)
	}
	return result
}
