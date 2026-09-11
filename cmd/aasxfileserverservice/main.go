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

// Package main implements the AASX File Server Service server.
package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	aasxapi "github.com/eclipse-basyx/basyx-go-components/internal/aasxfileserver/api"
	aasxpersistence "github.com/eclipse-basyx/basyx-go-components/internal/aasxfileserver/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/asyncjob"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/binarycontent"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/abacpolicy"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasxfileserverapi/go"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

const (
	minimumAASXAsyncDatabaseHeadroom = 1
	aasxAsyncDatabaseHeadroomDivisor = 5
)

type aasxAsyncProfile struct {
	manager *asyncjob.Manager
	uploads *aasxpersistence.AsyncUploadStore
}

func runServer(ctx context.Context, configPath string) error {
	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}
	if _, err = common.ConfigureLogging(cfg, "aasxfileserverservice", configPath, os.Stderr); err != nil {
		return err
	}
	telemetryRuntime, err := telemetry.Configure(ctx, "aasxfileserverservice")
	if err != nil {
		return err
	}
	defer telemetryRuntime.Shutdown(ctx)
	if err := commonmodel.SetVerificationMode(cfg.Server.StrictVerification); err != nil {
		return err
	}

	r := chi.NewRouter()
	r.Use(common.ConfigMiddleware(cfg))

	common.AddCors(r, cfg)

	common.AddHealthEndpoint(r, cfg)

	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "AASX File Server API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		slog.WarnContext(ctx, "Swagger UI unavailable", "error.code", "AASXFILES-SWAGGER-INIT", "error", err)
	}

	slog.InfoContext(ctx, "connecting to PostgreSQL")

	pools, err := common.OpenPostgresPoolsWithSchemaValidation(ctx, cfg.Postgres, "aasxfileserverservice", common.CURRENT_DATABASE_VERSION)
	if err != nil {
		slog.ErrorContext(ctx, "database connection failed", "error.code", "AASXFILES-DB-CONNECT", "error", err)
		return err
	}
	defer func() {
		if closeErr := pools.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "database pool shutdown failed", "error.code", "AASXFILES-DB-CLOSE", "error", closeErr)
		}
	}()
	sharedDB := pools.Writer
	aasxDatabase, err := aasxpersistence.NewAASXFileServerDatabaseFromPools(pools.Writer, pools.Reader)
	if err != nil {
		slog.ErrorContext(ctx, "AASX persistence initialization failed", "error.code", "AASXFILES-DB-INIT", "error", err)
		return err
	}
	slog.InfoContext(ctx, "PostgreSQL connection established")

	asyncProfile, err := newAASXAsyncProfile(ctx, sharedDB)
	if err != nil {
		return err
	}
	aasxSvc := aasxapi.NewAASXFileServerAPIAPIService(aasxDatabase, asyncProfile.serviceOptions()...)
	aasxCtrl := openapi.NewAASXFileServerAPIAPIController(
		aasxSvc,
		"",
		openapi.WithAASXFileServerUploadStager(binarycontent.NewStager(sharedDB), cfg.General.UploadMaxSizeBytes),
	)
	asyncControllers := asyncProfile.controllers(aasxSvc, sharedDB, cfg.General.UploadMaxSizeBytes)

	descSvc := aasxapi.NewDescriptionAPIAPIService(asyncProfile.enabled())
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc, "")

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "AASXFileServerService")

	abacRepo, err := abacpolicy.SetupConfiguredSecurity(
		ctx,
		cfg,
		apiRouter,
		sharedDB,
		"aasxfileserverservice",
		requireAsyncAuthentication,
	)
	if err != nil {
		return err
	}
	abacpolicy.RegisterManagementRoutesIfEnabled(cfg, apiRouter, abacRepo, "aasxfileserverservice")
	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(apiRouter, cfg, binarycontent.NewStager(sharedDB))
	}

	for _, rt := range aasxCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, controller := range asyncControllers {
		for _, rt := range controller.Routes() {
			apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
		}
	}

	for _, rt := range descCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	r.Mount(base, apiRouter)

	addr := common.ServerAddress(cfg.Server)
	slog.InfoContext(ctx, "HTTP server starting", "address", addr, "context_path", cfg.Server.ContextPath)

	return common.RunHTTPServer(ctx, "AASX", cfg.Server, r)
}

func aasxAsyncExecutionCapacity(maximumOpenConnections int) int {
	if maximumOpenConnections <= minimumAASXAsyncDatabaseHeadroom {
		return 0
	}
	databaseHeadroom := max(maximumOpenConnections/aasxAsyncDatabaseHeadroomDivisor, minimumAASXAsyncDatabaseHeadroom)
	return maximumOpenConnections - databaseHeadroom
}

func newAASXAsyncProfile(ctx context.Context, db *sql.DB) (aasxAsyncProfile, error) {
	maximumOpenConnections := db.Stats().MaxOpenConnections
	executionCapacity := aasxAsyncExecutionCapacity(maximumOpenConnections)
	if executionCapacity == 0 {
		slog.WarnContext(
			ctx,
			"SSP-002 asynchronous profile disabled because the database pool has insufficient capacity",
			"error.code", "AASXFILES-ASYNC-DISABLED",
			"max_open_connections", maximumOpenConnections,
			"required_open_connections", minimumAASXAsyncDatabaseHeadroom+1,
		)
		return aasxAsyncProfile{}, nil
	}

	manager, err := asyncjob.NewPostgresManagerWithExecutionCapacity(ctx, db, "AASXFS-UPLOAD", 15*time.Minute, executionCapacity)
	if err != nil {
		return aasxAsyncProfile{}, err
	}
	uploads, err := aasxpersistence.NewAsyncUploadStore(db)
	if err != nil {
		return aasxAsyncProfile{}, err
	}
	return aasxAsyncProfile{manager: manager, uploads: uploads}, nil
}

func (profile aasxAsyncProfile) enabled() bool {
	return profile.manager != nil && profile.uploads != nil
}

func (profile aasxAsyncProfile) serviceOptions() []aasxapi.AASXFileServerServiceOption {
	if !profile.enabled() {
		return nil
	}
	return []aasxapi.AASXFileServerServiceOption{aasxapi.WithAsyncPackageUploads(profile.manager, profile.uploads)}
}

func (profile aasxAsyncProfile) controllers(service *aasxapi.AASXFileServerAPIAPIService, db *sql.DB, maximumUploadSize int64) []openapi.Router {
	if !profile.enabled() {
		return nil
	}
	return []openapi.Router{
		openapi.NewAASXAsyncFileServerAPIAPIController(
			service,
			"",
			openapi.WithAASXAsyncFileServerUploadStager(binarycontent.NewStager(db), maximumUploadSize),
			openapi.WithAASXAsyncFileServerExecutionSlotAcquirer(profile.manager.TryAcquireExecutionSlotLease),
		),
		openapi.NewAASXAsyncFileServerStatusAPIAPIController(service, ""),
		openapi.NewAASXAsyncFileServerResultAPIAPIController(service, ""),
	}
}

func requireAsyncAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/packages-async") && !auth.IsAuthenticated(request.Context()) {
			_ = common.WriteErrorResponse(writer, errors.New("access denied"), http.StatusUnauthorized, "Middleware", "Rules", "Denied")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func main() {
	ctx, stop := common.SignalContext()
	// load config path from flag
	configPath := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		slog.ErrorContext(ctx, "server stopped", "error.code", "AASXFILES-MAIN-RUNSERVER", "error", err)
		stop()
		os.Exit(1)
	}
	stop()
}
