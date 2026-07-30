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

// Package main implements the Asset Administration Shell Repository Service server.
package main

import (
	"context"
	"crypto/rsa"
	"embed"
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/abacpolicy"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"
	submodelrepositoryapi "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/api"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasrepositoryapi/go"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string) error {
	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}
	if _, err = common.ConfigureLogging(cfg, "aasrepositoryservice", configPath, os.Stderr); err != nil {
		return err
	}
	telemetryRuntime, err := telemetry.Configure(ctx, "aasrepositoryservice")
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

	if err = aasenvironment.ValidateStandaloneAASRepositoryRegistrySyncConfig(cfg); err != nil {
		return err
	}
	registrySyncConfig, err := aasenvironment.NewRegistrySyncConfig(
		cfg.General.AASRegistryIntegration,
		cfg.General.SubmodelRegistryIntegration,
		cfg.General.ExternalURL,
	)
	if err != nil {
		return err
	}

	r := chi.NewRouter()
	r.Use(common.ConfigMiddleware(cfg))

	common.AddCors(r, cfg)

	common.AddHealthEndpoint(r, cfg)

	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Asset Administration Shell Repository API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		slog.WarnContext(ctx, "Swagger UI unavailable", "error.code", "AASREPOSITORY-SWAGGER-INIT", "error", err)
	}

	var privateKey *rsa.PrivateKey
	if cfg.JWS.PrivateKeyPath != "" {
		privateKey, err = jws.LoadPrivateKey(cfg.JWS.PrivateKeyPath)
		if err != nil {
			slog.WarnContext(ctx, "JWS private key unavailable; signed endpoints are disabled", "error.code", "AASREPOSITORY-JWS-LOADKEY", "error", err)
		} else {
			slog.InfoContext(ctx, "JWS private key loaded")
		}
	}
	signingOptions, err := jws.LoadSigningOptions(cfg.JWS.CertificateChainPath)
	if err != nil {
		slog.WarnContext(ctx, "JWS certificate chain unavailable; x5c headers are disabled", "error.code", "AASREPOSITORY-JWS-LOADCHAIN", "error", err)
	}

	slog.InfoContext(ctx, "connecting to PostgreSQL")

	pools, err := common.OpenPostgresPoolsWithSchemaValidation(ctx, cfg.Postgres, "aasrepositoryservice", common.CURRENT_DATABASE_VERSION)
	if err != nil {
		slog.ErrorContext(ctx, "database connection failed", "error.code", "AASREPOSITORY-DB-CONNECT", "error", err)
		return err
	}
	defer func() {
		if closeErr := pools.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "database pool shutdown failed", "error.code", "AASREPOSITORY-DB-CLOSE", "error", closeErr)
		}
	}()
	sharedDB := pools.Writer
	if err = history.ApplyPostgresGuardConfig(ctx, sharedDB); err != nil {
		return err
	}

	aasDatabase, err := persistencepostgresql.NewAssetAdministrationShellDatabaseFromPools(pools.Writer, pools.Reader, cfg.Server.StrictVerification)
	if err != nil {
		slog.ErrorContext(ctx, "AAS repository persistence initialization failed", "error.code", "AASREPOSITORY-DB-INIT", "error", err)
		return err
	}
	aasDatabase.SetJWSPrivateKey(privateKey)
	aasDatabase.SetJWSCertificateChain(signingOptions.CertificateChain)

	aasRegistryPersistence, err := aasregistrydb.NewPostgreSQLAASRegistryDatabaseFromPools(pools.Writer, pools.Reader, cfg.Server.CacheEnabled)
	if err != nil {
		slog.ErrorContext(ctx, "AAS registry persistence initialization failed", "error.code", "AASREPOSITORY-AASREGISTRY-INIT", "error", err)
		return err
	}

	submodelDatabase, err := submodelrepositorydb.NewSubmodelDatabaseFromPools(pools.Writer, pools.Reader, nil, cfg.Server.StrictVerification)
	if err != nil {
		slog.ErrorContext(ctx, "submodel persistence initialization failed", "error.code", "AASREPOSITORY-SMREPOSITORY-INIT", "error", err)
		return err
	}
	asyncJobManager, err := submodelrepositoryapi.NewAsyncJobManager(ctx, sharedDB)
	if err != nil {
		slog.ErrorContext(ctx, "async job persistence initialization failed", "error.code", "AASREPOSITORY-ASYNCJOB-INIT", "error", err)
		return err
	}
	slog.InfoContext(ctx, "PostgreSQL connection established")

	persistence := &aasenvironment.Persistence{
		DB:                 sharedDB,
		AASRegistry:        aasRegistryPersistence,
		AASRepository:      aasDatabase,
		SubmodelRepository: submodelDatabase,
	}
	aasSvc := aasenvironment.NewCustomAASRepositoryService(
		api.NewAssetAdministrationShellRepositoryAPIAPIService(ctx, aasDatabase, submodelDatabase, asyncJobManager),
		persistence,
		registrySyncConfig,
	)
	aasCtrl := openapi.NewAssetAdministrationShellRepositoryAPIAPIController(aasSvc, "", cfg.Server.StrictVerification)

	descSvc := openapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "AASRepositoryService")

	abacRepo, err := abacpolicy.SetupSecurityWithABACRepository(ctx, cfg, apiRouter, sharedDB, "aasrepositoryservice")
	if err != nil {
		return err
	}
	versioningGuard := history.NewMutationCoverageGuard(apiRouter)
	versioningGuard.Exempt(http.MethodPost, "/verify")
	apiRouter.Use(versioningGuard.Middleware)
	apiRouter.Use(history.AuditContextMiddleware(cfg))
	abacpolicy.ExemptManagementMutationRoutesIfEnabled(cfg, versioningGuard, "aasrepositoryservice")
	abacpolicy.RegisterManagementRoutesIfEnabled(cfg, apiRouter, abacRepo, "aasrepositoryservice")
	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(apiRouter, cfg, binarycontent.NewStager(sharedDB))
	}

	for operation, rt := range aasCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	for operation, rt := range descCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	r.Mount(base, apiRouter)

	addr := common.ServerAddress(cfg.Server)
	slog.InfoContext(ctx, "HTTP server starting", "address", addr, "context_path", cfg.Server.ContextPath)

	return common.RunHTTPServer(ctx, "AASREPO", cfg.Server, r)
}

func main() {
	ctx, stop := common.SignalContext()
	// load config path from flag
	configPath := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		slog.ErrorContext(ctx, "server stopped", "error.code", "AASREPOSITORY-MAIN-RUNSERVER", "error", err)
		stop()
		os.Exit(1)
	}
	stop()
}
