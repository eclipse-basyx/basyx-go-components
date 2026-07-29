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

// Package main implements the Digital Twin Registry service (AAS Registry + Discovery).
package main

import (
	"context"
	"embed"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	registryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	registrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/abacpolicy"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"
	"github.com/eclipse-basyx/basyx-go-components/internal/digitaltwinregistry"
	discoveryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
	discoverydb "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/persistence"
	registryapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasregistryapi"
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
	if _, err = common.ConfigureLogging(cfg, "digitaltwinregistryservice", configPath, os.Stderr); err != nil {
		return err
	}
	telemetryRuntime, err := telemetry.Configure(ctx, "digitaltwinregistryservice")
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

	// Digital Twin Registry always enables discovery integration.
	cfg.General.DiscoveryIntegration = true

	r := chi.NewRouter()

	r.Use(common.ConfigMiddleware(cfg))
	common.AddCors(r, cfg)
	common.AddHealthEndpoint(r, cfg)

	// Add Swagger UI
	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Digital Twin Registry API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		slog.WarnContext(ctx, "Swagger UI unavailable", "error.code", "DTR-SWAGGER-INIT", "error", err)
	}

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	// === Database ===
	dsn := common.BuildPostgresDSN(cfg.Postgres)

	if err := common.ValidateSchemaVersionByDSN(dsn, common.CURRENT_DATABASE_VERSION); err != nil {
		return err
	}

	slog.InfoContext(ctx, "connecting to PostgreSQL")

	sharedDB, err := common.NewDatabaseConnection(dsn)
	if err != nil {
		slog.ErrorContext(ctx, "database connection failed", "error.code", "DTR-DB-CONNECT", "error", err)
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

	registryDatabase, err := registrydb.NewPostgreSQLAASRegistryDatabaseFromDB(sharedDB, cfg.Server.CacheEnabled)
	if err != nil {
		slog.ErrorContext(ctx, "registry persistence initialization failed", "error.code", "DTR-REGISTRY-INIT", "error", err)
		return err
	}

	discoveryDatabase, err := discoverydb.NewPostgreSQLDiscoveryBackendFromDB(sharedDB)
	if err != nil {
		slog.ErrorContext(ctx, "discovery persistence initialization failed", "error.code", "DTR-DISCOVERY-INIT", "error", err)
		return err
	}
	slog.InfoContext(ctx, "PostgreSQL connection established")

	discoveryBaseSvc := discoveryapiinternal.NewAssetAdministrationShellBasicDiscoveryAPIAPIService(*discoveryDatabase)
	registrySvc := digitaltwinregistry.NewCustomRegistryService(
		registryapiinternal.NewAssetAdministrationShellRegistryAPIAPIService(*registryDatabase),
		discoveryBaseSvc,
	)
	discoverySvc := digitaltwinregistry.NewCustomDiscoveryService(
		discoveryBaseSvc,
		registryDatabase,
	)

	registryCtrl := registryapi.NewAssetAdministrationShellRegistryAPIAPIController(registrySvc, cfg.Server.ContextPath)
	bulkManager, err := asyncjob.NewPostgresManager(ctx, sharedDB, "DTR-BULK", 0)
	if err != nil {
		slog.ErrorContext(ctx, "async bulk persistence initialization failed", "error.code", "DTR-ASYNCJOB-INIT", "error", err)
		return err
	}
	bulkSvc := registryapiinternal.NewBulkService(registrySvc, bulkManager)
	bulkHandler := registryapiinternal.NewBulkHTTPHandler(bulkSvc)
	discoveryCtrl := openapi.NewAssetAdministrationShellBasicDiscoveryAPIAPIController(discoverySvc)
	descriptionSvc := digitaltwinregistry.NewDescriptionService()
	descriptionCtrl := openapi.NewDescriptionAPIAPIController(descriptionSvc)

	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "DigitalTwinRegistryService")
	var claimsMiddleware []func(http.Handler) http.Handler
	if cfg.General.EnableCustomMiddlewareHeaderInjection {
		claimsMiddleware = append(claimsMiddleware, auth.EdcBpnHeaderMiddleware)
	}

	abacRepo, err := abacpolicy.SetupSecurityWithABACRepository(ctx, cfg, apiRouter, sharedDB, "digitaltwinregistryservice", claimsMiddleware...)
	if err != nil {
		return err
	}
	versioningGuard := history.NewMutationCoverageGuard(apiRouter)
	versioningGuard.Exempt(http.MethodPost, "/verify")
	apiRouter.Use(versioningGuard.Middleware)
	apiRouter.Use(history.AuditContextMiddleware(cfg))
	abacpolicy.ExemptManagementMutationRoutesIfEnabled(cfg, versioningGuard, "digitaltwinregistryservice")
	abacpolicy.RegisterManagementRoutesIfEnabled(cfg, apiRouter, abacRepo, "digitaltwinregistryservice")
	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(apiRouter, cfg, binarycontent.NewStager(sharedDB))
	}

	for operation, rt := range registryCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		if rt.Method == "GET" && rt.Pattern == "/shell-descriptors" {
			apiRouter.With(digitaltwinregistry.CreatedAfterMiddleware).Method(rt.Method, rt.Pattern, rt.HandlerFunc)
			continue
		}
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for operation, rt := range discoveryCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		if (rt.Method == "POST" && rt.Pattern == "/lookup/shellsByAssetLink") || (rt.Method == "GET" && rt.Pattern == "/lookup/shells") {
			apiRouter.With(digitaltwinregistry.CreatedAfterMiddleware).Method(rt.Method, rt.Pattern, rt.HandlerFunc)
			continue
		}
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for operation, rt := range descriptionCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	versioningGuard.Cover(http.MethodPost, "/bulk/shell-descriptors")
	versioningGuard.Cover(http.MethodPut, "/bulk/shell-descriptors")
	versioningGuard.Cover(http.MethodDelete, "/bulk/shell-descriptors")
	bulkHandler.RegisterRoutes(apiRouter, true)

	r.Mount(base, apiRouter)

	addr := common.ServerAddress(cfg.Server)
	slog.InfoContext(ctx, "HTTP server starting", "address", addr, "context_path", cfg.Server.ContextPath)

	return common.RunHTTPServer(ctx, "DTR", cfg.Server, r)
}

func main() {
	ctx, stop := common.SignalContext()
	configPath := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		slog.ErrorContext(ctx, "server stopped", "error.code", "DTR-MAIN-RUNSERVER", "error", err)
		stop()
		os.Exit(1)
	}
	stop()
}
