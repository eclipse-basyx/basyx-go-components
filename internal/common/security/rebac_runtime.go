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

package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/audit"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/evidence"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

type rebacSecurity struct {
	runtime  *rebac.Runtime
	audit    *audit.Runtime
	cfg      *common.Config
	settings ABACSettings
}
type rebacRequestKey struct{}
type rebacRequest struct {
	security       *rebacSecurity
	identity       ReBACIdentity
	actor          rebac.GrantActor
	route          ReBACRoute
	revision       int64
	correlation    string
	readGrants     *rebacReadGrantCache
	fallbackMethod string
	fallbackPath   string
}

// SetupReBACSecurity installs OIDC and the combined resource authorization runtime.
func SetupReBACSecurity(ctx context.Context, cfg *common.Config, r *chi.Mux, db *sql.DB, provider AccessModelProvider, claimsMiddleware ...func(http.Handler) http.Handler) (*rebac.Runtime, error) {
	oidc, err := setupOIDC(ctx, cfg)
	if err != nil {
		return nil, err
	}
	fingerprint, err := rebacConfigurationFingerprint(cfg)
	if err != nil {
		return nil, err
	}
	runtime, err := rebac.NewRuntime(ctx, db, cfg.ReBAC.Scope, fingerprint, rebac.Config{URL: cfg.ReBAC.URL, StoreID: cfg.ReBAC.StoreID, ModelID: cfg.ReBAC.ModelID, Token: cfg.ReBAC.Token, Timeout: time.Duration(cfg.ReBAC.TimeoutSeconds) * time.Second})
	if err != nil {
		return nil, err
	}
	auditRuntime, err := audit.NewRuntime(ctx, db, sharedAuditRuntimeConfig(cfg))
	if err != nil {
		return nil, err
	}
	if err = auditRuntime.Prepare(ctx); err != nil {
		return nil, err
	}
	auditCtx, cancelAudit := context.WithCancel(ctx)
	started := false
	defer func() {
		if !started {
			cancelAudit()
		}
	}()
	go auditRuntime.Run(auditCtx)
	settings := ABACSettings{Enabled: true, EnableImplicitCasts: cfg.General.EnableImplicitCasts, ModelProvider: provider, DenyAsNotFoundPrefixes: abacDeniedAsNotFoundPrefixes(cfg.Server.ContextPath)}
	security := &rebacSecurity{runtime: runtime, audit: auditRuntime, cfg: cfg, settings: settings}
	runtime.ValidateMutation = security.validateMutation
	runtime.AuditMutation = security.auditMutation
	runtime.AfterMutation = security.syncDerivedRelationships
	runtime.Creator = security.resourceCreator
	runtime.State.Projected = security.auditProjection
	runtime.State.RecoveryAudit = security.auditRecovery
	if err = runtime.State.Reconcile(ctx, runtime.Client); err != nil {
		return nil, err
	}
	activation, err := runtime.State.ActivateIntegrations(ctx, rebac.IntegrationState{AASRegistry: cfg.General.AASRegistryIntegration, SubmodelRegistry: cfg.General.SubmodelRegistryIntegration, Discovery: cfg.General.DiscoveryIntegration})
	if err != nil {
		return nil, err
	}
	defer func() {
		if !started {
			activation.Close()
		}
	}()
	if err = drainReBACProjection(ctx, runtime); err != nil {
		return nil, err
	}
	if err = runtime.Bootstrap(ctx); err != nil {
		return nil, err
	}
	if err = drainReBACProjection(ctx, runtime); err != nil {
		return nil, err
	}
	if err = security.recordModelActivation(ctx); err != nil {
		return nil, err
	}
	applySecurityMiddleware(r, oidc.Middleware, security.middleware, claimsMiddleware...)
	BindEventFeedAuthorizer(settings)
	go runtime.Run(ctx, func(err error) {
		slog.ErrorContext(ctx, "Authorization projection unavailable", "error.code", "REBAC-RUNTIME-PROJECTION", "error", err)
	})
	go security.observeOperationalMetrics(ctx)
	started = true
	return runtime, nil
}

