package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	api "github.com/eclipse-basyx/basyx-go-sdk/internal/discovery/api"
	controller "github.com/eclipse-basyx/basyx-go-sdk/internal/discovery/api/controller"
	"github.com/eclipse-basyx/basyx-go-sdk/internal/discovery/config"
)

func runServer(ctx context.Context, configPath string) error {
	var cfg *config.Config

	cfg, r := config.ConfigureServer(configPath)

	c := api.ConfigureCors(cfg)
	r.Use(c.Handler)

	router, contextPath := ConfigureRouterWithContextPath(r, cfg.Server.ContextPath)

	// Register the endpoints for the discovery service
	// This includes the health check and OpenAPI spec as well as the Discovery API endpoints and Description API endpoints
	closeFunc := controller.RegisterEndpoints(r, contextPath, cfg)
	defer func() {
		if closeFunc != nil {
			closeFunc()
		}
	}()

	// Start the server with graceful shutdown
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)

	baseURL := fmt.Sprintf("http://%s:%s%s", cfg.Server.Host, cfg.Server.Port, contextPath)
	log.Printf("Discovery Service listening on %s", baseURL)
	log.Printf("Swagger UI available at %s/swagger-ui/index.html", baseURL)
	log.Printf("OpenAPI spec available at %s/docs/openapi.yaml", baseURL)

	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	// Start server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Create shutdown context
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	return server.Shutdown(shutdownCtx)
}

func ConfigureRouterWithContextPath(r *chi.Mux, contextPath string) (http.Handler, string) {
	var router http.Handler = r
	if contextPath != "" {
		contextRouter := chi.NewRouter()
		contextRouter.Mount(contextPath, r)
		router = contextRouter
	}
	return router, contextPath
}

func main() {
	ctx := context.Background()
	if err := runServer(ctx, ""); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
