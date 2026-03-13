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
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// DescriptionAPIAPIService is a service that implements the logic for the DescriptionAPIAPIServicer
// This service should implement the business logic for every endpoint for the DescriptionAPIAPI API.
// Include any external packages or services that will be required by this service.
type DescriptionAPIAPIService struct {
}

// NewDescriptionAPIAPIService creates a default api service
func NewDescriptionAPIAPIService() *DescriptionAPIAPIService {
	return &DescriptionAPIAPIService{}
}

// GetSelfDescription - Returns the self-describing information of a network resource (ServiceDescription)
func (s *DescriptionAPIAPIService) GetSelfDescription(_ context.Context) (model.ImplResponse, error) {
	sd := model.ServiceDescription{
		Profiles: []string{
			"https://admin-shell.io/aas/API/3/1/SubmodelRepositoryServiceSpecification/SSP-001",
			"https://basyx.org/aas/go-server/API/SubmodelRepositoryService/1.0",
		},
	}

	return model.Response(http.StatusOK, sd), nil
}
