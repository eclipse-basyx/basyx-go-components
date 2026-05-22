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
	"os"
	"strconv"
	"strings"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
)

// SchemaPatch applies a versioned SQL patch when the schema version is older than the patch version.
type SchemaPatch struct {
	ctx           *ExecutionContext
	patchFilePath string
	targetVersion string
}

// NewSchemaPatch creates a new versioned patch step.
func NewSchemaPatch(ctx *ExecutionContext, patchFilePath string, targetVersion string) *SchemaPatch {
	return &SchemaPatch{ctx: ctx, patchFilePath: patchFilePath, targetVersion: targetVersion}
}

// Execute runs a schema patch if required by the current schema version.
func (sp *SchemaPatch) Execute(stepIndex int) (int, error) {
	if sp.ctx == nil || sp.ctx.DB == nil {
		return 1, fmt.Errorf("BASYXCFG-PATCH-NODB: database connection is not initialized")
	}
	if strings.TrimSpace(sp.patchFilePath) == "" {
		return 1, fmt.Errorf("BASYXCFG-PATCH-NOPATH: patch file path is empty")
	}
	if strings.TrimSpace(sp.targetVersion) == "" {
		return 1, fmt.Errorf("BASYXCFG-PATCH-NOVERSION: patch target version is empty")
	}

	if _, err := sp.ctx.DB.Exec("SELECT pg_advisory_lock($1)", schemaAdvisoryLockID); err != nil {
		return 1, fmt.Errorf("BASYXCFG-PATCH-LOCK: %w", err)
	}
	defer func() {
		_, _ = sp.ctx.DB.Exec("SELECT pg_advisory_unlock($1)", schemaAdvisoryLockID)
	}()

	currentVersion, err := sp.getCurrentSchemaVersion()
	if err != nil {
		return 1, err
	}

	compareResult, err := compareSemanticVersions(currentVersion, sp.targetVersion)
	if err != nil {
		return 1, fmt.Errorf("BASYXCFG-PATCH-VERSIONCOMPARE: %w", err)
	}

	if compareResult >= 0 {
		_, _ = fmt.Printf("[Step %d] Patch %s skipped (DB version is %s)\n", stepIndex, sp.targetVersion, currentVersion)
		return 0, nil
	}

	patchSQL, err := os.ReadFile(sp.patchFilePath)
	if err != nil {
		return 1, fmt.Errorf("BASYXCFG-PATCH-READFILE: %w", err)
	}

	tx, err := sp.ctx.DB.Begin()
	if err != nil {
		return 1, fmt.Errorf("BASYXCFG-PATCH-BEGIN: %w", err)
	}

	if _, err = tx.Exec(string(patchSQL)); err != nil {
		_ = tx.Rollback()
		return 1, fmt.Errorf("BASYXCFG-PATCH-EXECUTE: %w", err)
	}

	updateSQL, args, err := goqu.Dialect("postgres").
		Update(goqu.T("basyxsystem")).
		Set(goqu.Record{"schema_version": sp.targetVersion}).
		Prepared(true).
		ToSQL()
	if err != nil {
		_ = tx.Rollback()
		return 1, fmt.Errorf("BASYXCFG-PATCH-BUILDUPDATE: %w", err)
	}
	if _, err = tx.Exec(updateSQL, args...); err != nil {
		_ = tx.Rollback()
		return 1, fmt.Errorf("BASYXCFG-PATCH-UPDATEVERSION: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 1, fmt.Errorf("BASYXCFG-PATCH-COMMIT: %w", err)
	}

	_, _ = fmt.Printf("[Step %d] Patch %s applied successfully\n", stepIndex, sp.targetVersion)
	return 0, nil
}

// GetDescription returns the step description for console output.
func (sp *SchemaPatch) GetDescription(stepIndex int) string {
	return fmt.Sprintf("[Step %d] Applying schema patch %s (%s)", stepIndex, sp.targetVersion, sp.patchFilePath)
}

func (sp *SchemaPatch) getCurrentSchemaVersion() (string, error) {
	query, _, err := goqu.Dialect("postgres").
		From(goqu.T("basyxsystem")).
		Select(goqu.C("schema_version")).
		Order(goqu.C("identifier").Asc()).
		Limit(1).
		ToSQL()
	if err != nil {
		return "", fmt.Errorf("BASYXCFG-PATCH-BUILDVERSIONQUERY: %w", err)
	}

	var version string
	err = sp.ctx.DB.QueryRow(query).Scan(&version)
	if err == nil {
		return strings.TrimSpace(version), nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		if _, seedErr := sp.ctx.DB.Exec(seedSystemTableQuery, initialSchemaVersion); seedErr != nil {
			return "", fmt.Errorf("BASYXCFG-PATCH-SEEDVERSION: %w", seedErr)
		}
		return initialSchemaVersion, nil
	}
	return "", fmt.Errorf("BASYXCFG-PATCH-READVERSION: %w", err)
}

func compareSemanticVersions(current string, target string) (int, error) {
	currentParts, err := parseSemanticVersion(current)
	if err != nil {
		return 0, fmt.Errorf("invalid current version %q: %w", current, err)
	}
	targetParts, err := parseSemanticVersion(target)
	if err != nil {
		return 0, fmt.Errorf("invalid target version %q: %w", target, err)
	}

	for idx := 0; idx < 3; idx++ {
		if currentParts[idx] < targetParts[idx] {
			return -1, nil
		}
		if currentParts[idx] > targetParts[idx] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseSemanticVersion(raw string) ([3]int, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	trimmed = strings.TrimPrefix(trimmed, "v")

	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("expected semantic version format major.minor.patch")
	}

	var parsed [3]int
	for idx, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return [3]int{}, fmt.Errorf("invalid numeric component %q", part)
		}
		if value < 0 {
			return [3]int{}, fmt.Errorf("negative version component %d", value)
		}
		parsed[idx] = value
	}

	return parsed, nil
}
