// Package main implements the Registry of Registries Service server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/registryofregistriesservice/api"
	registryofregistriespostgresql "github.com/eclipse-basyx/basyx-go-components/internal/registryofregistriesservice/persistence"
	registryofregistriesapi "github.com/eclipse-basyx/basyx-go-components/pkg/registryofregistryapi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
	log.Default().Println("Loading Registry of Registries Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}

	// === Main Router ===
	r := chi.NewRouter()

	// --- CORS ---
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions, http.MethodPut, http.MethodPatch},
		AllowedHeaders:   []string{"*"}, // includes Authorization
		AllowCredentials: true,
	})
	r.Use(c.Handler)

	// --- Health Endpoint (public) ---
	common.AddHealthEndpoint(r, cfg)

	log.Printf("🗄️  Connecting to Postgres with DSN: postgres://%s:****@%s:%d/%s?sslmode=disable",
		cfg.Postgres.User, cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.DBName)

	rorDatabase, err := registryofregistriespostgresql.NewPostgreSQLRegistryOfRegistriesBackend("postgres://"+cfg.Postgres.User+":"+cfg.Postgres.Password+"@"+cfg.Postgres.Host+":"+strconv.Itoa(cfg.Postgres.Port)+"/"+cfg.Postgres.DBName+"?sslmode=disable", cfg.Postgres.MaxOpenConnections, cfg.Postgres.MaxIdleConnections, cfg.Postgres.ConnMaxLifetimeMinutes, cfg.Server.CacheEnabled, databaseSchema)
	if err != nil {
		log.Printf("❌ DB connect failed: %v", err)
		return err
	}
	log.Println("✅ Postgres connection established")

	rorSvc := api.NewRegistryOfRegistriesAPIAPIService(*rorDatabase)
	rorCtrl := registryofregistriesapi.NewRegistryOfRegistriesAPIAPIController(rorSvc)

	// === Description Service (public) ===
	descSvc := registryofregistriesapi.NewDescriptionAPIAPIService()
	descCtrl := registryofregistriesapi.NewDescriptionAPIAPIController(descSvc)

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	// === Protected API Subrouter ===
	apiRouter := chi.NewRouter()

	// Register all registry of registries routes
	for _, rt := range rorCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Register all description routes
	for _, rt := range descCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Mount protected API under base path
	r.Mount(base, apiRouter)

	// === Start Server ===
	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Server.Port)
	log.Printf("▶️ Registry of Registries listening on %s (contextPath=%q)\n", addr, cfg.Server.ContextPath)

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
