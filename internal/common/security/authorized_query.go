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
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

type authorizationSessionContextKey struct{}
type authorizedQueryContextKey struct{}

// SemanticResourceKind identifies the authorization resource read by a query
// expression independently of its physical SQL tables.
type SemanticResourceKind string

// Semantic resource kinds identify the model resources supported by query authorization.
const (
	SemanticResourceAAS     SemanticResourceKind = "aas"
	SemanticResourceAASDesc SemanticResourceKind = "aasdesc"
	SemanticResourceSM      SemanticResourceKind = "sm"
	SemanticResourceSMDesc  SemanticResourceKind = "smdesc"
	SemanticResourceSME     SemanticResourceKind = "sme"
	SemanticResourceCD      SemanticResourceKind = "cd"
	SemanticResourceBD      SemanticResourceKind = "bd"
)

// SemanticAccessTarget identifies one resource or fragment observed by an
// untrusted predicate.
type SemanticAccessTarget struct {
	Resource SemanticResourceKind
	Field    grammar.ModelStringPattern
}

// StringSelector describes one public list selector that changes resource
// membership by comparing a semantic field with a caller-supplied value.
type StringSelector struct {
	Field grammar.ModelStringPattern
	Value string
}

// AccessViewDecision is the immutable authorization outcome for a semantic
// resource view.
type AccessViewDecision uint8

// Access view decisions describe how a semantic resource can be observed.
const (
	AccessViewDenied AccessViewDecision = iota
	AccessViewRestricted
	AccessViewUnrestricted
)

// CompiledGrantAlternative preserves one complete allow-rule witness. Its
// formula and filters are intentionally private so callers cannot mutate the
// request's policy snapshot.
type CompiledGrantAlternative struct {
	ruleID    string
	formula   grammar.LogicalExpression
	filters   FragmentFilters
	coverages []semanticObjectCoverage
}

type semanticObjectCoverage struct {
	resource            SemanticResourceKind
	submodelID          string
	allSubmodelIDs      bool
	allSubmodelElements bool
	submodelElementPath string
	submodelFragment    string
}

// SemanticAccessView is the immutable policy view applicable to one semantic
// resource kind during a request.
type SemanticAccessView struct {
	resource     SemanticResourceKind
	decision     AccessViewDecision
	queryFilter  *QueryFilter
	alternatives []CompiledGrantAlternative
}

// Decision returns whether the semantic resource is denied, restricted, or
// unrestricted for caller-controlled observation.
func (v SemanticAccessView) Decision() AccessViewDecision {
	return v.decision
}

// Resource returns the semantic resource represented by the view.
func (v SemanticAccessView) Resource() SemanticResourceKind {
	return v.resource
}

// AuthorizationSession pins the model, claims, globals, and simplification
// settings used for every authorization view in one request.
type AuthorizationSession struct {
	model       *AccessModel
	claims      Claims
	globals     GlobalAttributes
	options     grammar.SimplifyOptions
	policyID    string
	outerAccess SemanticAccessView
}

func newAuthorizationSession(
	model *AccessModel,
	claims Claims,
	configuredGlobals GlobalAttributes,
	options grammar.SimplifyOptions,
) *AuthorizationSession {
	frozenClaims := cloneClaims(claims)
	frozenGlobals := globalAttributesForEvaluation(
		cloneGlobalAttributes(configuredGlobals),
		frozenClaims,
		time.Now(),
	)
	policyID := ""
	if model != nil {
		policyID = model.PolicyID()
	}
	return &AuthorizationSession{
		model:    model,
		claims:   frozenClaims,
		globals:  frozenGlobals,
		options:  options,
		policyID: policyID,
	}
}

func (s *AuthorizationSession) withOuterAccess(view SemanticAccessView) *AuthorizationSession {
	if s == nil {
		return nil
	}
	clone := *s
	clone.outerAccess = view
	return &clone
}

// PolicyID returns the pinned policy version for the request.
func (s *AuthorizationSession) PolicyID() string {
	if s == nil {
		return ""
	}
	return s.policyID
}

