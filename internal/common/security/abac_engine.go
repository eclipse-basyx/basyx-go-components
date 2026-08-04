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
	"fmt"
	"sort"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/builder"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	api "github.com/go-chi/chi/v5"
	jsoniter "github.com/json-iterator/go"
)

// AccessModel is an evaluated, in-memory representation of the Access Rule Model
// (ARM) used by the ABAC engine. It holds the generated schema and provides
// evaluation helpers.
type AccessModel struct {
	gen       grammar.AccessRuleModelSchemaJSON
	apiRouter *api.Mux
	rules     []materializedRule
	basePath  string
	policyID  string
}

type materializedRule struct {
	id         string
	acl        grammar.ACL
	attrs      []grammar.AttributeItem
	objs       []grammar.ObjectItem
	lexpr      *grammar.LogicalExpression
	filterList []grammar.AccessPermissionRuleFILTER
}

// PolicyID returns the deterministic identifier of the policy version used to
// compile this access model. File-only compatibility models may return an empty
// value.
func (m *AccessModel) PolicyID() string {
	if m == nil {
		return ""
	}
	return m.policyID
}

// WithPolicyID stores the durable policy identifier on a compiled access model.
//
// The method returns the receiver so callers can chain it while building
// repository-backed models.
func (m *AccessModel) WithPolicyID(policyID string) *AccessModel {
	if m == nil {
		return nil
	}
	m.policyID = policyID
	return m
}

// ParseAccessModel parses a JSON (or YAML converted to JSON) payload that
// conforms to the Access Rule Model schema and returns a compiled AccessModel.
func ParseAccessModel(b []byte, apiRouter *api.Mux, basePath string) (*AccessModel, error) {
	var m grammar.AccessRuleModelSchemaJSON
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse access model: %w", err)
	}

	rules, err := materializeRules(m.AllAccessPermissionRules)
	if err != nil {
		return nil, fmt.Errorf("parse access model: %w", err)
	}

	return &AccessModel{
		gen:       m,
		apiRouter: apiRouter,
		rules:     rules,
		basePath:  basePath,
	}, nil
}

// QueryFilter captures optional, fine-grained restrictions produced by a rule
// even when ACCESS=ALLOW. Controllers can use it to restrict rows, constrain
// mutations, or redact fields. The Discovery Service currently does not require
// a concrete filter structure; extend this struct when needed.
type QueryFilter struct {
	Formula         *grammar.LogicalExpression                       `json:"Formula,omitempty" yaml:"Formula,omitempty" mapstructure:"Formula,omitempty"`
	FormulasByRight map[grammar.RightsEnum]grammar.LogicalExpression `json:"FormulasByRight,omitempty" yaml:"FormulasByRight,omitempty" mapstructure:"FormulasByRight,omitempty"`
	Filters         FragmentFilters                                  `json:"Filters,omitempty" yaml:"Filters,omitempty" mapstructure:"Filters,omitempty"`
}

// DecisionCode represents the result of an authorization check.
// It is serialized as a JSON string for consistent use in controller
// responses and API payloads.
type DecisionCode string

const (
	// DecisionAllow indicates that the authorization check succeeded
	// and the requested action is permitted.
	DecisionAllow DecisionCode = "ALLOW"

	// DecisionNoMatch indicates that no matching rule or policy was found
	// for the authorization check, resulting in a neutral or deny outcome.
	DecisionNoMatch DecisionCode = "NO_MATCH"

	// DecisionRouteNotFound indicates the requested method/path does not match
	// any registered route and should be handled by the router (e.g., 404).
	DecisionRouteNotFound DecisionCode = "ROUTE_NOT_FOUND"
)

