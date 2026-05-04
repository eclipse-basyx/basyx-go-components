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
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasrepositoryapi/go"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
	log.Default().Println("Loading Asset Administration Shell Repository Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}

	r := chi.NewRouter()
	r.Use(common.ConfigMiddleware(cfg))

	common.AddCors(r, cfg)

	common.AddHealthEndpoint(r, cfg)

	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Asset Administration Shell Repository API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
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

	sharedDB, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		log.Printf("❌ DB connect failed: %v", err)
		return err
	}
	if cfg.Postgres.MaxOpenConnections > 0 {
		sharedDB.SetMaxOpenConns(cfg.Postgres.MaxOpenConnections)
	}
	if cfg.Postgres.MaxIdleConnections > 0 {
		sharedDB.SetMaxIdleConns(cfg.Postgres.MaxIdleConnections)
	}
	if cfg.Postgres.ConnMaxLifetimeMinutes > 0 {
		sharedDB.SetConnMaxLifetime(time.Duration(cfg.Postgres.ConnMaxLifetimeMinutes) * time.Minute)
	}

	aasDatabase, err := persistencepostgresql.NewAssetAdministrationShellDatabaseFromDB(sharedDB, cfg.Server.StrictVerification)
	if err != nil {
		log.Printf("❌ AAS DB init failed: %v", err)
		return err
	}

	submodelDatabase, err := submodelrepositorydb.NewSubmodelDatabaseFromDB(sharedDB, nil, cfg.Server.StrictVerification)
	if err != nil {
		log.Printf("❌ Submodel DB connect failed: %v", err)
		return err
	}
	log.Println("✅ Postgres connection established")

	aasSvc := api.NewAssetAdministrationShellRepositoryAPIAPIService(aasDatabase, submodelDatabase)
	aasCtrl := openapi.NewAssetAdministrationShellRepositoryAPIAPIController(aasSvc, "", cfg.Server.StrictVerification)

	descSvc := openapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "AASRepositoryService")

	if err := auth.SetupSecurity(ctx, cfg, apiRouter); err != nil {
		return err
	}

	for _, rt := range aasCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	for _, rt := range descCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	r.Mount(base, apiRouter)

	addr := "0.0.0.0:" + fmt.Sprintf("%d", cfg.Server.Port)
	log.Printf("▶️  Asset Administration Shell Repository listening on %s (contextPath=%q)\n", addr, cfg.Server.ContextPath)

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
	flag.StringVar(&databaseSchema, "databaseSchema", "", "Path to Database Schema SQL file (overrides default)")
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
