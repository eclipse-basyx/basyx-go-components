package main

import (
	"flag"
	"log"
	"os"

	basyxconfigurationservice "github.com/eclipse-basyx/basyx-go-components/internal/basyxconfigurationservice"
	"github.com/eclipse-basyx/basyx-go-components/internal/basyxconfigurationservice/steps"
)

func main() {
	configPath := ""
	databaseSchema := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.StringVar(&databaseSchema, "databaseSchema", "", "Path to Database Schema file")
	flag.Parse()

	execCtx := &steps.ExecutionContext{}
	registry := basyxconfigurationservice.NewStepRegistry()
	registry.Register(steps.NewDatabaseConnection(execCtx, configPath))
	registry.Register(steps.NewSchemaUpload(execCtx, databaseSchema))

	if err := registry.Execute(); err != nil {
		log.Printf("BASYXCFG-MAIN-EXECUTE: %v", err)
		os.Exit(1)
	}

	if execCtx.DB != nil {
		_ = execCtx.DB.Close()
	}

	log.Println("BaSyx configuration completed successfully")
}
