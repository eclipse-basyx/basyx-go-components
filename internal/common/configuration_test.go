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
	withUnsetEnv(t, "BASYX_HISTORY_IMMUTABILITY")
	withUnsetEnv(t, "BASYX_AUDIT_IDENTITY_MODE")
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

	if cfg.History.Mode != "api" {
		t.Fatalf("expected default history mode api, got %q", cfg.History.Mode)
	}
	if cfg.History.RetentionDays != 0 {
		t.Fatalf("expected default retention 0, got %d", cfg.History.RetentionDays)
	}
	if cfg.History.Immutability != "none" {
		t.Fatalf("expected default immutability none, got %q", cfg.History.Immutability)
	}
	if cfg.History.AuditIdentityMode != "minimal" {
		t.Fatalf("expected default audit identity mode minimal, got %q", cfg.History.AuditIdentityMode)
	}
	if cfg.Eventing.Enabled || cfg.Eventing.Format != "cloudevents" || cfg.Eventing.TopicPrefix != "basyx" {
		t.Fatalf("unexpected eventing defaults: %+v", cfg.Eventing)
	}
}

func TestLoadConfigAppliesBasyxHistoryAndEventingEnvOverrides(t *testing.T) {
	t.Setenv("BASYX_HISTORY_MODE", "audit")
	t.Setenv("BASYX_HISTORY_RETENTION_DAYS", "30")
	t.Setenv("BASYX_HISTORY_IMMUTABILITY", "external_anchor")
	t.Setenv("BASYX_AUDIT_IDENTITY_MODE", "extended")
	t.Setenv("BASYX_EVENTING_ENABLED", "true")
	t.Setenv("BASYX_EVENTING_FORMAT", "cloudevents")
	t.Setenv("BASYX_EVENTING_SINKS", "mqtt,kafka")
	t.Setenv("BASYX_EVENTING_OUTBOX_ENABLED", "true")
	t.Setenv("BASYX_EVENTING_TOPIC_PREFIX", "custom")
	captureLogOutput(t)

	cfg, err := LoadConfig("", NORMAL)
	if err != nil {
		t.Fatalf("unexpected config load error: %v", err)
	}

	if cfg.History.Mode != "audit" || cfg.History.RetentionDays != 30 || cfg.History.Immutability != "external_anchor" || cfg.History.AuditIdentityMode != "extended" {
		t.Fatalf("unexpected history env override result: %+v", cfg.History)
	}
	if !cfg.Eventing.Enabled || !cfg.Eventing.OutboxEnabled || cfg.Eventing.TopicPrefix != "custom" {
		t.Fatalf("unexpected eventing env override result: %+v", cfg.Eventing)
	}
	if len(cfg.Eventing.Sinks) != 2 || cfg.Eventing.Sinks[0] != "mqtt" || cfg.Eventing.Sinks[1] != "kafka" {
		t.Fatalf("unexpected eventing sinks: %+v", cfg.Eventing.Sinks)
	}
}
