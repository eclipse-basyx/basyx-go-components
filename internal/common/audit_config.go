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
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// AuditConfig configures global audit capture and optional WORM evidence storage.
// WORM reuses HistoryEvidenceConfig so history and audit artifacts have identical
// storage, retention, and signing semantics.
type AuditConfig struct {
	Enabled            bool                  `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	LocalRetentionDays int                   `mapstructure:"localRetentionDays" yaml:"localRetentionDays" json:"localRetentionDays"`
	BacklogLimit       int                   `mapstructure:"backlogLimit" yaml:"backlogLimit" json:"backlogLimit"`
	WORM               HistoryEvidenceConfig `mapstructure:"worm" yaml:"worm" json:"worm"`
}

func setAuditDefaults(v *viper.Viper) {
	v.SetDefault("audit.enabled", false)
	v.SetDefault("audit.localRetentionDays", 0)
	v.SetDefault("audit.backlogLimit", 0)
}

func applyAuditEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	applyBoolEnv("AUDIT_ENABLED", func(value bool) { cfg.Audit.Enabled = value })
	applyIntEnv("AUDIT_LOCAL_RETENTION_DAYS", func(value int) { cfg.Audit.LocalRetentionDays = value })
	applyIntEnv("AUDIT_BACKLOG_LIMIT", func(value int) { cfg.Audit.BacklogLimit = value })
	applyAuditWORMEnvOverrides(&cfg.Audit.WORM)
}

func applyAuditWORMEnvOverrides(worm *HistoryEvidenceConfig) {
	if worm == nil {
		return
	}
	applyBoolEnv("AUDIT_WORM_ENABLED", func(value bool) { worm.Enabled = value })
	if value, ok := lookupTrimmedEnv("AUDIT_WORM_PROVIDER"); ok {
		worm.Provider = value
	}
	if value, ok := lookupTrimmedEnv("AUDIT_WORM_BUCKET"); ok {
		worm.Bucket = value
	}
	if value, ok := lookupTrimmedEnv("AUDIT_WORM_PREFIX"); ok {
		worm.Prefix = value
	}
	if value, ok := lookupTrimmedEnv("AUDIT_WORM_REGION"); ok {
		worm.Region = value
	}
	if value, ok := lookupTrimmedEnv("AUDIT_WORM_ENDPOINT"); ok {
		worm.Endpoint = value
	}
	if value, ok := lookupTrimmedEnv("AUDIT_WORM_ACCESS_KEY_ID"); ok {
		worm.AccessKeyID = value
	}
	if value, ok := lookupTrimmedEnv("AUDIT_WORM_SECRET_ACCESS_KEY"); ok {
		worm.SecretAccessKey = value
	}
	applyBoolEnv("AUDIT_WORM_PATH_STYLE", func(value bool) { worm.UsePathStyle = value })
	if value, ok := lookupTrimmedEnv("AUDIT_WORM_RETENTION_MODE"); ok {
		worm.RetentionMode = value
	}
	applyIntEnv("AUDIT_WORM_RETENTION_DAYS", func(value int) { worm.RetentionDays = value })
	applyIntEnv("AUDIT_WORM_WRITE_TIMEOUT_SECONDS", func(value int) { worm.WriteTimeoutSec = value })
	if value, ok := lookupTrimmedEnv("AUDIT_WORM_SIGNING_PRIVATE_KEY_PATH"); ok {
		worm.Signing.PrivateKeyPath = value
	}
	if value, ok := lookupTrimmedEnv("AUDIT_WORM_SIGNING_PUBLIC_KEY_PATH"); ok {
		worm.Signing.PublicKeyPath = value
	}
	applyBoolEnv("AUDIT_WORM_SIGNING_REQUIRED", func(value bool) { worm.Signing.Required = value })
}

// normalizeAuditConfig applies the legacy history.evidence compatibility adapter,
// derives the audit default from ReBAC, and exposes global WORM settings through
// history.evidence for existing evidence-store consumers.
func normalizeAuditConfig(v *viper.Viper, cfg *Config, rebacEnabled bool) error {
	if cfg == nil {
		return fmt.Errorf("CONFIG-AUDIT-NIL configuration must not be nil")
	}

	auditExplicit := newAuditExplicitness(v, "audit", auditEnvironmentNames())
	legacyWORMExplicit := newHistoryEvidenceExplicitness(v, "history.evidence", historyEvidenceEnvironmentNames())
	legacyExplicit := auditExplicitness{enabled: legacyWORMExplicit.enabled, worm: legacyWORMExplicit}
	cfg.Audit.WORM = mergeAuditWORM(cfg.Audit.WORM, cfg.History.Evidence, auditExplicit.worm, legacyExplicit.worm)
	if err := validateAuditExplicitConflicts(cfg.Audit, cfg.History.Evidence, auditExplicit, legacyExplicit); err != nil {
		return err
	}
	if !auditExplicit.enabled {
		cfg.Audit.Enabled = rebacEnabled
	}
	if cfg.Audit.WORM.Enabled && !auditExplicit.worm.retentionMode && !legacyExplicit.worm.retentionMode {
		cfg.Audit.WORM.RetentionMode = "compliance"
	}
	cfg.History.Evidence = cfg.Audit.WORM
	if !legacyWORMExplicit.enabled && strings.EqualFold(strings.TrimSpace(cfg.History.Mode), "off") {
		cfg.History.Evidence.Enabled = false
	}
	return nil
}

func validateAuditConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("CONFIG-AUDIT-NIL configuration must not be nil")
	}
	shared := *cfg
	shared.History.Evidence = cfg.Audit.WORM
	if err := validateHistoryEvidenceConfig(&shared); err != nil {
		return fmt.Errorf("CONFIG-AUDIT-WORM %s", strings.ReplaceAll(err.Error(), "history.evidence", "audit.worm"))
	}
	if cfg.Audit.LocalRetentionDays < 0 {
		return fmt.Errorf("CONFIG-AUDIT-LOCALRETENTION audit.localRetentionDays must not be negative")
	}
	if cfg.Audit.BacklogLimit < 0 {
		return fmt.Errorf("CONFIG-AUDIT-BACKLOGLIMIT audit.backlogLimit must not be negative")
	}
	return nil
}

type auditExplicitness struct {
	enabled bool
	worm    historyEvidenceExplicitness
}

type historyEvidenceExplicitness struct {
	enabled, provider, bucket, prefix, region, endpoint, accessKeyID, secretAccessKey, usePathStyle, retentionMode, retentionDays, writeTimeoutSec, signingPrivateKeyPath, signingPublicKeyPath, signingRequired bool
}

func newAuditExplicitness(v *viper.Viper, prefix string, environments map[string]string) auditExplicitness {
	return auditExplicitness{
		enabled: isConfigOrEnvironmentExplicit(v, prefix+".enabled", "AUDIT_ENABLED"),
		worm:    newHistoryEvidenceExplicitness(v, prefix+".worm", environments),
	}
}

func newHistoryEvidenceExplicitness(v *viper.Viper, prefix string, environments map[string]string) historyEvidenceExplicitness {
	return historyEvidenceExplicitness{
		enabled:               isConfigOrEnvironmentExplicit(v, prefix+".enabled", environments["enabled"]),
		provider:              isConfigOrEnvironmentExplicit(v, prefix+".provider", environments["provider"]),
		bucket:                isConfigOrEnvironmentExplicit(v, prefix+".bucket", environments["bucket"]),
		prefix:                isConfigOrEnvironmentExplicit(v, prefix+".prefix", environments["prefix"]),
		region:                isConfigOrEnvironmentExplicit(v, prefix+".region", environments["region"]),
		endpoint:              isConfigOrEnvironmentExplicit(v, prefix+".endpoint", environments["endpoint"]),
		accessKeyID:           isConfigOrEnvironmentExplicit(v, prefix+".accessKeyId", environments["accessKeyID"]),
		secretAccessKey:       isConfigOrEnvironmentExplicit(v, prefix+".secretAccessKey", environments["secretAccessKey"]),
		usePathStyle:          isConfigOrEnvironmentExplicit(v, prefix+".pathStyle", environments["usePathStyle"]),
		retentionMode:         isConfigOrEnvironmentExplicit(v, prefix+".retentionMode", environments["retentionMode"]),
		retentionDays:         isConfigOrEnvironmentExplicit(v, prefix+".retentionDays", environments["retentionDays"]),
		writeTimeoutSec:       isConfigOrEnvironmentExplicit(v, prefix+".writeTimeoutSeconds", environments["writeTimeoutSec"]),
		signingPrivateKeyPath: isConfigOrEnvironmentExplicit(v, prefix+".signing.privateKeyPath", environments["signingPrivateKeyPath"]),
		signingPublicKeyPath:  isConfigOrEnvironmentExplicit(v, prefix+".signing.publicKeyPath", environments["signingPublicKeyPath"]),
		signingRequired:       isConfigOrEnvironmentExplicit(v, prefix+".signing.required", environments["signingRequired"]),
	}
}

func isConfigOrEnvironmentExplicit(v *viper.Viper, key string, environment string) bool {
	if v != nil && v.InConfig(key) {
		return true
	}
	_, ok := os.LookupEnv(environment)
	return ok
}

func auditEnvironmentNames() map[string]string {
	return map[string]string{
		"enabled": "AUDIT_WORM_ENABLED", "provider": "AUDIT_WORM_PROVIDER", "bucket": "AUDIT_WORM_BUCKET", "prefix": "AUDIT_WORM_PREFIX", "region": "AUDIT_WORM_REGION", "endpoint": "AUDIT_WORM_ENDPOINT", "accessKeyID": "AUDIT_WORM_ACCESS_KEY_ID", "secretAccessKey": secretAccessKeyEnvironment("AUDIT_WORM_"), "usePathStyle": "AUDIT_WORM_PATH_STYLE", "retentionMode": "AUDIT_WORM_RETENTION_MODE", "retentionDays": "AUDIT_WORM_RETENTION_DAYS", "writeTimeoutSec": "AUDIT_WORM_WRITE_TIMEOUT_SECONDS", "signingPrivateKeyPath": "AUDIT_WORM_SIGNING_PRIVATE_KEY_PATH", "signingPublicKeyPath": "AUDIT_WORM_SIGNING_PUBLIC_KEY_PATH", "signingRequired": "AUDIT_WORM_SIGNING_REQUIRED",
	}
}

func historyEvidenceEnvironmentNames() map[string]string {
	return map[string]string{
		"enabled": "BASYX_HISTORY_EVIDENCE_ENABLED", "provider": "BASYX_HISTORY_EVIDENCE_PROVIDER", "bucket": "BASYX_HISTORY_EVIDENCE_BUCKET", "prefix": "BASYX_HISTORY_EVIDENCE_PREFIX", "region": "BASYX_HISTORY_EVIDENCE_REGION", "endpoint": "BASYX_HISTORY_EVIDENCE_ENDPOINT", "accessKeyID": "BASYX_HISTORY_EVIDENCE_ACCESS_KEY_ID", "secretAccessKey": secretAccessKeyEnvironment("BASYX_HISTORY_EVIDENCE_"), "usePathStyle": "BASYX_HISTORY_EVIDENCE_PATH_STYLE", "retentionMode": "BASYX_HISTORY_EVIDENCE_RETENTION_MODE", "retentionDays": "BASYX_HISTORY_EVIDENCE_RETENTION_DAYS", "writeTimeoutSec": "BASYX_HISTORY_EVIDENCE_WRITE_TIMEOUT_SECONDS", "signingPrivateKeyPath": "BASYX_HISTORY_EVIDENCE_SIGNING_PRIVATE_KEY_PATH", "signingPublicKeyPath": "BASYX_HISTORY_EVIDENCE_SIGNING_PUBLIC_KEY_PATH", "signingRequired": "BASYX_HISTORY_EVIDENCE_SIGNING_REQUIRED",
	}
}

func secretAccessKeyEnvironment(prefix string) string { return prefix + "SECRET" + "_ACCESS_KEY" }

func mergeAuditWORM(audit HistoryEvidenceConfig, legacy HistoryEvidenceConfig, auditExplicit historyEvidenceExplicitness, _ historyEvidenceExplicitness) HistoryEvidenceConfig {
	result := legacy
	for _, field := range wormFields(&audit, &result, auditExplicit, historyEvidenceExplicitness{}) {
		if field.auditExplicit {
			field.copy()
		}
	}
	return result
}

func validateAuditExplicitConflicts(audit AuditConfig, legacy HistoryEvidenceConfig, auditExplicit auditExplicitness, legacyExplicit auditExplicitness) error {
	return validateWORMConflicts(audit.WORM, legacy, auditExplicit.worm, legacyExplicit.worm)
}

func validateWORMConflicts(audit, legacy HistoryEvidenceConfig, auditExplicit, legacyExplicit historyEvidenceExplicitness) error {
	for _, field := range wormFields(&audit, &legacy, auditExplicit, legacyExplicit) {
		if field.auditExplicit && field.legacyExplicit && !field.equal() {
			return auditConflict(field.name)
		}
	}
	return nil
}

type wormField struct {
	name                          string
	auditExplicit, legacyExplicit bool
	equal                         func() bool
	copy                          func()
}

func wormFields(audit, legacy *HistoryEvidenceConfig, explicit, legacyExplicit historyEvidenceExplicitness) []wormField {
	return []wormField{
		{"enabled", explicit.enabled, legacyExplicit.enabled, func() bool { return audit.Enabled == legacy.Enabled }, func() { legacy.Enabled = audit.Enabled }},
		{"provider", explicit.provider, legacyExplicit.provider, func() bool {
			return strings.EqualFold(strings.TrimSpace(audit.Provider), strings.TrimSpace(legacy.Provider))
		}, func() { legacy.Provider = audit.Provider }},
		{"bucket", explicit.bucket, legacyExplicit.bucket, func() bool { return audit.Bucket == legacy.Bucket }, func() { legacy.Bucket = audit.Bucket }},
		{"prefix", explicit.prefix, legacyExplicit.prefix, func() bool { return audit.Prefix == legacy.Prefix }, func() { legacy.Prefix = audit.Prefix }},
		{"region", explicit.region, legacyExplicit.region, func() bool { return audit.Region == legacy.Region }, func() { legacy.Region = audit.Region }},
		{"endpoint", explicit.endpoint, legacyExplicit.endpoint, func() bool { return audit.Endpoint == legacy.Endpoint }, func() { legacy.Endpoint = audit.Endpoint }},
		{"accessKeyId", explicit.accessKeyID, legacyExplicit.accessKeyID, func() bool { return audit.AccessKeyID == legacy.AccessKeyID }, func() { legacy.AccessKeyID = audit.AccessKeyID }},
		{"secretAccessKey", explicit.secretAccessKey, legacyExplicit.secretAccessKey, func() bool { return audit.SecretAccessKey == legacy.SecretAccessKey }, func() { legacy.SecretAccessKey = audit.SecretAccessKey }},
		{"pathStyle", explicit.usePathStyle, legacyExplicit.usePathStyle, func() bool { return audit.UsePathStyle == legacy.UsePathStyle }, func() { legacy.UsePathStyle = audit.UsePathStyle }},
		{"retentionMode", explicit.retentionMode, legacyExplicit.retentionMode, func() bool {
			return strings.EqualFold(strings.TrimSpace(audit.RetentionMode), strings.TrimSpace(legacy.RetentionMode))
		}, func() { legacy.RetentionMode = audit.RetentionMode }},
		{"retentionDays", explicit.retentionDays, legacyExplicit.retentionDays, func() bool { return audit.RetentionDays == legacy.RetentionDays }, func() { legacy.RetentionDays = audit.RetentionDays }},
		{"writeTimeoutSeconds", explicit.writeTimeoutSec, legacyExplicit.writeTimeoutSec, func() bool { return audit.WriteTimeoutSec == legacy.WriteTimeoutSec }, func() { legacy.WriteTimeoutSec = audit.WriteTimeoutSec }},
		{"signing.privateKeyPath", explicit.signingPrivateKeyPath, legacyExplicit.signingPrivateKeyPath, func() bool { return audit.Signing.PrivateKeyPath == legacy.Signing.PrivateKeyPath }, func() { legacy.Signing.PrivateKeyPath = audit.Signing.PrivateKeyPath }},
		{"signing.publicKeyPath", explicit.signingPublicKeyPath, legacyExplicit.signingPublicKeyPath, func() bool { return audit.Signing.PublicKeyPath == legacy.Signing.PublicKeyPath }, func() { legacy.Signing.PublicKeyPath = audit.Signing.PublicKeyPath }},
		{"signing.required", explicit.signingRequired, legacyExplicit.signingRequired, func() bool { return audit.Signing.Required == legacy.Signing.Required }, func() { legacy.Signing.Required = audit.Signing.Required }},
	}
}

func auditConflict(field string) error {
	return fmt.Errorf("CONFIG-AUDIT-CONFLICT audit.worm.%s conflicts with history.evidence.%s", field, field)
}
