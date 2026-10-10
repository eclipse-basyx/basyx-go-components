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
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

type environmentAuthorizationContextKey struct{}

type environmentAuthorization struct {
	evaluations map[SemanticResourceKind]AuthorizationEvaluation
	rights      []grammar.RightsEnum
	policyID    string
}

func isEnvironmentRequest(method, requestPath, basePath string) bool {
	return method == http.MethodGet && requestPath == joinBasePath(basePath, "/serialization") ||
		method == http.MethodPost && requestPath == joinBasePath(basePath, "/upload")
}

func (s *AuthorizationSession) evaluateEnvironment(method, requestPath, routePath string) *environmentAuthorization {
	if !isEnvironmentRequest(method, requestPath, s.model.basePath) {
		return nil
	}
	input := EvalInput{Method: method, Path: requestPath, RoutePath: routePath, Claims: s.claims, Globals: s.globals}
	alternatives, mapped, routeFound := s.model.mapMethodAndPathToRights(input)
	if !mapped || !routeFound {
		return nil
	}
	result := &environmentAuthorization{
		evaluations: make(map[SemanticResourceKind]AuthorizationEvaluation, 3),
		rights:      collectRelevantRights(alternatives),
		policyID:    s.policyID,
	}
	for _, resource := range []SemanticResourceKind{SemanticResourceAAS, SemanticResourceSM, SemanticResourceCD} {
		input.objectContext = &objectMatchContext{identifiableResource: resource}
		result.evaluations[resource] = s.model.AuthorizeWithFilterWithOptions(input, s.options)
	}
	return result
}

func (a *environmentAuthorization) admit(evaluation AuthorizationEvaluation) AuthorizationEvaluation {
	if a == nil || evaluation.Allowed || evaluation.Reason == DecisionRouteNotFound {
		return evaluation
	}
	var ruleIDs []string
	for _, resource := range []SemanticResourceKind{SemanticResourceAAS, SemanticResourceSM, SemanticResourceCD} {
		scoped := a.evaluations[resource]
		if scoped.Allowed {
			ruleIDs = append(ruleIDs, scoped.MatchedRuleID)
		}
	}
	if len(ruleIDs) == 0 {
		return evaluation
	}
	return AuthorizationEvaluation{
		Allowed: true, Reason: DecisionAllow, PolicyID: a.policyID,
		MatchedRuleID: strings.Join(ruleIDs, ","),
		QueryFilter:   failClosedQueryFilter(a.rights...),
	}
}

// EnvironmentResourceContext binds the request's aggregate authorization to one
// repository type. It preserves ReBAC grants and replaces all previous ABAC
// views together. Internal startup imports and disabled security keep their context.
func EnvironmentResourceContext(ctx context.Context, resource SemanticResourceKind) (context.Context, error) {
	aggregate, _ := ctx.Value(environmentAuthorizationContextKey{}).(*environmentAuthorization)
	session := AuthorizationSessionFromContext(ctx)
	if aggregate == nil {
		if cfg, ok := common.ConfigFromContext(ctx); ok && !cfg.ABAC.Enabled {
			return ctx, nil
		}
		if session != nil || ClaimsFromContext(ctx) != nil {
			return nil, common.NewErrDenied("AUTH-ENVCONTEXT-MISSING aggregate authorization is missing")
		}
		return ctx, nil
	}
	evaluation, found := aggregate.evaluations[resource]
	if !found || session == nil {
		return nil, common.NewErrDenied("AUTH-ENVCONTEXT-RESOURCE unsupported environment resource")
	}
	filter, err := CloneQueryFilter(evaluation.QueryFilter)
	if err != nil {
		return nil, common.NewInternalServerError("AUTH-ENVCONTEXT-CLONE " + err.Error())
	}
	if !evaluation.Allowed {
		filter = failClosedQueryFilter(aggregate.rights...)
	}
	ctx = context.WithValue(ctx, filterKey, filter)
	ctx = context.WithValue(ctx, authorizedQueryContextKey{}, (*AuthorizedQuery)(nil))
	ctx = context.WithValue(ctx, selectedFormulaRightKey, struct{}{})
	ctx = context.WithValue(ctx, authorizationSessionContextKey{}, session.withOuterAccess(accessViewFromEvaluation(resource, evaluation)))
	return ctx, nil
}
