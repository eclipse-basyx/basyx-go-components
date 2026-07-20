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
	"context"
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

func TestViperAndStructSwaggerEnabledDefaultsMatch(t *testing.T) {
	v := viper.New()
	setDefaults(v)

	actual := v.GetBool("swagger.enabled")
	if actual != DefaultConfig.SwaggerEnabled {
		t.Fatalf("viper default %t differs from DefaultConfig %t", actual, DefaultConfig.SwaggerEnabled)
	}
}

func TestBulkBatchLimitDefaultIsOneThousand(t *testing.T) {
	for _, key := range []string{"GENERAL_BULKBATCHLIMIT", "GENERAL_BULK_BATCH_LIMIT", "BASYX_GENERAL_BULK_BATCH_LIMIT"} {
		withUnsetEnv(t, key)
	}
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.General.BulkBatchLimit != DefaultConfig.GeneralBulkBatchLimit {
		t.Fatalf("bulkBatchLimit default mismatch: cfg=%d default=%d", cfg.General.BulkBatchLimit, DefaultConfig.GeneralBulkBatchLimit)
	}
	if cfg.General.BulkBatchLimit != 1000 {
		t.Fatalf("expected bulkBatchLimit default 1000, got %d", cfg.General.BulkBatchLimit)
	}
	if actual := BulkBatchLimitFromContext(context.Background()); actual != 1000 {
		t.Fatalf("expected context fallback bulkBatchLimit 1000, got %d", actual)
	}
}

func TestBulkBatchLimitCanBeOverriddenByReadableEnvironmentVariable(t *testing.T) {
	for _, key := range []string{"GENERAL_BULKBATCHLIMIT", "GENERAL_BULK_BATCH_LIMIT", "BASYX_GENERAL_BULK_BATCH_LIMIT"} {
		withUnsetEnv(t, key)
	}
	t.Setenv("GENERAL_BULK_BATCH_LIMIT", "17")
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.General.BulkBatchLimit != 17 {
		t.Fatalf("expected env bulkBatchLimit 17, got %d", cfg.General.BulkBatchLimit)
	}
}