// AuthorizationEvaluation records the detailed outcome of an ABAC evaluation.
//
// ABACMiddleware uses this richer result to pass allow metadata to audit
// middleware while preserving the older AuthorizeWithFilter return contract for
// callers that only need access and query-filter decisions.
type AuthorizationEvaluation struct {
	// Allowed is true when the request is permitted by at least one allow rule.
	Allowed bool

	// Reason describes the authorization result, including deny/pass-through cases.
	Reason DecisionCode

	// QueryFilter contains optional backend filter constraints for allowed requests.
	QueryFilter *QueryFilter

	// PolicyID identifies the active policy version that produced the decision.
	PolicyID string

	// MatchedRuleID contains deterministic IDs for matched allow rules.
	//
	// Multiple IDs are comma-separated in configured rule order. The field is
	// empty when no allow rule matched or rule metadata is unavailable.
	MatchedRuleID string
}

// AuthorizeWithFilter evaluates the request against the model rules in order.
// It returns whether access is allowed, a human-readable reason, and an optional
// QueryFilter for controllers to enforce (e.g., tenant scoping, redactions).
// It is important that this function does not throw errors. It either gives access or no access.
//
//nolint:revive // i will refactor this function at some point
func (m *AccessModel) AuthorizeWithFilter(in EvalInput) (bool, DecisionCode, *QueryFilter) {
	result := m.AuthorizeWithFilterWithOptions(in, grammar.DefaultSimplifyOptions())
	return result.Allowed, result.Reason, result.QueryFilter
}

