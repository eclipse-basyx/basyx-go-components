// Package main starts the AAS Repository Service.
// It loads configuration, sets up routes, and runs the HTTP server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	aasrepositoryapi "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasrepositoryapi/go"
	"github.com/go-chi/chi/v5"
)

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
	// _ = databaseSchema // intentionally unused for now
	log.Default().Println("Loading AAS Repository Service...")
	log.Default().Println("Config Path:", configPath)

	config, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}

	common.PrintConfiguration(config)

	r := chi.NewRouter()
	common.AddCors(r, config)
	common.AddHealthEndpoint(r, config)

	// ========== database ==========
	// === Database ===
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		config.Postgres.User,
		config.Postgres.Password,
		config.Postgres.Host,
		config.Postgres.Port,
		config.Postgres.DBName,
	)

	log.Printf("🗄️  Connecting to Postgres with DSN: postgres://%s:****@%s:%d/%s?sslmode=disable",
		config.Postgres.User, config.Postgres.Host, config.Postgres.Port, config.Postgres.DBName)

	// ==== AAS Repository Service Custom ====
	// aasSvc := api.NewAASRepositoryService()
	// aasCtrl := openapi.NewAssetAdministrationShellRepositoryAPIAPIController(aasSvc)
	aasDatabase, err := persistencepostgresql.NewPostgreSQLAASDatabaseBackend(
		dsn,
		config.Postgres.MaxOpenConnections,
		config.Postgres.MaxIdleConnections,
		config.Postgres.ConnMaxLifetimeMinutes,
		databaseSchema,
	)
	if err != nil {
		log.Fatalf("Failed to initialize database connection: %v", err)
		return err
	}

	// ==== AAS Repository Service ====
	aasSvc := aasrepositoryapi.NewAssetAdministrationShellRepositoryAPIAPIService(*aasDatabase)
	aasCtrl := openapi.NewAssetAdministrationShellRepositoryAPIAPIController(aasSvc)
	for _, rt := range aasCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// ==== Description Service ====
	descSvc := openapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)
	for _, rt := range descCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	addr := "0.0.0.0:" + fmt.Sprintf("%d", config.Server.Port)
	log.Printf("▶️  AAS Repository listening on %s\n", addr)

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
	configPath := ""
	databaseSchema := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.StringVar(&databaseSchema, "databaseSchema", "", "Path to Database Schema")
	flag.Parse()

	if databaseSchema != "" {
		if _, err := os.ReadFile(databaseSchema); err != nil {
			_, _ = fmt.Println("The specified database schema path is invalid or not found.")
			os.Exit(1)
		}
	}

	if err := runServer(ctx, configPath, databaseSchema); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
