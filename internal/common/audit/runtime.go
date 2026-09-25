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

package audit

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/evidence"
	commonjws "github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	jose "gopkg.in/go-jose/go-jose.v2"
)

// RuntimeConfig contains the shared audit settings resolved by configuration loading.
type RuntimeConfig struct {
	Enabled, WORM, SigningRequired              bool
	BacklogLimit                                int
	Store                                       evidence.S3EvidenceStoreConfig
	SigningPrivateKeyPath, SigningPublicKeyPath string
	WriteTimeout                                time.Duration
}

// Runtime owns shared audit persistence and the cancellable archival worker.
type Runtime struct {
	Repository       Repository
	Enabled          bool
	archiver         *Archiver
	timeout          time.Duration
	mu               sync.RWMutex
	degraded         bool
	verificationKey  *rsa.PublicKey
	verificationKeys map[string]*rsa.PublicKey
	signingRequired  bool
}

// NewRuntime prepares audit persistence and configured signing without enabling history.
func NewRuntime(ctx context.Context, db *sql.DB, cfg RuntimeConfig) (*Runtime, error) {
	runtime := &Runtime{Enabled: cfg.Enabled, timeout: cfg.WriteTimeout}
	if !cfg.Enabled {
		return runtime, nil
	}
	if db == nil || cfg.BacklogLimit < 0 {
		return nil, fmt.Errorf("AUDIT-RUNTIME-CONFIG database and nonnegative backlog limit are required")
	}
	runtime.Repository = Repository{DB: db, WORM: cfg.WORM, BacklogLimit: cfg.BacklogLimit}
	if !cfg.WORM {
		return runtime, nil
	}
	if cfg.Store.RetentionDays <= 0 {
		return nil, fmt.Errorf("AUDIT-RUNTIME-RETENTION explicit positive retention is required")
	}
	if runtime.timeout <= 0 {
		return nil, fmt.Errorf("AUDIT-RUNTIME-TIMEOUT positive write timeout is required")
	}
	if cfg.Store.RetentionMode == "" {
		cfg.Store.RetentionMode = "compliance"
	}
	signer, err := configuredSigner(cfg)
	if err != nil {
		return nil, err
	}
	store, err := evidence.NewS3EvidenceStore(ctx, cfg.Store)
	if err != nil {
		return nil, fmt.Errorf("AUDIT-RUNTIME-STORE: %w", err)
	}
	if err = store.Validate(ctx); err != nil {
		return nil, err
	}
	runtime.verificationKey, err = configuredVerificationKey(cfg, signer)
	if err != nil {
		return nil, err
	}
	runtime.verificationKeys, err = retainedVerificationKeys(cfg.SigningPublicKeyPath, runtime.verificationKey)
	if err != nil {
		return nil, err
	}
	runtime.signingRequired = cfg.SigningRequired
	runtime.archiver = &Archiver{DB: db, Store: store, Signer: signer}
	return runtime, nil
}

// Run delivers queued evidence until cancellation, retaining pending work on failure.
func (runtime *Runtime) Run(ctx context.Context) {
	if runtime.archiver == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		completed, err := runtime.archiveNext(ctx)
		runtime.mu.Lock()
		runtime.degraded = err != nil
		runtime.mu.Unlock()
		if completed && err == nil {
			continue
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (runtime *Runtime) archiveNext(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, runtime.timeout)
	defer cancel()
	ctx, span := otel.Tracer("basyx/audit").Start(ctx, "audit.archive")
	defer span.End()
	started := time.Now()
	completed, err := runtime.archiver.ArchiveNext(ctx)
	outcome := "idle"
	if completed {
		outcome = "archived"
	}
	if err != nil {
		outcome = "failed"
	}
	meter := otel.Meter("basyx/audit")
	options := metric.WithAttributes(attribute.String("outcome", outcome))
	if counter, metricErr := meter.Int64Counter("basyx.audit.worm.deliveries"); metricErr == nil {
		counter.Add(ctx, 1, options)
	}
	if duration, metricErr := meter.Float64Histogram("basyx.audit.worm.delivery_latency", metric.WithUnit("s")); metricErr == nil {
		duration.Record(ctx, time.Since(started).Seconds(), options)
	}
	if err != nil {
		span.RecordError(err)
	}
	return completed, err
}

// Degraded reports whether the most recent archival attempt failed.
func (runtime *Runtime) Degraded() bool {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return runtime.degraded
}

type rsaManifestSigner struct {
	key   *rsa.PrivateKey
	keyID string
}

func configuredSigner(cfg RuntimeConfig) (ManifestSigner, error) {
	if cfg.SigningPrivateKeyPath == "" {
		if cfg.SigningRequired {
			return nil, fmt.Errorf("AUDIT-SIGNER-REQUIRED private signing key is required")
		}
		return nil, nil
	}
	key, err := commonjws.LoadPrivateKey(cfg.SigningPrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("AUDIT-SIGNER-LOAD: %w", err)
	}
	if err = validateSigningPublicKey(key, cfg.SigningPublicKeyPath); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	return rsaManifestSigner{key: key, keyID: hex.EncodeToString(digest[:])}, nil
}

func validateSigningPublicKey(key *rsa.PrivateKey, path string) error {
	if key.N.BitLen() < 2048 {
		return fmt.Errorf("AUDIT-SIGNER-KEYSIZE RSA key must have at least 2048 bits")
	}
	if path == "" {
		return nil
	}
	public, err := commonjws.LoadPublicKey(path)
	if err != nil {
		return fmt.Errorf("AUDIT-SIGNER-PUBLICKEY: %w", err)
	}
	if !public.Equal(&key.PublicKey) {
		return fmt.Errorf("AUDIT-SIGNER-MISMATCH signing and verification keys differ")
	}
	return nil
}

func (signer rsaManifestSigner) SignManifest(ctx context.Context, envelope []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	options := (&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), signer.keyID)
	jws, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: signer.key}, options)
	if err != nil {
		return nil, fmt.Errorf("AUDIT-SIGNER-CREATE: %w", err)
	}
	signed, err := jws.Sign(envelope)
	if err != nil {
		return nil, fmt.Errorf("AUDIT-SIGNER-SIGN: %w", err)
	}
	compact, err := signed.CompactSerialize()
	if err != nil {
		return nil, fmt.Errorf("AUDIT-SIGNER-SERIALIZE: %w", err)
	}
	return []byte(compact), nil
}

// Prepare drains a saturated durable queue before startup itself produces audit records.
func (runtime *Runtime) Prepare(ctx context.Context) error {
	if runtime.archiver == nil {
		return nil
	}
	query, args, err := dialect.From("audit_delivery").Select(goqu.COUNT("*")).Where(goqu.C("archived_at").IsNull()).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("AUDIT-PREPARE-QUERY: %w", err)
	}
	for {
		var pending int
		if err := runtime.Repository.DB.QueryRowContext(ctx, query, args...).Scan(&pending); err != nil {
			return fmt.Errorf("AUDIT-PREPARE-BACKLOG: %w", err)
		}
		if pending < runtime.Repository.BacklogLimit {
			return nil
		}
		completed, err := runtime.archiveNext(ctx)
		if err != nil {
			return err
		}
		if !completed {
			return fmt.Errorf("AUDIT-PREPARE-BUSY durable backlog is being processed")
		}
	}
}
