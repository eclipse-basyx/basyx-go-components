package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"github.com/eclipse-basyx/basyx-go-components/internal/auth"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	api "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
	persistence_postgresql "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/persistence"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
)

func runServer(ctx context.Context, configPath string) error {
	log.Default().Println("Loading Discovery Service...")
	log.Default().Println("Config Path:", configPath)

	cfg, err := common.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
		return err
	}
	common.PrintConfiguration(cfg)

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
	r.Get(cfg.Server.ContextPath+"/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	})

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

	smDatabase, err := persistence_postgresql.NewPostgreSQLDiscoveryBackend(
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

	// === OIDC & ABAC Setup ===
	oidc, err := auth.NewOIDC(ctx, auth.OIDCSettings{
		Issuer:   cfg.OIDC.Issuer,
		Audience: cfg.OIDC.Audience,
	})
	if err != nil {
		log.Fatalf("OIDC init failed: %v", err)
	}

	base := normalizeBasePath(cfg.Server.ContextPath)

	// === Protected API Subrouter ===
	apiRouter := chi.NewRouter()

	// Apply OIDC + ABAC once for all discovery endpoints
	if cfg.ABAC.Enabled {
		// === Load Access Model ===
		var model *auth.AccessModel
		if cfg.ABAC.ModelPath != "" {
			if data, err := os.ReadFile(cfg.ABAC.ModelPath); err == nil {
				if m, err := auth.ParseAccessModel(data); err == nil {
					model = m
					log.Printf("✅ Access Rule Model loaded: %s", cfg.ABAC.ModelPath)
				} else {
					log.Printf("⚠️  Could not parse Access Rule Model: %v", err)
				}
			} else {
				log.Printf("⚠️  Could not read Access Rule Model: %v", err)
			}
		}

		abacSettings := auth.ABACSettings{
			Enabled:             cfg.ABAC.Enabled,
			ClientRolesAudience: cfg.ABAC.ClientRolesAudience,
			Model:               model,
		}

		apiRouter.Use(
			oidc.Middleware,
			auth.ABACMiddleware(abacSettings, nil), // resolver removed
		)
	}

	// Register all discovery routes (protected)
	for _, rt := range smCtrl.Routes() {
		apiRouter.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// Register all description routes (public)
	for _, rt := range descCtrl.Routes() {
		r.Method(rt.Method, join(base, rt.Pattern), rt.HandlerFunc)
	}

	// Health (public, duplicate for base path)
	r.Get(join(base, "/health"), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	})

	// Mount protected API under base path
	r.Mount(base, apiRouter)

	// === Start Server ===
	addr := fmt.Sprintf("0.0.0.0:%d", cfg.Server.Port)
	log.Printf("▶️  Discovery Service listening on %s (contextPath=%q)\n", addr, cfg.Server.ContextPath)

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

func normalizeBasePath(p string) string {
	if p == "" || p == "/" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

func join(base, suffix string) string {
	if base == "/" {
		return suffix
	}
	return base + suffix
}