func rebacConfigurationFingerprint(cfg *common.Config) (string, error) {
	connection := cfg.ReBAC
	connection.Token = ""
	data, err := json.Marshal(connection)
	if err != nil {
		return "", fmt.Errorf("REBAC-RUNTIME-FINGERPRINT: %w", err)
	}
	return rebac.IdentityDigest(string(data)), nil
}

func sharedAuditRuntimeConfig(cfg *common.Config) audit.RuntimeConfig {
	worm := cfg.Audit.WORM
	return audit.RuntimeConfig{Enabled: cfg.Audit.Enabled, WORM: worm.Enabled, BacklogLimit: cfg.Audit.BacklogLimit, SigningRequired: worm.Signing.Required, SigningPrivateKeyPath: worm.Signing.PrivateKeyPath, SigningPublicKeyPath: worm.Signing.PublicKeyPath, WriteTimeout: time.Duration(worm.WriteTimeoutSec) * time.Second,
		Store: evidence.S3EvidenceStoreConfig{Bucket: worm.Bucket, Prefix: worm.Prefix, Region: worm.Region, Endpoint: worm.Endpoint, AccessKeyID: worm.AccessKeyID, SecretAccessKey: worm.SecretAccessKey, UsePathStyle: worm.UsePathStyle, RetentionMode: worm.RetentionMode, RetentionDays: worm.RetentionDays}}
}

func (s *rebacSecurity) auditEvent(ctx context.Context, resource, action, source, outcome string) audit.Event {
	request, _ := ctx.Value(rebacRequestKey{}).(*rebacRequest)
	event := audit.Event{CorrelationID: uuid.NewString(), Resource: resource, Outcome: outcome, Actor: "anonymous", Payload: audit.Payload{Action: action, Details: map[string]string{"scope": s.cfg.ReBAC.Scope, "store_id": s.cfg.ReBAC.StoreID, "model_id": s.cfg.ReBAC.ModelID, "source": source}}}
	if request != nil {
		event.Actor = request.actor.User
		event.CorrelationID = request.correlation
		event.Payload.Details["revision"] = fmt.Sprint(request.revision)
		groups, _ := json.Marshal(request.identity.Groups)
		event.Payload.Details["groups"] = string(groups)
	}
	span := trace.SpanContextFromContext(ctx)
	if span.IsValid() {
		event.Payload.Details["trace_id"] = span.TraceID().String()
		event.Payload.Details["span_id"] = span.SpanID().String()
	}
	return event
}

func (s *rebacSecurity) recordDecision(ctx context.Context, resource, action, source, outcome string) error {
	if !s.audit.Enabled {
		return nil
	}
	event := s.auditEvent(ctx, resource, action, source, outcome)
	if tx, ok := ctx.Value(rebacWriterTransactionKey{}).(*sql.Tx); ok && tx != nil {
		rememberReBACMutationAudit(ctx, event)
		_, err := s.audit.Repository.Append(ctx, tx, "authorization/"+s.cfg.ReBAC.Scope, event)
		return err
	}
	_, err := s.audit.Repository.Record(ctx, "authorization/"+s.cfg.ReBAC.Scope, event)
	return err
}

func (s *rebacSecurity) auditMutation(ctx context.Context, tx *sql.Tx, kind, id string, revision int64) error {
	if !s.audit.Enabled {
		return nil
	}
	event := s.auditEvent(ctx, kind+":"+id, "mutation", "coordinator", "applied")
	event.Payload.Details["revision"] = fmt.Sprint(revision)
	rememberReBACMutationAudit(ctx, event)
	_, err := s.audit.Repository.Append(ctx, tx, "authorization/"+s.cfg.ReBAC.Scope, event)
	return err
}