func (s *AuthorizationSession) evaluate(method string, requestPath string) AuthorizationEvaluation {
	if s == nil || s.model == nil {
		return AuthorizationEvaluation{Reason: DecisionNoMatch}
	}
	return s.model.AuthorizeWithFilterWithOptions(EvalInput{
		Method:    method,
		Path:      requestPath,
		RoutePath: requestPath,
		Claims:    s.claims,
		Globals:   s.globals,
	}, s.options)
}

func (s *AuthorizationSession) semanticView(resource SemanticResourceKind) SemanticAccessView {
	if s == nil || s.model == nil {
		return SemanticAccessView{resource: resource, decision: AccessViewUnrestricted}
	}
	if resource == SemanticResourceSM || resource == SemanticResourceSME {
		return s.semanticReadView(resource)
	}
	route, ok := semanticReadRoute(resource)
	if !ok {
		return SemanticAccessView{resource: resource, decision: AccessViewDenied}
	}
	requestPath := joinBasePath(s.model.basePath, route)
	evaluation := s.evaluate(http.MethodGet, requestPath)
	return accessViewFromEvaluation(resource, evaluation)
}

func (s *AuthorizationSession) semanticReadView(resource SemanticResourceKind) SemanticAccessView {
	resolver := func(attribute grammar.AttributeValue) any {
		return resolveAttributeValue(attribute, s.claims, s.globals)
	}
	var alternatives []CompiledGrantAlternative
	for _, ruleIndex := range s.model.semanticReadRuleIndexes[resource] {
		rule := s.model.rules[ruleIndex]
		if rule.acl.ACCESS == grammar.ACLACCESSDISABLED ||
			!ruleAllowsRight(rule.acl.RIGHTS, grammar.RightsEnumREAD) ||
			!attributesSatisfiedAll(rule.attrs, s.claims) ||
			rule.lexpr == nil {
			continue
		}
		formula, decision := rule.lexpr.SimplifyForBackendFilterWithOptions(resolver, s.options)
		if decision == grammar.SimplifyFalse || decision == grammar.SimplifyIndeterminate {
			continue
		}
		filters := simplifyRuleFragmentFilters(rule.filterList, resolver, s.options)
		coverages := semanticCoveragesForRule(rule, resource, s.model.basePath)
		if len(coverages) == 0 {
			continue
		}
		alternatives = append(alternatives, CompiledGrantAlternative{
			ruleID:    rule.id,
			formula:   formula,
			filters:   filters,
			coverages: coverages,
		})
	}
	if len(alternatives) == 0 {
		return SemanticAccessView{resource: resource, decision: AccessViewDenied}
	}
	return SemanticAccessView{
		resource:     resource,
		decision:     AccessViewRestricted,
		queryFilter:  &QueryFilter{},
		alternatives: alternatives,
	}
}

func buildSemanticReadRuleIndexes(
	rules []materializedRule,
	basePath string,
) map[SemanticResourceKind][]int {
	result := make(map[SemanticResourceKind][]int)
	for index, rule := range rules {
		for _, resource := range []SemanticResourceKind{SemanticResourceSM, SemanticResourceSME} {
			if len(semanticCoveragesForRule(rule, resource, basePath)) > 0 {
				result[resource] = append(result[resource], index)
			}
		}
	}
	return result
}

func semanticCoveragesForRule(
	rule materializedRule,
	resource SemanticResourceKind,
	basePath string,
) []semanticObjectCoverage {
	var coverages []semanticObjectCoverage
	for _, object := range rule.objs {
		coverages = append(coverages, semanticCoveragesForObject(object, resource, basePath)...)
	}
	return coverages
}

func semanticCoveragesForObject(
	object grammar.ObjectItem,
	resource SemanticResourceKind,
	basePath string,
) []semanticObjectCoverage {
	switch object.Kind {
	case grammar.Route:
		if object.Route != nil {
			if coverage, covered := semanticSubmodelRouteCoverage(object.Route.Route, resource, basePath); covered {
				return []semanticObjectCoverage{coverage}
			}
		}
	case grammar.Identifiable:
		if object.Identifiable != nil && object.Identifiable.Scope == "$sm" {
			if resource == SemanticResourceSM || resource == SemanticResourceSME {
				return []semanticObjectCoverage{identifierSubmodelCoverage(resource, object.Identifiable.ID)}
			}
		}
	case grammar.Referable:
		if resource == SemanticResourceSME && object.Referable != nil && object.Referable.Scope == "$sme" {
			return []semanticObjectCoverage{{
				resource:            resource,
				submodelID:          object.Referable.ID.ID,
				allSubmodelIDs:      object.Referable.ID.IsAll,
				submodelElementPath: strings.TrimSpace(object.Referable.IDShortPath),
			}}
		}
	case grammar.Fragment:
		if resource == SemanticResourceSME && object.Fragment != nil && object.Fragment.Scope == "$sme" {
			return fragmentObjectCoverages(resource, *object.Fragment)
		}
	}
	return nil
}

