package steps

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// SchemaPatch applies a versioned SQL patch when the database version is older than the patch version.
type SchemaPatch struct {
	ctx           *ExecutionContext
	patchFilePath string
	targetVersion string
}

// NewSchemaPatch creates a new versioned patch step.
func NewSchemaPatch(ctx *ExecutionContext, patchFilePath string, targetVersion string) *SchemaPatch {
	return &SchemaPatch{ctx: ctx, patchFilePath: patchFilePath, targetVersion: targetVersion}
}

// Execute runs a schema patch if required by the current database version.
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

	currentVersion, err := sp.getCurrentDBVersion()
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

	approved, err := sp.resolvePatchPolicy(stepIndex, currentVersion)
	if err != nil {
		return 1, err
	}
	if !approved {
		return 1, fmt.Errorf("BASYXCFG-PATCH-REJECTED: patch %s was not authorized by user", sp.targetVersion)
	}

	patchSQL, err := os.ReadFile(sp.patchFilePath)
	if err != nil {
		return 1, fmt.Errorf("BASYXCFG-PATCH-READFILE: %w", err)
	}

	if _, err = sp.ctx.DB.Exec("SELECT pg_advisory_lock($1)", schemaAdvisoryLockID); err != nil {
		return 1, fmt.Errorf("BASYXCFG-PATCH-LOCK: %w", err)
	}
	defer func() {
		_, _ = sp.ctx.DB.Exec("SELECT pg_advisory_unlock($1)", schemaAdvisoryLockID)
	}()

	tx, err := sp.ctx.DB.Begin()
	if err != nil {
		return 1, fmt.Errorf("BASYXCFG-PATCH-BEGIN: %w", err)
	}

	if _, err = tx.Exec(string(patchSQL)); err != nil {
		_ = tx.Rollback()
		return 1, fmt.Errorf("BASYXCFG-PATCH-EXECUTE: %w", err)
	}

	if _, err = tx.Exec(`
		UPDATE basyxsystem
		SET database_version = $1
		WHERE identifier = (
			SELECT identifier
			FROM basyxsystem
			ORDER BY identifier ASC
			LIMIT 1
		)
	`, sp.targetVersion); err != nil {
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

func (sp *SchemaPatch) getCurrentDBVersion() (string, error) {
	row := sp.ctx.DB.QueryRow(`
		SELECT database_version
		FROM basyxsystem
		ORDER BY identifier ASC
		LIMIT 1
	`)

	var version string
	err := row.Scan(&version)
	if err == nil {
		return strings.TrimSpace(version), nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("BASYXCFG-PATCH-NOVERSIONROW: basyxsystem does not contain a version row")
	}
	return "", fmt.Errorf("BASYXCFG-PATCH-READVERSION: %w", err)
}

func (sp *SchemaPatch) resolvePatchPolicy(stepIndex int, currentVersion string) (bool, error) {
	approvedVersions := strings.TrimSpace(os.Getenv("BASYXCFG_PATCH_APPROVED_VERSIONS"))
	approvedUpToVersion := strings.TrimSpace(os.Getenv("BASYXCFG_PATCH_APPROVED_UP_TO_VERSION"))
	backupReference := strings.TrimSpace(os.Getenv("BASYXCFG_PATCH_BACKUP_REFERENCE"))

	_, _ = fmt.Printf(
		"[Step %d] Patch policy: approvedVersions=%q approvedUpToVersion=%q backupReference=%q currentVersion=%s targetVersion=%s\n",
		stepIndex,
		approvedVersions,
		approvedUpToVersion,
		backupReference,
		currentVersion,
		sp.targetVersion,
	)

	if backupReference == "" {
		return false, fmt.Errorf(
			"BASYXCFG-PATCH-BACKUP-UNCONFIRMED: patch %s requires BASYXCFG_PATCH_BACKUP_REFERENCE to confirm a backup exists",
			sp.targetVersion,
		)
	}

	if !isTargetVersionApprovedUpTo(approvedVersions, approvedUpToVersion, sp.targetVersion) {
		return false, fmt.Errorf(
			"BASYXCFG-PATCH-NOT-APPROVED: patch %s is not approved by BASYXCFG_PATCH_APPROVED_VERSIONS/BASYXCFG_PATCH_APPROVED_UP_TO_VERSION",
			sp.targetVersion,
		)
	}

	_, _ = fmt.Printf("[Step %d] Backup confirmation accepted (reference=%s)\n", stepIndex, backupReference)
	return true, nil
}

func isTargetVersionApproved(approvedVersions string, targetVersion string) bool {
	targetNormalized := normalizeVersionToken(targetVersion)
	for _, raw := range strings.Split(approvedVersions, ",") {
		candidate := normalizeVersionToken(raw)
		if candidate != "" && candidate == targetNormalized {
			return true
		}
	}
	return false
}

func isTargetVersionApprovedUpTo(approvedVersions string, approvedUpToVersion string, targetVersion string) bool {
	if isTargetVersionApproved(approvedVersions, targetVersion) {
		return true
	}
	if strings.TrimSpace(approvedUpToVersion) == "" {
		return false
	}
	compareResult, err := compareSemanticVersions(targetVersion, approvedUpToVersion)
	if err != nil {
		return false
	}
	return compareResult <= 0
}

func normalizeVersionToken(version string) string {
	trimmed := strings.TrimSpace(strings.ToLower(version))
	trimmed = strings.TrimPrefix(trimmed, "v")
	return trimmed
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
