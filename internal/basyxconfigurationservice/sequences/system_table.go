package steps

import "fmt"

const initialDatabaseVersion = "v1.0.0"

const createSystemTableQuery = `
		CREATE TABLE IF NOT EXISTS basyxsystem (
			identifier BIGSERIAL PRIMARY KEY,
			database_version VARCHAR NOT NULL DEFAULT 'v1.0.0'
		)
	`

const seedSystemTableQuery = `
		INSERT INTO basyxsystem (database_version)
		SELECT $1
		WHERE NOT EXISTS (SELECT 1 FROM basyxsystem)
	`

// SystemTable ensures the schema-version table exists before schema upload and patches run.
type SystemTable struct {
	ctx *ExecutionContext
}

// NewSystemTable creates a system-table initialization step.
func NewSystemTable(ctx *ExecutionContext) *SystemTable {
	return &SystemTable{ctx: ctx}
}

// Execute creates and seeds basyxsystem when required.
func (st *SystemTable) Execute(stepIndex int) (int, error) {
	if st.ctx == nil || st.ctx.DB == nil {
		return 1, fmt.Errorf("BASYXCFG-SYSTEM-NODB: database connection is not initialized")
	}

	if _, err := st.ctx.DB.Exec("SELECT pg_advisory_lock($1)", schemaAdvisoryLockID); err != nil {
		return 1, fmt.Errorf("BASYXCFG-SYSTEM-LOCK: %w", err)
	}
	defer func() {
		_, _ = st.ctx.DB.Exec("SELECT pg_advisory_unlock($1)", schemaAdvisoryLockID)
	}()

	if _, err := st.ctx.DB.Exec(createSystemTableQuery); err != nil {
		return 1, fmt.Errorf("BASYXCFG-SYSTEM-CREATETABLE: %w", err)
	}

	if _, err := st.ctx.DB.Exec(seedSystemTableQuery, initialDatabaseVersion); err != nil {
		return 1, fmt.Errorf("BASYXCFG-SYSTEM-SEEDVERSION: %w", err)
	}

	_, _ = fmt.Printf("[Step %d] System table initialized\n", stepIndex)
	return 0, nil
}

// GetDescription returns the step description for console output.
func (st *SystemTable) GetDescription(stepIndex int) string {
	return fmt.Sprintf("[Step %d] Initializing system table", stepIndex)
}
