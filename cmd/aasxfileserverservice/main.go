// Package main implements the AASX File Server Service server.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	aasxapi "github.com/eclipse-basyx/basyx-go-components/internal/aasxfileserver/api"
	aasxpersistence "github.com/eclipse-basyx/basyx-go-components/internal/aasxfileserver/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasxfileserverapi/go"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string) error {
	log.Default().Println("Loading AASX File Server Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath, common.NORMAL)
	if err != nil {
		return err
	}
	if err := commonmodel.SetVerificationMode(cfg.Server.StrictVerification); err != nil {
		return err
	}

	r := chi.NewRouter()
	r.Use(common.ConfigMiddleware(cfg))

	common.AddCors(r, cfg)

	common.AddHealthEndpoint(r, cfg)

	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(r, cfg)
	}

	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "AASX File Server API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
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

	sharedDB, err := common.NewDatabaseConnection(dsn)
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

	aasxDatabase, err := aasxpersistence.NewAASXFileServerDatabaseFromDB(sharedDB)
	if err != nil {
		log.Printf("❌ AASX DB init failed: %v", err)
		return err
	}
	log.Println("✅ Postgres connection established")

	aasxSvc := aasxapi.NewAASXFileServerAPIAPIService(aasxDatabase)
	aasxCtrl := openapi.NewAASXFileServerAPIAPIController(aasxSvc, "")

	descSvc := aasxapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc, "")

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "AASXFileServerService")

	if err := auth.SetupSecurity(ctx, cfg, apiRouter); err != nil {
		return err
	}

	for _, rt := range aasxCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	for _, rt := range descCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	r.Mount(base, apiRouter)

	addr := "0.0.0.0:" + fmt.Sprintf("%d", cfg.Server.Port)
	log.Printf("▶️  AASX File Server listening on %s (contextPath=%q)\n", addr, cfg.Server.ContextPath)

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
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
