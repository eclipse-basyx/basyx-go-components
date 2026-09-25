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

package common

import (
	"strings"
	"testing"
)

func TestLoadConfigAuditDefaultsPreserveHistoryEvidenceStoragePrefix(t *testing.T) {
	clearAuditEnvironment(t)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Audit.Enabled {
		t.Fatal("expected audit to remain disabled without ReBAC or an explicit audit setting")
	}
	if cfg.Audit.WORM.Prefix != DefaultConfig.HistoryEvidencePrefix {
		t.Fatalf("unexpected audit WORM prefix %q", cfg.Audit.WORM.Prefix)
	}
	if cfg.History.Evidence != cfg.Audit.WORM {
		t.Fatal("expected history evidence consumers to use the shared audit WORM configuration")
	}
}

func TestLoadConfigEnablesAuditByDefaultForReBAC(t *testing.T) {
	clearAuditEnvironment(t)

	cfg, err := LoadConfig(writeTempConfig(t, enabledReBACConfig()))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.Audit.Enabled {
		t.Fatal("expected ReBAC to enable audit by default")
	}
}

func TestLoadConfigHonorsExplicitGlobalAuditDisablement(t *testing.T) {
	clearAuditEnvironment(t)

	cfg, err := LoadConfig(writeTempConfig(t, enabledReBACConfig()+"audit:\n  enabled: false\n"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Audit.Enabled {
		t.Fatal("expected explicit audit.enabled=false to override the ReBAC default")
	}
}

func TestLoadConfigMapsAuditWORMToHistoryEvidence(t *testing.T) {
	clearAuditEnvironment(t)

	cfg, err := LoadConfig(writeTempConfig(t, "audit:\n  enabled: true\n  worm:\n    enabled: true\n    provider: s3\n    bucket: evidence\n    prefix: global-audit\n    region: eu-central-1\n    retentionMode: compliance\n    retentionDays: 30\n"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.History.Mode != "off" {
		t.Fatalf("expected resource history to remain disabled, got %q", cfg.History.Mode)
	}
	if cfg.History.Evidence.Enabled || !cfg.Audit.WORM.Enabled {
		t.Fatal("global WORM must protect authorization evidence without enabling disabled resource history")
	}
	if cfg.History.Evidence.Prefix != "global-audit" {
		t.Fatalf("expected history evidence prefix from audit WORM, got %q", cfg.History.Evidence.Prefix)
	}
}

func TestLoadConfigAdaptsLegacyHistoryEvidenceWhenAuditWORMIsAbsent(t *testing.T) {
	clearAuditEnvironment(t)

	cfg, err := LoadConfig(writeTempConfig(t, "history:\n  evidence:\n    prefix: retained-history-prefix\n"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Audit.WORM.Prefix != "retained-history-prefix" {
		t.Fatalf("expected legacy prefix in audit WORM, got %q", cfg.Audit.WORM.Prefix)
	}
}

func TestLoadConfigRejectsConflictingAuditAndLegacyWORMValues(t *testing.T) {
	clearAuditEnvironment(t)

	_, err := LoadConfig(writeTempConfig(t, "audit:\n  worm:\n    prefix: global-audit\nhistory:\n  evidence:\n    prefix: legacy-history\n"))
	if err == nil || !strings.Contains(err.Error(), "CONFIG-AUDIT-CONFLICT") {
		t.Fatalf("expected audit compatibility conflict, got %v", err)
	}
}

func TestLoadConfigAcceptsIdenticalAuditAndLegacyWORMValues(t *testing.T) {
	clearAuditEnvironment(t)

	cfg, err := LoadConfig(writeTempConfig(t, "audit:\n  worm:\n    prefix: shared-prefix\nhistory:\n  evidence:\n    prefix: shared-prefix\n"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Audit.WORM.Prefix != "shared-prefix" {
		t.Fatalf("unexpected WORM prefix %q", cfg.Audit.WORM.Prefix)
	}
}

func TestLoadConfigAppliesAuditEnvironmentWithoutReBACNamespace(t *testing.T) {
	clearAuditEnvironment(t)
	t.Setenv("AUDIT_ENABLED", "false")
	t.Setenv("AUDIT_WORM_PREFIX", "audit-environment")

	cfg, err := LoadConfig(writeTempConfig(t, enabledReBACConfig()))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Audit.Enabled {
		t.Fatal("expected AUDIT_ENABLED=false to override the ReBAC default")
	}
	if cfg.History.Evidence.Prefix != "audit-environment" {
		t.Fatalf("expected audit WORM environment prefix, got %q", cfg.History.Evidence.Prefix)
	}
}

func TestLoadConfigKeepsGlobalAuditAndWORMEnablementIndependent(t *testing.T) {
	clearAuditEnvironment(t)
	t.Setenv("AUDIT_ENABLED", "true")
	t.Setenv("AUDIT_WORM_ENABLED", "false")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.Audit.Enabled || cfg.Audit.WORM.Enabled {
		t.Fatalf("unexpected independent audit settings: %+v", cfg.Audit)
	}
}

func TestLoadConfigRejectsNegativeAuditLimits(t *testing.T) {
	clearAuditEnvironment(t)

	_, err := LoadConfig(writeTempConfig(t, "audit:\n  localRetentionDays: -1\n"))
	if err == nil || !strings.Contains(err.Error(), "CONFIG-AUDIT-LOCALRETENTION") {
		t.Fatalf("expected local retention validation error, got %v", err)
	}
}

func clearAuditEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AUDIT_ENABLED", "AUDIT_LOCAL_RETENTION_DAYS", "AUDIT_BACKLOG_LIMIT",
		"AUDIT_WORM_ENABLED", "AUDIT_WORM_PROVIDER", "AUDIT_WORM_BUCKET", "AUDIT_WORM_PREFIX", "AUDIT_WORM_REGION", "AUDIT_WORM_ENDPOINT", "AUDIT_WORM_ACCESS_KEY_ID", "AUDIT_WORM_SECRET_ACCESS_KEY", "AUDIT_WORM_PATH_STYLE", "AUDIT_WORM_RETENTION_MODE", "AUDIT_WORM_RETENTION_DAYS", "AUDIT_WORM_WRITE_TIMEOUT_SECONDS", "AUDIT_WORM_SIGNING_PRIVATE_KEY_PATH", "AUDIT_WORM_SIGNING_PUBLIC_KEY_PATH", "AUDIT_WORM_SIGNING_REQUIRED",
		"BASYX_HISTORY_EVIDENCE_ENABLED", "BASYX_HISTORY_EVIDENCE_PROVIDER", "BASYX_HISTORY_EVIDENCE_BUCKET", "BASYX_HISTORY_EVIDENCE_PREFIX", "BASYX_HISTORY_EVIDENCE_REGION", "BASYX_HISTORY_EVIDENCE_ENDPOINT", "BASYX_HISTORY_EVIDENCE_ACCESS_KEY_ID", "BASYX_HISTORY_EVIDENCE_SECRET_ACCESS_KEY", "BASYX_HISTORY_EVIDENCE_PATH_STYLE", "BASYX_HISTORY_EVIDENCE_RETENTION_MODE", "BASYX_HISTORY_EVIDENCE_RETENTION_DAYS", "BASYX_HISTORY_EVIDENCE_WRITE_TIMEOUT_SECONDS", "BASYX_HISTORY_EVIDENCE_SIGNING_PRIVATE_KEY_PATH", "BASYX_HISTORY_EVIDENCE_SIGNING_PUBLIC_KEY_PATH", "BASYX_HISTORY_EVIDENCE_SIGNING_REQUIRED",
	} {
		withUnsetEnv(t, key)
	}
}

func enabledReBACConfig() string {
	return "abac:\n  enabled: true\nrebac:\n  enabled: true\n  url: https://openfga.example\n  storeId: audit-store\n  modelId: audit-model\n  scope: audit\n  administrators:\n    - type: user\n      issuer: https://issuer.example\n      subject: auditor\n"
}
