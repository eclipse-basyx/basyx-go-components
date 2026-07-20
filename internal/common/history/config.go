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

package history

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

var (
	configMu     sync.RWMutex
	activeConfig = Config{
		Mode:                 ModeOff,
		RetentionDays:        0,
		FullSnapshotInterval: DefaultFullSnapshotInterval,
		Immutability:         ImmutabilityNone,
		AuditIdentityMode:    AuditIdentityNone,
		EvidenceProvider:     EvidenceProviderNone,
		EvidenceWriteTimeout: DefaultEvidenceWriteTimeout,
	}
)

// DefaultEvidenceWriteTimeout bounds synchronous evidence writes inside database transactions.
const DefaultEvidenceWriteTimeout = 10 * time.Second

// Config controls process-local history and audit behavior.
//
// Services populate this from their common configuration during startup. Mode
// decides whether history writes are skipped, stored for API history, or stored
// for audit use. FullSnapshotInterval controls how many rows can be restored
// from one checkpoint before a new full snapshot is forced.
type Config struct {
	Mode                 string
	RetentionDays        int
	FullSnapshotInterval int
	Immutability         string
	AuditIdentityMode    string
	EvidenceEnabled      bool
	EvidenceProvider     string
	EvidenceStore        EvidenceStore
	EvidenceWriteTimeout time.Duration
}

// Configure replaces the process-local history configuration.
//
// Values are normalized before they become active: empty or unknown modes fall
// back to a supported mode, negative retention is disabled, and snapshot
// intervals below one use DefaultFullSnapshotInterval.
//
// Parameters:
//   - cfg: Desired history configuration for the current service process.
//
// Example:
//
//	Configure(Config{
//		Mode:                 ModeAPI,
//		FullSnapshotInterval: 3,
//		Immutability:         ImmutabilityNone,
//		AuditIdentityMode:    AuditIdentityNone,
//	})
func Configure(cfg Config) {
	configMu.Lock()
	defer configMu.Unlock()
	activeConfig = normalizeConfig(cfg)
}

// ConfigureEvidence creates and activates the configured WORM evidence store.
//
// The store is used synchronously by history append operations while the
// PostgreSQL transaction is still open. A positive write timeout is therefore
// required so object-store stalls cannot hold history advisory locks forever.
//
// Parameters:
//   - ctx: Startup context used while initializing provider clients.
//   - cfg: Common history evidence configuration loaded from YAML and environment.
//
// Returns:
//   - error: Error when evidence is enabled with an unsupported provider,
//     incomplete S3 settings, or an invalid write timeout.
func ConfigureEvidence(ctx context.Context, cfg common.HistoryEvidenceConfig) error {
	provider := normalizeEvidenceProvider(cfg.Provider)
	if !cfg.Enabled {
		configureEvidenceStore(false, EvidenceProviderNone, nil, DefaultEvidenceWriteTimeout)
		return nil
	}
	if provider == EvidenceProviderNone {
		return fmt.Errorf("HISTORY-EVIDENCE-CONFIG-PROVIDER history.evidence.enabled requires an evidence provider")
	}
	if provider != EvidenceProviderS3 {
		return fmt.Errorf("HISTORY-EVIDENCE-CONFIG-PROVIDER unsupported evidence provider %q", cfg.Provider)
	}
	writeTimeout := time.Duration(cfg.WriteTimeoutSec) * time.Second
	if writeTimeout < time.Second {
		return fmt.Errorf("HISTORY-EVIDENCE-CONFIG-TIMEOUT history.evidence.writeTimeoutSeconds must be at least 1")
	}
	store, err := NewS3EvidenceStore(ctx, S3EvidenceStoreConfig{
		Bucket:          cfg.Bucket,
		Prefix:          cfg.Prefix,
		Region:          cfg.Region,
		Endpoint:        cfg.Endpoint,
		AccessKeyID:     cfg.AccessKeyID,
		SecretAccessKey: cfg.SecretAccessKey,
		UsePathStyle:    cfg.UsePathStyle,
		RetentionMode:   cfg.RetentionMode,
		RetentionDays:   cfg.RetentionDays,
	})
	if err != nil {
		return fmt.Errorf("HISTORY-EVIDENCE-CONFIG-STORE %w", err)
	}
	configureEvidenceStore(true, provider, store, writeTimeout)
	return nil
}

// ActiveConfig returns the normalized process-local history configuration.
//
// The returned value is a copy and can be read safely while other goroutines
// call Configure during service initialization or tests.
//
// Returns:
//   - Config: Current normalized process-local configuration.
//
// Example:
//
//	cfg := ActiveConfig()
//	if cfg.Mode == ModeOff {
//		return nil
//	}
func ActiveConfig() Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return activeConfig
}

// MutationRecordingEnabled reports whether PostgreSQL history or independent
// WORM mutation evidence must be recorded for acknowledged model changes.
func MutationRecordingEnabled() bool {
	cfg := ActiveConfig()
	return cfg.Mode != ModeOff || cfg.EvidenceEnabled
}

func normalizeConfig(cfg Config) Config {
	cfg.Mode = normalizeHistoryMode(cfg.Mode)
	cfg.Immutability = normalizeImmutability(cfg.Immutability)
	cfg.AuditIdentityMode = normalizeAuditIdentityMode(cfg.AuditIdentityMode)
	cfg.EvidenceProvider = normalizeEvidenceProvider(cfg.EvidenceProvider)
	if cfg.RetentionDays < 0 {
		cfg.RetentionDays = 0
	}
	if cfg.FullSnapshotInterval < 1 {
		cfg.FullSnapshotInterval = DefaultFullSnapshotInterval
	}
	if cfg.EvidenceWriteTimeout < time.Second {
		cfg.EvidenceWriteTimeout = DefaultEvidenceWriteTimeout
	}
	if !cfg.EvidenceEnabled {
		cfg.EvidenceProvider = EvidenceProviderNone
		cfg.EvidenceStore = nil
	}
	return cfg
}

func configureEvidenceStore(enabled bool, provider string, store EvidenceStore, writeTimeout time.Duration) {
	configMu.Lock()
	defer configMu.Unlock()
	activeConfig.EvidenceEnabled = enabled
	activeConfig.EvidenceProvider = normalizeEvidenceProvider(provider)
	activeConfig.EvidenceStore = store
	if writeTimeout < time.Second {
		writeTimeout = DefaultEvidenceWriteTimeout
	}
	activeConfig.EvidenceWriteTimeout = writeTimeout
	if !enabled {
		activeConfig.EvidenceProvider = EvidenceProviderNone
		activeConfig.EvidenceStore = nil
	}
}

func normalizeHistoryMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ModeOff:
		return ModeOff
	case ModeAPI:
		return ModeAPI
	case ModeAudit:
		return ModeAudit
	default:
		return ModeAPI
	}
}

func normalizeImmutability(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ImmutabilityNone:
		return ImmutabilityNone
	case ImmutabilityPostgresGuarded:
		return ImmutabilityPostgresGuarded
	case ImmutabilityExternalAnchor:
		return ImmutabilityExternalAnchor
	default:
		return ImmutabilityNone
	}
}

func normalizeAuditIdentityMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", AuditIdentityNone:
		return AuditIdentityNone
	case AuditIdentityMinimal:
		return AuditIdentityMinimal
	case AuditIdentityExtended:
		return AuditIdentityExtended
	default:
		return AuditIdentityMinimal
	}
}

func normalizeEvidenceProvider(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" {
		return EvidenceProviderNone
	}
	return normalized
}
