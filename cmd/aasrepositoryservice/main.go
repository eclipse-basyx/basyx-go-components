// Package main implements the Asset Administration Shell Repository Service server.
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

	"github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasrepositoryapi/go"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
	log.Default().Println("Loading Asset Administration Shell Repository Service...")
	log.Default().Println("Config Path:", configPath)
	// Load configuration
	config, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}

	common.PrintConfiguration(config)

	// Create Chi router
	r := chi.NewRouter()
	common.AddDefaultRouterErrorHandlers(r, "AASRepositoryService")
	common.AddCors(r, config)

	// Add health endpoint
	common.AddHealthEndpoint(r, config)

	// Add Swagger UI
	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Asset Administration Shell Repository API", "/swagger", "/api-docs/openapi.yaml", config); err != nil {
		log.Printf("Warning: failed to load OpenAPI spec for Swagger UI: %v", err)
	}

	// Instantiate generated services & controllers
	// ==== Asset Administration Shell Repository Service ====

	aasDatabase, err := persistencepostgresql.NewAssetAdministrationShellDatabase("postgres://"+config.Postgres.User+":"+config.Postgres.Password+"@"+config.Postgres.Host+":"+strconv.Itoa(config.Postgres.Port)+"/"+config.Postgres.DBName+"?sslmode=disable", config.Postgres.MaxOpenConnections, config.Postgres.MaxIdleConnections, config.Postgres.ConnMaxLifetimeMinutes, databaseSchema, config.Server.StrictVerification)
	if err != nil {
		return err
	}

	aasSvc := api.NewAssetAdministrationShellRepositoryAPIAPIService(*aasDatabase)
	aasCtrl := openapi.NewAssetAdministrationShellRepositoryAPIAPIController(aasSvc, config.Server.ContextPath, config.Server.StrictVerification)
	for _, rt := range aasCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// ==== Description Service ====
	descSvc := openapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)
	for _, rt := range descCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Start the server
	addr := "0.0.0.0:" + fmt.Sprintf("%d", config.Server.Port)
	log.Printf("▶️  Asset Administration Shell Repository listening on %s\n", addr)
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
