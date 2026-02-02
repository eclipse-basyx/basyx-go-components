// Package main implements the Digital Twin Registry service (AAS Registry + Discovery).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	registryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	registrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/eclipse-basyx/basyx-go-components/internal/digitaltwinregistry"
	discoveryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
	discoverydb "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/persistence"
	registryapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasregistryapi"
	discoveryapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
	"github.com/go-chi/chi/v5"
)

const (
	discoveryProfile = "https://admin-shell.io/aas/API/3/1/DiscoveryServiceSpecification/SSP-001"
	registryProfile  = "https://admin-shell.io/aas/API/3/1/AssetAdministrationShellRegistryServiceSpecification/SSP-001"
)

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
	log.Default().Println("Loading Digital Twin Registry Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}

	r := chi.NewRouter()

	r.Use(common.ConfigMiddleware(cfg))
	common.AddCors(r, cfg)
	common.AddHealthEndpoint(r, cfg)

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
		cfg.Postgres.MaxOpenConnections,
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
		cfg.Postgres.MaxOpenConnections,
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
	discoveryCtrl := discoveryapi.NewAssetAdministrationShellBasicDiscoveryAPIAPIController(discoverySvc)

	apiRouter := chi.NewRouter()
	if err := auth.SetupSecurity(ctx, cfg, apiRouter); err != nil {
		return err
	}

	for _, rt := range registryCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, rt := range discoveryCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Single combined /description to avoid conflicts.
	apiRouter.Get("/description", func(w http.ResponseWriter, r *http.Request) {
		desc := model.ServiceDescription{
			Profiles: []string{registryProfile, discoveryProfile},
		}
		code := http.StatusOK
		_ = registryapi.EncodeJSONResponse(desc, &code, w)
	})

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
