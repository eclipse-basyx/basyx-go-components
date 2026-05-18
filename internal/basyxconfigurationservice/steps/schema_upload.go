package steps

import (
	"fmt"
	"os"
)

const (
	schemaFilePath       = "/app/3_1_full.sql"
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

	schemaToLoad := schemaFilePath

	if su.databaseSchemaPath != "" {
		schemaToLoad = su.databaseSchemaPath
	}

	schemaSQL, err := os.ReadFile(schemaToLoad)
	if err != nil {
		return 1, fmt.Errorf("BASYXCFG-SCHEMA-READFILE: %w", err)
	}

	if _, err = su.ctx.DB.Exec("SELECT pg_advisory_lock($1)", schemaAdvisoryLockID); err != nil {
		return 1, fmt.Errorf("BASYXCFG-SCHEMA-LOCK: %w", err)
	}
	defer func() {
		_, _ = su.ctx.DB.Exec("SELECT pg_advisory_unlock($1)", schemaAdvisoryLockID)
	}()

	if _, err = su.ctx.DB.Exec(string(schemaSQL)); err != nil {
		return 1, fmt.Errorf("BASYXCFG-SCHEMA-EXECUTE: %w", err)
	}

	_, _ = fmt.Printf("[Step %d] Schema upload completed\n", stepIndex)
	return 0, nil
}

// GetDescription returns the step description for console output.
func (su *SchemaUpload) GetDescription(stepIndex int) string {
	return fmt.Sprintf("[Step %d] Uploading SQL schema", stepIndex)
}
