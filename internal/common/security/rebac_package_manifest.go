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
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
)

// ReBACPackageManifest binds one persisted AASX body to resource generations.
type ReBACPackageManifest struct {
	ContentSHA256 string
	Members       []ReBACPackageManifestMember
}

// ReBACPackageManifestMember identifies one immutable included resource generation.
type ReBACPackageManifestMember struct {
	UUID       string
	Kind       string
	Identifier string
}

// ResolveReBACPackageManifestMutation binds parsed members to resource
// generations in the package transaction. Package-only members receive stable
// identities and creator ownership without publishing repository data.
func ResolveReBACPackageManifestMutation(ctx context.Context, tx *sql.Tx, manifest ReBACPackageManifest) (ReBACPackageManifest, error) {
	request, ok := ctx.Value(rebacRequestKey{}).(*rebacRequest)
	if !ok || request.security == nil || tx == nil {
		return manifest, common.NewErrServiceUnavailable("REBAC-PACKAGE-MANIFEST-CONTEXT validated authorization transaction is required")
	}
	if manifest.ContentSHA256 == "" || len(manifest.Members) == 0 {
		return manifest, common.NewErrBadRequest("REBAC-PACKAGE-MANIFEST-INVALID package manifest is incomplete")
	}
	creator := ""
	if request.identity.Principal != nil {
		creator = request.actor.User
	}
	writes := make([]rebac.Tuple, 0, len(manifest.Members))
	seen := make(map[string]struct{}, len(manifest.Members))
	for index := range manifest.Members {
		member := &manifest.Members[index]
		key := member.Kind + "\x00" + member.Identifier
		if member.Identifier == "" || !packageManifestKind(member.Kind) {
			return manifest, common.NewErrBadRequest("REBAC-PACKAGE-MANIFEST-MEMBER unsupported or empty package member")
		}
		if _, duplicate := seen[key]; duplicate {
			return manifest, common.NewErrBadRequest("REBAC-PACKAGE-MANIFEST-DUPLICATE duplicate package member")
		}
		seen[key] = struct{}{}
		resource, ownerTuples, err := request.security.runtime.State.EnsureResource(ctx, tx, member.Kind, member.Identifier, "", creator)
		if err != nil {
			return manifest, err
		}
		member.UUID = resource.UUID
		writes = append(writes, ownerTuples...)
	}
	if len(writes) > 0 {
		if _, err := request.security.runtime.State.QueueChanged(ctx, tx, writes, nil); err != nil {
			return manifest, err
		}
	}
	return manifest, nil
}

func packageManifestKind(kind string) bool {
	return kind == "aas" || kind == "submodel" || kind == "concept_description"
}

// AuthorizeReBACPackageManifest requires access to every archived resource generation.
func AuthorizeReBACPackageManifest(ctx context.Context, manifest ReBACPackageManifest) error {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return nil
	}
	request, ok := ctx.Value(rebacRequestKey{}).(*rebacRequest)
	if !ok || request.security == nil {
		return common.NewErrServiceUnavailable("REBAC-PACKAGE-READ-CONTEXT validated authorization context is required")
	}
	if manifest.ContentSHA256 == "" || len(manifest.Members) == 0 {
		return common.NewErrServiceUnavailable("REBAC-PACKAGE-READ-MANIFEST package manifest is unavailable")
	}
	fallbackAllowed := rebacPackageUnconditionalABAC(ctx, request)
	return withReBACPackageState(ctx, request, func(tx *sql.Tx) error {
		for _, member := range manifest.Members {
			if err := authorizePackageMember(ctx, tx, request, member, fallbackAllowed); err != nil {
				return err
			}
		}
		return nil
	})
}

