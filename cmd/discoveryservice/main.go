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

// Package main implements the Discovery Service server.
package main

import (
	"context"
	"embed"
	"flag"
	"log/slog"
	"os"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/abacpolicy"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"
	"github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string) error {
	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}
	if _, err = common.ConfigureLogging(cfg, "discoveryservice", configPath, os.Stderr); err != nil {
		return err
	}
	telemetryRuntime, err := telemetry.Configure(ctx, "discoveryservice")
	if err != nil {
		return err
	}
	defer telemetryRuntime.Shutdown(ctx)
	if err := commonmodel.SetVerificationMode(cfg.Server.StrictVerification); err != nil {
		return err
	}

	// === Main Router ===
	r := chi.NewRouter()

	// Inject config into request context (used by descriptor debug helpers)
	r.Use(common.ConfigMiddleware(cfg))

	common.AddCors(r, cfg)

	// --- Health Endpoint (public) ---
	common.AddHealthEndpoint(r, cfg)

	// Add Swagger UI
	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Discovery Service API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		slog.WarnContext(ctx, "Swagger UI unavailable", "error.code", "DISCOVERY-SWAGGER-INIT", "error", err)
	}

	// === Database ===
	slog.InfoContext(ctx, "connecting to PostgreSQL")

	sharedDB, err := common.OpenPostgresWithSchemaValidation(ctx, cfg.Postgres, "discoveryservice", common.CURRENT_DATABASE_VERSION)
	if err != nil {
		slog.ErrorContext(ctx, "database connection failed", "error.code", "DISCOVERY-DB-CONNECT", "error", err)
		return err
	}
	smDatabase, err := persistencepostgresql.NewPostgreSQLDiscoveryBackendFromDB(sharedDB)
	if err != nil {
		slog.ErrorContext(ctx, "discovery persistence initialization failed", "error.code", "DISCOVERY-DB-INIT", "error", err)
		return err
	}
	slog.InfoContext(ctx, "PostgreSQL connection established")

	smSvc := api.NewAssetAdministrationShellBasicDiscoveryAPIAPIService(*smDatabase)
	smCtrl := openapi.NewAssetAdministrationShellBasicDiscoveryAPIAPIController(smSvc)

	// === Description Service (public) ===
	descSvc := openapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	// === Protected API Subrouter ===
	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "DiscoveryService")

	// Apply OIDC + ABAC once for all discovery endpoints
	abacRepo, err := abacpolicy.SetupSecurityWithABACRepository(ctx, cfg, apiRouter, sharedDB, "discoveryservice")
	if err != nil {
		return err
	}
	abacpolicy.RegisterManagementRoutesIfEnabled(cfg, apiRouter, abacRepo, "discoveryservice")
	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(apiRouter, cfg, binarycontent.NewStager(sharedDB))
	}

	// Register all discovery routes (protected)
	for _, rt := range smCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Register all description routes (protected)
	for _, rt := range descCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Mount protected API under base path
	r.Mount(base, apiRouter)

	addr := common.ServerAddress(cfg.Server)
	slog.InfoContext(ctx, "HTTP server starting", "address", addr, "context_path", cfg.Server.ContextPath)

	return common.RunHTTPServer(ctx, "DISCOVERY", cfg.Server, r)
}

func main() {
	ctx, stop := common.SignalContext()
	configPath := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		slog.ErrorContext(ctx, "server stopped", "error.code", "DISCOVERY-MAIN-RUNSERVER", "error", err)
		stop()
		os.Exit(1)
	}
	stop()
}
