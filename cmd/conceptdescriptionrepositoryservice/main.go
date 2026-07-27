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

// Package main implements the Concept Description Repository Service server.
package main

import (
	"context"
	"embed"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/abacpolicy"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"
	"github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/api"
	"github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/conceptdescriptionrepositoryapi/go"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string) error {
	// Load configuration
	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}
	if _, err = common.ConfigureLogging(cfg, "conceptdescriptionrepositoryservice", configPath, os.Stderr); err != nil {
		return err
	}
	telemetryRuntime, err := telemetry.Configure(ctx, "conceptdescriptionrepositoryservice")
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

	// Create Chi router
	r := chi.NewRouter()
	common.AddDefaultRouterErrorHandlers(r, "ConceptDescriptionRepositoryService")

	// Make configuration available in request contexts.
	r.Use(common.ConfigMiddleware(cfg))

	common.AddCors(r, cfg)
	common.AddHealthEndpoint(r, cfg)

	// Add Swagger UI
	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Concept Description Repository API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		slog.WarnContext(ctx, "Swagger UI unavailable", "error.code", "CDREPOSITORY-SWAGGER-INIT", "error", err)
	}

	// Instantiate generated services & controllers
	// ==== Concept Description Repository Service ====

	dsn := common.BuildPostgresDSN(cfg.Postgres)

	if err := common.ValidateSchemaVersionByDSN(dsn, common.CURRENT_DATABASE_VERSION); err != nil {
		return err
	}

	sharedDB, err := common.NewDatabaseConnection(dsn)
	if err != nil {
		return err
	}
	if cfg.Postgres.MaxOpenConnections > 0 {
		sharedDB.SetMaxOpenConns(cfg.Postgres.MaxOpenConnections)
	}
	if cfg.Postgres.MaxIdleConnections > 0 {
		sharedDB.SetMaxIdleConns(cfg.Postgres.MaxIdleConnections)
	}
	if cfg.Postgres.ConnMaxLifetimeMinutes > 0 {
		sharedDB.SetConnMaxLifetime(time.Duration(cfg.Postgres.ConnMaxLifetimeMinutes) * time.Minute)
	}
	if err = history.ApplyPostgresGuardConfig(ctx, sharedDB); err != nil {
		return err
	}

	cdDatabase, err := persistence.NewConceptDescriptionBackendFromDB(sharedDB)
	if err != nil {
		return err
	}

	cdSvc := api.NewConceptDescriptionRepositoryAPIAPIService(cdDatabase)
	cdCtrl := openapi.NewConceptDescriptionRepositoryAPIAPIController(cdSvc, "", cfg.Server.StrictVerification)

	// ==== Description Service ====
	descSvc := api.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	// === Protected API Subrouter ===
	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "ConceptDescriptionRepositoryService")

	// Apply OIDC + ABAC once for all repository endpoints
	abacRepo, err := abacpolicy.SetupSecurityWithABACRepository(ctx, cfg, apiRouter, sharedDB, "conceptdescriptionrepositoryservice")
	if err != nil {
		return err
	}
	versioningGuard := history.NewMutationCoverageGuard(apiRouter)
	versioningGuard.Exempt(http.MethodPost, "/verify")
	apiRouter.Use(versioningGuard.Middleware)
	apiRouter.Use(history.AuditContextMiddleware(cfg))
	abacpolicy.ExemptManagementMutationRoutesIfEnabled(cfg, versioningGuard, "conceptdescriptionrepositoryservice")
	abacpolicy.RegisterManagementRoutesIfEnabled(cfg, apiRouter, abacRepo, "conceptdescriptionrepositoryservice")
	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(apiRouter, cfg, binarycontent.NewStager(sharedDB))
	}

	for operation, rt := range cdCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for operation, rt := range descCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Mount protected API under base path
	r.Mount(base, apiRouter)

	addr := common.ServerAddress(cfg.Server)
	slog.InfoContext(ctx, "HTTP server starting", "address", addr, "context_path", cfg.Server.ContextPath)

	return common.RunHTTPServer(ctx, "CDREPO", cfg.Server, r)
}

func main() {
	ctx, stop := common.SignalContext()
	// load config path from flag
	configPath := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		slog.ErrorContext(ctx, "server stopped", "error.code", "CDREPOSITORY-MAIN-RUNSERVER", "error", err)
		stop()
		os.Exit(1)
	}
	stop()
}
