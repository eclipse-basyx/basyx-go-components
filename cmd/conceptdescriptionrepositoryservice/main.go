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
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/api"
	"github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/conceptdescriptionrepositoryapi/go"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
	log.Default().Println("Loading Concept Description Repository Service...")
	log.Default().Println("Config Path:", configPath)
	// Load configuration
	config, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}

	// Create Chi router
	r := chi.NewRouter()

	// Enable CORS
	common.AddCors(r, config)

	// Add health endpoint
	common.AddHealthEndpoint(r, config)

	// Add Swagger UI
	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Concept Description Repository API", "/swagger", "/api-docs/openapi.yaml", config); err != nil {
		log.Printf("Warning: failed to load OpenAPI spec for Swagger UI: %v", err)
	}

	// Instantiate generated services & controllers
	// ==== Concept Description Repository Service ====

	cdDatabase, err := persistence.NewConceptDescriptionBackend("postgres://"+config.Postgres.User+":"+config.Postgres.Password+"@"+config.Postgres.Host+":"+strconv.Itoa(config.Postgres.Port)+"/"+config.Postgres.DBName+"?sslmode=disable", config.Postgres.MaxOpenConnections, config.Postgres.MaxIdleConnections, config.Postgres.ConnMaxLifetimeMinutes, databaseSchema)
	if err != nil {
		return err
	}

	cdSvc := api.NewConceptDescriptionRepositoryAPIAPIService(cdDatabase)
	cdCtrl := openapi.NewConceptDescriptionRepositoryAPIAPIController(cdSvc, config.Server.ContextPath, config.Server.StrictVerification)
	for _, rt := range cdCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// ==== Description Service ====
	// descSvc := openapi.NewDescriptionAPIAPIService()
	// descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)
	// for _, rt := range descCtrl.Routes() {
	// 	r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	// }

	// Start the server
	addr := "0.0.0.0:" + fmt.Sprintf("%d", config.Server.Port)
	log.Printf("▶️  Concept Description Repository listening on %s\n", addr)
	// Start server in a goroutine
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
	// load config path from flag
	configPath := ""
	databaseSchema := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.StringVar(&databaseSchema, "databaseSchema", "", "Path to Database Schema")
	flag.Parse()

	if databaseSchema != "" {
		_, fileError := os.ReadFile(databaseSchema)
		if fileError != nil {
			_, _ = fmt.Println("The specified database schema path is invalid or the file was not found.")
			os.Exit(1)
		}
	}

	if err := runServer(ctx, configPath, databaseSchema); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
