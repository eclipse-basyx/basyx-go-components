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
	"strings"
	"sync"
)

var (
	configMu     sync.RWMutex
	activeConfig = Config{
		Mode:                 ModeOff,
		RetentionDays:        0,
		FullSnapshotInterval: DefaultFullSnapshotInterval,
		Immutability:         ImmutabilityNone,
		AuditIdentityMode:    AuditIdentityNone,
	}
)

// Config controls the lightweight history/audit behavior.
type Config struct {
	Mode                 string
	RetentionDays        int
	FullSnapshotInterval int
	Immutability         string
	AuditIdentityMode    string
}

// Configure replaces the process-local history configuration.
func Configure(cfg Config) {
	configMu.Lock()
	defer configMu.Unlock()
	activeConfig = normalizeConfig(cfg)
}

// ActiveConfig returns the normalized process-local history configuration.
func ActiveConfig() Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return activeConfig
}

func normalizeConfig(cfg Config) Config {
	cfg.Mode = normalizeHistoryMode(cfg.Mode)
	cfg.Immutability = normalizeImmutability(cfg.Immutability)
	cfg.AuditIdentityMode = normalizeAuditIdentityMode(cfg.AuditIdentityMode)
	if cfg.RetentionDays < 0 {
		cfg.RetentionDays = 0
	}
	if cfg.FullSnapshotInterval < 1 {
		cfg.FullSnapshotInterval = DefaultFullSnapshotInterval
	}
	return cfg
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