func fragmentObjectCoverages(
	resource SemanticResourceKind,
	fragment grammar.FragmentValue,
) []semanticObjectCoverage {
	coverages := make([]semanticObjectCoverage, 0, len(fragment.Fragments))
	for _, fieldFragment := range fragment.Fragments {
		fieldFragment = strings.TrimPrefix(strings.TrimSpace(fieldFragment), "#")
		if fieldFragment == "" {
			continue
		}
		coverages = append(coverages, semanticObjectCoverage{
			resource:            resource,
			submodelID:          fragment.ID.ID,
			allSubmodelIDs:      fragment.ID.IsAll,
			submodelElementPath: strings.TrimSpace(fragment.IDShortPath),
			submodelFragment:    fieldFragment,
		})
	}
	return coverages
}

func allSubmodelCoverage(resource SemanticResourceKind) semanticObjectCoverage {
	return semanticObjectCoverage{
		resource:            resource,
		allSubmodelIDs:      true,
		allSubmodelElements: resource == SemanticResourceSME,
	}
}

func identifierSubmodelCoverage(
	resource SemanticResourceKind,
	identifier grammar.Identifier,
) semanticObjectCoverage {
	return semanticObjectCoverage{
		resource:            resource,
		submodelID:          identifier.ID,
		allSubmodelIDs:      identifier.IsAll,
		allSubmodelElements: resource == SemanticResourceSME,
	}
}

func semanticSubmodelRouteCoverage(
	route string,
	resource SemanticResourceKind,
	basePath string,
) (semanticObjectCoverage, bool) {
	normalized := stripBasePath(basePath, normalize(route))
	if normalized == "*" || normalized == "/*" {
		return allSubmodelCoverage(resource), true
	}
	if normalized == "/query/submodels" {
		return allSubmodelCoverage(resource), true
	}
	if normalized == "/submodels" || strings.HasPrefix(normalized, "/submodels/") {
		return submodelPathCoverage(strings.TrimPrefix(normalized, "/submodels"), resource)
	}
	if strings.HasPrefix(normalized, "/shells/") {
		segments := strings.Split(strings.Trim(normalized, "/"), "/")
		for index, segment := range segments {
			if segment != "submodels" {
				continue
			}
			return submodelPathCoverage(strings.Join(segments[index+1:], "/"), resource)
		}
	}
	return semanticObjectCoverage{}, false
}

func submodelPathCoverage(
	suffix string,
	resource SemanticResourceKind,
) (semanticObjectCoverage, bool) {
	suffix = strings.Trim(suffix, "/")
	if suffix == "" {
		return allSubmodelCoverage(resource), true
	}
	segments := strings.Split(suffix, "/")
	if len(segments) == 0 {
		return semanticObjectCoverage{}, false
	}
	if strings.HasPrefix(segments[0], "$") {
		if resource == SemanticResourceSME {
			return semanticObjectCoverage{}, false
		}
		return allSubmodelCoverage(resource), true
	}
	submodelID, allSubmodels, valid := decodeSemanticRouteSegment(segments[0])
	if !valid {
		return semanticObjectCoverage{}, false
	}
	elementIndex := -1
	for index, segment := range segments {
		if segment == "submodel-elements" {
			elementIndex = index
			break
		}
	}
	if resource == SemanticResourceSM {
		if elementIndex >= 0 {
			return semanticObjectCoverage{}, false
		}
		return semanticObjectCoverage{
			resource:       resource,
			submodelID:     submodelID,
			allSubmodelIDs: allSubmodels,
		}, true
	}
	if resource != SemanticResourceSME {
		return semanticObjectCoverage{}, false
	}
	if elementIndex < 0 && len(segments) > 1 && segments[1] != "**" {
		return semanticObjectCoverage{}, false
	}
	coverage := semanticObjectCoverage{
		resource:            resource,
		submodelID:          submodelID,
		allSubmodelIDs:      allSubmodels,
		allSubmodelElements: true,
	}
	if elementIndex < 0 || elementIndex+1 >= len(segments) {
		return coverage, true
	}
	if elementIndex+2 < len(segments) && segments[elementIndex+2] != "**" {
		return semanticObjectCoverage{}, false
	}
	elementPath, allElements, elementValid := decodeSemanticRouteSegment(segments[elementIndex+1])
	if !elementValid {
		return semanticObjectCoverage{}, false
	}
	coverage.allSubmodelElements = allElements
	coverage.submodelElementPath = elementPath
	return coverage, true
}

