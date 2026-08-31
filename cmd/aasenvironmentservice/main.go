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

// Package main starts the AAS Environment Service HTTP server.
package main

import (
	"context"
	"crypto/rsa"
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"

	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	aasregistryapi "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	aasrepositoryapi "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/abacpolicy"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"
	cdrapi "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/api"
	cdrdb "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
	discoveryapi "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
	discoverydb "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/persistence"
	smregistryapi "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/api"
	smregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/persistence"
	submodelrepositoryapi "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/api"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	aasregistryopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasregistryapi"
	aasrepositoryopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasrepositoryapi/go"
	cdropenapi "github.com/eclipse-basyx/basyx-go-components/pkg/conceptdescriptionrepositoryapi/go"
	discoveryopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
	smregistryopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/smregistry"
	submodelrepositoryopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi"
	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string) error {
	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}
	if _, err = common.ConfigureLogging(cfg, "aasenvironmentservice", configPath, os.Stderr); err != nil {
		return err
	}
	telemetryRuntime, err := telemetry.Configure(ctx, "aasenvironmentservice")
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

	registrySyncConfig, err := aasenvironment.NewRegistrySyncConfig(
		cfg.General.AASRegistryIntegration,
		cfg.General.SubmodelRegistryIntegration,
		cfg.General.ExternalURL,
	)
	if err != nil {
		return err
	}
	commonmodel.SetSupportsSingularSupplementalSemanticId(cfg.General.SupportsSingularSupplementalSemanticId)

	// AAS Environment Service always enables discovery integration.
	cfg.General.DiscoveryIntegration = true

	r := chi.NewRouter()
	r.Use(common.ConfigMiddleware(cfg))
	common.AddCors(r, cfg)

	preconfigurationCompleted := atomic.Bool{}
	common.AddHealthEndpointWithProbe(r, cfg, func() (bool, string) {
		if preconfigurationCompleted.Load() {
			return true, ""
		}
		return false, "AAS preconfiguration in progress"
	})

	if err = common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "AAS Environment Service API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		slog.WarnContext(ctx, "Swagger UI unavailable", "error.code", "AASENV-SWAGGER-INIT", "error", err)
	}

	pools, asyncJobManager, sharedBulkManager, err := openSharedDatabase(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := pools.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "database pool shutdown failed", "error.code", "AASENV-DB-CLOSE", "error", closeErr)
		}
	}()
	sharedDB := pools.Writer

	var privateKey *rsa.PrivateKey
	if cfg.JWS.PrivateKeyPath != "" {
		privateKey, err = jws.LoadPrivateKey(cfg.JWS.PrivateKeyPath)
		if err != nil {
			return err
		}
	}
	signingOptions, err := jws.LoadSigningOptions(cfg.JWS.CertificateChainPath)
	if err != nil {
		slog.WarnContext(ctx, "JWS certificate chain unavailable; x5c headers are disabled", "error.code", "AASENV-JWS-LOADCHAIN", "error", err)
	}

	aasRegistryPersistence, err := aasregistrydb.NewPostgreSQLAASRegistryDatabaseFromPools(pools.Writer, pools.Reader, cfg.Server.CacheEnabled)
	if err != nil {
		return err
	}
	smRegistryPersistence, err := smregistrydb.NewPostgreSQLSMBackendFromPools(pools.Writer, pools.Reader)
	if err != nil {
		return err
	}
	aasRepositoryPersistence, err := aasrepositorydb.NewAssetAdministrationShellDatabaseFromPools(pools.Writer, pools.Reader, cfg.Server.StrictVerification)
	if err != nil {
		return err
	}
	aasRepositoryPersistence.SetJWSPrivateKey(privateKey)
	aasRepositoryPersistence.SetJWSCertificateChain(signingOptions.CertificateChain)
	submodelRepositoryPersistence, err := submodelrepositorydb.NewSubmodelDatabaseFromPools(pools.Writer, pools.Reader, privateKey, cfg.Server.StrictVerification)
	if err != nil {
		return err
	}
	submodelRepositoryPersistence.SetJWSCertificateChain(signingOptions.CertificateChain)
	cdrPersistence, err := cdrdb.NewConceptDescriptionBackendFromPools(pools.Writer, pools.Reader)
	if err != nil {
		return err
	}
	discoveryPersistence, err := discoverydb.NewPostgreSQLDiscoveryBackendFromPools(pools.Writer, pools.Reader)
	if err != nil {
		return err
	}

	persistence := &aasenvironment.Persistence{
		DB:                           sharedDB,
		AASRegistry:                  aasRegistryPersistence,
		SubmodelRegistry:             smRegistryPersistence,
		AASRepository:                aasRepositoryPersistence,
		SubmodelRepository:           submodelRepositoryPersistence,
		ConceptDescriptionRepository: cdrPersistence,
		Discovery:                    discoveryPersistence,
	}
	eventFeedModule, err := eventfeed.NewModule(sharedDB, common.NewEventFeedConfig(cfg.Eventing))
	if err != nil {
		return err
	}
	defer eventFeedModule.Stop()
	eventFeedModule.StartRetentionLoop(ctx)

	customAASRegistry := aasenvironment.NewCustomAASRegistryService(
		aasregistryapi.NewAssetAdministrationShellRegistryAPIAPIService(*aasRegistryPersistence),
		persistence,
	)
	customSMRegistry := aasenvironment.NewCustomSubmodelRegistryService(
		smregistryapi.NewSubmodelRegistryAPIAPIService(*smRegistryPersistence),
		persistence,
	)
	customAASRepository := aasenvironment.NewCustomAASRepositoryService(
		aasrepositoryapi.NewAssetAdministrationShellRepositoryAPIAPIService(ctx, aasRepositoryPersistence, submodelRepositoryPersistence, asyncJobManager),
		persistence,
		registrySyncConfig,
	)
	customAASRepository.SetEventFeed(eventFeedModule)
	customSMRepository := aasenvironment.NewCustomSubmodelRepositoryService(
		submodelrepositoryapi.NewSubmodelRepositoryAPIAPIService(ctx, *submodelRepositoryPersistence, asyncJobManager),
		persistence,
		registrySyncConfig,
	)
	customSMRepository.SetEventFeed(eventFeedModule)
	customCDRepository := aasenvironment.NewCustomConceptDescriptionRepositoryService(
		cdrapi.NewConceptDescriptionRepositoryAPIAPIService(cdrPersistence),
		persistence,
	)
	customDiscovery := aasenvironment.NewCustomDiscoveryService(
		discoveryapi.NewAssetAdministrationShellBasicDiscoveryAPIAPIService(*discoveryPersistence),
		persistence,
	)
	environmentStager := common.NewConnectionReservedUploadStager(
		binarycontent.NewStager(sharedDB), sharedDB.Stats().MaxOpenConnections, 1,
	)
	serializationService := aasenvironment.NewSerializationAPIService(persistence, environmentStager)
	aasBulkSvc := aasregistryapi.NewBulkService(customAASRegistry, sharedBulkManager)
	smBulkSvc := smregistryapi.NewBulkService(customSMRegistry, sharedBulkManager)
	aasBulkHandler := aasregistryapi.NewBulkHTTPHandler(aasBulkSvc)
	smBulkHandler := smregistryapi.NewBulkHTTPHandler(smBulkSvc)

	aasRegistryCtrl := aasregistryopenapi.NewAssetAdministrationShellRegistryAPIAPIController(customAASRegistry, cfg.Server.ContextPath)
	smRegistryCtrl := smregistryopenapi.NewSubmodelRegistryAPIAPIController(customSMRegistry, cfg.Server.ContextPath)
	aasRepositoryCtrl := aasrepositoryopenapi.NewAssetAdministrationShellRepositoryAPIAPIController(customAASRepository, "", cfg.Server.StrictVerification)
	smRepositoryCtrl := submodelrepositoryopenapi.NewSubmodelRepositoryAPIAPIController(customSMRepository, "", cfg.Server.StrictVerification)
	cdrCtrl := cdropenapi.NewConceptDescriptionRepositoryAPIAPIController(customCDRepository, "", cfg.Server.StrictVerification)
	discoveryCtrl := discoveryopenapi.NewAssetAdministrationShellBasicDiscoveryAPIAPIController(customDiscovery)
	descriptionCtrl := discoveryopenapi.NewDescriptionAPIAPIController(aasenvironment.NewDescriptionService())

	base := common.NormalizeBasePath(cfg.Server.ContextPath)
	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "AASEnvironmentService")

	abacRepo, err := abacpolicy.SetupSecurityWithABACRepository(ctx, cfg, apiRouter, sharedDB, "aasenvironmentservice")
	if err != nil {
		return err
	}
	versioningGuard := history.NewMutationCoverageGuard(apiRouter)
	versioningGuard.Exempt(http.MethodPost, "/verify")
	apiRouter.Use(versioningGuard.Middleware)
	apiRouter.Use(history.AuditContextMiddleware(cfg))
	abacpolicy.ExemptManagementMutationRoutesIfEnabled(cfg, versioningGuard, "aasenvironmentservice")
	abacpolicy.RegisterManagementRoutesIfEnabled(cfg, apiRouter, abacRepo, "aasenvironmentservice")
	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(apiRouter, cfg, environmentStager)
	}

	for operation, rt := range aasRegistryCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for operation, rt := range smRegistryCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for operation, rt := range aasRepositoryCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for operation, rt := range smRepositoryCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for operation, rt := range cdrCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for operation, rt := range discoveryCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for operation, rt := range descriptionCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	versioningGuard.Cover(http.MethodPost, "/bulk/shell-descriptors")
	versioningGuard.Cover(http.MethodPut, "/bulk/shell-descriptors")
	versioningGuard.Cover(http.MethodDelete, "/bulk/shell-descriptors")
	versioningGuard.Cover(http.MethodPost, "/bulk/submodel-descriptors")
	versioningGuard.Cover(http.MethodPut, "/bulk/submodel-descriptors")
	versioningGuard.Cover(http.MethodDelete, "/bulk/submodel-descriptors")
	aasBulkHandler.RegisterRoutes(apiRouter, true)
	smBulkHandler.RegisterRoutes(apiRouter, false)

	eventFeedModule.RegisterRoutes(apiRouter)

	r.Mount(base, apiRouter)

	// Register /upload endpoint
	uploadService := aasenvironment.NewUploadAPIService(persistence, customAASRepository, customSMRepository)
	versioningGuard.Cover(http.MethodPost, "/upload")
	aasenvironment.RegisterUploadAPI(apiRouter, uploadService, cfg.General.UploadMaxSizeBytes, environmentStager)
	aasenvironment.RegisterSerializationAPI(apiRouter, serializationService)

	addr := common.ServerAddress(cfg.Server)
	slog.InfoContext(ctx, "HTTP server starting", "address", addr, "context_path", cfg.Server.ContextPath)
	runner, err := common.StartHTTPServer(ctx, "AASENV", cfg.Server, r)
	if err != nil {
		return err
	}

	preconfigurationCtx := aasenvironment.ContextWithAASPreconfigurationAudit(common.ContextWithConfig(ctx, cfg))
	aasenvironment.RunAASPreconfiguration(preconfigurationCtx, uploadService, cfg.General.AASPreconfigPaths)
	preconfigurationCompleted.Store(true)

	return runner.Wait(ctx)
}