// AuthorizeWithFilterWithOptions evaluates the request and returns audit-friendly metadata.
//
// The evaluation semantics are identical to AuthorizeWithFilter, but callers can
// control backend simplification behavior, for example implicit casts. In
// addition to the allow decision and optional query filter, the returned value
// contains the deterministic matched rule identifiers used by history audit
// enrichment.
//
// Parameters:
//   - in: Request method, path, and claims used by the ABAC evaluator.
//   - opts: Simplification options for backend filter generation.
//
// Returns:
//   - AuthorizationEvaluation: Access decision, decision reason, optional query
//     filter, and comma-separated matched allow rule IDs.
//
// Example:
//
//	opts := grammar.DefaultSimplifyOptions()
//	opts.EnableImplicitCasts = true
//	result := model.AuthorizeWithFilterWithOptions(input, opts)
//	if result.Allowed {
//		ctx = ContextWithAuthorizationDecision(ctx, AuthorizationDecision{
//			Result:        string(DecisionAllow),
//			MatchedRuleID: result.MatchedRuleID,
//		})
//	}
//
// nolint:revive // This function is the heart of ABAC and is complicated. Sorry cognitive-complexity!
func (m *AccessModel) AuthorizeWithFilterWithOptions(in EvalInput, opts grammar.SimplifyOptions) AuthorizationEvaluation {
	rightAlternatives, mapped, routeFound := m.mapMethodAndPathToRights(in)
	if !routeFound {
		return AuthorizationEvaluation{Reason: DecisionRouteNotFound}
	}
	if !mapped {
		return AuthorizationEvaluation{Reason: DecisionNoMatch}
	}

	var ruleExprs []QueryFilter
	allFragments := make(map[grammar.FragmentStringPattern]struct{})
	matchedRuleIDs := make([]string, 0, len(m.rules))
	relevantRights := collectRelevantRights(rightAlternatives)
	ruleExprsByRight := make(map[grammar.RightsEnum][]grammar.LogicalExpression, len(relevantRights))

	for _, r := range m.rules {
		acl, attrs, objs, lexpr := r.acl, r.attrs, r.objs, r.lexpr
		// Gate 0: check disabled
		if acl.ACCESS == grammar.ACLACCESSDISABLED {
			continue
		}
		// Gate 1: rights
		if !rightsContainsAny(acl.RIGHTS, rightAlternatives) {
			continue
		}
		// Gate 2: attributes
		if !attributesSatisfiedAll(attrs, in.Claims) {
			continue
		}
		// Gate 3: objects
		accessWithOptinalFilter := matchRouteObjectsObjItem(objs, in.Path, m.basePath)
		if !accessWithOptinalFilter.access {
			continue
		}

		combinedLE := lexpr
		if accessWithOptinalFilter.le != nil {
			if combinedLE == nil {
				// no rule formula -> use route formula as-is
				combinedLE = accessWithOptinalFilter.le
			} else {
				// wrap both in an AND
				andExpr := grammar.LogicalExpression{
					And: []grammar.LogicalExpression{
						*combinedLE,
						*accessWithOptinalFilter.le,
					},
				}
				combinedLE = &andExpr
			}
		}

		// Gate 4: formula → adapt for backend filtering
		if combinedLE == nil {
			// rule has no formula: should not happen -> deny access
			return AuthorizationEvaluation{Reason: DecisionNoMatch}
		}

		resolver := func(attr grammar.AttributeValue) any {
			return resolveAttributeValue(attr, in.Claims)
		}
		adapted, decision := combinedLE.SimplifyForBackendFilterWithOptions(resolver, opts)
		if decision == grammar.SimplifyFalse {
			continue
		}
		if r.id != "" {
			matchedRuleIDs = append(matchedRuleIDs, r.id)
		}
		fragments := make(FragmentFilters)
		for _, filter := range r.filterList {
			fragment := *filter.FRAGMENT
			matchMode := filter.MATCH != nil && *filter.MATCH
			simplifiedCondition, _ := filter.CONDITION.SimplifyForBackendFilterWithOptions(resolver, opts)
			condition := NewFragmentFilterPredicate(simplifiedCondition, matchMode)
			if existing, ok := fragments[fragment]; ok {
				fragments[fragment] = AndFragmentFilterPredicates(existing, condition)
			} else {
				fragments[fragment] = condition
			}
			allFragments[fragment] = struct{}{}
		}

		for _, right := range relevantRights {
			if !ruleAllowsRight(acl.RIGHTS, right) {
				continue
			}
			ruleExprsByRight[right] = append(ruleExprsByRight[right], adapted)
		}

		ruleExprs = append(ruleExprs, QueryFilter{
			Formula: &adapted,
			Filters: fragments,
		})
	}

	if len(ruleExprs) == 0 {
		return AuthorizationEvaluation{Reason: DecisionNoMatch}
	}

	combined := grammar.LogicalExpression{Or: []grammar.LogicalExpression{}}
	combinedFragments := mergeRuleFragmentFilters(ruleExprs, allFragments)
	for _, qfr := range ruleExprs {
		combined.Or = append(combined.Or, *qfr.Formula)
	}

	resolver := func(attr grammar.AttributeValue) any {
		return resolveAttributeValue(attr, in.Claims)
	}
	simplified, decision := combined.SimplifyForBackendFilterWithOptions(resolver, opts)
	combinedByRight := make(map[grammar.RightsEnum]grammar.LogicalExpression, len(relevantRights))
	for _, right := range relevantRights {
		combinedByRight[right] = boolExpression(false)
		expressions := ruleExprsByRight[right]
		if len(expressions) == 0 {
			continue
		}

		combinedRight := grammar.LogicalExpression{
			Or: expressions,
		}
		simplifiedRight, rightDecision := combinedRight.SimplifyForBackendFilterWithOptions(resolver, opts)
		switch rightDecision {
		case grammar.SimplifyFalse:
			combinedByRight[right] = boolExpression(false)
		case grammar.SimplifyTrue:
			combinedByRight[right] = boolExpression(true)
		default:
			combinedByRight[right] = simplifiedRight
		}
	}

	hasFormula := true
	switch decision {
	case grammar.SimplifyFalse:
		return AuthorizationEvaluation{Reason: DecisionNoMatch}
	case grammar.SimplifyTrue:
		hasFormula = false
	}

	hasRightScopedFormulas := len(combinedByRight) > 0
	var qf *QueryFilter
	if hasFormula || len(combinedFragments) > 0 || hasRightScopedFormulas {
		qf = &QueryFilter{}
		if hasFormula {
			qf.Formula = &simplified
		}
		if hasRightScopedFormulas {
			qf.FormulasByRight = combinedByRight
		}
		if len(combinedFragments) > 0 {
			qf.Filters = combinedFragments
		}
	}

	return AuthorizationEvaluation{
		Allowed:       true,
		Reason:        DecisionAllow,
		QueryFilter:   qf,
		PolicyID:      m.policyID,
		MatchedRuleID: strings.Join(matchedRuleIDs, ","),
	}
}

