package main

import (
    "context"
    "flag"
    "fmt"
    "log"
    "net/http"
    "os"
    "strconv"

    ass_registry_api "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/api"
    persistence_postgresql "github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/persistence"
    "github.com/eclipse-basyx/basyx-go-components/internal/common"
    apis "github.com/eclipse-basyx/basyx-go-components/pkg/aasregistryapi"
    "github.com/go-chi/chi/v5"
)

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
    log.Default().Println("Loading AAS Registry Service...")
    log.Default().Println("Config Path:", configPath)
    // Load configuration
    config, err := common.LoadConfig(configPath)
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
        return err
    }

    // Create Chi router
    r := chi.NewRouter()

    // Enable CORS and health endpoint via common helpers
    common.AddCors(r, config)
    common.AddHealthEndpoint(r, config)

    // Instantiate generated services & controllers
    // ==== AAS Registry Service ====
    smDatabase, err := persistence_postgresql.NewPostgreSQLAASRegistryDatabase(
        "postgres://"+config.Postgres.User+":"+config.Postgres.Password+"@"+config.Postgres.Host+":"+strconv.Itoa(config.Postgres.Port)+"/"+config.Postgres.DBName+"?sslmode=disable",
        config.Postgres.MaxOpenConnections,
        config.Postgres.MaxIdleConnections,
        config.Postgres.ConnMaxLifetimeMinutes,
        config.Server.CacheEnabled,
        databaseSchema,
    )
    if err != nil {
        log.Fatalf("Failed to initialize database connection: %v", err)
        return err
    }
    smSvc := ass_registry_api.NewAssetAdministrationShellRegistryAPIAPIService(*smDatabase)
    smCtrl := apis.NewAssetAdministrationShellRegistryAPIAPIController(smSvc, config.Server.ContextPath)
    for _, rt := range smCtrl.Routes() {
        r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
    }

    // ==== Description Service ====
    descSvc := apis.NewDescriptionAPIAPIService()
    descCtrl := apis.NewDescriptionAPIAPIController(descSvc)
    for _, rt := range descCtrl.Routes() {
        r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
    }

    // Start the server
    addr := "0.0.0.0:" + fmt.Sprintf("%d", config.Server.Port)
    log.Printf("▶️  AAS Registry listening on %s\n", addr)
    // Start server in a goroutine
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
    // load config and optional schema path from flags
    configPath := ""
    databaseSchema := ""
    flag.StringVar(&configPath, "config", "", "Path to config file")
    flag.StringVar(&databaseSchema, "databaseSchema", "", "Path to Database Schema SQL file (overrides default)")
    flag.Parse()

    if databaseSchema != "" {
        if _, fileError := os.ReadFile(databaseSchema); fileError != nil {
            fmt.Println("The specified database schema path is invalid or the file was not found.")
            os.Exit(1)
        }
    }

    if err := runServer(ctx, configPath, databaseSchema); err != nil {
        log.Fatalf("Server error: %v", err)
    }
}
// Note: configuration loading, CORS, and health endpoint are provided by internal/common

//
