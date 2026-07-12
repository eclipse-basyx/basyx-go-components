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
	"database/sql"
	"embed"
	"flag"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	aasregistryapi "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	aasrepositoryapi "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncbulk"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/abacpolicy"
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
	log.Default().Println("Loading AAS Environment Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath, common.NORMAL)
	if err != nil {
		return err
	}

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
		log.Printf("Warning: failed to load OpenAPI spec for Swagger UI: %v", err)
	}

	dsn := common.BuildPostgresDSN(cfg.Postgres)

	if err := common.ValidateSchemaVersionByDSN(dsn, common.CURRENT_DATABASE_VERSION); err != nil {
		return err
	}

	sharedDB, err := openSharedDatabase(ctx, cfg, dsn)
	if err != nil {
		return err
	}

	var privateKey *rsa.PrivateKey
	if cfg.JWS.PrivateKeyPath != "" {
		privateKey, err = jws.LoadPrivateKey(cfg.JWS.PrivateKeyPath)
		if err != nil {
			return err
		}
	}
	signingOptions, err := jws.LoadSigningOptions(cfg.JWS.CertificateChainPath)
	if err != nil {
		log.Printf("Warning: failed to load JWS certificate chain: %v - x5c header will be omitted", err)
	}

	aasRegistryPersistence, err := aasregistrydb.NewPostgreSQLAASRegistryDatabaseFromDB(sharedDB, cfg.Server.CacheEnabled)
	if err != nil {
		return err
	}
	smRegistryPersistence, err := smregistrydb.NewPostgreSQLSMBackendFromDB(sharedDB)
	if err != nil {
		return err
	}
	aasRepositoryPersistence, err := aasrepositorydb.NewAssetAdministrationShellDatabaseFromDB(sharedDB, cfg.Server.StrictVerification)
	if err != nil {
		return err
	}
	aasRepositoryPersistence.SetJWSPrivateKey(privateKey)
	aasRepositoryPersistence.SetJWSCertificateChain(signingOptions.CertificateChain)
	submodelRepositoryPersistence, err := submodelrepositorydb.NewSubmodelDatabaseFromDB(sharedDB, privateKey, cfg.Server.StrictVerification)
	if err != nil {
		return err
	}
	submodelRepositoryPersistence.SetJWSCertificateChain(signingOptions.CertificateChain)
	cdrPersistence, err := cdrdb.NewConceptDescriptionBackendFromDB(sharedDB)
	if err != nil {
		return err
	}
	discoveryPersistence, err := discoverydb.NewPostgreSQLDiscoveryBackendFromDB(sharedDB)
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

	customAASRegistry := aasenvironment.NewCustomAASRegistryService(
		aasregistryapi.NewAssetAdministrationShellRegistryAPIAPIService(*aasRegistryPersistence),
		persistence,
	)
	customSMRegistry := aasenvironment.NewCustomSubmodelRegistryService(
		smregistryapi.NewSubmodelRegistryAPIAPIService(*smRegistryPersistence),
		persistence,
	)
	customAASRepository := aasenvironment.NewCustomAASRepositoryService(
		aasrepositoryapi.NewAssetAdministrationShellRepositoryAPIAPIService(aasRepositoryPersistence, submodelRepositoryPersistence),
		persistence,
		registrySyncConfig,
	)
	customSMRepository := aasenvironment.NewCustomSubmodelRepositoryService(
		submodelrepositoryapi.NewSubmodelRepositoryAPIAPIService(*submodelRepositoryPersistence),
		persistence,
		registrySyncConfig,
	)
	customCDRepository := aasenvironment.NewCustomConceptDescriptionRepositoryService(
		cdrapi.NewConceptDescriptionRepositoryAPIAPIService(cdrPersistence),
		persistence,
	)
	customDiscovery := aasenvironment.NewCustomDiscoveryService(
		discoveryapi.NewAssetAdministrationShellBasicDiscoveryAPIAPIService(*discoveryPersistence),
		persistence,
	)
	serializationService := aasenvironment.NewSerializationAPIService(persistence)
	sharedBulkManager := asyncbulk.NewManager("AASENV-BULK", 0)
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
		common.AddVerificationEndpoint(apiRouter, cfg)
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

	r.Mount(base, apiRouter)

	// Register /upload endpoint
	uploadService := aasenvironment.NewUploadAPIService(persistence, customAASRepository, customSMRepository)
	versioningGuard.Cover(http.MethodPost, "/upload")
	aasenvironment.RegisterUploadAPI(apiRouter, uploadService, cfg.General.UploadMaxSizeBytes)
	aasenvironment.RegisterSerializationAPI(apiRouter, serializationService)

	addr := common.ServerAddress(cfg.Server)
	log.Printf("AAS Environment Service listening on %s (contextPath=%q)\n", addr, cfg.Server.ContextPath)
	runner, err := common.StartHTTPServer(ctx, "AASENV", cfg.Server, r)
	if err != nil {
		return err
	}

	preconfigurationCtx := aasenvironment.ContextWithAASPreconfigurationAudit(common.ContextWithConfig(ctx, cfg))
	preconfigurationSummary := aasenvironment.RunAASPreconfiguration(preconfigurationCtx, uploadService, cfg.General.AASPreconfigPaths)
	preconfigurationCompleted.Store(true)
	//nolint:gosec // summary fields are internal integer counters and cannot carry log-control characters.
	log.Printf(
		"AASENV-SRV-PRECONFIGDONE configured=%d resolved=%d imported=%d failed=%d skipped=%d",
		preconfigurationSummary.ConfiguredSourceCount,
		preconfigurationSummary.ResolvedFileCount,
		preconfigurationSummary.ImportedFileCount,
		preconfigurationSummary.FailedFileCount,
		preconfigurationSummary.SkippedFileCount,
	)

	return runner.Wait(ctx)
}

func openSharedDatabase(ctx context.Context, cfg *common.Config, dsn string) (*sql.DB, error) {
	db, err := common.NewDatabaseConnection(dsn)
	if err != nil {
		return nil, err
	}
	configurePostgresPool(db, cfg.Postgres)
	if err = history.ApplyPostgresGuardConfig(ctx, db); err != nil {
		return nil, err
	}
	return db, nil
}

func configurePostgresPool(db *sql.DB, cfg common.PostgresConfig) {
	if cfg.MaxOpenConnections > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConnections)
	}
	if cfg.MaxIdleConnections > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConnections)
	}
	if cfg.ConnMaxLifetimeMinutes > 0 {
		db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeMinutes) * time.Minute)
	}
}

func main() {
	ctx, stop := common.SignalContext()
	configPath := ""

	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		stop()
		log.Fatalf("Server error: %v", err)
	}
	stop()
}