func rightsContainsAny(hay []grammar.RightsEnum, alternatives [][]grammar.RightsEnum) bool {
	for _, rights := range alternatives {
		for _, right := range rights {
			if ruleAllowsRight(hay, right) {
				return true
			}
		}
	}
	return false
}

func collectRelevantRights(alternatives [][]grammar.RightsEnum) []grammar.RightsEnum {
	seen := make(map[grammar.RightsEnum]struct{})
	rights := make([]grammar.RightsEnum, 0)
	for _, alternative := range alternatives {
		for _, right := range alternative {
			if _, ok := seen[right]; ok {
				continue
			}
			seen[right] = struct{}{}
			rights = append(rights, right)
		}
	}
	return rights
}

func ruleAllowsRight(ruleRights []grammar.RightsEnum, right grammar.RightsEnum) bool {
	for _, ruleRight := range ruleRights {
		if ruleRight == grammar.RightsEnumALL || ruleRight == right {
			return true
		}
	}
	return false
}

func boolExpression(v bool) grammar.LogicalExpression {
	return grammar.LogicalExpression{
		Boolean: &v,
	}
}

type fragmentShapeGroup struct {
	fragment  grammar.FragmentStringPattern
	fragments []grammar.FragmentStringPattern
}

func mergeRuleFragmentFilters(
	ruleExprs []QueryFilter,
	allFragments map[grammar.FragmentStringPattern]struct{},
) FragmentFilters {
	combined := make(FragmentFilters)
	for _, group := range groupFragmentsByShape(allFragments) {
		alternatives := make([]FragmentFilterPredicate, 0, len(ruleExprs))
		for _, ruleExpr := range ruleExprs {
			alternative := []FragmentFilterPredicate{newGlobalFragmentFilterPredicate(*ruleExpr.Formula)}
			for _, fragment := range group.fragments {
				if predicate, ok := ruleExpr.Filters[fragment]; ok {
					alternative = append(alternative, scopeFragmentFilterPredicate(predicate, fragment))
				}
			}
			alternatives = append(alternatives, AndFragmentFilterPredicates(alternative...))
		}
		predicate := OrFragmentFilterPredicates(alternatives...)
		if unrestricted, constant := predicate.globalBooleanValue(); constant && unrestricted {
			continue
		}
		combined[group.fragment] = predicate
	}
	return combined
}

func groupFragmentsByShape(fragments map[grammar.FragmentStringPattern]struct{}) []fragmentShapeGroup {
	groupsByShape := make(map[string]*fragmentShapeGroup)
	for fragment := range fragments {
		shape := fragmentShapeKey(fragment)
		group, ok := groupsByShape[shape]
		if !ok {
			group = &fragmentShapeGroup{fragment: wildcardFragment(fragment)}
			groupsByShape[shape] = group
		}
		group.fragments = append(group.fragments, fragment)
	}

	shapes := make([]string, 0, len(groupsByShape))
	for shape := range groupsByShape {
		shapes = append(shapes, shape)
	}
	sort.Strings(shapes)

	groups := make([]fragmentShapeGroup, 0, len(shapes))
	for _, shape := range shapes {
		group := groupsByShape[shape]
		sort.Slice(group.fragments, func(i, j int) bool {
			iIndexes := indexedFragmentTokenCount(group.fragments[i])
			jIndexes := indexedFragmentTokenCount(group.fragments[j])
			if iIndexes != jIndexes {
				return iIndexes < jIndexes
			}
			return group.fragments[i] < group.fragments[j]
		})
		groups = append(groups, *group)
	}
	return groups
}

