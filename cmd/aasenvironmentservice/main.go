// Package main starts the AAS Environment Service HTTP server.
package main

import (
	"context"
	"crypto/rsa"
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/aasenvironment"
	aasregistryapi "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
	aasregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
	aasrepositoryapi "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/api"
	aasrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/persistence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	commonmodel "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	cdrapi "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/api"
	cdrdb "github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/persistence"
	discoveryapi "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
	discoverydb "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/persistence"
	smregistryapi "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/api"
	smregistrydb "github.com/eclipse-basyx/basyx-go-components/internal/smregistry/persistence"
	submodelrepositoryapi "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/api"
	submodelrepositorydb "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence"
	aasregistryopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasregistryapi"
	aasrepositoryopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasrepositoryapi/go"
	cdropenapi "github.com/eclipse-basyx/basyx-go-components/pkg/conceptdescriptionrepositoryapi/go"
	discoveryopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
	smregistryopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/smregistry"
	submodelrepositoryopenapi "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var openapiSpec embed.FS

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
	log.Default().Println("Loading AAS Environment Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		return err
	}
	commonmodel.SetStrictVerificationEnabled(cfg.Server.StrictVerification)
	commonmodel.SetSupportsSingularSupplementalSemanticId(cfg.General.SupportsSingularSupplementalSemanticId)

	// AAS Environment Service always enables discovery integration.
	cfg.General.DiscoveryIntegration = true

	r := chi.NewRouter()
	r.Use(common.ConfigMiddleware(cfg))
	common.AddCors(r, cfg)
	common.AddHealthEndpoint(r, cfg)

	if err = common.AddSwaggerUIFromFS(r, openapiSpec, "openapi.yaml", "AAS Environment Service API", "/swagger", "/api-docs/openapi.yaml", cfg); err != nil {
		log.Printf("Warning: failed to load OpenAPI spec for Swagger UI: %v", err)
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Postgres.User,
		cfg.Postgres.Password,
		cfg.Postgres.Host,
		cfg.Postgres.Port,
		cfg.Postgres.DBName,
	)

	sharedDB, err := common.InitializeDatabase(dsn, databaseSchema)
	if err != nil {
		return err
	}
	if cfg.Postgres.MaxOpenConnections > 0 {
		sharedDB.SetMaxOpenConns(cfg.Postgres.MaxOpenConnections)
	}
	if cfg.Postgres.MaxIdleConnections > 0 {
		sharedDB.SetMaxIdleConns(cfg.Postgres.MaxIdleConnections)
	}

	var privateKey *rsa.PrivateKey
	if cfg.JWS.PrivateKeyPath != "" {
		privateKey, err = jws.LoadPrivateKey(cfg.JWS.PrivateKeyPath)
		if err != nil {
			return err
		}
	}

	aasRegistryPersistence, err := aasregistrydb.NewPostgreSQLAASRegistryDatabaseFromDB(sharedDB, cfg.Server.CacheEnabled)
	if err != nil {
		return err
	}
	smRegistryPersistence, err := smregistrydb.NewPostgreSQLSMBackendFromDB(sharedDB)
	if err != nil {
		return err
	}
	aasRepositoryPersistence, err := aasrepositorydb.NewAssetAdministrationShellDatabaseFromDB(sharedDB, cfg.Server.StrictVerification)
	if err != nil {
		return err
	}
	submodelRepositoryPersistence, err := submodelrepositorydb.NewSubmodelDatabaseFromDB(sharedDB, privateKey, cfg.Server.StrictVerification)
	if err != nil {
		return err
	}
	cdrPersistence, err := cdrdb.NewConceptDescriptionBackendFromDB(sharedDB)
	if err != nil {
		return err
	}
	discoveryPersistence, err := discoverydb.NewPostgreSQLDiscoveryBackendFromDB(sharedDB)
	if err != nil {
		return err
	}

	persistence := &aasenvironment.Persistence{
		DB:                           sharedDB,
		AASRegistry:                  aasRegistryPersistence,
		SubmodelRegistry:             smRegistryPersistence,
		AASRepository:                aasRepositoryPersistence,
		SubmodelRepository:           submodelRepositoryPersistence,
		ConceptDescriptionRepository: cdrPersistence,
		Discovery:                    discoveryPersistence,
	}

	customAASRegistry := aasenvironment.NewCustomAASRegistryService(
		aasregistryapi.NewAssetAdministrationShellRegistryAPIAPIService(*aasRegistryPersistence),
		persistence,
	)
	customSMRegistry := aasenvironment.NewCustomSubmodelRegistryService(
		smregistryapi.NewSubmodelRegistryAPIAPIService(*smRegistryPersistence),
		persistence,
	)
	customAASRepository := aasenvironment.NewCustomAASRepositoryService(
		aasrepositoryapi.NewAssetAdministrationShellRepositoryAPIAPIService(*aasRepositoryPersistence),
		persistence,
	)
	customSMRepository := aasenvironment.NewCustomSubmodelRepositoryService(
		submodelrepositoryapi.NewSubmodelRepositoryAPIAPIService(*submodelRepositoryPersistence),
		persistence,
	)
	customCDRepository := aasenvironment.NewCustomConceptDescriptionRepositoryService(
		cdrapi.NewConceptDescriptionRepositoryAPIAPIService(cdrPersistence),
		persistence,
	)
	customDiscovery := aasenvironment.NewCustomDiscoveryService(
		discoveryapi.NewAssetAdministrationShellBasicDiscoveryAPIAPIService(*discoveryPersistence),
		persistence,
	)

	aasRegistryCtrl := aasregistryopenapi.NewAssetAdministrationShellRegistryAPIAPIController(customAASRegistry, cfg.Server.ContextPath)
	smRegistryCtrl := smregistryopenapi.NewSubmodelRegistryAPIAPIController(customSMRegistry, cfg.Server.ContextPath)
	aasRepositoryCtrl := aasrepositoryopenapi.NewAssetAdministrationShellRepositoryAPIAPIController(customAASRepository, "", cfg.Server.StrictVerification)
	smRepositoryCtrl := submodelrepositoryopenapi.NewSubmodelRepositoryAPIAPIController(customSMRepository, "", cfg.Server.StrictVerification)
	cdrCtrl := cdropenapi.NewConceptDescriptionRepositoryAPIAPIController(customCDRepository, "", cfg.Server.StrictVerification)
	discoveryCtrl := discoveryopenapi.NewAssetAdministrationShellBasicDiscoveryAPIAPIController(customDiscovery)
	descriptionCtrl := discoveryopenapi.NewDescriptionAPIAPIController(aasenvironment.NewDescriptionService())

	base := common.NormalizeBasePath(cfg.Server.ContextPath)
	apiRouter := chi.NewRouter()
	common.AddDefaultRouterErrorHandlers(apiRouter, "AASEnvironmentService")

	if err = auth.SetupSecurity(ctx, cfg, apiRouter); err != nil {
		return err
	}

	for _, rt := range aasRegistryCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, rt := range smRegistryCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, rt := range aasRepositoryCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, rt := range smRepositoryCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, rt := range cdrCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, rt := range discoveryCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}
	for _, rt := range descriptionCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	r.Mount(base, apiRouter)

	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Server.Port)
	log.Printf("AAS Environment Service listening on %s (contextPath=%q)\n", addr, cfg.Server.ContextPath)
	server := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		if serveErr := server.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Printf("Server error: %v", serveErr)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("AASENV-SRV-SHUTDOWN %w", err)
	}
	return nil
}

func main() {
	ctx := context.TODO()
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