func openSharedDatabase(
	ctx context.Context,
	cfg *common.Config,
) (*common.PostgresPools, *asyncjob.Manager, *asyncjob.Manager, error) {
	pools, err := common.OpenPostgresPoolsWithSchemaValidation(ctx, cfg.Postgres, "aasenvironmentservice", common.CURRENT_DATABASE_VERSION)
	if err != nil {
		return nil, nil, nil, err
	}
	db := pools.Writer
	if err = history.ApplyPostgresGuardConfig(ctx, db); err != nil {
		_ = pools.Close()
		return nil, nil, nil, err
	}
	asyncJobManager, err := submodelrepositoryapi.NewAsyncJobManager(ctx, db)
	if err != nil {
		_ = pools.Close()
		return nil, nil, nil, fmt.Errorf("AASENV-ASYNCJOB-INIT %w", err)
	}
	bulkManager, err := asyncjob.NewPostgresManager(ctx, db, "AASENV-BULK", 0)
	if err != nil {
		_ = pools.Close()
		return nil, nil, nil, fmt.Errorf("AASENV-ASYNCJOB-INIT %w", err)
	}
	return pools, asyncJobManager, bulkManager, nil
}

func main() {
	ctx, stop := common.SignalContext()
	configPath := ""

	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		slog.ErrorContext(ctx, "server stopped", "error.code", "AASENV-MAIN-RUNSERVER", "error", err)
		stop()
		os.Exit(1)
	}
	stop()
}