func decodeSemanticRouteSegment(segment string) (string, bool, bool) {
	segment = strings.TrimSpace(segment)
	if segment == "*" || segment == "**" || strings.HasPrefix(segment, "{") {
		return "", true, true
	}
	if segment == "" || strings.HasPrefix(segment, "$") {
		return "", false, false
	}
	decoded, err := common.DecodeString(segment)
	if err != nil || strings.TrimSpace(decoded) == "" {
		return "", false, false
	}
	return decoded, false, true
}

func simplifyRuleFragmentFilters(
	filters []grammar.AccessPermissionRuleFILTER,
	resolver grammar.AttributeResolver,
	options grammar.SimplifyOptions,
) FragmentFilters {
	result := make(FragmentFilters)
	for _, filter := range filters {
		if filter.FRAGMENT == nil || filter.CONDITION == nil {
			continue
		}
		condition, _ := filter.CONDITION.SimplifyForBackendFilterWithOptions(resolver, options)
		predicate := NewFragmentFilterPredicate(condition, filter.MATCH != nil && *filter.MATCH)
		if existing, ok := result[*filter.FRAGMENT]; ok {
			result[*filter.FRAGMENT] = AndFragmentFilterPredicates(existing, predicate)
		} else {
			result[*filter.FRAGMENT] = predicate
		}
	}
	return result
}

func semanticReadRoute(resource SemanticResourceKind) (string, bool) {
	switch resource {
	case SemanticResourceAAS:
		return "/shells", true
	case SemanticResourceAASDesc:
		return "/shell-descriptors", true
	case SemanticResourceSM, SemanticResourceSME:
		return "/submodels", true
	case SemanticResourceSMDesc:
		return "/submodel-descriptors", true
	case SemanticResourceCD:
		return "/concept-descriptions", true
	case SemanticResourceBD:
		return "/lookup/shells", true
	default:
		return "", false
	}
}

func accessViewFromEvaluation(resource SemanticResourceKind, evaluation AuthorizationEvaluation) SemanticAccessView {
	if !evaluation.Allowed {
		return SemanticAccessView{resource: resource, decision: AccessViewDenied}
	}
	queryFilter, err := CloneQueryFilter(evaluation.QueryFilter)
	if err != nil {
		return SemanticAccessView{resource: resource, decision: AccessViewDenied}
	}
	decision := AccessViewUnrestricted
	if queryFilter != nil {
		decision = AccessViewRestricted
	}
	return SemanticAccessView{
		resource:     resource,
		decision:     decision,
		queryFilter:  queryFilter,
		alternatives: cloneGrantAlternatives(evaluation.alternatives),
	}
}

// AuthorizedQuery preserves the provenance of caller expressions and policy
// access views. All fields are private to prevent mutation after publication.
type AuthorizedQuery struct {
	caller           grammar.Query
	backendCaller    grammar.Query
	backendCallerSet bool
	outer            SemanticAccessView
	relatedViews     map[SemanticResourceKind]SemanticAccessView
	session          *AuthorizationSession
}