func indexedFragmentTokenCount(fragment grammar.FragmentStringPattern) int {
	count := 0
	for _, token := range builder.TokenizeField(string(fragment)) {
		array, isArray := token.(builder.ArrayToken)
		if isArray && array.Index >= 0 {
			count++
		}
	}
	return count
}

func fragmentShapeKey(fragment grammar.FragmentStringPattern) string {
	parts := []string{fragmentRoot(fragment)}
	for _, token := range builder.TokenizeField(string(fragment)) {
		kind := "simple"
		if _, isArray := token.(builder.ArrayToken); isArray {
			kind = "array"
		}
		parts = append(parts, fmt.Sprintf("|%s:%d:%s", kind, len(token.GetName()), token.GetName()))
	}
	return strings.Join(parts, "")
}

func wildcardFragment(fragment grammar.FragmentStringPattern) grammar.FragmentStringPattern {
	value := string(fragment)
	wildcard := make([]byte, 0, len(value))
	for len(value) > 0 {
		open := strings.IndexByte(value, '[')
		if open < 0 {
			wildcard = append(wildcard, value...)
			break
		}
		closeOffset := strings.IndexByte(value[open:], ']')
		if closeOffset < 0 {
			return fragment
		}
		wildcard = append(wildcard, value[:open]...)
		wildcard = append(wildcard, '[', ']')
		value = value[open+closeOffset+1:]
	}
	return grammar.FragmentStringPattern(string(wildcard))
}

// FilterPredicateEntriesFor returns all (fragment, predicate) pairs from
// q.Filters whose tokenized fragment path matches the tokenized `key`.
//
// A fragment matches when, for every token position i:
//   - token name matches (Token.GetName()), and
//   - token kind matches (ArrayToken vs SimpleToken).
//
// For ArrayToken positions, the array index is ignored. This means a wildcard
// key such as "...[]" matches entries like "...[0]", "...[1]", etc.
//
// Note: the returned slice order is not guaranteed (q.Filters is a map).
func (q *QueryFilter) FilterPredicateEntriesFor(key grammar.FragmentStringPattern) []FragmentFilterEntry {
	var out []FragmentFilterEntry
	keyTokens := builder.TokenizeField(string(key))

	for k, expr := range q.Filters {
		if !fragmentRootsEqual(k, key) {
			continue
		}
		kTokens := builder.TokenizeField(string(k))

		if len(kTokens) != len(keyTokens) {
			continue
		}

		matches := true
		for i := 0; i < len(kTokens); i++ {
			if kTokens[i].GetName() != keyTokens[i].GetName() {
				matches = false
				break
			}

			_, kIsArray := kTokens[i].(builder.ArrayToken)
			_, keyIsArray := keyTokens[i].(builder.ArrayToken)
			if kIsArray != keyIsArray {
				matches = false
				break
			}
			// If both are ArrayToken we intentionally ignore the ArrayToken.Index.
		}

		if matches {
			out = append(out, FragmentFilterEntry{
				Fragment:  k,
				Predicate: expr,
			})
		}
	}

	return out
}

func fragmentRootsEqual(first grammar.FragmentStringPattern, second grammar.FragmentStringPattern) bool {
	return fragmentRoot(first) == fragmentRoot(second)
}

func fragmentRoot(fragment grammar.FragmentStringPattern) string {
	fragmentString := string(fragment)
	fragmentSeparator := strings.IndexAny(fragmentString, ".[#")
	if fragmentSeparator < 0 {
		return fragmentString
	}
	return fragmentString[:fragmentSeparator]
}
