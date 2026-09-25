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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebac

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/go-chi/chi/v5"
)

const (
	accessSuffix          = "/$access"
	managementRoot        = "/security/rebac"
	managementComponent   = "REBAC"
	maxManagementBodySize = 1 << 20
	paramInvitation       = "invitationId"
	paramOperation        = "operationId"
	paramRepositoryKind   = "repositoryKind"
	paramObjectType       = "objectType"
	paramIdentifier       = "identifier"
)

// managementRoute is one route of the ReBAC management API.
type managementRoute struct {
	method  string
	pattern string
	handler http.HandlerFunc
}

// accessBase is the $access sub-resource of one resource kind.
type accessBase struct {
	pattern string
	target  func(r *http.Request) (accessTarget, bool)
	element bool
}

// BindABAC receives the active ABAC policy for effective-rights reports.
func (c *Coordinator) BindABAC(provider auth.AccessModelProvider, enableImplicitCasts bool) {
	c.abacProvider = provider
	c.abacImplicitCasts = enableImplicitCasts
}

// RegisterManagementRoutes mounts the ReBAC management API for the given
// resource kinds. The routes bypass ABAC route evaluation and authorize
// callers themselves: sharing requires can_manage or administrator status,
// never an ABAC data right. Denials answer 404 like missing resources.
func RegisterManagementRoutes(r chi.Router, runtime *Runtime, kinds ...ResourceKind) {
	if runtime == nil {
		return
	}
	for _, route := range runtime.Coordinator.managementRoutes(kinds) {
		runtime.Coordinator.registerManagementRoute(route.method, route.pattern)
		r.Method(route.method, route.pattern, route.handler)
	}
}

// ExemptManagementMutationRoutes classifies the management mutations for the
// history mutation guard; grant changes are not resource mutations.
func ExemptManagementMutationRoutes(guard *history.MutationCoverageGuard, runtime *Runtime, kinds ...ResourceKind) {
	if guard == nil || runtime == nil {
		return
	}
	for _, route := range runtime.Coordinator.managementRoutes(kinds) {
		if route.method != http.MethodGet {
			guard.Exempt(route.method, route.pattern)
		}
	}
}

func (c *Coordinator) managementRoutes(kinds []ResourceKind) []managementRoute {
	var routes []managementRoute
	for _, base := range c.accessBases(kinds) {
		routes = append(routes, c.accessRoutes(base)...)
	}
	return append(routes,
		managementRoute{http.MethodPost, managementRoot + "/invitations/accept", c.handleAcceptInvitation},
		managementRoute{http.MethodGet, managementRoot + "/status", c.handleStatus},
		managementRoute{http.MethodGet, managementRoot + "/operations/{" + paramOperation + "}", c.handleOperation},
		managementRoute{http.MethodGet, managementRoot + "/repositories/{" + paramRepositoryKind + "}" + accessSuffix, c.handleGetRepositoryAccess},
		managementRoute{http.MethodPut, managementRoot + "/repositories/{" + paramRepositoryKind + "}" + accessSuffix + "/grants", c.handlePutRepositoryGrants},
		managementRoute{http.MethodPost, managementRoot + "/admin/reconcile", c.handleReconcile},
		managementRoute{http.MethodPut, managementRoot + "/admin/owners/{" + paramObjectType + "}/{" + paramIdentifier + "}", c.handleRecoverOwners},
	)
}

func (c *Coordinator) accessBases(kinds []ResourceKind) []accessBase {
	var bases []accessBase
	for _, kind := range kinds {
		kind := kind
		param := identifierParam(kind)
		prefix := resourcePrefix(kind) + "/{" + param + "}"
		bases = append(bases, accessBase{pattern: prefix + accessSuffix, target: func(r *http.Request) (accessTarget, bool) {
			return c.resolveTarget(r, kind, param, "")
		}})
		if kind.ObjectType == TypeSubmodel {
			bases = append(bases, accessBase{
				pattern: prefix + "/submodel-elements/{" + paramPath + "}" + accessSuffix,
				element: true,
				target: func(r *http.Request) (accessTarget, bool) {
					return c.resolveTarget(r, kind, param, strings.TrimSpace(chi.URLParam(r, paramPath)))
				},
			})
		}
	}
	return bases
}

func resourcePrefix(kind ResourceKind) string {
	switch kind.ObjectType {
	case TypeAAS:
		return "/shells"
	case TypeConceptDescription:
		return "/concept-descriptions"
	default:
		return "/submodels"
	}
}

func (c *Coordinator) accessRoutes(base accessBase) []managementRoute {
	routes := []managementRoute{
		{http.MethodGet, base.pattern, c.withManagedTarget(base, c.handleGetAccess)},
		{http.MethodPut, base.pattern + "/grants", c.withManagedTarget(base, c.handlePutGrants)},
		{http.MethodGet, base.pattern + "/effective", c.withTarget(base, false, c.handleEffective)},
		{http.MethodGet, base.pattern + "/invitations", c.withManagedTarget(base, c.handleListInvitations)},
		{http.MethodPost, base.pattern + "/invitations", c.withManagedTarget(base, c.handleCreateInvitation)},
		{http.MethodDelete, base.pattern + "/invitations/{" + paramInvitation + "}", c.withManagedTarget(base, c.handleRevokeInvitation)},
	}
	if strings.HasPrefix(base.pattern, "/submodels/") && !base.element {
		routes = append(routes, managementRoute{http.MethodPut, base.pattern + "/inheritance", c.withManagedTarget(base, c.handlePutInheritance)})
	}
	return routes
}

// accessRequest is an authenticated management request on one target.
type accessRequest struct {
	principal Principal
	target    accessTarget
	admin     bool
}

