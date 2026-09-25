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
	"fmt"
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
	"github.com/google/uuid"
)

func (s *rebacSecurity) middleware(next http.Handler) http.Handler {
	abacOnly := ABACMiddleware(s.settings)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := s.relativePath(r)
		route, err := ClassifyReBACRoute(r.Method, path)
		if route.ABACOnly || errors.Is(err, ErrReBACRouteExcluded) || rebacAdministrativeABACPath(path) {
			abacOnly.ServeHTTP(w, r)
			return
		}
		if err != nil {
			writeReBACError(w, err, http.StatusNotFound)
			return
		}
		identity, err := ReBACIdentityFromContext(r.Context(), s.cfg.ReBAC)
		if err != nil {
			writeReBACError(w, err, http.StatusUnauthorized)
			return
		}
		actor, err := rebacActor(identity)
		if err != nil {
			writeReBACError(w, err, http.StatusUnauthorized)
			return
		}

		request := &rebacRequest{security: s, identity: identity, actor: actor, route: route, correlation: uuid.NewString(), readGrants: &rebacReadGrantCache{}}
		r = r.WithContext(context.WithValue(r.Context(), rebacRequestKey{}, request))
		s.dispatchReBACRequest(w, r, next, request, path)
	})
}

func rebacAdministrativeABACPath(path string) bool {
	return path == "/description" || path == "/health" || path == "/health/ready" || path == "/verify" || strings.HasPrefix(path, "/security/abac/")
}

func (s *rebacSecurity) serveAuthorized(w http.ResponseWriter, r *http.Request, next http.Handler, request *rebacRequest) {
	var authorized *http.Request
	err := s.runtime.State.WithApplied(r.Context(), func(tx *sql.Tx) error {
		state, err := s.runtime.State.Lock(r.Context(), tx, false)
		if err != nil {
			return err
		}
		request.revision = state.Applied
		authorized, err = s.authorizeRequest(r, tx, request)
		if err != nil {
			return err
		}
		if rebacRequestReads(r, request.route) {
			authorized = authorized.WithContext(context.WithValue(authorized.Context(), rebacReadTransactionKey{}, tx))
			next.ServeHTTP(w, authorized)
		}
		return nil
	})
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, rebac.ErrForbidden) {
			status = http.StatusForbidden
		}
		writeReBACError(w, err, status)
		return
	}
	if !rebacRequestReads(r, request.route) {
		user := ""
		if request.identity.Principal != nil {
			user = request.actor.User
		}
		ctx := rebac.ContextWithLifecycleActor(authorized.Context(), rebac.LifecycleActor{User: user, Revision: request.revision})
		next.ServeHTTP(w, authorized.WithContext(ctx))
	}
}

func (s *rebacSecurity) authorizeRequest(r *http.Request, tx *sql.Tx, request *rebacRequest) (*http.Request, error) {
	model := activeAccessModel(s.settings)
	if model == nil {
		return nil, rebac.ErrForbidden
	}
	opts := grammar.DefaultSimplifyOptions()
	opts.EnableImplicitCasts = s.settings.EnableImplicitCasts
	session := newAuthorizationSession(model, ClaimsFromContext(r.Context()), nil, opts)
	method, path := r.Method, r.URL.Path
	if request.fallbackPath != "" {
		method, path = request.fallbackMethod, request.fallbackPath
	}
	evaluation := session.evaluate(method, path)
	if request.route.Collection && request.route.Action == ReBACRouteActionRead {
		return s.authorizeCollection(r, tx, request, session, evaluation)
	}
	resource, relation, err := s.resolveTarget(r.Context(), tx, request.route)
	if err != nil {
		return nil, err
	}
	object, err := rebac.ResourceObject(s.cfg.ReBAC.Scope, resource.Kind, resource.UUID)
	if err != nil {
		return nil, err
	}
	coordinator := rebac.Coordinator{Checker: s.runtime.Client, Record: s.resourceDecisionRecorder(resource.Kind+":"+resource.Identifier, relation)}
	decision, err := coordinator.Authorize(r.Context(), rebac.AccessRequest{User: request.actor.User, Relation: relation, Object: object, ContextualTuples: request.actor.Groups}, func(context.Context) (rebac.Decision, error) {
		return rebac.Decision{Allowed: evaluation.Allowed, Partial: evaluation.QueryFilter != nil}, nil
	})
	if err != nil {
		return nil, err
	}
	if !decision.Allowed {
		return nil, rebac.ErrForbidden
	}
	if decision.Source == "rebac" {
		evaluation.QueryFilter = nil
		evaluation.Allowed = true
		evaluation.alternatives = nil
	}
	if request.route.Action == ReBACRouteActionRead {
		session.rebacRead, err = s.requestReadGrantSets(r.Context(), tx, request)
		if err != nil {
			return nil, err
		}
	}
	return withReBACEvaluation(r, session, evaluation), nil
}

func withReBACEvaluation(r *http.Request, session *AuthorizationSession, evaluation AuthorizationEvaluation) *http.Request {
	ctx := ContextWithAuthorizationDecision(r.Context(), AuthorizationDecision{Result: string(DecisionAllow), PolicyID: evaluation.PolicyID, MatchedRuleID: evaluation.MatchedRuleID})
	ctx = context.WithValue(ctx, filterKey, evaluation.QueryFilter)
	session = session.withOuterAccess(accessViewFromEvaluation("", evaluation))
	ctx = context.WithValue(ctx, authorizationSessionContextKey{}, session)
	return r.WithContext(ctx)
}

