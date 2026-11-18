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
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

const (
	superpathRoutePrefix     = "/shells/{aasIdentifier}/submodels/"
	superpathSubmodelPattern = superpathRoutePrefix + "{submodelIdentifier}"
)

// superpathShellRight returns the right a superpath route requires on its
// shell: UPDATE for PUT and DELETE of the Submodel, which change the shell's
// Submodel references, and READ otherwise.
func superpathShellRight(route ReBACRoute) (grammar.RightsEnum, bool) {
	if !strings.HasPrefix(route.Pattern, superpathRoutePrefix) {
		return "", false
	}
	if route.Pattern == superpathSubmodelPattern && (route.Method == http.MethodPut || route.Method == http.MethodDelete) {
		return grammar.RightsEnumUPDATE, true
	}
	return grammar.RightsEnumREAD, true
}

// superpathShellNeedsReBAC reports superpath routes whose shell ABAC does not
// allow unconditionally, so that ReBAC grants on the shell must be resolved.
func (s *AuthorizationSession) superpathShellNeedsReBAC(route ReBACRoute) bool {
	right, ok := superpathShellRight(route)
	if !ok {
		return false
	}
	shellPath := joinBasePath(s.model.basePath, "/shells/"+route.Params["aasIdentifier"])
	return len(reBACPendingRights(s.evaluateShellRight(right, shellPath), []grammar.RightsEnum{right})) > 0
}

// SuperpathShellContext binds the authorization of the enclosing Asset
// Administration Shell of a superpath request (/shells/{aasIdentifier}/submodels/...)
// to ctx. The request itself is authorized by Submodel rules, so the shell
// lookups of the request must use the returned context: it evaluates the $aas
// and ROUTE rules of /shells/{aasIdentifier} for right. Submodel lookups keep
// the request context. ReBAC grants are preserved, and disabled security and
// internal calls keep their context.
func SuperpathShellContext(ctx context.Context, aasIdentifier string, right grammar.RightsEnum) (context.Context, error) {
	session := AuthorizationSessionFromContext(ctx)
	if session == nil || session.model == nil {
		return contextWithoutResourceAuthorization(ctx, false, "AUTH-SUPERPATHSHELL-MISSING request authorization is missing")
	}
	shellPath := joinBasePath(session.model.basePath, "/shells/"+common.EncodeString(aasIdentifier))
	evaluation := session.evaluateShellRight(right, shellPath)
	filter, err := superpathShellFilter(ctx, evaluation, right)
	if err != nil {
		return nil, err
	}
	return bindResourceAuthorization(ctx, session, SemanticResourceAAS, evaluation, filter), nil
}

func superpathShellFilter(ctx context.Context, evaluation AuthorizationEvaluation, right grammar.RightsEnum) (*QueryFilter, error) {
	if !evaluation.Allowed {
		if !hasReBACGrantForResource(ctx, SemanticResourceAAS) {
			return nil, common.NewErrDenied("AUTH-SUPERPATHSHELL-DENIED access to the Asset Administration Shell is denied")
		}
		return failClosedQueryFilter(right), nil
	}
	filter, err := CloneQueryFilter(evaluation.QueryFilter)
	if err != nil {
		return nil, common.NewInternalServerError("AUTH-SUPERPATHSHELL-CLONE " + err.Error())
	}
	return filter, nil
}

func hasReBACGrantForResource(ctx context.Context, resource SemanticResourceKind) bool {
	grants := ReBACGrantsFromContext(ctx)
	if grants.IsEmpty() {
		return false
	}
	return len(grants.entriesFor(resource, activeReBACRights(ctx, grants))) > 0
}

func superpathFormulaResource(in EvalInput, basePath string) SemanticResourceKind {
	if in.formulaResource != "" {
		return in.formulaResource
	}
	if isSubmodelSuperpath(basePath, routePath(in)) {
		return SemanticResourceSM
	}
	return ""
}

func isSubmodelSuperpath(basePath, requestPath string) bool {
	segments := strings.SplitN(strings.TrimPrefix(stripBasePath(basePath, requestPath), "/"), "/", 5)
	return len(segments) >= 4 && segments[0] == "shells" && segments[2] == "submodels"
}

// Submodel superpath PATCH operations return no model fragments.
func operationOmitsResponseFilters(method, basePath, requestPath string) bool {
	return method == http.MethodDelete || method == http.MethodPatch && isSubmodelSuperpath(basePath, requestPath)
}

func superpathFormulaSupported(formula grammar.LogicalExpression, resource SemanticResourceKind) bool {
	foreignRoots := []string{"$aasdesc", "$smdesc", "$cd", "$bd"}
	switch resource {
	case "":
		return true
	case SemanticResourceAAS:
		foreignRoots = append(foreignRoots, "$sm", "$sme")
	case SemanticResourceSM:
		foreignRoots = append(foreignRoots, "$aas")
	default:
		return false
	}
	_, found := grammar.FindModelFieldByRoot(grammar.Query{Condition: &formula}, foreignRoots...)
	return !found
}

func superpathGrantSupported(formula grammar.LogicalExpression, filters FragmentFilters, resource SemanticResourceKind) bool {
	if resource == "" {
		return true
	}
	if !superpathFormulaSupported(formula, resource) {
		return false
	}
	for fragment, filter := range filters {
		if superpathFilterApplies(fragment, resource) && !superpathFilterSupported(filter, resource) {
			return false
		}
	}
	return true
}

func superpathFilterApplies(fragment grammar.FragmentStringPattern, resource SemanticResourceKind) bool {
	root := fragmentRoot(fragment)
	switch resource {
	case SemanticResourceAAS:
		return root == "$aas"
	case SemanticResourceSM:
		return root == "$sm" || root == "$sme"
	default:
		return false
	}
}

func superpathFilterSupported(filter FragmentFilterPredicate, resource SemanticResourceKind) bool {
	if filter.Condition != nil && !superpathFormulaSupported(*filter.Condition, resource) {
		return false
	}
	for _, child := range filter.And {
		if !superpathFilterSupported(child, resource) {
			return false
		}
	}
	for _, child := range filter.Or {
		if !superpathFilterSupported(child, resource) {
			return false
		}
	}
	return true
}
