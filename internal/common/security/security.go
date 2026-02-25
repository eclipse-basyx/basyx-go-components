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

// Package auth provides authentication and authorization functionality
// for BaSyx Go components. It implements OIDC (OpenID Connect) authentication
// and ABAC (Attribute-Based Access Control) authorization mechanisms
// to secure API endpoints and resources.
//
// The package supports:
//   - OIDC token validation and user authentication
//   - ABAC policy-based authorization
//   - Configurable security middleware for Chi routers
//   - Role-based access control through JWT claims
//
// Author: Martin Stemmer ( Fraunhofer IESE )
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	api "github.com/go-chi/chi/v5"
)

// SetupSecurity configures and applies security middleware to a Chi router
// based on the provided configuration. It sets up OIDC authentication and
// ABAC authorization if enabled in the configuration.
//
// The function performs the following operations:
//   - Checks if ABAC is enabled, returns early if disabled
//   - Initializes OIDC provider with issuer and audience settings
//   - Loads and parses ABAC access model from file if specified
//   - Applies both OIDC and ABAC middleware to the router
//
// Parameters:
//   - ctx: Context for managing the setup lifecycle and cancellation
//   - cfg: Configuration object containing OIDC and ABAC settings
//   - r: Chi router instance to apply security middleware to
//
// Returns:
//   - error: Returns an error if OIDC initialization fails, access model
//     loading fails, or any other security setup issue occurs
//
// Example:
//
//	router := chi.NewRouter()
//	config := &common.Config{
//	  ABAC: common.ABACConfig{Enabled: true, ModelPath: "access_model.json"},
//	  OIDC: common.OIDCConfig{TrustlistPath: "config/trustlist.json"},
//	}
//	err := SetupSecurity(context.Background(), config, router)
//	if err != nil {
//	  log.Fatal("Security setup failed:", err)
//	}
//
// Security Flow:
//  1. Incoming requests are first processed by OIDC middleware for authentication
//  2. Authenticated requests are then evaluated by ABAC middleware for authorization
//  3. Only requests that pass both checks are allowed to proceed to handlers
func SetupSecurity(ctx context.Context, cfg *common.Config, r *api.Mux) error {
	return setupSecurity(ctx, cfg, r, SecuritySetupOptions{})
}

// SetupSecurityForComponent configures security and a component-specific rules table.
func SetupSecurityForComponent(ctx context.Context, cfg *common.Config, r *api.Mux, opts SecuritySetupOptions) error {
	return setupSecurity(ctx, cfg, r, opts)
}

// SetupSecurityWithClaimsMiddleware configures security and optionally injects
// middleware between OIDC and ABAC (e.g., to enrich claims).
func SetupSecurityWithClaimsMiddleware(
	ctx context.Context,
	cfg *common.Config,
	r *api.Mux,
	claimsMiddleware ...func(http.Handler) http.Handler,
) error {
	return setupSecurityWithClaimsMiddleware(ctx, cfg, r, SecuritySetupOptions{}, claimsMiddleware...)
}

// SetupSecurityWithClaimsMiddlewareForComponent configures security with a component-specific rules table.
func SetupSecurityWithClaimsMiddlewareForComponent(
	ctx context.Context,
	cfg *common.Config,
	r *api.Mux,
	opts SecuritySetupOptions,
	claimsMiddleware ...func(http.Handler) http.Handler,
) error {
	return setupSecurityWithClaimsMiddleware(ctx, cfg, r, opts, claimsMiddleware...)
}

func setupSecurity(
	ctx context.Context,
	cfg *common.Config,
	r *api.Mux,
	opts SecuritySetupOptions,
) error {
	return setupSecurityWithClaimsMiddleware(ctx, cfg, r, opts)
}

func setupSecurityWithClaimsMiddleware(
	ctx context.Context,
	cfg *common.Config,
	r *api.Mux,
	opts SecuritySetupOptions,
	claimsMiddleware ...func(http.Handler) http.Handler,
) error {
	if !cfg.ABAC.Enabled {
		return nil
	}

	trustlistData, err := os.ReadFile(cfg.OIDC.TrustlistPath)
	if err != nil {
		return fmt.Errorf("read OIDC trustlist: %w", err)
	}

	var trustlist []common.OIDCProviderConfig
	if err := json.Unmarshal(trustlistData, &trustlist); err != nil {
		return fmt.Errorf("parse OIDC trustlist: %w", err)
	}

	oidcProviders := make([]OIDCProviderSettings, 0, len(trustlist))
	for _, p := range trustlist {
		oidcProviders = append(oidcProviders, OIDCProviderSettings{
			Issuer:   p.Issuer,
			Audience: p.Audience,
			Scopes:   p.Scopes,
		})
	}

	oidc, err := NewOIDC(ctx, OIDCSettings{
		Providers:      oidcProviders,
		AllowAnonymous: true,
	})
	if err != nil {
		return err
	}

	var (
		model        *AccessModel
		modelStore   *AccessModelStore
		rulesRuntime *accessRulesRuntime
	)
	if strings.TrimSpace(opts.RulesTableName) != "" {
		rulesRuntime, err = newAccessRulesRuntime(ctx, cfg, r, opts)
		if err != nil {
			return err
		}
		modelStore = rulesRuntime.ModelStore()
		model = modelStore.Get()
	} else if cfg.ABAC.ModelPath != "" {
		data, err := os.ReadFile(cfg.ABAC.ModelPath)
		if err != nil {
			return err
		}
		m, err := ParseAccessModel(data, r, cfg.Server.ContextPath)
		if err != nil {
			return err
		}
		model = m
	}

	abacSettings := ABACSettings{
		Enabled:             cfg.ABAC.Enabled,
		EnableImplicitCasts: cfg.General.EnableImplicitCasts,
		Model:               model,
		ModelStore:          modelStore,
	}

	// ✅ Apply middlewares to the router
	if len(claimsMiddleware) > 0 {
		chain := append([]func(http.Handler) http.Handler{oidc.Middleware}, claimsMiddleware...)
		chain = append(chain, ABACMiddleware(abacSettings))
		r.Use(chain...)
		if rulesRuntime != nil {
			rulesRuntime.RegisterRoutes(r)
		}
		return nil
	}

	r.Use(
		oidc.Middleware,
		ABACMiddleware(abacSettings),
	)
	if rulesRuntime != nil {
		rulesRuntime.RegisterRoutes(r)
	}

	return nil
}
