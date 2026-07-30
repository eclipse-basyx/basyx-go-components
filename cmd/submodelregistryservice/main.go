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

// Package main implements the Submodel Registry Service server.
package main

import (
	"context"
	"embed"
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/abacpolicy"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"
	smregistryapi "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/api"
	smregistrypostgresql "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/persistence"
	smregistryopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/smregistry"
	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string) error {
	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}
	if _, err = common.ConfigureLogging(cfg, "submodelregistryservice", configPath, os.Stderr); err != nil {
		return err
	}
	telemetryRuntime, err := telemetry.Configure(ctx, "submodelregistryservice")
	if err != nil {
		return err
	}
	defer telemetryRuntime.Shutdown(ctx)
	if err := commonmodel.SetVerificationMode(cfg.Server.StrictVerification); err != nil {
		return err
	}
	history.Configure(history.Config{
		Mode:                 cfg.History.Mode,
		RetentionDays:        cfg.History.RetentionDays,
		FullSnapshotInterval: cfg.History.FullSnapshotInterval,
		Immutability:         cfg.History.Immutability,
		AuditIdentityMode:    cfg.History.AuditIdentityMode,
	})
	if err = history.ConfigureEvidence(ctx, cfg.History.Evidence); err != nil {
		return err
	}
	commonmodel.SetSupportsSingularSupplementalSemanticId(cfg.General.SupportsSingularSupplementalSemanticId)

	r := chi.NewRouter()

	// Make configuration available in request contexts.
	r.Use(common.ConfigMiddleware(cfg))

	common.AddCors(r, cfg)
	common.AddHealthEndpoint(r, cfg)

	// Add Swagger UI
	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Submodel Registry Service API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		slog.WarnContext(ctx, "Swagger UI unavailable", "error.code", "SMREGISTRY-SWAGGER-INIT", "error", err)
	}

	slog.InfoContext(ctx, "connecting to PostgreSQL")

	pools, err := common.OpenPostgresPoolsWithSchemaValidation(ctx, cfg.Postgres, "submodelregistryservice", common.CURRENT_DATABASE_VERSION)
	if err != nil {
		slog.ErrorContext(ctx, "database connection failed", "error.code", "SMREGISTRY-DB-CONNECT", "error", err)
		return err
	}
	defer func() {
		if closeErr := pools.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "database pool shutdown failed", "error.code", "SMREGISTRY-DB-CLOSE", "error", closeErr)
		}
	}()
	sharedDB := pools.Writer
	if err = history.ApplyPostgresGuardConfig(ctx, sharedDB); err != nil {
		return err
	}
	smDatabase, err := smregistrypostgresql.NewPostgreSQLSMBackendFromPools(pools.Writer, pools.Reader)
	if err != nil {
		slog.ErrorContext(ctx, "submodel registry persistence initialization failed", "error.code", "SMREGISTRY-DB-INIT", "error", err)
		return err
	}
	slog.InfoContext(ctx, "PostgreSQL connection established")

	smSvc := smregistryapi.NewSubmodelRegistryAPIAPIService(*smDatabase)
	smCtrl := smregistryopenapi.NewSubmodelRegistryAPIAPIController(smSvc, cfg.Server.ContextPath)
	bulkManager, err := asyncjob.NewPostgresManager(ctx, sharedDB, "SMR-BULK", 0)
	if err != nil {
		slog.ErrorContext(ctx, "async job persistence initialization failed", "error.code", "SMREGISTRY-ASYNCJOB-INIT", "error", err)
		return err
	}
	bulkSvc := smregistryapi.NewBulkService(smSvc, bulkManager)
	bulkHandler := smregistryapi.NewBulkHTTPHandler(bulkSvc)

	descSvc := smregistryapi.NewDescriptionAPIAPIService()
	descCtrl := smregistryopenapi.NewDescriptionAPIAPIController(descSvc)

	base := common.NormalizeBasePath(cfg.Server.ContextPath)
	// luk
	// === Protected API Subrouter ===
	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "SubmodelRegistryService")

	// Apply OIDC + ABAC once for all registry endpoints
	abacRepo, err := abacpolicy.SetupSecurityWithABACRepository(ctx, cfg, apiRouter, sharedDB, "submodelregistryservice")
	if err != nil {
		return err
	}
	versioningGuard := history.NewMutationCoverageGuard(apiRouter)
	versioningGuard.Exempt(http.MethodPost, "/verify")
	apiRouter.Use(versioningGuard.Middleware)
	apiRouter.Use(history.AuditContextMiddleware(cfg))
	abacpolicy.ExemptManagementMutationRoutesIfEnabled(cfg, versioningGuard, "submodelregistryservice")
	abacpolicy.RegisterManagementRoutesIfEnabled(cfg, apiRouter, abacRepo, "submodelregistryservice")
	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(apiRouter, cfg, binarycontent.NewStager(sharedDB))
	}

	// Register all registry routes (protected)
	for _, rt := range smCtrl.OrderedRoutes() {
		versioningGuard.ClassifyRoute(rt.Name, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Register all description routes (protected)
	for _, rt := range descCtrl.OrderedRoutes() {
		versioningGuard.ClassifyRoute(rt.Name, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	versioningGuard.Cover(http.MethodPost, "/bulk/submodel-descriptors")
	versioningGuard.Cover(http.MethodPut, "/bulk/submodel-descriptors")
	versioningGuard.Cover(http.MethodDelete, "/bulk/submodel-descriptors")
	bulkHandler.RegisterRoutes(apiRouter, true)

	// Mount protected API under base path
	r.Mount(base, apiRouter)

	addr := common.ServerAddress(cfg.Server)
	slog.InfoContext(ctx, "HTTP server starting", "address", addr, "context_path", cfg.Server.ContextPath)

	return common.RunHTTPServer(ctx, "SMR", cfg.Server, r)
}

func main() {
	ctx, stop := common.SignalContext()

	configPath := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		slog.ErrorContext(ctx, "server stopped", "error.code", "SMREGISTRY-MAIN-RUNSERVER", "error", err)
		stop()
		os.Exit(1)
	}
	stop()
}
