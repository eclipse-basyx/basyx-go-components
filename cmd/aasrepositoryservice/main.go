// Package main implements the Asset Administration Shell Repository Service server.
package main

import (
	"context"
	"crypto/rsa"
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/abacpolicy"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasrepositoryapi/go"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string) error {
	log.Default().Println("Loading Asset Administration Shell Repository Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath, common.NORMAL)
	if err != nil {
		return err
	}

	if err := commonmodel.SetVerificationMode(cfg.Server.StrictVerification); err != nil {
		return err
	}
	history.Configure(history.Config{
		Mode:                 cfg.History.Mode,
		RetentionDays:        cfg.History.RetentionDays,
		FullSnapshotInterval: cfg.History.FullSnapshotInterval,
		Immutability:         cfg.History.Immutability,
		AuditIdentityMode:    cfg.History.AuditIdentityMode,
	})
	if err = history.ConfigureEvidence(ctx, cfg.History.Evidence); err != nil {
		return err
	}

	if err = aasenvironment.ValidateStandaloneAASRepositoryRegistrySyncConfig(cfg); err != nil {
		return err
	}
	registrySyncConfig, err := aasenvironment.NewRegistrySyncConfig(
		cfg.General.AASRegistryIntegration,
		cfg.General.SubmodelRegistryIntegration,
		cfg.General.ExternalURL,
	)
	if err != nil {
		return err
	}

	r := chi.NewRouter()
	r.Use(common.ConfigMiddleware(cfg))

	common.AddCors(r, cfg)

	common.AddHealthEndpoint(r, cfg)

	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(r, cfg)
	}

	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Asset Administration Shell Repository API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		log.Printf("Warning: failed to load OpenAPI spec for Swagger UI: %v", err)
	}

	var privateKey *rsa.PrivateKey
	if cfg.JWS.PrivateKeyPath != "" {
		privateKey, err = jws.LoadPrivateKey(cfg.JWS.PrivateKeyPath)
		if err != nil {
			log.Printf("Warning: failed to load JWS private key: %v - /$signed Endpoints will be unavailable", err)
		} else {
			log.Println("JWS private key loaded successfully")
		}
	}

	dsn := common.BuildPostgresDSN(cfg.Postgres)

	if err := common.ValidateSchemaVersionByDSN(dsn, common.CURRENT_DATABASE_VERSION); err != nil {
		return err
	}

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
	if err = history.ApplyPostgresGuardConfig(ctx, sharedDB); err != nil {
		return err
	}

	aasDatabase, err := persistencepostgresql.NewAssetAdministrationShellDatabaseFromDB(sharedDB, cfg.Server.StrictVerification)
	if err != nil {
		log.Printf("❌ AAS DB init failed: %v", err)
		return err
	}
	aasDatabase.SetJWSPrivateKey(privateKey)

	aasRegistryPersistence, err := aasregistrydb.NewPostgreSQLAASRegistryDatabaseFromDB(sharedDB, cfg.Server.CacheEnabled)
	if err != nil {
		log.Printf("AAS Registry DB init failed: %v", err)
		return err
	}

	submodelDatabase, err := submodelrepositorydb.NewSubmodelDatabaseFromDB(sharedDB, nil, cfg.Server.StrictVerification)
	if err != nil {
		log.Printf("❌ Submodel DB connect failed: %v", err)
		return err
	}
	log.Println("✅ Postgres connection established")

	persistence := &aasenvironment.Persistence{
		DB:                 sharedDB,
		AASRegistry:        aasRegistryPersistence,
		AASRepository:      aasDatabase,
		SubmodelRepository: submodelDatabase,
	}
	aasSvc := aasenvironment.NewCustomAASRepositoryService(
		api.NewAssetAdministrationShellRepositoryAPIAPIService(aasDatabase, submodelDatabase),
		persistence,
		registrySyncConfig,
	)
	aasCtrl := openapi.NewAssetAdministrationShellRepositoryAPIAPIController(aasSvc, "", cfg.Server.StrictVerification)

	descSvc := openapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)

	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "AASRepositoryService")

	abacRepo, err := abacpolicy.SetupSecurityWithABACRepository(ctx, cfg, apiRouter, sharedDB, "aasrepositoryservice")
	if err != nil {
		return err
	}
	versioningGuard := history.NewMutationCoverageGuard(apiRouter)
	apiRouter.Use(versioningGuard.Middleware)
	apiRouter.Use(history.AuditContextMiddleware(cfg))
	abacpolicy.ExemptManagementMutationRoutesIfEnabled(cfg, versioningGuard, "aasrepositoryservice")
	abacpolicy.RegisterManagementRoutesIfEnabled(cfg, apiRouter, abacRepo, "aasrepositoryservice")

	for operation, rt := range aasCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	for operation, rt := range descCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
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
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.Parse()

	if err := runServer(ctx, configPath); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