func (s *rebacSecurity) resolveTarget(ctx context.Context, tx *sql.Tx, route ReBACRoute) (rebac.StoredResource, string, error) {
	if route.Kind == ReBACRouteKindSubmodelDescriptor && route.Parent != nil && route.Collection {
		return s.resolveEmbeddedCreation(ctx, tx, route)
	}
	kind, id, relation, err := rebacTargetIdentity(route)
	if err != nil {
		return rebac.StoredResource{}, "", err
	}
	resource, err := s.runtime.State.Resource(ctx, tx, kind, id)
	if errors.Is(err, sql.ErrNoRows) && route.Kind == ReBACRouteKindSubmodelDescriptor && route.Parent != nil && route.Action == ReBACRouteActionUpdate {
		return s.resolveEmbeddedCreation(ctx, tx, route)
	}
	if errors.Is(err, sql.ErrNoRows) && route.Action == ReBACRouteActionUpdate && route.Kind != ReBACRouteKindElement {
		resource, err = s.runtime.State.Resource(ctx, tx, "repository", string(route.Kind))
		relation = "create"
	}
	return resource, relation, err
}

func writeReBACError(w http.ResponseWriter, err error, status int) {
	_ = common.WriteErrorResponse(w, err, status, "ReBAC", "Authorization", "Request")
}

func rebacActor(identity ReBACIdentity) (rebac.GrantActor, error) {
	actor := rebac.GrantActor{User: "user:anonymous", Groups: identity.ContextualTuples, Administrator: identity.Administrator}
	if identity.Principal != nil {
		user, err := identity.Principal.Object()
		if err != nil {
			return actor, err
		}
		actor.User = user
	}
	return actor, nil
}

func (s *rebacSecurity) dispatchReBACRequest(w http.ResponseWriter, r *http.Request, next http.Handler, request *rebacRequest, path string) {
	if request.route.Management != "" {
		s.serveManagement(w, r, request)
		return
	}
	if (request.route.Kind == ReBACRouteKindPackage && request.route.Collection && request.route.Action == ReBACRouteActionRead) || request.route.Kind == ReBACRouteKindBulk || (request.route.Kind == ReBACRouteKindImport && (path == "/upload" || strings.HasPrefix(path, "/packages-async"))) || path == "/serialization" || request.route.Kind == ReBACRouteKindDPP {
		s.serveResourceAggregate(w, r, next, request)
		return
	}
	if request.route.Aggregate && request.route.Kind != ReBACRouteKindShell && request.route.Kind != ReBACRouteKindSubmodel && request.route.Kind != ReBACRouteKindConceptDescription && request.route.Kind != ReBACRouteKindShellDescriptor && request.route.Kind != ReBACRouteKindSubmodelDescriptor && request.route.Kind != ReBACRouteKindDiscovery && request.route.Kind != ReBACRouteKindElement {
		writeReBACError(w, fmt.Errorf("REBAC-REQUEST-AGGREGATE aggregate preflight is unavailable"), http.StatusServiceUnavailable)
		return
	}
	s.serveAuthorized(w, r, next, request)
}

func rebacRequestReads(r *http.Request, route ReBACRoute) bool {
	return r.Method == http.MethodGet || r.Method == http.MethodHead || route.Action == ReBACRouteActionRead
}

func (s *rebacSecurity) resolveEmbeddedCreation(ctx context.Context, tx *sql.Tx, route ReBACRoute) (rebac.StoredResource, string, error) {
	resource, err := s.runtime.State.Resource(ctx, tx, "aas_descriptor", route.Parent.Identifier)
	return resource, "create_child", err
}

func (s *rebacSecurity) resourceDecisionRecorder(resource, action string) rebac.DecisionRecorder {
	return func(ctx context.Context, _ rebac.AccessRequest, decision rebac.Decision, decisionErr error) error {
		outcome := "denied"
		if decision.Allowed {
			outcome = "allowed"
		}
		if decisionErr != nil {
			outcome = "unavailable"
		}
		return s.recordDecision(ctx, resource, action, decision.Source, outcome)
	}
}

func rebacTargetIdentity(route ReBACRoute) (string, string, string, error) {
	kind, id, relation := string(route.Kind), route.Identifier, string(route.Action)
	if route.Kind == ReBACRouteKindElement {
		if route.Parent == nil {
			return "", "", "", fmt.Errorf("REBAC-TARGET-PARENT missing Submodel")
		}
		if route.Collection {
			return "submodel", route.Parent.Identifier, "create_child", nil
		}
		return kind, rebac.ElementIdentifier(route.Parent.Identifier, route.ElementPath), relation, nil
	}
	if route.Collection {
		return "repository", kind, "create", nil
	}
	if route.Kind == ReBACRouteKindSubmodelDescriptor && route.Parent != nil {
		return "embedded_submodel_descriptor", rebac.EmbeddedDescriptorIdentifier(route.Parent.Identifier, id), relation, nil
	}
	return kind, id, relation, nil
}