// WithAuthorizedQuery attaches an immutable authorized-query IR to ctx. The
// response filter stored in ctx retains security projection and adds caller
// projection, while the main caller condition remains separate until secure
// SQL compilation.
func WithAuthorizedQuery(
	ctx context.Context,
	outerResource SemanticResourceKind,
	query grammar.Query,
) (context.Context, error) {
	callerInput := query
	existingAuthorized := AuthorizedQueryFromContext(ctx)
	if existingAuthorized != nil {
		callerInput = combineCallerQueries(existingAuthorized.caller, query)
	}
	caller, err := cloneQuery(callerInput)
	if err != nil {
		return ctx, fmt.Errorf("AUTH-AUTHQUERY-CLONECALLER: %w", err)
	}

	session := AuthorizationSessionFromContext(ctx)
	backendCaller := simplifyCallerQueryForBackend(ctx, session, caller)
	outer := SemanticAccessView{resource: outerResource, decision: AccessViewUnrestricted}
	related := make(map[SemanticResourceKind]SemanticAccessView)
	if existingAuthorized != nil {
		outer = existingAuthorized.outer
		for resource, view := range existingAuthorized.relatedViews {
			related[resource] = view
		}
		if outer.resource != "" && outer.resource != outerResource {
			related[outer.resource] = outer
		}
		outer.resource = outerResource
	} else if session != nil {
		outer = session.outerAccess
		outer.resource = outerResource
	} else if existing := GetQueryFilter(ctx); existing != nil {
		outer = accessViewFromQueryFilter(outerResource, existing)
	}

	for _, resource := range queryRelatedResources(caller, outerResource) {
		if _, exists := related[resource]; exists {
			continue
		}
		if session == nil {
			related[resource] = SemanticAccessView{resource: resource, decision: AccessViewUnrestricted}
			continue
		}
		related[resource] = session.semanticView(resource)
	}

	authorized := &AuthorizedQuery{
		caller:           caller,
		backendCaller:    backendCaller,
		backendCallerSet: true,
		outer:            outer,
		relatedViews:     related,
		session:          session,
	}
	projection, err := buildAuthorizedProjectionFilter(outer.queryFilter, backendCaller)
	if err != nil {
		return ctx, err
	}
	ctx = context.WithValue(ctx, authorizedQueryContextKey{}, authorized)
	if projection != nil {
		ctx = context.WithValue(ctx, filterKey, projection)
	}
	return ctx, nil
}

func (q *AuthorizedQuery) callerForBackend() grammar.Query {
	if q != nil && q.backendCallerSet {
		return q.backendCaller
	}
	if q == nil {
		return grammar.Query{}
	}
	return q.caller
}

func simplifyCallerQueryForBackend(
	ctx context.Context,
	session *AuthorizationSession,
	query grammar.Query,
) grammar.Query {
	options := grammar.DefaultSimplifyOptions()
	if session != nil {
		options = session.options
	} else if cfg, ok := common.ConfigFromContext(ctx); ok {
		options.EnableImplicitCasts = cfg.General.EnableImplicitCasts
	}
	resolver := func(grammar.AttributeValue) any { return nil }
	if query.Condition != nil {
		condition, _ := query.Condition.SimplifyForBackendFilterWithOptions(resolver, options)
		query.Condition = &condition
	}
	for index := range query.FilterConditions {
		if query.FilterConditions[index].Condition == nil {
			continue
		}
		condition, _ := query.FilterConditions[index].Condition.SimplifyForBackendFilterWithOptions(resolver, options)
		query.FilterConditions[index].Condition = &condition
	}
	return query
}

// WithAuthorizedStringSelectors lowers public list parameters into the same
// caller-controlled query boundary as query-language conditions. Callers must
// not also pass the lowered selectors to a backend's native filter arguments.
func WithAuthorizedStringSelectors(
	ctx context.Context,
	outerResource SemanticResourceKind,
	selectors ...StringSelector,
) (context.Context, error) {
	conditions := make([]grammar.LogicalExpression, 0, len(selectors))
	for _, selector := range selectors {
		if selector.Value == "" {
			continue
		}
		field := selector.Field
		value := grammar.StandardString(selector.Value)
		conditions = append(conditions, grammar.LogicalExpression{Eq: grammar.ComparisonItems{
			{Field: &field},
			{StrVal: &value},
		}})
	}
	if len(conditions) == 0 {
		return ctx, nil
	}
	condition := conditions[0]
	if len(conditions) > 1 {
		condition = grammar.LogicalExpression{And: conditions}
	}
	return WithAuthorizedQuery(ctx, outerResource, grammar.Query{Condition: &condition})
}