func authorizePackageMember(ctx context.Context, tx *sql.Tx, request *rebacRequest, member ReBACPackageManifestMember, fallbackAllowed bool) error {
	resource, err := request.security.runtime.State.Resource(ctx, tx, member.Kind, member.Identifier)
	if errors.Is(err, sql.ErrNoRows) {
		return common.NewErrDenied("REBAC-PACKAGE-READ-STALEMEMBER package member no longer exists")
	}
	if err != nil {
		return common.NewErrServiceUnavailable("REBAC-PACKAGE-READ-RESOURCE " + err.Error())
	}
	if resource.UUID != member.UUID {
		return common.NewErrDenied("REBAC-PACKAGE-READ-STALEMEMBER package member no longer has its recorded identity")
	}
	object, err := rebac.ResourceObject(request.security.cfg.ReBAC.Scope, member.Kind, member.UUID)
	if err != nil {
		return err
	}
	coordinator := rebac.Coordinator{Checker: request.security.runtime.Client, Record: packageDecisionRecorder(request.security, member)}
	decision, err := coordinator.Authorize(ctx, rebac.AccessRequest{User: request.actor.User, Relation: "read", Object: object, ContextualTuples: request.actor.Groups}, func(context.Context) (rebac.Decision, error) { return rebac.Decision{Allowed: fallbackAllowed}, nil })
	if err != nil {
		return common.NewErrServiceUnavailable("REBAC-PACKAGE-READ-CHECK " + err.Error())
	}
	if !decision.Allowed {
		return common.NewErrDenied("REBAC-PACKAGE-READ-DENIED package member access denied")
	}
	return nil
}

func withReBACPackageState(ctx context.Context, request *rebacRequest, fn func(*sql.Tx) error) error {
	check := func(tx *sql.Tx) error {
		state, err := request.security.runtime.State.Lock(ctx, tx, false)
		if err != nil {
			return err
		}
		if state.Applied != request.revision {
			return rebac.ErrStaleRevision
		}
		return fn(tx)
	}
	if tx, ok := ctx.Value(rebacReadTransactionKey{}).(*sql.Tx); ok && tx != nil {
		return check(tx)
	}
	return request.security.runtime.State.WithApplied(ctx, check)
}

func packageDecisionRecorder(security *rebacSecurity, member ReBACPackageManifestMember) rebac.DecisionRecorder {
	return security.resourceDecisionRecorder(member.Kind+":"+member.Identifier, "read")
}

// ValidateReBACPackageManifestMutation rechecks members in the package writer transaction.
func ValidateReBACPackageManifestMutation(ctx context.Context, tx *sql.Tx, manifest ReBACPackageManifest) error {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return nil
	}
	request, ok := ctx.Value(rebacRequestKey{}).(*rebacRequest)
	if !ok || request.security == nil || tx == nil {
		return common.NewErrServiceUnavailable("REBAC-PACKAGE-MANIFEST-WRITER validated transaction is required")
	}
	if err := request.security.runtime.State.CheckMutationRevision(ctx, tx, request.revision); err != nil {
		return common.NewErrServiceUnavailable("REBAC-PACKAGE-MANIFEST-REVISION " + err.Error())
	}
	for _, member := range manifest.Members {
		if err := request.security.runtime.State.RequireLiveResource(ctx, tx, rebac.StoredResource{UUID: member.UUID, Kind: member.Kind, Identifier: member.Identifier}); err != nil {
			return common.NewErrDenied("REBAC-PACKAGE-MANIFEST-STALEMEMBER source resource changed")
		}
	}
	return nil
}

func rebacPackageUnconditionalABAC(ctx context.Context, request *rebacRequest) bool {
	model := activeAccessModel(request.security.settings)
	if model == nil {
		return false
	}
	opts := grammar.DefaultSimplifyOptions()
	opts.EnableImplicitCasts = request.security.settings.EnableImplicitCasts
	session := newAuthorizationSession(model, ClaimsFromContext(ctx), nil, opts)
	path, err := rebacCanonicalPath("package", request.route.Identifier)
	if err != nil {
		return false
	}
	path = strings.TrimRight(common.NormalizeBasePath(request.security.cfg.Server.ContextPath), "/") + path
	evaluation := session.evaluate(http.MethodGet, path)
	return evaluation.Allowed && evaluation.QueryFilter == nil
}

// AuthorizeReBACPackageCreation checks the repository creator permission for asynchronous ingestion.
func AuthorizeReBACPackageCreation(ctx context.Context) (context.Context, error) {
	return authorizeReBACPath(ctx, "/packages", http.MethodPost)
}
