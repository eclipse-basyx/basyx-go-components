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
	"database/sql"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	smregistryapi "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/api"
)

// CustomSubmodelRegistryService is a pass-through stub for future combined logic.
type CustomSubmodelRegistryService struct {
	*smregistryapi.SubmodelRegistryAPIAPIService
	persistence *Persistence
}

// NewCustomSubmodelRegistryService creates a new pass-through submodel registry decorator.
func NewCustomSubmodelRegistryService(
	base *smregistryapi.SubmodelRegistryAPIAPIService,
	persistence *Persistence,
) *CustomSubmodelRegistryService {
	return &CustomSubmodelRegistryService{
		SubmodelRegistryAPIAPIService: base,
		persistence:                   persistence,
	}
}

// ExecuteInTransaction exposes shared transaction execution for future endpoint customizations.
func (s *CustomSubmodelRegistryService) ExecuteInTransaction(fn func(tx *sql.Tx) error) error {
	if s == nil || s.persistence == nil {
		return common.NewErrBadRequest("AASENV-SMREG-TX-NILSERVICE service must not be nil")
	}
	return s.persistence.ExecuteInTransaction("AASENV-SMREG-STARTTX", "AASENV-SMREG-COMMITTX", fn)
}