func combineCallerQueries(first grammar.Query, second grammar.Query) grammar.Query {
	combined := grammar.Query{
		FilterConditions: append(append([]grammar.SubFilter(nil), first.FilterConditions...), second.FilterConditions...),
		Select:           append(append([]grammar.ModelStringPattern(nil), first.Select...), second.Select...),
	}
	switch {
	case first.Condition != nil && second.Condition != nil:
		combined.Condition = &grammar.LogicalExpression{And: []grammar.LogicalExpression{
			*first.Condition,
			*second.Condition,
		}}
	case first.Condition != nil:
		combined.Condition = first.Condition
	case second.Condition != nil:
		combined.Condition = second.Condition
	}
	return combined
}

// AuthorizedQueryFromContext returns the immutable request query. Callers must
// not expose or mutate its internals.
func AuthorizedQueryFromContext(ctx context.Context) *AuthorizedQuery {
	if ctx == nil {
		return nil
	}
	authorized, _ := ctx.Value(authorizedQueryContextKey{}).(*AuthorizedQuery)
	return authorized
}

// AuthorizationSessionFromContext returns the request-pinned authorization
// session, or nil when ABAC is disabled.
func AuthorizationSessionFromContext(ctx context.Context) *AuthorizationSession {
	if ctx == nil {
		return nil
	}
	session, _ := ctx.Value(authorizationSessionContextKey{}).(*AuthorizationSession)
	return session
}

func accessViewFromQueryFilter(resource SemanticResourceKind, queryFilter *QueryFilter) SemanticAccessView {
	clone, err := CloneQueryFilter(queryFilter)
	if err != nil {
		return SemanticAccessView{resource: resource, decision: AccessViewDenied}
	}
	decision := AccessViewUnrestricted
	if clone != nil {
		decision = AccessViewRestricted
	}
	return SemanticAccessView{resource: resource, decision: decision, queryFilter: clone}
}

func queryRelatedResources(query grammar.Query, outer SemanticResourceKind) []SemanticResourceKind {
	resources := make([]SemanticResourceKind, 0, 2)
	if outer == SemanticResourceAAS {
		if _, found := grammar.FindModelFieldByRoot(query, "$sm"); found {
			resources = append(resources, SemanticResourceSM)
		}
	}
	if outer == SemanticResourceAAS || outer == SemanticResourceSM {
		if _, found := grammar.FindModelFieldByRoot(query, "$sme"); found {
			resources = append(resources, SemanticResourceSME)
		}
	}
	return resources
}

func buildAuthorizedProjectionFilter(outer *QueryFilter, query grammar.Query) (*QueryFilter, error) {
	projection, err := CloneQueryFilter(outer)
	if err != nil {
		return nil, fmt.Errorf("AUTH-AUTHQUERY-CLONEOUTER: %w", err)
	}
	if projection == nil {
		projection = &QueryFilter{}
	}
	if query.Condition != nil && projection.Formula == nil {
		marker := boolExpression(true)
		projection.Formula = &marker
		projection.FormulasByRight = map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumREAD: marker,
		}
	}
	for _, filter := range query.FilterConditions {
		if filter.Fragment == nil || filter.Condition == nil {
			continue
		}
		if projection.Filters == nil {
			projection.Filters = make(FragmentFilters)
		}
		match := filter.Match != nil && *filter.Match
		predicate := newCallerFragmentFilterPredicate(*filter.Condition, match)
		if existing, ok := projection.Filters[*filter.Fragment]; ok {
			projection.Filters[*filter.Fragment] = AndFragmentFilterPredicates(existing, predicate)
		} else {
			projection.Filters[*filter.Fragment] = predicate
		}
	}
	if projection.Formula == nil && len(projection.FormulasByRight) == 0 && len(projection.Filters) == 0 {
		return nil, nil
	}
	return projection, nil
}