func TestBulkBatchLimitRejectsNonPositiveValues(t *testing.T) {
	for _, key := range []string{"GENERAL_BULKBATCHLIMIT", "GENERAL_BULK_BATCH_LIMIT", "BASYX_GENERAL_BULK_BATCH_LIMIT"} {
		withUnsetEnv(t, key)
	}
	t.Setenv("GENERAL_BULK_BATCH_LIMIT", "0")
	captureLogOutput(t)

	_, err := LoadConfig("", NORMAL)
	if err == nil {
		t.Fatal("expected config load error for non-positive bulkBatchLimit")
	}
	if !strings.Contains(err.Error(), "CONFIG-GENERAL-BULKBATCHLIMIT") {
		t.Fatalf("expected CONFIG-GENERAL-BULKBATCHLIMIT error, got %v", err)
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
			SSLMode:                DefaultConfig.PgSSLMode,
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
		Swagger: SwaggerConfig{
			Enabled: DefaultConfig.SwaggerEnabled,
		},
	}

	PrintConfiguration(cfg)

	if !strings.Contains(output.String(), "Verification Mode: permissive (default)") {
		t.Fatalf("printed configuration did not mark permissive verification mode as default:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "Enabled: true (default)") {
		t.Fatalf("printed configuration did not mark Swagger enabled as default:\n%s", output.String())
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

func TestLoadConfigAppliesSwaggerEnabled(t *testing.T) {
	withUnsetEnv(t, "SWAGGER_ENABLED")
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}
	if !cfg.Swagger.Enabled {
		t.Fatal("expected Swagger to be enabled by default")
	}

	path := writeTempConfig(t, "swagger:\n  enabled: false\n")
	cfg, err = LoadConfig(path, NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}
	if cfg.Swagger.Enabled {
		t.Fatal("expected Swagger to be disabled from config")
	}
}

func TestLoadConfigAppliesPostgresConnectionParameters(t *testing.T) {
	captureLogOutput(t)
	path := writeTempConfig(t, `postgres:
  sslmode: verify-full
  sslcert: /certs/client.crt
  sslkey: /certs/client.key
  sslrootcert: /certs/root.crt
  connectTimeoutSeconds: 15
  applicationName: basyx-service
  fallbackApplicationName: basyx
  searchPath: tenant_a,public
  options: "-c statement_timeout=5000"
  timezone: Europe/Berlin
`)

	cfg, err := LoadConfig(path, NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.Postgres.SSLMode != "verify-full" ||
		cfg.Postgres.SSLCert != "/certs/client.crt" ||
		cfg.Postgres.SSLKey != "/certs/client.key" ||
		cfg.Postgres.SSLRootCert != "/certs/root.crt" ||
		cfg.Postgres.ConnectTimeoutSeconds != 15 ||
		cfg.Postgres.ApplicationName != "basyx-service" ||
		cfg.Postgres.FallbackApplicationName != "basyx" ||
		cfg.Postgres.SearchPath != "tenant_a,public" ||
		cfg.Postgres.Options != "-c statement_timeout=5000" ||
		cfg.Postgres.TimeZone != "Europe/Berlin" {
		t.Fatalf("unexpected postgres config: %+v", cfg.Postgres)
	}
}

func TestLoadConfigAppliesPostgresEnvironmentOverrides(t *testing.T) {
	t.Setenv("POSTGRES_SSLMODE", "require")
	t.Setenv("POSTGRES_SSLCERT", "/env/client.crt")
	t.Setenv("POSTGRES_SSLKEY", "/env/client.key")
	t.Setenv("POSTGRES_SSLROOTCERT", "/env/root.crt")
	t.Setenv("POSTGRES_CONNECTTIMEOUTSECONDS", "7")
	t.Setenv("POSTGRES_APPLICATIONNAME", "basyx-env-service")
	t.Setenv("POSTGRES_FALLBACKAPPLICATIONNAME", "basyx-env")
	t.Setenv("POSTGRES_SEARCHPATH", "env_schema,public")
	t.Setenv("POSTGRES_OPTIONS", "-c lock_timeout=1000")
	t.Setenv("POSTGRES_TIMEZONE", "UTC")
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.Postgres.SSLMode != "require" ||
		cfg.Postgres.SSLCert != "/env/client.crt" ||
		cfg.Postgres.SSLKey != "/env/client.key" ||
		cfg.Postgres.SSLRootCert != "/env/root.crt" ||
		cfg.Postgres.ConnectTimeoutSeconds != 7 ||
		cfg.Postgres.ApplicationName != "basyx-env-service" ||
		cfg.Postgres.FallbackApplicationName != "basyx-env" ||
		cfg.Postgres.SearchPath != "env_schema,public" ||
		cfg.Postgres.Options != "-c lock_timeout=1000" ||
		cfg.Postgres.TimeZone != "UTC" {
		t.Fatalf("unexpected postgres env config: %+v", cfg.Postgres)
	}
}

func TestLoadConfigAppliesPostgresDSN(t *testing.T) {
	captureLogOutput(t)
	path := writeTempConfig(t, `postgres:
  dsn: postgres://postgres.example:5432/basyx?sslmode=require
  maxOpenConnections: 100
`)

	cfg, err := LoadConfig(path, NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.Postgres.DSN != "postgres://postgres.example:5432/basyx?sslmode=require" {
		t.Fatalf("unexpected postgres dsn: %q", cfg.Postgres.DSN)
	}
	if cfg.Postgres.MaxOpenConnections != 100 {
		t.Fatalf("unexpected max open connections: %d", cfg.Postgres.MaxOpenConnections)
	}
}

func TestLoadConfigAppliesPostgresDSNEnvironmentOverride(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://postgres.example:5432/basyx?sslmode=require")
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.Postgres.DSN != "postgres://postgres.example:5432/basyx?sslmode=require" {
		t.Fatalf("unexpected postgres dsn: %q", cfg.Postgres.DSN)
	}
}

func TestLoadConfigRejectsPostgresDSNWithConnectionField(t *testing.T) {
	captureLogOutput(t)
	path := writeTempConfig(t, `postgres:
  dsn: postgres://postgres.example:5432/basyx?sslmode=require
  host: db
`)

	_, err := LoadConfig(path, NORMAL)
	if err == nil {
		t.Fatal("expected postgres dsn conflict error")
	}
	if !strings.Contains(err.Error(), "CONFIG-POSTGRES-DSN-CONFLICT") ||
		!strings.Contains(err.Error(), "postgres.host") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsPostgresDSNWithConnectionFieldEnv(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://postgres.example:5432/basyx?sslmode=require")
	t.Setenv("POSTGRES_SSLMODE", "disable")
	captureLogOutput(t)

	_, err := LoadConfig("", NORMAL)
	if err == nil {
		t.Fatal("expected postgres dsn conflict error")
	}
	if !strings.Contains(err.Error(), "CONFIG-POSTGRES-DSN-CONFLICT") ||
		!strings.Contains(err.Error(), "postgres.sslmode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsUnsupportedPostgresSSLMode(t *testing.T) {
	captureLogOutput(t)
	path := writeTempConfig(t, "postgres:\n  sslmode: invalid\n")

	_, err := LoadConfig(path, NORMAL)
	if err == nil {
		t.Fatal("expected unsupported postgres sslmode error")
	}
	if !strings.Contains(err.Error(), "CONFIG-POSTGRES-SSLMODE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsNegativePostgresConnectTimeout(t *testing.T) {
	captureLogOutput(t)
	path := writeTempConfig(t, "postgres:\n  connectTimeoutSeconds: -1\n")

	_, err := LoadConfig(path, NORMAL)
	if err == nil {
		t.Fatal("expected negative postgres connect timeout error")
	}
	if !strings.Contains(err.Error(), "CONFIG-POSTGRES-CONNECTTIMEOUT") {
		t.Fatalf("unexpected error: %v", err)
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
	withUnsetEnv(t, "ABAC_POLICY_FILE_IMPORT")
	withUnsetEnv(t, "BASYX_ABAC_POLICY_FILE_IMPORT")
	withUnsetEnv(t, "ABAC_POLICY_SCOPE")
	withUnsetEnv(t, "BASYX_ABAC_POLICY_SCOPE")
	withUnsetEnv(t, "ABAC_MANAGEMENT_API_ENABLED")
	withUnsetEnv(t, "ABAC_MANAGEMENTAPI_ENABLED")
	withUnsetEnv(t, "BASYX_ABAC_MANAGEMENT_API_ENABLED")
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

func TestLoadConfigAppliesABACPolicyRepositoryEnvOverrides(t *testing.T) {
	t.Setenv("ABAC_POLICY_FILE_IMPORT", "if_missing")
	t.Setenv("ABAC_POLICY_SCOPE", "aasregistry-internal")
	t.Setenv("ABAC_MANAGEMENT_API_ENABLED", "true")
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.ABAC.PolicyFileImport != ABACPolicyFileImportIfMissing {
		t.Fatalf("expected policy file import override, got %q", cfg.ABAC.PolicyFileImport)
	}
	if cfg.ABAC.PolicyScope != "aasregistry-internal" {
		t.Fatalf("expected policy scope override, got %q", cfg.ABAC.PolicyScope)
	}
	if !cfg.ABAC.ManagementAPI.Enabled {
		t.Fatal("expected ABAC management API env override")
	}
}

func TestLoadConfigAppliesABACPolicyScopeFromConfig(t *testing.T) {
	withUnsetEnv(t, "ABAC_POLICY_SCOPE")
	withUnsetEnv(t, "BASYX_ABAC_POLICY_SCOPE")
	path := writeTempConfig(t, `
abac:
  policyScope: " dtr:public-01 "
`)
	captureLogOutput(t)

	cfg, err := LoadConfig(path, NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.ABAC.PolicyScope != "dtr:public-01" {
		t.Fatalf("expected trimmed policy scope, got %q", cfg.ABAC.PolicyScope)
	}
}

func TestLoadConfigAppliesABACPolicyScopeBasyxEnvOverride(t *testing.T) {
	withUnsetEnv(t, "ABAC_POLICY_SCOPE")
	t.Setenv("BASYX_ABAC_POLICY_SCOPE", "shared.scope")
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.ABAC.PolicyScope != "shared.scope" {
		t.Fatalf("expected BASYX policy scope override, got %q", cfg.ABAC.PolicyScope)
	}
}

func TestConfiguredPolicyScopeUsesDefaultForEmptyConfig(t *testing.T) {
	cfg := &Config{
		ABAC: ABACConfig{
			PolicyScope: " ",
		},
	}

	scope, err := ConfiguredPolicyScope(cfg, "aasregistryservice")
	if err != nil {
		t.Fatalf("unexpected policy scope error: %v", err)
	}
	if scope != "aasregistryservice" {
		t.Fatalf("expected default service scope, got %q", scope)
	}
}

func TestConfiguredPolicyScopeRejectsEmptyDefault(t *testing.T) {
	_, err := ConfiguredPolicyScope(&Config{}, " ")
	if err == nil {
		t.Fatal("expected empty default service scope error")
	}
	if !strings.Contains(err.Error(), "CONFIG-ABAC-POLICYSCOPE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsInvalidABACPolicyScope(t *testing.T) {
	t.Setenv("ABAC_POLICY_SCOPE", "aasregistry/internal")
	captureLogOutput(t)

	_, err := LoadConfig("", NORMAL)
	if err == nil {
		t.Fatal("expected invalid ABAC policy scope error")
	}
	if !strings.Contains(err.Error(), "CONFIG-ABAC-POLICYSCOPE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsTooLongABACPolicyScope(t *testing.T) {
	t.Setenv("ABAC_POLICY_SCOPE", strings.Repeat("a", maxABACPolicyScopeLength+1))
	captureLogOutput(t)

	_, err := LoadConfig("", NORMAL)
	if err == nil {
		t.Fatal("expected too long ABAC policy scope error")
	}
	if !strings.Contains(err.Error(), "CONFIG-ABAC-POLICYSCOPE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadConfigRejectsUnsupportedABACPolicyFileImport(t *testing.T) {
	t.Setenv("ABAC_POLICY_FILE_IMPORT", "sometimes")
	captureLogOutput(t)

	_, err := LoadConfig("", NORMAL)
	if err == nil {
		t.Fatal("expected unsupported ABAC policy file import error")
	}
	if !strings.Contains(err.Error(), "CONFIG-ABAC-POLICYFILEIMPORT") {
		t.Fatalf("unexpected error: %v", err)
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

func TestValidateHistoryAndEventingConfigAcceptsEvidenceWithHistoryOff(t *testing.T) {
	cfg := Config{History: HistoryConfig{
		Mode:                 "off",
		FullSnapshotInterval: 5,
		Immutability:         "none",
		AuditIdentityMode:    "none",
		Evidence: HistoryEvidenceConfig{
			Enabled:         true,
			Provider:        "s3",
			Bucket:          "history-evidence",
			Region:          "us-east-1",
			RetentionMode:   "governance",
			RetentionDays:   1,
			WriteTimeoutSec: 10,
		},
	}}

	if err := validateHistoryAndEventingConfig(&cfg); err != nil {
		t.Fatalf("expected evidence-only configuration to be accepted, got %v", err)
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
