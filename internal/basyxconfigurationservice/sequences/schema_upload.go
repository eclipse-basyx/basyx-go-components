package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	alreadyInitialized, err := su.isBaseSchemaInitialized()
	if err != nil {
		return 1, err
	}
	if alreadyInitialized {
		_, _ = fmt.Printf("[Step %d] Base schema already initialized, skipping upload\n", stepIndex)
		return 0, nil
	}

	schemaToLoad, err := su.resolveSchemaPath()
	if err != nil {
		return 1, err
	}

	// #nosec G304 -- schema path is validated via resolveSchemaPath before file access.
	schemaSQL, err := os.ReadFile(schemaToLoad)
	if err != nil {
		return 1, fmt.Errorf("BASYXCFG-SCHEMA-READFILE: %w", err)
	}

	if _, err = su.ctx.DB.Exec(string(schemaSQL)); err != nil {
		return 1, fmt.Errorf("BASYXCFG-SCHEMA-EXECUTE: %w", err)
	}

	_, _ = fmt.Printf("[Step %d] Schema upload completed\n", stepIndex)
	return 0, nil
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
		candidate := filepath.Join(schemaToLoad, "3_1_full.sql")
		candidateInfo, candidateErr := os.Stat(candidate)
		if candidateErr == nil && !candidateInfo.IsDir() {
			return candidate, nil
		}
		return "", fmt.Errorf("BASYXCFG-SCHEMA-ISDIR: %s is a directory; provide a schema file path", schemaToLoad)
	}

	return schemaToLoad, nil
}

func (su *SchemaUpload) isBaseSchemaInitialized() (bool, error) {
	var tableName string
	err := su.ctx.DB.QueryRow(`SELECT COALESCE(to_regclass('public.basyxsystem')::text, '')`).Scan(&tableName)
	if err != nil {
		return false, fmt.Errorf("BASYXCFG-SCHEMA-CHECK: %w", err)
	}
	return strings.TrimSpace(tableName) != "", nil
}

// GetDescription returns the step description for console output.
func (su *SchemaUpload) GetDescription(stepIndex int) string {
	return fmt.Sprintf("[Step %d] Uploading SQL schema", stepIndex)
}