func cloneQuery(query grammar.Query) (grammar.Query, error) {
	clone := grammar.Query{
		Select: make([]grammar.ModelStringPattern, len(query.Select)),
	}
	copy(clone.Select, query.Select)
	if query.Condition != nil {
		condition := cloneLogicalExpression(*query.Condition)
		clone.Condition = &condition
	}
	clone.FilterConditions = make([]grammar.SubFilter, len(query.FilterConditions))
	for index, filter := range query.FilterConditions {
		clone.FilterConditions[index] = cloneSubFilter(filter)
	}
	return clone, nil
}

func cloneSubFilter(filter grammar.SubFilter) grammar.SubFilter {
	clone := filter
	if filter.Condition != nil {
		condition := cloneLogicalExpression(*filter.Condition)
		clone.Condition = &condition
	}
	if filter.Fragment != nil {
		fragment := *filter.Fragment
		clone.Fragment = &fragment
	}
	if filter.Match != nil {
		match := *filter.Match
		clone.Match = &match
	}
	return clone
}

func cloneLogicalExpression(expression grammar.LogicalExpression) grammar.LogicalExpression {
	clone := expression
	if expression.Boolean != nil {
		boolean := *expression.Boolean
		clone.Boolean = &boolean
	}
	clone.And = cloneLogicalExpressions(expression.And)
	clone.Or = cloneLogicalExpressions(expression.Or)
	if expression.Not != nil {
		not := cloneLogicalExpression(*expression.Not)
		clone.Not = &not
	}
	clone.BoolCast = cloneGrammarValue(expression.BoolCast)
	clone.Eq = cloneComparisonItems(expression.Eq)
	clone.Ne = cloneComparisonItems(expression.Ne)
	clone.Gt = cloneComparisonItems(expression.Gt)
	clone.Ge = cloneComparisonItems(expression.Ge)
	clone.Lt = cloneComparisonItems(expression.Lt)
	clone.Le = cloneComparisonItems(expression.Le)
	clone.Contains = cloneStringItems(expression.Contains)
	clone.StartsWith = cloneStringItems(expression.StartsWith)
	clone.EndsWith = cloneStringItems(expression.EndsWith)
	clone.Regex = cloneStringItems(expression.Regex)
	clone.Match = cloneMatchExpressions(expression.Match)
	return clone
}

func cloneLogicalExpressions(expressions []grammar.LogicalExpression) []grammar.LogicalExpression {
	if expressions == nil {
		return nil
	}
	clone := make([]grammar.LogicalExpression, len(expressions))
	for index, expression := range expressions {
		clone[index] = cloneLogicalExpression(expression)
	}
	return clone
}

func cloneMatchExpressions(expressions []grammar.MatchExpression) []grammar.MatchExpression {
	if expressions == nil {
		return nil
	}
	clone := make([]grammar.MatchExpression, len(expressions))
	for index, expression := range expressions {
		clone[index] = expression
		if expression.Boolean != nil {
			boolean := *expression.Boolean
			clone[index].Boolean = &boolean
		}
		clone[index].Eq = cloneComparisonItems(expression.Eq)
		clone[index].Ne = cloneComparisonItems(expression.Ne)
		clone[index].Gt = cloneComparisonItems(expression.Gt)
		clone[index].Ge = cloneComparisonItems(expression.Ge)
		clone[index].Lt = cloneComparisonItems(expression.Lt)
		clone[index].Le = cloneComparisonItems(expression.Le)
		clone[index].Contains = cloneStringItems(expression.Contains)
		clone[index].StartsWith = cloneStringItems(expression.StartsWith)
		clone[index].EndsWith = cloneStringItems(expression.EndsWith)
		clone[index].Regex = cloneStringItems(expression.Regex)
		clone[index].Match = cloneMatchExpressions(expression.Match)
	}
	return clone
}

func cloneComparisonItems(items grammar.ComparisonItems) grammar.ComparisonItems {
	if items == nil {
		return nil
	}
	clone := make(grammar.ComparisonItems, len(items))
	for index := range items {
		clone[index] = *cloneGrammarValue(&items[index])
	}
	return clone
}

