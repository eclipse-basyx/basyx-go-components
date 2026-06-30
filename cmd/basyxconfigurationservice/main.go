/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

// Package main implements the BaSyx configuration service binary.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/eclipse-basyx/basyx-go-components/internal/basyxconfigurationservice"
	"github.com/eclipse-basyx/basyx-go-components/internal/basyxconfigurationservice/sequences"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
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

	execCtx := &sequences.ExecutionContext{}
	schemInit := basyxconfigurationservice.NewSchemaInitializer()
	schemInit.Register(sequences.NewDatabaseConnection(execCtx, configPath))
	schemInit.Register(sequences.NewSystemTable(execCtx))
	schemInit.Register(sequences.NewSchemaUpload(execCtx, databaseSchema))
	schemInit.Register(sequences.NewSchemaPatch(execCtx, filepath.Join(patchBasePath, "1_0_1.sql"), "v1.0.1"))
	schemInit.Register(sequences.NewSchemaPatch(execCtx, filepath.Join(patchBasePath, "1_0_2.sql"), "v1.0.2"))
	schemInit.Register(sequences.NewSchemaPatch(execCtx, filepath.Join(patchBasePath, "1_1_0.sql"), "v1.1.0"))
	schemInit.Register(sequences.NewSchemaPatch(execCtx, filepath.Join(patchBasePath, "1_1_1.sql"), "v1.1.1"))
	schemInit.Register(sequences.NewSchemaPatch(execCtx, filepath.Join(patchBasePath, "1_1_2.sql"), "v1.1.2"))
	schemInit.Register(sequences.NewSchemaPatch(execCtx, filepath.Join(patchBasePath, "1_1_3.sql"), "v1.1.3"))
	schemInit.Register(sequences.NewSchemaPatch(execCtx, filepath.Join(patchBasePath, "1_1_4.sql"), "v1.1.4"))
	schemInit.Register(sequences.NewSchemaPatch(execCtx, filepath.Join(patchBasePath, "1_1_5.sql"), "v1.1.5"))
	schemInit.Register(sequences.NewSchemaPatch(execCtx, filepath.Join(patchBasePath, "1_1_7.sql"), common.CURRENT_DATABASE_VERSION))

	if err := schemInit.Execute(); err != nil {
		log.Printf("BASYXCFG-MAIN-EXECUTE: %v", err)
		os.Exit(1)
	}

	if execCtx.DB != nil {
		_ = execCtx.DB.Close()
	}

	log.Println("BaSyx configuration completed successfully")
}
