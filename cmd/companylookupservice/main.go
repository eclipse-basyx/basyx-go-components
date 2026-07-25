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

// Package main implements the Company Lookup Service server.
package main

import (
	"context"
	"embed"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/companylookupservice/api"
	companylookuppostgresql "github.com/eclipse-basyx/basyx-go-components/internal/companylookupservice/persistence"
	"github.com/eclipse-basyx/basyx-go-components/pkg/companylookupapi"
	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string) error {
	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}
	if _, err = common.ConfigureLogging(cfg, "companylookupservice", configPath, os.Stderr); err != nil {
		return err
	}
	if err := commonmodel.SetVerificationMode(cfg.Server.StrictVerification); err != nil {
		return err
	}

	// === Main Router ===
	r := chi.NewRouter()
	r.Use(common.ConfigMiddleware(cfg))

	common.AddCors(r, cfg)

	// --- Health Endpoint (public) ---
	common.AddHealthEndpoint(r, cfg)

	// Add Swagger UI
	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Company Lookup Service API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		slog.WarnContext(ctx, "Swagger UI unavailable", "error.code", "COMPANYLOOKUP-SWAGGER-INIT", "error", err)
	}

	dsn := common.BuildPostgresDSN(cfg.Postgres)

	if err := common.ValidateSchemaVersionByDSN(dsn, common.CURRENT_DATABASE_VERSION); err != nil {
		return err
	}
	slog.InfoContext(ctx, "connecting to PostgreSQL")

	sharedDB, err := common.NewDatabaseConnection(dsn)
	if err != nil {
		slog.ErrorContext(ctx, "database connection failed", "error.code", "COMPANYLOOKUP-DB-CONNECT", "error", err)
		return err
	}
	defer func() { _ = sharedDB.Close() }()
	if cfg.Postgres.MaxOpenConnections > 0 {
		sharedDB.SetMaxOpenConns(cfg.Postgres.MaxOpenConnections)
	}
	if cfg.Postgres.MaxIdleConnections > 0 {
		sharedDB.SetMaxIdleConns(cfg.Postgres.MaxIdleConnections)
	}
	if cfg.Postgres.ConnMaxLifetimeMinutes > 0 {
		sharedDB.SetConnMaxLifetime(time.Duration(cfg.Postgres.ConnMaxLifetimeMinutes) * time.Minute)
	}
	companyLookupDatabase, err := companylookuppostgresql.NewPostgreSQLCompanyLookupBackendFromDB(sharedDB, cfg.Server.CacheEnabled)
	if err != nil {
		slog.ErrorContext(ctx, "company lookup persistence initialization failed", "error.code", "COMPANYLOOKUP-DB-INIT", "error", err)
		return err
	}
	slog.InfoContext(ctx, "PostgreSQL connection established")

	companyLookupSvc := api.NewCompanyLookupAPIService(*companyLookupDatabase)
	companyLookupCtrl := companylookupapi.NewCompanyLookupAPIAPIController(companyLookupSvc)

	// === Description Service (public) ===
	descSvc := companylookupapi.NewDescriptionAPIAPIService()
	descCtrl := companylookupapi.NewDescriptionAPIAPIController(descSvc)

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	// === Protected API Subrouter ===
	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "CompanyLookupService")
	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(apiRouter, cfg, binarycontent.NewStager(sharedDB))
	}

	// Register all company lookup routes
	for _, rt := range companyLookupCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Register all description routes
	for _, rt := range descCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Mount protected API under base path
	r.Mount(base, apiRouter)

	addr := common.ServerAddress(cfg.Server)
	slog.InfoContext(ctx, "HTTP server starting", "address", addr, "context_path", cfg.Server.ContextPath)

	return common.RunHTTPServer(ctx, "COMPANYLOOKUP", cfg.Server, r)
}

func main() {
	ctx, stop := common.SignalContext()
	// load config path from flag
	configPath := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		slog.ErrorContext(ctx, "server stopped", "error.code", "COMPANYLOOKUP-MAIN-RUNSERVER", "error", err)
		stop()
		os.Exit(1)
	}
	stop()
}
