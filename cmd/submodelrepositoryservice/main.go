// Package main implements the Submodel Repository Service server.
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
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	smregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/api"
	persistencepostgresql "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string) error {
	log.Default().Println("Loading Submodel Repository Service...")
	log.Default().Println("Config Path:", configPath)
	// Load configuration
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

	if err = aasenvironment.ValidateStandaloneSubmodelRepositoryRegistrySyncConfig(cfg); err != nil {
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

	// Create Chi router
	r := chi.NewRouter()

	// Make configuration available in request contexts.
	r.Use(common.ConfigMiddleware(cfg))

	common.AddCors(r, cfg)
	common.AddHealthEndpoint(r, cfg)
	if cfg.Server.VerificationEndpointAvailable {
		common.AddVerificationEndpoint(r, cfg)
	}

	// Add Swagger UI
	if err := common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "Submodel Repository API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		log.Printf("Warning: failed to load OpenAPI spec for Swagger UI: %v", err)
	}

	// Instantiate generated services & controllers
	// ==== Submodel Repository Service ====

	// Load JWS private key if configured
	var privateKey *rsa.PrivateKey
	if cfg.JWS.PrivateKeyPath != "" {
		privateKey, err = jws.LoadPrivateKey(cfg.JWS.PrivateKeyPath)
		if err != nil {
			log.Printf("Warning: failed to load JWS private key: %v - /$signed Endpoints will be unavailable", err)
		} else {
			log.Println("JWS private key loaded successfully")
		}
	}
	signingOptions, err := jws.LoadSigningOptions(cfg.JWS.CertificateChainPath)
	if err != nil {
		log.Printf("Warning: failed to load JWS certificate chain: %v - x5c header will be omitted", err)
	}

	dsn := common.BuildPostgresDSN(cfg.Postgres)

	if err := common.ValidateSchemaVersionByDSN(dsn, common.CURRENT_DATABASE_VERSION); err != nil {
		return err
	}

	sharedDB, err := common.NewDatabaseConnection(dsn)
	if err != nil {
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

	smDatabase, err := persistencepostgresql.NewSubmodelDatabaseFromDB(sharedDB, privateKey, cfg.Server.StrictVerification)
	if err != nil {
		return err
	}
	smDatabase.SetJWSCertificateChain(signingOptions.CertificateChain)
	smRegistryPersistence, err := smregistrydb.NewPostgreSQLSMBackendFromDB(sharedDB)
	if err != nil {
		return err
	}
	aasRepositoryPersistence, err := aasrepositorydb.NewAssetAdministrationShellDatabaseFromDB(sharedDB, cfg.Server.StrictVerification)
	if err != nil {
		return err
	}
	aasRegistryPersistence, err := aasregistrydb.NewPostgreSQLAASRegistryDatabaseFromDB(sharedDB, cfg.Server.CacheEnabled)

	if err != nil {
		return err
	}

	persistence := &aasenvironment.Persistence{
		DB:                 sharedDB,
		AASRegistry:        aasRegistryPersistence,
		AASRepository:      aasRepositoryPersistence,
		SubmodelRegistry:   smRegistryPersistence,
		SubmodelRepository: smDatabase,
	}
	enableReferencingAASDescriptorEmbeddingSync := registrySyncConfig.SubmodelRegistryIntegration
	smSvc := aasenvironment.NewCustomSubmodelRepositoryServiceWithAASDescriptorEmbeddingSync(
		api.NewSubmodelRepositoryAPIAPIService(*smDatabase),
		persistence,
		registrySyncConfig,
		enableReferencingAASDescriptorEmbeddingSync,
	)
	smCtrl := openapi.NewSubmodelRepositoryAPIAPIController(smSvc, "", cfg.Server.StrictVerification)

	serializationSvc := api.NewSerializationAPIAPIService()
	serializationCtrl := openapi.NewSerializationAPIAPIController(serializationSvc, "")

	// ==== Description Service ====
	descSvc := api.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)
	base := common.NormalizeBasePath(cfg.Server.ContextPath)

	// === Protected API Subrouter ===
	apiRouter := chi.NewRouter()
	common.ConfigureAPIRouter(apiRouter, "SubmodelRepositoryService")

	// Apply OIDC + ABAC once for all repository endpoints
	if err := auth.SetupSecurity(ctx, cfg, apiRouter); err != nil {
		return err
	}
	versioningGuard := history.NewMutationCoverageGuard(apiRouter)
	apiRouter.Use(versioningGuard.Middleware)
	apiRouter.Use(history.AuditContextMiddleware(cfg))

	for operation, rt := range smCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for operation, rt := range serializationCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for operation, rt := range descCtrl.Routes() {
		versioningGuard.ClassifyRoute(operation, rt.Method, rt.Pattern)
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Mount protected API under base path
	r.Mount(base, apiRouter)

	// Start the server
	addr := "0.0.0.0:" + fmt.Sprintf("%d", cfg.Server.Port)
	log.Printf("▶️  Submodel Repository listening on %s (contextPath=%q)\n", addr, cfg.Server.ContextPath)
	// Start server in a goroutine
	go func() {
		//nolint:gosec // implementing this fix would cause errors.
		if err := http.ListenAndServe(addr, r); err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	// submodelrepository.TestNewSubmodelHandler(smDatabase)

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
