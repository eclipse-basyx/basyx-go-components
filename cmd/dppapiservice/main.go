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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

// Package main starts the Digital Product Passport API HTTP server.
package main

import (
	"context"
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	dppapi "github.com/eclipse-basyx/basyx-go-components/pkg/dppapiservice/go"

	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string) error {
	cfg, err := common.LoadConfig(configPath, common.NORMAL)
	if err != nil {
		return err
	}

	configureDPPHistory()
	if err = history.ConfigureEvidence(ctx, common.HistoryEvidenceConfig{}); err != nil {
		return err
	}

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Server.Port)

	dsn := common.BuildPostgresDSN(cfg.Postgres)
	sharedDB, err := openSharedDatabase(ctx, cfg, dsn)
	if err != nil {
		return err
	}

	aasRepositoryPersistence, err := aasrepositorydb.NewAssetAdministrationShellDatabaseFromDB(sharedDB, cfg.Server.StrictVerification)
	if err != nil {
		return err
	}

	submodelRepositoryPersistence, err := submodelrepositorydb.NewSubmodelDatabaseFromDB(sharedDB, nil, cfg.Server.StrictVerification)
	if err != nil {
		return err
	}

	router := createRouter(cfg, aasRepositoryPersistence, submodelRepositoryPersistence)

	log.Printf("Server started on %s", addr)
	go func() {
		//nolint:gosec // The configured BaSyx services listen on all interfaces.
		if err := http.ListenAndServe(addr, router); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down server...")
	return nil
}

func configureDPPHistory() {
	history.Configure(history.Config{
		Mode:                 history.ModeAudit,
		RetentionDays:        0,
		FullSnapshotInterval: 1,
		Immutability:         history.ImmutabilityNone,
		AuditIdentityMode:    history.AuditIdentityExtended,
	})
}

func createRouter(cfg *common.Config, aasRepo *aasrepositorydb.AssetAdministrationShellDatabase, submodelRepo *submodelrepositorydb.SubmodelDatabase) http.Handler {
	dppService := dppapi.NewDPPRepositoryService(aasRepo, submodelRepo)
	apiRouter := dppapi.NewRouter(dppapi.NewDPPRepositoryRouter(dppService))
	apiRouterWithConfig := withConfigMiddleware(apiRouter, cfg)
	contextPath := common.NormalizeBasePath(cfg.Server.ContextPath)

	rootRouter := chi.NewRouter()
	common.AddCors(rootRouter, cfg)
	common.AddHealthEndpoint(rootRouter, cfg)
	if err := common.AddSwaggerUIFromFS(rootRouter, openapiSpec, "openapi.yaml", "Digital Product Passport API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		log.Printf("Warning: failed to load OpenAPI spec for Swagger UI: %v", err)
	}
	apiRouterWithConfig.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, contextPath+"/swagger", http.StatusFound)
	})
	rootRouter.Mount(contextPath, apiRouterWithConfig)
	return rootRouter
}

func withConfigMiddleware(apiRouter chi.Router, cfg *common.Config) chi.Router {
	router := chi.NewRouter()
	router.Use(common.ConfigMiddleware(cfg))
	router.Mount("/", apiRouter)
	return router
}

func openSharedDatabase(ctx context.Context, cfg *common.Config, dsn string) (*sql.DB, error) {
	if err := common.ValidateSchemaVersionByDSN(dsn, common.CURRENT_DATABASE_VERSION); err != nil {
		return nil, err
	}
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
	ctx, stop := signal.NotifyContext(context.TODO(), os.Interrupt, syscall.SIGTERM)

	configPath := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		stop()
		log.Fatalf("Server error: %v", err)
	}
	stop()
}
