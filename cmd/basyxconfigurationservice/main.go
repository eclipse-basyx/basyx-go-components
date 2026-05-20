// Package main implements the BaSyx configuration service binary.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/eclipse-basyx/basyx-go-components/internal/basyxconfigurationservice"
	"github.com/eclipse-basyx/basyx-go-components/internal/basyxconfigurationservice/steps"
)

func main() {
	configPath := ""
	databaseSchema := ""
	customPatchPath := ""
	flag.StringVar(&configPath, "config", "", "Path to config file")
	flag.StringVar(&databaseSchema, "databaseSchema", "", "Path to Database Schema file")
	flag.StringVar(&customPatchPath, "customPatchPath", "", "Path to Database Schema Patch files")
	flag.Parse()

	patchBasePath := "/app/patches"
	if customPatchPath != "" {
		patchBasePath = customPatchPath
	}

	execCtx := &steps.ExecutionContext{}
	registry := basyxconfigurationservice.NewSchemaInitializer()
	registry.Register(steps.NewDatabaseConnection(execCtx, configPath))
	registry.Register(steps.NewSchemaUpload(execCtx, databaseSchema))
	registry.Register(steps.NewSchemaPatch(execCtx, filepath.Join(patchBasePath, "101.sql"), "v1.0.1"))

	if err := registry.Execute(); err != nil {
		log.Printf("BASYXCFG-MAIN-EXECUTE: %v", err)
		os.Exit(1)
	}

	if execCtx.DB != nil {
		_ = execCtx.DB.Close()
	}

	log.Println("BaSyx configuration completed successfully")
}
