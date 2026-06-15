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

	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	cdrdb "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
	discoverydb "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/persistence"
	smregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/persistence"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
)

// Persistence bundles all component persistence backends and the shared DB pool.
type Persistence struct {
	DB *sql.DB

	AASRegistry                  *aasregistrydb.PostgreSQLAASRegistryDatabase
	SubmodelRegistry             *smregistrydb.PostgreSQLSMDatabase
	AASRepository                *aasrepositorydb.AssetAdministrationShellDatabase
	SubmodelRepository           *submodelrepositorydb.SubmodelDatabase
	ConceptDescriptionRepository *cdrdb.ConceptDescriptionBackend
	Discovery                    *discoverydb.PostgreSQLDiscoveryDatabase
}

// ExecuteInTransaction runs fn in a single shared DB transaction.
func (p *Persistence) ExecuteInTransaction(startErrorCode string, commitErrorCode string, fn func(tx *sql.Tx) error) error {
	if p == nil {
		return common.NewErrBadRequest("AASENV-TX-NILPERSISTENCE persistence bundle must not be nil")
	}
	if p.DB == nil {
		return common.NewErrBadRequest("AASENV-TX-NILDB shared DB pool must not be nil")
	}
	return common.ExecuteInTransaction(p.DB, startErrorCode, commitErrorCode, fn)
}
