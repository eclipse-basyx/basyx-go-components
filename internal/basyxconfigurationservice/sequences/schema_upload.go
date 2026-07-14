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

package sequences

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	schemaFilePath       = "/app/base.sql"
	schemaAdvisoryLockID = int64(860424611912345001)
)

// SchemaUpload uploads the SQL schema to the configured PostgreSQL database.
type SchemaUpload struct {
	ctx                *ExecutionContext
	databaseSchemaPath string
}

// NewSchemaUpload creates a schema upload step.
func NewSchemaUpload(ctx *ExecutionContext, databaseSchemaPath string) *SchemaUpload {
	return &SchemaUpload{ctx: ctx, databaseSchemaPath: databaseSchemaPath}
}

// Execute runs the schema upload.
func (su *SchemaUpload) Execute(stepIndex int) (int, error) {
	if su.ctx == nil || su.ctx.DB == nil {
		return 1, fmt.Errorf("BASYXCFG-SCHEMA-NODB: database connection is not initialized")
	}

	if _, err := su.ctx.DB.Exec("SELECT pg_advisory_lock($1)", schemaAdvisoryLockID); err != nil {
		return 1, fmt.Errorf("BASYXCFG-SCHEMA-LOCK: %w", err)
	}
	defer func() {
		_, _ = su.ctx.DB.Exec("SELECT pg_advisory_unlock($1)", schemaAdvisoryLockID)
	}()

	schemaToLoad, err := su.resolveSchemaPath()
	if err != nil {
		return 1, err
	}

	// #nosec G304 -- schema path is validated via resolveSchemaPath before file access.
	schemaSQL, err := os.ReadFile(schemaToLoad)
	if err != nil {
		return 1, fmt.Errorf("BASYXCFG-SCHEMA-READFILE: %w", err)
	}

	if err = su.executeSchemaWithRc01Compatibility(schemaToLoad, string(schemaSQL)); err != nil {
		return 1, err
	}

	_, _ = fmt.Printf("[Step %d] Schema upload completed\n", stepIndex)
	return 0, nil
}

func (su *SchemaUpload) executeSchemaWithRc01Compatibility(schemaPath string, schemaSQL string) error {
	err := su.executeSchema(schemaSQL)
	if err == nil || !isRc01UpgradeError(err) {
		return err
	}

	if err = su.applyRc01Compatibility(schemaPath); err != nil {
		return err
	}

	return su.executeSchema(schemaSQL)
}

func (su *SchemaUpload) executeSchema(schemaSQL string) error {
	if _, err := su.ctx.DB.Exec(schemaSQL); err != nil {
		return fmt.Errorf("BASYXCFG-SCHEMA-EXECUTE: %w", err)
	}
	return nil
}

func (su *SchemaUpload) resolveSchemaPath() (string, error) {
	schemaToLoad := schemaFilePath
	if su.databaseSchemaPath != "" {
		schemaToLoad = su.databaseSchemaPath
	}

	info, err := os.Stat(schemaToLoad)
	if err != nil {
		return "", fmt.Errorf("BASYXCFG-SCHEMA-STAT: %w", err)
	}
	if info.IsDir() {
		candidate := filepath.Join(schemaToLoad, "base.sql")
		candidateInfo, candidateErr := os.Stat(candidate)
		if candidateErr == nil && !candidateInfo.IsDir() {
			return candidate, nil
		}
		return "", fmt.Errorf("BASYXCFG-SCHEMA-ISDIR: %s is a directory; provide a schema file path", schemaToLoad)
	}

	return schemaToLoad, nil
}

// GetDescription returns the step description for console output.
func (su *SchemaUpload) GetDescription(stepIndex int) string {
	return fmt.Sprintf("[Step %d] Uploading SQL schema", stepIndex)
}
