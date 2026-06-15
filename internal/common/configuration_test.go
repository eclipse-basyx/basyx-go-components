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
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp config: %v", err)
	}
	return file.Name()
}

func withUnsetEnv(t *testing.T, key string) {
	t.Helper()
	oldValue, hadValue := os.LookupEnv(key)
	if hadValue {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset env %s: %v", key, err)
		}
	}
	t.Cleanup(func() {
		if !hadValue {
			_ = os.Unsetenv(key)
			return
		}
		_ = os.Setenv(key, oldValue)
	})
}

func captureLogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})
	return &output
}

func TestServerStrictVerificationDefaultIsPermissive(t *testing.T) {
	withUnsetEnv(t, "SERVER_STRICTVERIFICATION")
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.Server.StrictVerification != DefaultConfig.ServerStrictVerification {
		t.Fatalf("strictVerification default mismatch: cfg=%q default=%q", cfg.Server.StrictVerification, DefaultConfig.ServerStrictVerification)
	}
	if cfg.Server.StrictVerification != "permissive" {
		t.Fatalf("expected permissive strictVerification default, got %q", cfg.Server.StrictVerification)
	}
}

func TestViperAndStructStrictVerificationDefaultsMatch(t *testing.T) {
	v := viper.New()
	setDefaults(v)

	actual := v.GetString("server.strictVerification")
	if actual != DefaultConfig.ServerStrictVerification {
		t.Fatalf("viper default %q differs from DefaultConfig %q", actual, DefaultConfig.ServerStrictVerification)
	}
}

func TestPrintConfigurationMarksPermissiveVerificationModeAsDefault(t *testing.T) {
	output := captureLogOutput(t)
	cfg := &Config{
		Server: ServerConfig{
			Port:                          DefaultConfig.ServerPort,
			ContextPath:                   DefaultConfig.ServerContextPath,
			CacheEnabled:                  DefaultConfig.ServerCacheEnabled,
			StrictVerification:            DefaultConfig.ServerStrictVerification,
			VerificationEndpointAvailable: DefaultConfig.ServerVerificationEndpointAvailable,
		},
		Postgres: PostgresConfig{
			Port:                   DefaultConfig.PgPort,
			DBName:                 DefaultConfig.PgDBName,
			MaxOpenConnections:     DefaultConfig.PgMaxOpen,
			MaxIdleConnections:     DefaultConfig.PgMaxIdle,
			ConnMaxLifetimeMinutes: DefaultConfig.PgConnLifetime,
		},
		CorsConfig: CorsConfig{
			AllowedOrigins:   DefaultConfig.AllowedOrigins,
			AllowedMethods:   DefaultConfig.AllowedMethods,
			AllowedHeaders:   DefaultConfig.AllowedHeaders,
			AllowCredentials: DefaultConfig.AllowCredentials,
		},
		ABAC: ABACConfig{
			Enabled: DefaultConfig.ABACEnabled,
		},
	}

	PrintConfiguration(cfg)

	if !strings.Contains(output.String(), "Verification Mode: permissive (default)") {
		t.Fatalf("printed configuration did not mark permissive verification mode as default:\n%s", output.String())
	}
}

func TestLoadConfigWithoutStrictVerificationUsesPermissiveDefault(t *testing.T) {
	withUnsetEnv(t, "SERVER_STRICTVERIFICATION")
	captureLogOutput(t)
	path := writeTempConfig(t, "server:\n  port: 5004\n")

	cfg, err := LoadConfig(path, NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}
	if cfg.Server.StrictVerification != "permissive" {
		t.Fatalf("expected permissive strictVerification default, got %q", cfg.Server.StrictVerification)
	}
}

