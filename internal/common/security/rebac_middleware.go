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

package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	api "github.com/go-chi/chi/v5"
)

// ReBACDecisionRuleID marks authorization decisions that were granted through
// relationship-based access control instead of an ABAC rule.
const ReBACDecisionRuleID = "rebac"

// ErrReBACUnavailable reports that ReBAC was required for a decision but could
// not answer. The middleware maps it to 503 and never falls back to ABAC-only.
var ErrReBACUnavailable = errors.New("SECURITY-REBAC-UNAVAILABLE relationship-based authorization is unavailable")

// ReBACRoute is the matched API route of a request, relative to the context path.
type ReBACRoute struct {
	Method  string
	Pattern string
	Params  map[string]string
	Rights  []grammar.RightsEnum
}

// ReBACRequest describes one request that ABAC did not unconditionally allow.
// PendingRights lists the route rights ABAC does not grant without conditions;
// only those need a relationship check.
type ReBACRequest struct {
	Route         ReBACRoute
	Claims        Claims
	PendingRights []grammar.RightsEnum
	ABACAllowed   bool
}

// ReBACResolver plugs relationship-based authorization into ABACMiddleware.
//
// Implementations must be safe for concurrent use. Resolve returns the grants
// for the concrete resources of the request or an empty set when ReBAC does not
// allow; an error means ReBAC could not decide and results in 503.
type ReBACResolver interface {
	Covers(route ReBACRoute) bool
	IsManagementRoute(route ReBACRoute) bool
	Resolve(ctx context.Context, request ReBACRequest) (*ReBACGrantSet, error)
}

// ReBACRouteFromContext returns the route matched by ABACMiddleware for
// ReBAC-covered and management requests.
func ReBACRouteFromContext(ctx context.Context) (ReBACRoute, bool) {
	if ctx == nil {
		return ReBACRoute{}, false
	}
	route, ok := ctx.Value(reBACRouteContextKey{}).(ReBACRoute)
	return route, ok
}

type reBACRouteContextKey struct{}

// matchReBACRoute resolves the chi pattern, decoded URL parameters and mapped
// rights of a request against the model's API router.
func (m *AccessModel) matchReBACRoute(method string, routePath string) (ReBACRoute, bool) {
	if m == nil || m.apiRouter == nil {
		return ReBACRoute{}, false
	}
	matchPath := stripBasePath(m.basePath, routePath)
	if isNonRootTrailingSlashPath(matchPath) {
		return ReBACRoute{}, false
	}
	rctx := api.NewRouteContext()
	pattern := m.apiRouter.Find(rctx, method, matchPath)
	if pattern == "" {
		return ReBACRoute{}, false
	}
	params := make(map[string]string, len(rctx.URLParams.Keys))
	for index, key := range rctx.URLParams.Keys {
		value := rctx.URLParams.Values[index]
		if decoded, err := url.PathUnescape(value); err == nil {
			value = decoded
		}
		params[key] = value
	}
	alternatives, _, _ := m.mapMethodAndPathToRights(EvalInput{Method: method, Path: routePath, RoutePath: routePath})
	return ReBACRoute{
		Method:  method,
		Pattern: pattern,
		Params:  params,
		Rights:  collectRelevantRights(alternatives),
	}, true
}

// reBACPendingRights returns the route rights that ABAC does not allow
// unconditionally: without formula restriction and without fragment filters.
func reBACPendingRights(evaluation AuthorizationEvaluation, rights []grammar.RightsEnum) []grammar.RightsEnum {
	if !evaluation.Allowed {
		return slices.Clone(rights)
	}
	queryFilter := evaluation.QueryFilter
	if queryFilter == nil {
		return nil
	}
	pending := make([]grammar.RightsEnum, 0, len(rights))
	for _, right := range rights {
		if len(queryFilter.Filters) > 0 || !formulaIsTrue(queryFilter.FormulasByRight, right) {
			pending = append(pending, right)
		}
	}
	return pending
}

func formulaIsTrue(formulas map[grammar.RightsEnum]grammar.LogicalExpression, right grammar.RightsEnum) bool {
	formula, ok := formulas[right]
	return ok && formula.Boolean != nil && *formula.Boolean
}

type reBACOutcome uint8

const (
	reBACOutcomeABACOnly reBACOutcome = iota
	reBACOutcomeGranted
	reBACOutcomeManagement
	reBACOutcomeUnavailable
)

// resolveReBAC consults the resolver for requests ABAC does not fully allow.
// Anonymous requests and uncovered routes keep ABAC-only semantics.
func resolveReBAC(
	r *http.Request,
	settings ABACSettings,
	model *AccessModel,
	evaluation AuthorizationEvaluation,
	routePath string,
) (reBACOutcome, ReBACRoute, *ReBACGrantSet) {
	if settings.ReBAC == nil || evaluation.Reason == DecisionRouteNotFound {
		return reBACOutcomeABACOnly, ReBACRoute{}, nil
	}
	route, found := model.matchReBACRoute(r.Method, routePath)
	if !found {
		return reBACOutcomeABACOnly, ReBACRoute{}, nil
	}
	if settings.ReBAC.IsManagementRoute(route) {
		return reBACOutcomeManagement, route, nil
	}
	if !IsAuthenticated(r.Context()) || !settings.ReBAC.Covers(route) {
		return reBACOutcomeABACOnly, route, nil
	}
	pending := reBACPendingRights(evaluation, route.Rights)
	if len(pending) == 0 {
		return reBACOutcomeABACOnly, route, nil
	}
	grants, err := settings.ReBAC.Resolve(r.Context(), ReBACRequest{
		Route:         route,
		Claims:        FromContext(r),
		PendingRights: pending,
		ABACAllowed:   evaluation.Allowed,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "ReBAC decision unavailable", "error.code", "SECURITY-REBAC-UNAVAILABLE", "error", err)
		return reBACOutcomeUnavailable, route, nil
	}
	if grants.IsEmpty() {
		return reBACOutcomeABACOnly, route, nil
	}
	return reBACOutcomeGranted, route, grants
}

// reBACGrantedEvaluation turns an ABAC deny into a fail-closed allow that
// only the published ReBAC grants can widen. ABAC allows keep their filter.
func reBACGrantedEvaluation(evaluation AuthorizationEvaluation, rights []grammar.RightsEnum) AuthorizationEvaluation {
	if evaluation.Allowed {
		return evaluation
	}
	return AuthorizationEvaluation{
		Allowed:       true,
		Reason:        DecisionAllow,
		QueryFilter:   failClosedQueryFilter(rights...),
		PolicyID:      evaluation.PolicyID,
		MatchedRuleID: ReBACDecisionRuleID,
	}
}

func writeReBACUnavailable(w http.ResponseWriter, r *http.Request) {
	if err := common.WriteErrorResponse(w, ErrReBACUnavailable, http.StatusServiceUnavailable, "Middleware", "ReBAC", "Unavailable"); err != nil {
		slog.ErrorContext(r.Context(), "ReBAC unavailable response encoding failed", "error.code", "SECURITY-REBAC-ENCODERESPONSE", "error", err)
	}
}
