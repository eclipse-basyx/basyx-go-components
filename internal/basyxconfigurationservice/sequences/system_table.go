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
	"database/sql"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
)

const (
	initialSchemaVersion = "v1.0.0"
	schemaStateClean     = "clean"
	schemaStateDirty     = "dirty"
)

// Not supported by goqu
const createSystemTableQuery = `
		CREATE TABLE IF NOT EXISTS basyxsystem (
			identifier BIGSERIAL PRIMARY KEY,
			schema_version VARCHAR NOT NULL DEFAULT 'v1.0.0',
			state VARCHAR NOT NULL DEFAULT 'clean'
		)
	`

// Not supported by goqu
const ensureSystemTableStateColumnQuery = `
		ALTER TABLE basyxsystem
		ADD COLUMN IF NOT EXISTS state VARCHAR NOT NULL DEFAULT 'clean'
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

	if _, err := st.ctx.DB.Exec(ensureSystemTableStateColumnQuery); err != nil {
		return 1, fmt.Errorf("BASYXCFG-SYSTEM-ENSURESTATE: %w", err)
	}

	if err := seedSystemTableIfMissing(st.ctx.DB); err != nil {
		return 1, fmt.Errorf("BASYXCFG-SYSTEM-SEEDVERSION: %w", err)
	}

	_, _ = fmt.Printf("[Step %d] System table initialized\n", stepIndex)
	return 0, nil
}

// GetDescription returns the step description for console output.
func (st *SystemTable) GetDescription(stepIndex int) string {
	return fmt.Sprintf("[Step %d] Initializing system table", stepIndex)
}

func seedSystemTableIfMissing(db *sql.DB) error {
	existsQuery, _, err := goqu.Dialect("postgres").
		From(goqu.T("basyxsystem")).
		Select(goqu.C("identifier")).
		Limit(1).
		ToSQL()
	if err != nil {
		return fmt.Errorf("BASYXCFG-SYSTEM-BUILDSEEDCHECK: %w", err)
	}

	var identifier int64
	err = db.QueryRow(existsQuery).Scan(&identifier)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("BASYXCFG-SYSTEM-CHECKSEED: %w", err)
	}

	insertSQL, args, err := goqu.Dialect("postgres").
		Insert(goqu.T("basyxsystem")).
		Rows(goqu.Record{
			"schema_version": initialSchemaVersion,
			"state":          schemaStateClean,
		}).
		Prepared(true).
		ToSQL()
	if err != nil {
		return fmt.Errorf("BASYXCFG-SYSTEM-BUILDSEEDINSERT: %w", err)
	}
	if _, err = db.Exec(insertSQL, args...); err != nil {
		return fmt.Errorf("BASYXCFG-SYSTEM-INSERTSEED: %w", err)
	}
	return nil
}
