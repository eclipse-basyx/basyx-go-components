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

package abacpolicy

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/go-chi/chi/v5"
)

// SetupConfiguredSecurity keeps the legacy ABAC setup untouched and selects
// the resource-bound integration only when that mode is explicitly configured.
func SetupConfiguredSecurity(
	ctx context.Context,
	cfg *common.Config,
	r *chi.Mux,
	db *sql.DB,
	serviceType string,
	claimsMiddleware ...func(http.Handler) http.Handler,
) (*Repository, error) {
	if !common.ResourceBoundEnabled(cfg) {
		return SetupSecurityWithABACRepository(ctx, cfg, r, db, serviceType, claimsMiddleware...)
	}
	if !resourceBoundServiceSupported(serviceType) {
		return nil, fmt.Errorf("REBAC-SETUP-SERVICE resource-bound mode is unsupported for %s", serviceType)
	}
	if !cfg.ABAC.Enabled {
		return nil, auth.SetupResourceBoundSecurity(ctx, cfg, r, db, nil, claimsMiddleware...)
	}
	repo, err := initializeConfiguredABACRepository(ctx, cfg, r, db, serviceType)
	if err != nil {
		return nil, err
	}
	if err = auth.SetupResourceBoundSecurity(ctx, cfg, r, db, repo, claimsMiddleware...); err != nil {
		return nil, err
	}
	return repo, nil
}

func initializeConfiguredABACRepository(ctx context.Context, cfg *common.Config, r *chi.Mux, db *sql.DB, serviceType string) (*Repository, error) {
	policyScope, err := common.ConfiguredPolicyScope(cfg, serviceType)
	if err != nil {
		return nil, err
	}
	repo, err := NewRepository(db, policyScope, r, cfg.Server.ContextPath)
	if err != nil {
		return nil, err
	}
	mode, err := resolvePolicyFileImportMode(cfg.ABAC.PolicyFileImport, serviceType)
	if err != nil {
		return nil, err
	}
	if err = initializeRepository(ctx, repo, cfg.ABAC.ModelPath, policyScope, mode); err != nil {
		return nil, err
	}
	return repo, nil
}

func resourceBoundServiceSupported(serviceType string) bool {
	switch serviceType {
	case "aasenvironmentservice", "aasrepositoryservice", "submodelrepositoryservice", "aasregistryservice", "submodelregistryservice", "discoveryservice", "conceptdescriptionrepositoryservice":
		return true
	default:
		return false
	}
}