type accessHandler func(w http.ResponseWriter, r *http.Request, request accessRequest)

// withManagedTarget serves a route that requires can_manage on the target.
func (c *Coordinator) withManagedTarget(base accessBase, next accessHandler) http.HandlerFunc {
	return c.withTarget(base, true, next)
}

// withTarget authenticates the caller, resolves the target and, when
// requireManage is set, authorizes can_manage. Every failure except
// unavailability looks like a missing resource.
func (c *Coordinator) withTarget(base accessBase, requireManage bool, next accessHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := c.managementPrincipal(w, r)
		if !ok {
			return
		}
		target, found := base.target(r)
		if !found {
			writeNotFound(w)
			return
		}
		request := accessRequest{principal: principal, target: target, admin: c.isAdministrator(principal)}
		if !requireManage {
			next(w, r, request)
			return
		}
		allowed, err := c.canManage(r.Context(), request)
		if err != nil {
			writeUnavailable(w, r, err)
			return
		}
		if !allowed {
			writeNotFound(w)
			return
		}
		next(w, r, request)
	}
}

// managementPrincipal returns the authenticated caller once ReBAC is ready.
func (c *Coordinator) managementPrincipal(w http.ResponseWriter, r *http.Request) (Principal, bool) {
	if !c.Ready() {
		writeUnavailable(w, r, ErrNotReady)
		return Principal{}, false
	}
	principal, ok := PrincipalFromClaims(auth.ClaimsFromContext(r.Context()), c.groupClaim)
	if !ok || !auth.IsAuthenticated(r.Context()) {
		writeNotFound(w)
		return Principal{}, false
	}
	return principal, true
}

func (c *Coordinator) resolveTarget(r *http.Request, kind ResourceKind, param string, elementPath string) (accessTarget, bool) {
	identifier, err := common.DecodeAPIIdentifier(chi.URLParam(r, param))
	if err != nil {
		return accessTarget{}, false
	}
	authUUID, found, err := LookupAuthUUID(r.Context(), c.db, kind, identifier)
	if err != nil || !found {
		return accessTarget{}, false
	}
	return accessTarget{kind: kind, identifier: identifier, authUUID: authUUID, elementPath: elementPath}, true
}

// isAdministrator reports configured bootstrap and recovery principals.
func (c *Coordinator) isAdministrator(principal Principal) bool {
	for _, administrator := range c.administrators {
		if administrator.Issuer != principal.Issuer {
			continue
		}
		if administrator.Subject != "" && administrator.Subject == principal.Subject {
			return true
		}
		for _, group := range principal.Groups {
			if administrator.Group != "" && administrator.Group == group {
				return true
			}
		}
	}
	return false
}

func (c *Coordinator) canManage(ctx context.Context, request accessRequest) (bool, error) {
	if request.admin {
		return true, nil
	}
	resolution := &resolution{coordinator: c, principal: request.principal}
	return resolution.checkResource(ctx, request.target.kind, RelationCanManage, request.target.objectKey(), request.target.structure())
}

// waitForProjection waits briefly for an operation to reach OpenFGA.
func (c *Coordinator) waitForProjection(ctx context.Context, operationID string) bool {
	if operationID == "" {
		return true
	}
	waitCtx, cancel := context.WithTimeout(ctx, managementWaitTimeout)
	defer cancel()
	applied, err := c.projector.WaitApplied(waitCtx, operationID)
	if err != nil {
		slog.WarnContext(ctx, "ReBAC operation not yet applied", "error.code", "REBAC-MANAGEMENT-WAITAPPLIED", "error", err)
	}
	return applied
}

const managementWaitTimeout = 5 * time.Second

// writeApplied answers 200 once an operation reached OpenFGA and 202 with an
// operation location otherwise.
func writeApplied(w http.ResponseWriter, r *http.Request, applied bool, operationID string, body any) {
	if applied {
		writeJSON(w, http.StatusOK, body)
		return
	}
	w.Header().Set("Location", managementLocation(r, "/operations/"+operationID))
	writeJSON(w, http.StatusAccepted, map[string]string{"operationId": operationID, "status": "pending"})
}

func managementLocation(r *http.Request, suffix string) string {
	path := r.URL.Path
	if index := strings.Index(path, managementRoot); index >= 0 {
		return path[:index] + managementRoot + suffix
	}
	base := strings.TrimSuffix(path, chi.RouteContext(r.Context()).RoutePattern())
	return strings.TrimSuffix(base, "/") + managementRoot + suffix
}

func decodeBody(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxManagementBodySize))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return common.NewErrBadRequest("REBAC-MANAGEMENT-DECODEBODY " + err.Error())
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

func writeNotFound(w http.ResponseWriter) {
	common.WriteRouterNotFound(w, managementComponent)
}

func writeUnavailable(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "ReBAC management unavailable", "error.code", "REBAC-MANAGEMENT-UNAVAILABLE", "error", err)
	_ = common.WriteErrorResponse(w, auth.ErrReBACUnavailable, http.StatusServiceUnavailable, managementComponent, "Management", "Unavailable")
}

func writeManagementError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case common.IsErrBadRequest(err):
		status = http.StatusBadRequest
	case common.IsErrConflict(err):
		status = http.StatusConflict
	case errors.Is(err, errPreconditionRequired):
		status = http.StatusPreconditionRequired
	case errors.Is(err, errPreconditionFailed):
		status = http.StatusPreconditionFailed
	case errors.Is(err, errTargetGone), common.IsErrNotFound(err):
		writeNotFound(w)
		return
	}
	if status == http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "ReBAC management failed", "error.code", "REBAC-MANAGEMENT-FAILED", "error", err)
	}
	_ = common.WriteErrorResponse(w, err, status, managementComponent, "Management", "Request")
}
