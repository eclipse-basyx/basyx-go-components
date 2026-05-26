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
	"fmt"
	"log"
	"net/http"
	"time"

	registryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	registrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncbulk"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
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
	log.Default().Println("Loading Digital Twin Registry Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath, common.NORMAL)
	if err != nil {
		return err
	}
	if err := commonmodel.SetVerificationMode(cfg.Server.StrictVerification); err != nil {
		return err
	}
	commonmodel.SetSupportsSingularSupplementalSemanticId(cfg.General.SupportsSingularSupplementalSemanticId)

	// Digital Twin Registry always enables discovery integration.
	cfg.General.DiscoveryIntegration = true

	r := chi.NewRouter()

	r.Use(common.ConfigMiddleware(cfg))
	common.AddCors(r, cfg)
	common.AddHealthEndpoint(r, cfg)
	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(r, cfg)
	}

	// Add Swagger UI
	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Digital Twin Registry API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		log.Printf("Warning: failed to load OpenAPI spec for Swagger UI: %v", err)
	}

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	// === Database ===
	dsn := common.BuildPostgresDSN(cfg.Postgres)

	if err := common.ValidateSchemaVersionByDSN(dsn, common.CURRENT_DATABASE_VERSION); err != nil {
		return err
	}

	log.Printf("🗄️  Connecting to Postgres with DSN: postgres://%s:****@%s:%d/%s?sslmode=disable",
		cfg.Postgres.User, cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.DBName)

	sharedDB, err := common.NewDatabaseConnection(dsn)
	if err != nil {
		log.Printf("Shared DB connect failed: %v", err)
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

	registryDatabase, err := registrydb.NewPostgreSQLAASRegistryDatabaseFromDB(sharedDB, cfg.Server.CacheEnabled)
	if err != nil {
		log.Printf("❌ Registry DB connect failed: %v", err)
		return err
	}

	discoveryDatabase, err := discoverydb.NewPostgreSQLDiscoveryBackendFromDB(sharedDB)
	if err != nil {
		log.Printf("❌ Discovery DB connect failed: %v", err)
		return err
	}
	log.Println("✅ Postgres connection established")

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
	bulkManager := asyncbulk.NewManager("DTR-BULK", 0)
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

	if err := auth.SetupSecurityWithClaimsMiddleware(ctx, cfg, apiRouter, claimsMiddleware...); err != nil {
		return err
	}

	for _, rt := range registryCtrl.Routes() {
		if rt.Method == "GET" && rt.Pattern == "/shell-descriptors" {
			apiRouter.With(digitaltwinregistry.CreatedAfterMiddleware).Method(rt.Method, rt.Pattern, rt.HandlerFunc)
			continue
		}
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, rt := range discoveryCtrl.Routes() {
		if (rt.Method == "POST" && rt.Pattern == "/lookup/shellsByAssetLink") || (rt.Method == "GET" && rt.Pattern == "/lookup/shells") {
			apiRouter.With(digitaltwinregistry.CreatedAfterMiddleware).Method(rt.Method, rt.Pattern, rt.HandlerFunc)
			continue
		}
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, rt := range descriptionCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	bulkHandler.RegisterRoutes(apiRouter, true)

	r.Mount(base, apiRouter)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Server.Port)
	log.Printf("▶️ Digital Twin Registry listening on %s (contextPath=%q)\n", addr, cfg.Server.ContextPath)

	go func() {
		//nolint:gosec // implementing this fix would cause errors.
		if err := http.ListenAndServe(addr, r); err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down server...")
	return nil
}

func main() {
	ctx := context.Background()
	configPath := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
