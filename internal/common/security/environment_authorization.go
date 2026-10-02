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

func environmentRights(method, requestPath, basePath string) []grammar.RightsEnum {
	switch {
	case method == http.MethodGet && requestPath == joinBasePath(basePath, "/serialization"):
		return []grammar.RightsEnum{grammar.RightsEnumREAD}
	case method == http.MethodPost && requestPath == joinBasePath(basePath, "/upload"):
		return []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}
	default:
		return nil
	}
}

func environmentResourceRoute(resource SemanticResourceKind) string {
	switch resource {
	case SemanticResourceAAS:
		return "/shells"
	case SemanticResourceSM:
		return "/submodels"
	case SemanticResourceCD:
		return "/concept-descriptions"
	default:
		return ""
	}
}

func matchEvaluationObjects(objects []grammar.ObjectItem, in EvalInput, basePath string) AccessWithLE {
	if in.environmentResource == "" {
		return matchRouteObjectsObjItem(objects, in.Path, basePath)
	}
	target := environmentResourceRoute(in.environmentResource)
	if target == "" || len(environmentRights(in.Method, in.Path, basePath)) == 0 {
		return AccessWithLE{}
	}
	eligible := make([]grammar.ObjectItem, 0, len(objects))
	for _, object := range objects {
		if object.Kind == grammar.Route || object.Kind == grammar.Identifiable {
			eligible = append(eligible, object)
		}
	}
	return matchRouteObjectsWithIdentifiablePath(eligible, in.Path, joinBasePath(basePath, target), basePath)
}

func (s *AuthorizationSession) evaluateEnvironment(method, requestPath, routePath string) *environmentAuthorization {
	rights := environmentRights(method, requestPath, s.model.basePath)
	if len(rights) == 0 {
		return nil
	}
	result := &environmentAuthorization{
		evaluations: make(map[SemanticResourceKind]AuthorizationEvaluation, 3),
		rights:      rights,
		policyID:    s.policyID,
	}
	for _, resource := range []SemanticResourceKind{SemanticResourceAAS, SemanticResourceSM, SemanticResourceCD} {
		result.evaluations[resource] = s.model.AuthorizeWithFilterWithOptions(EvalInput{
			Method: method, Path: requestPath, RoutePath: routePath,
			Claims: s.claims, Globals: s.globals, environmentResource: resource,
		}, s.options)
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
