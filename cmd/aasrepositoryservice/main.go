package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/aasrepositoryapi/go"
)

func runServer(ctx context.Context, configPath string, databaseSchema string) error {
	log.Default().Println("Loading AAS Repository Service...")
	log.Default().Println("Config Path:", configPath)

	config, err := common.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
		return err
	}

	common.PrintConfiguration(config)

	r := chi.NewRouter()
	common.AddCors(r, config)
	common.AddHealthEndpoint(r, config)

	// ==== AAS Repository Service Custom ====
	// aasSvc := api.NewAASRepositoryService()
	// aasCtrl := openapi.NewAssetAdministrationShellRepositoryAPIAPIController(aasSvc)

	// ==== AAS Repository Service - currently pointing to the openAPI generated ====
	aasSvc := openapi.NewAssetAdministrationShellRepositoryAPIAPIService()
	aasCtrl := openapi.NewAssetAdministrationShellRepositoryAPIAPIController(aasSvc)
	for _, rt := range aasCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	// ==== Description Service ====
	descSvc := openapi.NewDescriptionAPIAPIService()
	descCtrl := openapi.NewDescriptionAPIAPIController(descSvc)
	for _, rt := range descCtrl.Routes() {
		r.Method(rt.Method, rt.Pattern, rt.HandlerFunc)
	}

	addr := "0.0.0.0:" + fmt.Sprintf("%d", config.Server.Port)
	log.Printf("▶️  AAS Repository listening on %s\n", addr)

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
	databaseSchema := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.StringVar(&databaseSchema, "databaseSchema", "", "Path to Database Schema")
	flag.Parse()

	if databaseSchema != "" {
		if _, err := os.ReadFile(databaseSchema); err != nil {
			fmt.Println("The specified database schema path is invalid or not found.")
			os.Exit(1)
		}
	}

	if err := runServer(ctx, configPath, databaseSchema); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
