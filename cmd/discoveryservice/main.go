// Package main implements the Discovery Service server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	api "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

func runServer(ctx context.Context, configPath string) error {
	log.Default().Println("Loading Discovery Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
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

	smDatabase, err := persistencepostgresql.NewPostgreSQLDiscoveryBackend(
		dsn,
		cfg.Postgres.MaxOpenConnections,
	)
	if err != nil {
		log.Printf("❌ DB connect failed: %v", err)
		return err
	}
	log.Println("✅ Postgres connection established")

	smSvc := api.NewAssetAdministrationShellBasicDiscoveryAPIAPIService(*smDatabase)
	smCtrl := openapi.NewAssetAdministrationShellBasicDiscoveryAPIAPIController(smSvc)

	// === Description Service (public) ===
	descSvc := openapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	// === Protected API Subrouter ===
	apiRouter := chi.NewRouter()

	// Apply OIDC + ABAC once for all discovery endpoints
	if err := auth.SetupSecurity(ctx, cfg, apiRouter); err != nil {
		return err
	}

	// Register all discovery routes (protected)
	for _, rt := range smCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Register all description routes (protected)
	for _, rt := range descCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Mount protected API under base path
	r.Mount(base, apiRouter)

	// === Start Server ===
	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Server.Port)
	log.Printf("▶️ AAS Discovery listening on %s (contextPath=%q)\n", addr, cfg.Server.ContextPath)

	go func() {
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
