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
	"os"

	registryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	registrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
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

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
	log.Default().Println("Loading Digital Twin Registry Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}

	// Digital Twin Registry always enables discovery integration.
	cfg.General.DiscoveryIntegration = true

	r := chi.NewRouter()

	r.Use(common.ConfigMiddleware(cfg))
	common.AddCors(r, cfg)
	common.AddHealthEndpoint(r, cfg)

	// Add Swagger UI
	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Digital Twin Registry API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		log.Printf("Warning: failed to load OpenAPI spec for Swagger UI: %v", err)
	}

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	// === Database ===
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Postgres.User,
		cfg.Postgres.Password,
		cfg.Postgres.Host,
		cfg.Postgres.Port,
		cfg.Postgres.DBName,
	)
	log.Printf("🗄️  Connecting to Postgres with DSN: postgres://%s:****@%s:%d/%s?sslmode=disable",
		cfg.Postgres.User, cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.DBName)

	registryDatabase, err := registrydb.NewPostgreSQLAASRegistryDatabase(
		dsn,
		//nolint:gosec // configured value is bounded by deployment configuration
		int32(cfg.Postgres.MaxOpenConnections),
		cfg.Postgres.MaxIdleConnections,
		cfg.Postgres.ConnMaxLifetimeMinutes,
		cfg.Server.CacheEnabled,
		databaseSchema,
	)
	if err != nil {
		log.Printf("❌ Registry DB connect failed: %v", err)
		return err
	}

	discoveryDatabase, err := discoverydb.NewPostgreSQLDiscoveryBackend(
		dsn,
		//nolint:gosec // configured value is bounded by deployment configuration
		int32(cfg.Postgres.MaxOpenConnections),
		cfg.Postgres.MaxIdleConnections,
		cfg.Postgres.ConnMaxLifetimeMinutes,
		databaseSchema,
	)
	if err != nil {
		log.Printf("❌ Discovery DB connect failed: %v", err)
		return err
	}
	log.Println("✅ Postgres connection established")

	registrySvc := digitaltwinregistry.NewCustomRegistryService(
		registryapiinternal.NewAssetAdministrationShellRegistryAPIAPIService(*registryDatabase),
	)
	discoverySvc := digitaltwinregistry.NewCustomDiscoveryService(
		discoveryapiinternal.NewAssetAdministrationShellBasicDiscoveryAPIAPIService(*discoveryDatabase),
	)

	registryCtrl := registryapi.NewAssetAdministrationShellRegistryAPIAPIController(registrySvc, cfg.Server.ContextPath)
	discoveryCtrl := openapi.NewAssetAdministrationShellBasicDiscoveryAPIAPIController(discoverySvc)
	descriptionSvc := digitaltwinregistry.NewDescriptionService()
	descriptionCtrl := openapi.NewDescriptionAPIAPIController(descriptionSvc)

	apiRouter := chi.NewRouter()
	if err := auth.SetupSecurityWithClaimsMiddleware(ctx, cfg, apiRouter, auth.EdcBpnHeaderMiddleware); err != nil {
		return err
	}

	for _, rt := range registryCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, rt := range discoveryCtrl.Routes() {
		if rt.Method == "POST" && rt.Pattern == "/lookup/shellsByAssetLink" {
			apiRouter.With(digitaltwinregistry.CreatedAfterMiddleware).Method(rt.Method, rt.Pattern, rt.HandlerFunc)
			continue
		}
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, rt := range descriptionCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

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
	databaseSchema := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.StringVar(&databaseSchema, "databaseSchema", "", "Path to Database Schema SQL file (overrides default)")
	flag.Parse()

	if databaseSchema != "" {
		if _, fileError := os.ReadFile(databaseSchema); fileError != nil {
			_, _ = fmt.Println("The specified database schema path is invalid or the file was not found.")
			os.Exit(1)
		}
	}

	if err := runServer(ctx, configPath, databaseSchema); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