func cloneStringItems(items grammar.StringItems) grammar.StringItems {
	if items == nil {
		return nil
	}
	clone := make(grammar.StringItems, len(items))
	for index, item := range items {
		clone[index] = item
		clone[index].Attribute = cloneImmutableValue(item.Attribute)
		if item.Field != nil {
			field := *item.Field
			clone[index].Field = &field
		}
		clone[index].StrCast = cloneGrammarValue(item.StrCast)
		if item.StrVal != nil {
			value := *item.StrVal
			clone[index].StrVal = &value
		}
	}
	return clone
}

func cloneGrammarValue(value *grammar.Value) *grammar.Value {
	if value == nil {
		return nil
	}
	clone := *value
	clone.Attribute = cloneImmutableValue(value.Attribute)
	if value.Field != nil {
		field := *value.Field
		clone.Field = &field
	}
	if value.Boolean != nil {
		boolean := *value.Boolean
		clone.Boolean = &boolean
	}
	if value.DateTimeVal != nil {
		dateTime := *value.DateTimeVal
		clone.DateTimeVal = &dateTime
	}
	if value.HexVal != nil {
		hex := *value.HexVal
		clone.HexVal = &hex
	}
	if value.NumVal != nil {
		number := *value.NumVal
		clone.NumVal = &number
	}
	if value.StrVal != nil {
		str := *value.StrVal
		clone.StrVal = &str
	}
	if value.TimeVal != nil {
		timeValue := *value.TimeVal
		clone.TimeVal = &timeValue
	}
	clone.BoolCast = cloneGrammarValue(value.BoolCast)
	clone.DateTimeCast = cloneGrammarValue(value.DateTimeCast)
	clone.DayOfMonth = cloneGrammarValue(value.DayOfMonth)
	clone.DayOfWeek = cloneGrammarValue(value.DayOfWeek)
	clone.HexCast = cloneGrammarValue(value.HexCast)
	clone.Month = cloneGrammarValue(value.Month)
	clone.NumCast = cloneGrammarValue(value.NumCast)
	clone.StrCast = cloneGrammarValue(value.StrCast)
	clone.TimeCast = cloneGrammarValue(value.TimeCast)
	clone.Year = cloneGrammarValue(value.Year)
	return &clone
}

func cloneClaims(claims Claims) Claims {
	if claims == nil {
		return nil
	}
	clone := make(Claims, len(claims))
	for key, value := range claims {
		clone[key] = cloneImmutableValue(value)
	}
	return clone
}

func cloneGlobalAttributes(globals GlobalAttributes) GlobalAttributes {
	if globals == nil {
		return nil
	}
	clone := make(GlobalAttributes, len(globals))
	for key, value := range globals {
		clone[key] = cloneImmutableValue(value)
	}
	return clone
}

func cloneImmutableValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		clone := make(map[string]any, len(typed))
		for key, item := range typed {
			clone[key] = cloneImmutableValue(item)
		}
		return clone
	case map[string]string:
		clone := make(map[string]string, len(typed))
		for key, item := range typed {
			clone[key] = item
		}
		return clone
	case []any:
		clone := make([]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneImmutableValue(item)
		}
		return clone
	case []map[string]any:
		clone := make([]map[string]any, len(typed))
		for index, item := range typed {
			clone[index] = cloneImmutableValue(item).(map[string]any)
		}
		return clone
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func cloneGrantAlternatives(alternatives []CompiledGrantAlternative) []CompiledGrantAlternative {
	if len(alternatives) == 0 {
		return nil
	}
	cloned := make([]CompiledGrantAlternative, 0, len(alternatives))
	for _, alternative := range alternatives {
		queryFilter, err := CloneQueryFilter(&QueryFilter{
			Formula: &alternative.formula,
			Filters: alternative.filters,
		})
		if err != nil || queryFilter == nil || queryFilter.Formula == nil {
			continue
		}
		cloned = append(cloned, CompiledGrantAlternative{
			ruleID:    strings.TrimSpace(alternative.ruleID),
			formula:   *queryFilter.Formula,
			filters:   queryFilter.Filters,
			coverages: cloneSemanticObjectCoverages(alternative.coverages),
		})
	}
	return cloned
}

func cloneSemanticObjectCoverages(coverages []semanticObjectCoverage) []semanticObjectCoverage {
	if len(coverages) == 0 {
		return nil
	}
	cloned := make([]semanticObjectCoverage, len(coverages))
	copy(cloned, coverages)
	return cloned
}