func (s *rebacSecurity) validateMutation(ctx context.Context, tx *sql.Tx, mutation events.Mutation, kind string) error {
	request, ok := ctx.Value(rebacRequestKey{}).(*rebacRequest)
	if !ok {
		return fmt.Errorf("REBAC-MUTATION-CONTEXT missing initiating request")
	}
	if !request.identity.ExpiresAt.IsZero() && !time.Now().Before(request.identity.ExpiresAt) {
		return fmt.Errorf("REBAC-MUTATION-EXPIRED initiating credentials expired")
	}
	if request.route.Aggregate {
		return fmt.Errorf("REBAC-MUTATION-PREFLIGHT aggregate authorization has not been established")
	}
	if request.identity.Principal != nil {
		if err := s.runtime.State.SavePrincipal(ctx, tx, *request.identity.Principal); err != nil {
			return err
		}
	}
	derived, derivedErr := s.authorizeDerivedMutation(ctx, tx, kind, mutation.Identifier)
	if derivedErr != nil {
		return derivedErr
	}
	if derived {
		return nil
	}
	if kind == "aas_descriptor" {
		if err := s.validateEmbeddedMutation(ctx, tx, request, mutation.Identifier, mutation.Deleted); err != nil {
			return err
		}
		if request.route.Kind == ReBACRouteKindSubmodelDescriptor && request.route.Parent != nil && request.route.Parent.Identifier == mutation.Identifier {
			return nil
		}
	}
	return validateMutationTarget(request.route, mutation, kind)
}

func validateMutationTarget(route ReBACRoute, mutation events.Mutation, kind string) error {
	targetKind := string(route.Kind)
	if route.Kind == ReBACRouteKindElement {
		targetKind = "submodel"
	}
	if targetKind != kind {
		return fmt.Errorf("REBAC-MUTATION-TARGET unauthorized generated target")
	}
	targetID := route.Identifier
	if route.Kind == ReBACRouteKindElement && route.Parent != nil {
		targetID = route.Parent.Identifier
	}
	if targetID != "" && targetID != mutation.Identifier {
		return fmt.Errorf("REBAC-MUTATION-IDENTIFIER target differs from authorized resource")
	}
	return nil
}

func (s *rebacSecurity) relativePath(r *http.Request) string {
	path := r.URL.EscapedPath()
	base := strings.TrimRight(s.cfg.Server.ContextPath, "/")
	if base != "" && strings.HasPrefix(path, base+"/") {
		path = strings.TrimPrefix(path, base)
	}
	return path
}

func (s *rebacSecurity) auditProjection(ctx context.Context, tx *sql.Tx, change rebac.ProjectionChange) error {
	if !s.audit.Enabled {
		return nil
	}
	event := s.auditEvent(ctx, s.cfg.ReBAC.Scope, "projection_applied", "openfga", "applied")
	event.Actor = "system:rebac-projection"
	event.Payload.Details["revision"] = fmt.Sprint(change.Revision)
	_, err := s.audit.Repository.Append(ctx, tx, "authorization/"+s.cfg.ReBAC.Scope, event)
	return err
}

func drainReBACProjection(ctx context.Context, runtime *rebac.Runtime) error {
	for {
		worked, err := runtime.State.ProjectNext(ctx, runtime.Client)
		if err != nil {
			return err
		}
		if !worked {
			return nil
		}
	}
}

func (s *rebacSecurity) recordModelActivation(ctx context.Context) error {
	if !s.audit.Enabled {
		return nil
	}
	model, err := rebac.AuthorizationModelJSON()
	if err != nil {
		return err
	}
	digest := sha256.Sum256(model)
	event := s.auditEvent(ctx, s.cfg.ReBAC.ModelID, "model_activation", "provisioning", "active")
	event.Actor = "service:" + s.cfg.ReBAC.Scope
	event.Payload.Details["model_sha256"] = hex.EncodeToString(digest[:])
	_, err = s.audit.Repository.Record(ctx, "authorization/"+s.cfg.ReBAC.Scope, event)
	return err
}

func (s *rebacSecurity) auditRecovery(ctx context.Context, tx *sql.Tx, phase string, details map[string]string) error {
	if !s.audit.Enabled {
		return nil
	}
	event := s.auditEvent(ctx, "scope:"+s.cfg.ReBAC.Scope, "reconciliation", "coordinator", phase)
	event.Actor = "service:" + s.cfg.ReBAC.Scope
	event.CorrelationID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(s.cfg.ReBAC.Scope+":recovery:"+details["revision"])).String()
	for key, value := range details {
		event.Payload.Details[key] = value
	}
	_, err := s.audit.Repository.Append(ctx, tx, "authorization/"+s.cfg.ReBAC.Scope, event)
	return err
}