func TestLoadConfigRejectsBooleanStrictVerification(t *testing.T) {
	withUnsetEnv(t, "SERVER_STRICTVERIFICATION")
	captureLogOutput(t)
	path := writeTempConfig(t, "server:\n  strictVerification: true\n")

	_, err := LoadConfig(path, NORMAL)
	if err == nil {
		t.Fatal("expected invalid strictVerification mode error")
	}
	if !strings.Contains(err.Error(), "invalid server.strictVerification") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigAcceptsPermissiveStrictVerification(t *testing.T) {
	withUnsetEnv(t, "SERVER_STRICTVERIFICATION")
	captureLogOutput(t)
	path := writeTempConfig(t, "server:\n  strictVerification: permissive\n")

	cfg, err := LoadConfig(path, NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}
	if cfg.Server.StrictVerification != "permissive" {
		t.Fatalf("unexpected strictVerification mode: %q", cfg.Server.StrictVerification)
	}
}

func TestLoadConfigAppliesHistoryAndEventingDefaults(t *testing.T) {
	withUnsetEnv(t, "BASYX_HISTORY_MODE")
	withUnsetEnv(t, "BASYX_HISTORY_RETENTION_DAYS")
	withUnsetEnv(t, "BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL")
	withUnsetEnv(t, "BASYX_HISTORY_IMMUTABILITY")
	withUnsetEnv(t, "BASYX_AUDIT_IDENTITY_MODE")
	withUnsetEnv(t, "BASYX_HISTORY_EVIDENCE_ENABLED")
	withUnsetEnv(t, "BASYX_HISTORY_EVIDENCE_PROVIDER")
	withUnsetEnv(t, "BASYX_HISTORY_EVIDENCE_BUCKET")
	withUnsetEnv(t, "BASYX_HISTORY_EVIDENCE_REGION")
	withUnsetEnv(t, "BASYX_HISTORY_EVIDENCE_WRITE_TIMEOUT_SECONDS")
	withUnsetEnv(t, "BASYX_HISTORY_INTEGRITY_ANCHOR_PROVIDER")
	withUnsetEnv(t, "BASYX_EVENTING_ENABLED")
	withUnsetEnv(t, "BASYX_EVENTING_FORMAT")
	withUnsetEnv(t, "BASYX_EVENTING_SINKS")
	withUnsetEnv(t, "BASYX_EVENTING_OUTBOX_ENABLED")
	withUnsetEnv(t, "BASYX_EVENTING_TOPIC_PREFIX")
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.History.Mode != "off" {
		t.Fatalf("expected default history mode off, got %q", cfg.History.Mode)
	}
	if cfg.History.RetentionDays != 0 {
		t.Fatalf("expected default retention 0, got %d", cfg.History.RetentionDays)
	}
	if cfg.History.FullSnapshotInterval != 1 {
		t.Fatalf("expected default full snapshot interval 1, got %d", cfg.History.FullSnapshotInterval)
	}
	if cfg.History.Immutability != "none" {
		t.Fatalf("expected default immutability none, got %q", cfg.History.Immutability)
	}
	if cfg.History.AuditIdentityMode != "none" {
		t.Fatalf("expected default audit identity mode none, got %q", cfg.History.AuditIdentityMode)
	}
	if cfg.History.Evidence.Enabled || cfg.History.Evidence.Provider != "none" || cfg.History.IntegrityAnchor.Provider != "none" {
		t.Fatalf("unexpected history evidence defaults: %+v", cfg.History)
	}
	if cfg.Eventing.Enabled || cfg.Eventing.Format != "cloudevents" || cfg.Eventing.TopicPrefix != "basyx" {
		t.Fatalf("unexpected eventing defaults: %+v", cfg.Eventing)
	}
}

func TestLoadConfigAppliesSupportedBasyxHistoryEnvOverrides(t *testing.T) {
	t.Setenv("BASYX_HISTORY_MODE", "audit")
	t.Setenv("BASYX_HISTORY_RETENTION_DAYS", "0")
	t.Setenv("BASYX_HISTORY_FULL_SNAPSHOT_INTERVAL", "3")
	t.Setenv("BASYX_HISTORY_IMMUTABILITY", "postgres_guarded")
	t.Setenv("BASYX_AUDIT_IDENTITY_MODE", "none")
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.History.Mode != "audit" || cfg.History.RetentionDays != 0 || cfg.History.FullSnapshotInterval != 3 || cfg.History.Immutability != "postgres_guarded" || cfg.History.AuditIdentityMode != "none" {
		t.Fatalf("unexpected history env override result: %+v", cfg.History)
	}
}

func TestLoadConfigAppliesHistoryEvidenceEnvOverrides(t *testing.T) {
	t.Setenv("BASYX_HISTORY_MODE", "audit")
	t.Setenv("BASYX_HISTORY_IMMUTABILITY", "postgres_guarded")
	t.Setenv("BASYX_AUDIT_IDENTITY_MODE", "none")
	t.Setenv("BASYX_HISTORY_EVIDENCE_ENABLED", "true")
	t.Setenv("BASYX_HISTORY_EVIDENCE_PROVIDER", "s3")
	t.Setenv("BASYX_HISTORY_EVIDENCE_BUCKET", "history-evidence")
	t.Setenv("BASYX_HISTORY_EVIDENCE_PREFIX", "test-prefix")
	t.Setenv("BASYX_HISTORY_EVIDENCE_REGION", "us-east-1")
	t.Setenv("BASYX_HISTORY_EVIDENCE_ENDPOINT", "http://minio:9000")
	t.Setenv("BASYX_HISTORY_EVIDENCE_ACCESS_KEY_ID", "minio")
	t.Setenv("BASYX_HISTORY_EVIDENCE_SECRET_ACCESS_KEY", "minio123")
	t.Setenv("BASYX_HISTORY_EVIDENCE_PATH_STYLE", "true")
	t.Setenv("BASYX_HISTORY_EVIDENCE_RETENTION_MODE", "governance")
	t.Setenv("BASYX_HISTORY_EVIDENCE_RETENTION_DAYS", "7")
	t.Setenv("BASYX_HISTORY_EVIDENCE_WRITE_TIMEOUT_SECONDS", "12")
	t.Setenv("BASYX_HISTORY_EVIDENCE_SIGNING_PUBLIC_KEY_PATH", "/keys/manifest-public.pem")
	t.Setenv("BASYX_HISTORY_INTEGRITY_ANCHOR_PROVIDER", "none")
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if !cfg.History.Evidence.Enabled || cfg.History.Evidence.Provider != "s3" || cfg.History.Evidence.Bucket != "history-evidence" || cfg.History.Evidence.Prefix != "test-prefix" {
		t.Fatalf("unexpected evidence env override result: %+v", cfg.History.Evidence)
	}
	if !cfg.History.Evidence.UsePathStyle || cfg.History.Evidence.RetentionMode != "governance" || cfg.History.Evidence.RetentionDays != 7 || cfg.History.Evidence.WriteTimeoutSec != 12 {
		t.Fatalf("unexpected evidence retention/path-style result: %+v", cfg.History.Evidence)
	}
	if cfg.History.Evidence.Signing.PublicKeyPath != "/keys/manifest-public.pem" {
		t.Fatalf("unexpected evidence signing public key: %+v", cfg.History.Evidence.Signing)
	}
}

func TestValidateHistoryAndEventingConfigAcceptsDiffBackedSnapshotInterval(t *testing.T) {
	cfg := Config{History: HistoryConfig{Mode: "api", FullSnapshotInterval: 10, Immutability: "none", AuditIdentityMode: "extended"}}

	if err := validateHistoryAndEventingConfig(&cfg); err != nil {
		t.Fatalf("expected diff-backed full snapshot interval to be accepted, got %v", err)
	}
}

func TestValidateHistoryAndEventingConfigAcceptsCompleteS3EvidenceConfig(t *testing.T) {
	cfg := Config{
		JWS: JWSConfig{PrivateKeyPath: "fallback-key.pem"},
		History: HistoryConfig{
			Mode:                 "audit",
			FullSnapshotInterval: 5,
			Immutability:         "postgres_guarded",
			AuditIdentityMode:    "none",
			Evidence: HistoryEvidenceConfig{
				Enabled:         true,
				Provider:        "s3",
				Bucket:          "history-evidence",
				Region:          "us-east-1",
				RetentionMode:   "governance",
				RetentionDays:   1,
				WriteTimeoutSec: 10,
				Signing:         HistoryEvidenceSigningConfig{Required: true},
			},
			IntegrityAnchor: HistoryIntegrityAnchorConfig{Provider: "none"},
		},
	}

	if err := validateHistoryAndEventingConfig(&cfg); err != nil {
		t.Fatalf("expected complete S3 evidence config to be accepted, got %v", err)
	}
}

func TestValidateHistoryAndEventingConfigRejectsUnsupportedFeatures(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name:   "retention",
			config: Config{History: HistoryConfig{Mode: "api", RetentionDays: 30, FullSnapshotInterval: 1, Immutability: "none", AuditIdentityMode: "none"}},
		},
		{
			name:   "full snapshot interval zero",
			config: Config{History: HistoryConfig{Mode: "api", FullSnapshotInterval: 0, Immutability: "none", AuditIdentityMode: "none"}},
		},
		{
			name:   "external anchor",
			config: Config{History: HistoryConfig{Mode: "api", FullSnapshotInterval: 1, Immutability: "external_anchor", AuditIdentityMode: "none"}},
		},
		{
			name: "incomplete evidence",
			config: Config{History: HistoryConfig{
				Mode:                 "api",
				FullSnapshotInterval: 1,
				Immutability:         "none",
				AuditIdentityMode:    "none",
				Evidence:             HistoryEvidenceConfig{Enabled: true, Provider: "s3", Region: "us-east-1"},
			}},
		},
		{
			name: "evidence with history off",
			config: Config{History: HistoryConfig{
				Mode:                 "off",
				FullSnapshotInterval: 1,
				Immutability:         "none",
				AuditIdentityMode:    "none",
				Evidence:             HistoryEvidenceConfig{Enabled: true, Provider: "s3", Bucket: "history-evidence", Region: "us-east-1"},
			}},
		},
		{
			name: "evidence without retention",
			config: Config{History: HistoryConfig{
				Mode:                 "api",
				FullSnapshotInterval: 1,
				Immutability:         "none",
				AuditIdentityMode:    "none",
				Evidence:             HistoryEvidenceConfig{Enabled: true, Provider: "s3", Bucket: "history-evidence", Region: "us-east-1", WriteTimeoutSec: 10},
			}},
		},
		{
			name: "evidence without write timeout",
			config: Config{History: HistoryConfig{
				Mode:                 "api",
				FullSnapshotInterval: 1,
				Immutability:         "none",
				AuditIdentityMode:    "none",
				Evidence:             HistoryEvidenceConfig{Enabled: true, Provider: "s3", Bucket: "history-evidence", Region: "us-east-1", RetentionMode: "governance", RetentionDays: 1},
			}},
		},
		{
			name: "reserved integrity anchor provider",
			config: Config{History: HistoryConfig{
				Mode:                 "api",
				FullSnapshotInterval: 1,
				Immutability:         "none",
				AuditIdentityMode:    "none",
				IntegrityAnchor:      HistoryIntegrityAnchorConfig{Provider: "immudb"},
			}},
		},
		{
			name:   "eventing",
			config: Config{History: HistoryConfig{Mode: "off", FullSnapshotInterval: 1, Immutability: "none", AuditIdentityMode: "none"}, Eventing: EventingConfig{Enabled: true}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateHistoryAndEventingConfig(&test.config); err == nil {
				t.Fatal("expected unsupported configuration error")
			}
		})
	}
}
