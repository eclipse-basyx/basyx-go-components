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
// Author: Martin Stemmer ( Fraunhofer IESE )

package digitaltwinregistry

import (
	"context"
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const (
	aasRegistrySSP001Profile      = "https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-001"
	aasRegistrySSP003Profile      = "https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-003"
	aasRegistrySSP004Profile      = "https://admin-shell.io/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-004"
	discoverySSP001Profile        = "https://admin-shell.io/aas/API/3/2/DiscoveryServiceSpecification/SSP-001"
	basyxAASRegistrySSP001Profile = "https://basyx.org/aas/API/3/2/AssetAdministrationShellRegistryServiceSpecification/SSP-001"
	basyxDiscoverySSP001Profile   = "https://basyx.org/aas/API/3/2/DiscoveryServiceSpecification/SSP-001"
)

// DescriptionService provides the combined service description for the Digital Twin Registry.
type DescriptionService struct{}

// NewDescriptionService constructs the description service.
func NewDescriptionService() *DescriptionService {
	return &DescriptionService{}
}

// GetDescription - Returns the self-describing information of the Digital Twin Registry.
func (s *DescriptionService) GetDescription(_ context.Context) (model.ImplResponse, error) {
	return model.Response(http.StatusOK, model.ServiceDescription{
		Profiles: []string{
			aasRegistrySSP001Profile,
			aasRegistrySSP003Profile,
			aasRegistrySSP004Profile,
			discoverySSP001Profile,
			basyxAASRegistrySSP001Profile,
			basyxDiscoverySSP001Profile,
		},
	}), nil
}
