// Command aasregistryservice starts the AAS Registry HTTP service.
//
// It loads configuration, initializes the PostgreSQL-backed Asset Administration
// Shell (AAS) registry persistence, registers the API routes, and serves the
// HTTP endpoints. CORS and a health endpoint are enabled via common helpers.
//
// Flags:
//
//	-config          Path to service configuration file
//	-databaseSchema  Optional path to a SQL schema file to initialize the
//	                 database (overrides the default bundled schema)
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	aasregistryapi "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	aasregistrydatabase "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	apis "github.com/eclipse-basyx/basyx-go-components/pkg/aasregistryapi"
	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
	log.Default().Println("Loading AAS Registry Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}

	r := chi.NewRouter()

	// Make configuration available in request contexts.
	r.Use(common.ConfigMiddleware(cfg))

	common.AddCors(r, cfg)
	common.AddHealthEndpoint(r, cfg)

	// Add Swagger UI
	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "AAS Registry Service API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		log.Printf("Warning: failed to load OpenAPI spec for Swagger UI: %v", err)
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Postgres.User,
		cfg.Postgres.Password,
		cfg.Postgres.Host,
		cfg.Postgres.Port,
		cfg.Postgres.DBName,
	)
	log.Printf("🗄️  Connecting to Postgres with DSN: postgres://%s:****@%s:%d/%s?sslmode=disable",
		cfg.Postgres.User, cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.DBName)

	smDatabase, err := aasregistrydatabase.NewPostgreSQLAASRegistryDatabase(
		dsn,
		cfg.Postgres.MaxOpenConnections,
		cfg.Postgres.MaxIdleConnections,
		cfg.Postgres.ConnMaxLifetimeMinutes,
		cfg.Server.CacheEnabled,
		databaseSchema,
	)

	if err != nil {
		log.Printf("❌ DB connect failed: %v", err)
		return err
	}
	log.Println("✅ Postgres connection established")

	smSvc := aasregistryapi.NewAssetAdministrationShellRegistryAPIAPIService(*smDatabase)
	smCtrl := apis.NewAssetAdministrationShellRegistryAPIAPIController(smSvc, cfg.Server.ContextPath)

	descSvc := aasregistryapi.NewDescriptionAPIAPIService()
	descCtrl := apis.NewDescriptionAPIAPIController(descSvc)

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	// === Protected API Subrouter ===
	apiRouter := chi.NewRouter()

	// Apply OIDC + ABAC once for all registry endpoints
	if err := auth.SetupSecurity(ctx, cfg, apiRouter); err != nil {
		return err
	}

	// Register all registry routes (protected)
	for _, rt := range smCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Register all description routes (protected)
	for _, rt := range descCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Mount protected API under base path
	r.Mount(base, apiRouter)

	addr := "0.0.0.0:" + fmt.Sprintf("%d", cfg.Server.Port)
	log.Printf("▶️ AAS Registry listening on %s (contextPath=%q)\n", addr, cfg.Server.ContextPath)

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
