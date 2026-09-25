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
// Author: BaSyx Authors

package auth

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

var (
	// ErrReBACReadGrantInvalid rejects malformed concrete resource grants.
	ErrReBACReadGrantInvalid = errors.New("ReBAC read grant invalid")
	// ErrReBACReadGrantUnsupported identifies a grant that needs a query scope not available to the caller.
	ErrReBACReadGrantUnsupported = errors.New("ReBAC read grant unsupported")
)

// ReBACElementReadGrant identifies one readable submodel element by its
// containing submodel identifier and canonical idShort path.
type ReBACElementReadGrant struct {
	SubmodelID  string
	ElementPath string
}

// ReBACEmbeddedSubmodelDescriptorReadGrant identifies a Submodel Descriptor
// embedded in one specific AAS Descriptor.
type ReBACEmbeddedSubmodelDescriptorReadGrant struct {
	AASDescriptorID string
	SubmodelID      string
}

// ReBACReadGrantSet contains concrete OpenFGA read results for one semantic resource.
type ReBACReadGrantSet struct {
	Identifiers                 []string
	Elements                    []ReBACElementReadGrant
	EmbeddedSubmodelDescriptors []ReBACEmbeddedSubmodelDescriptorReadGrant
}

// UnionAuthorizationEvaluationWithReBACReadGrants returns an immutable union
// of the existing ABAC evaluation and concrete ReBAC read grants. It never
// evaluates ABAC and therefore cannot prefilter the OpenFGA grant set.
func UnionAuthorizationEvaluationWithReBACReadGrants(
	resource SemanticResourceKind,
	evaluation AuthorizationEvaluation,
	grants ReBACReadGrantSet,
) (AuthorizationEvaluation, error) {
	formula, coverage, ok, err := rebacReadGrantFormula(resource, grants)
	if err != nil || !ok {
		return evaluation, err
	}
	filter, err := unionQueryFilterWithReBACReadGrant(resource, evaluation.QueryFilter, evaluation.Allowed, formula, grants)
	if err != nil {
		return AuthorizationEvaluation{}, err
	}
	result := evaluation
	result.Allowed = true
	result.Reason = DecisionAllow
	result.QueryFilter = filter
	result.alternatives = append(cloneGrantAlternatives(evaluation.alternatives), CompiledGrantAlternative{
		ruleID:    "rebac-read-grant",
		formula:   formula,
		coverages: []semanticObjectCoverage{coverage},
	})
	return result, nil
}

// UnionSemanticAccessViewWithReBACReadGrants returns an immutable union of an
// existing semantic view and concrete ReBAC read grants. The added alternative
// has no fragment filters, so a matching grant permits the full selected field.
func UnionSemanticAccessViewWithReBACReadGrants(
	view SemanticAccessView,
	grants ReBACReadGrantSet,
) (SemanticAccessView, error) {
	if view.decision == AccessViewUnrestricted {
		return view, nil
	}
	formula, coverage, ok, err := rebacReadGrantFormula(view.resource, grants)
	if err != nil || !ok {
		return view, err
	}
	filter, err := unionQueryFilterWithReBACReadGrant(view.resource, view.queryFilter, view.decision != AccessViewDenied, formula, grants)
	if err != nil {
		return SemanticAccessView{}, err
	}
	result := view
	result.decision = AccessViewRestricted
	result.queryFilter = filter
	result.alternatives = append(cloneGrantAlternatives(view.alternatives), CompiledGrantAlternative{
		ruleID:    "rebac-read-grant",
		formula:   formula,
		coverages: []semanticObjectCoverage{coverage},
	})
	return result, nil
}

func unionQueryFilterWithReBACReadGrant(
	resource SemanticResourceKind,
	existing *QueryFilter,
	abacAllowed bool,
	grant grammar.LogicalExpression,
	grants ReBACReadGrantSet,
) (*QueryFilter, error) {
	if abacAllowed && existing == nil {
		return nil, nil
	}
	filter, err := CloneQueryFilter(existing)
	if err != nil {
		return nil, fmt.Errorf("REBAC-QUERYUNION-CLONEFILTER: %w", err)
	}
	if filter == nil {
		filter = &QueryFilter{}
	}
	if err = configureReBACGrantFormula(filter, resource, abacAllowed, grant, grants); err != nil {
		return nil, err
	}
	if resource != SemanticResourceSMDesc {
		unionReBACFragmentFilters(filter, grant)
	}
	return filter, nil
}

func configureReBACGrantFormula(filter *QueryFilter, resource SemanticResourceKind, abacAllowed bool, grant grammar.LogicalExpression, grants ReBACReadGrantSet) error {
	if resource == SemanticResourceSME || resource == SemanticResourceSMDesc {
		if filter.Formula == nil {
			base := boolExpression(abacAllowed)
			filter.Formula = &base
		}
		if filter.FormulasByRight == nil {
			filter.FormulasByRight = map[grammar.RightsEnum]grammar.LogicalExpression{}
		}
		filter.FormulasByRight[grammar.RightsEnumREAD] = *filter.Formula
	}
	switch resource {
	case SemanticResourceSME:
		filter.SMERowFormula = &grant
	case SemanticResourceSMDesc:
		standalone, embedded, err := rebacSubmodelDescriptorGrantFormulas(grants)
		if err != nil {
			return err
		}
		filter.SMDescStandaloneFormula = standalone
		filter.SMDescEmbeddedFormula = embedded
	default:
		configureStandardReBACGrantFormula(filter, abacAllowed, grant)
	}
	return nil
}

func configureStandardReBACGrantFormula(filter *QueryFilter, abacAllowed bool, grant grammar.LogicalExpression) {
	if !abacAllowed {
		filter.Formula = &grant
	} else if filter.Formula != nil {
		combined := orReBACGrantFormula(*filter.Formula, grant)
		filter.Formula = &combined
	}
	if filter.FormulasByRight == nil {
		if !abacAllowed {
			filter.FormulasByRight = map[grammar.RightsEnum]grammar.LogicalExpression{grammar.RightsEnumREAD: grant}
		}
		return
	}
	if readFormula, found := filter.FormulasByRight[grammar.RightsEnumREAD]; found {
		filter.FormulasByRight[grammar.RightsEnumREAD] = orReBACGrantFormula(readFormula, grant)
		return
	}
	filter.FormulasByRight[grammar.RightsEnumREAD] = grant
}

func unionReBACFragmentFilters(filter *QueryFilter, grant grammar.LogicalExpression) {
	for fragment, predicate := range filter.Filters {
		filter.Filters[fragment] = OrFragmentFilterPredicates(predicate, NewFragmentFilterPredicate(grant, false))
	}
}

func orReBACGrantFormula(abac, grant grammar.LogicalExpression) grammar.LogicalExpression {
	return grammar.LogicalExpression{Or: []grammar.LogicalExpression{abac, grant}}
}

func rebacReadGrantFormula(
	resource SemanticResourceKind,
	grants ReBACReadGrantSet,
) (grammar.LogicalExpression, semanticObjectCoverage, bool, error) {
	if resource == SemanticResourceSME {
		return rebacElementReadGrantFormula(grants)
	}
	if resource == SemanticResourceSMDesc {
		standalone, embedded, err := rebacSubmodelDescriptorGrantFormulas(grants)
		if err != nil {
			return grammar.LogicalExpression{}, semanticObjectCoverage{}, false, err
		}
		if standalone == nil && embedded == nil {
			return grammar.LogicalExpression{}, semanticObjectCoverage{}, false, nil
		}
		expressions := make([]grammar.LogicalExpression, 0, 2)
		if standalone != nil {
			expressions = append(expressions, *standalone)
		}
		if embedded != nil {
			expressions = append(expressions, *embedded)
		}
		return rebacOrFormula(expressions), rebacFullCoverage(resource), true, nil
	}
	if len(grants.Elements) > 0 {
		return grammar.LogicalExpression{}, semanticObjectCoverage{}, false, fmt.Errorf("REBAC-QUERYUNION-ELEMENTS %w for %s", ErrReBACReadGrantInvalid, resource)
	}
	field, found := rebacResourceIdentifierField(resource)
	if !found {
		return grammar.LogicalExpression{}, semanticObjectCoverage{}, false, fmt.Errorf("REBAC-QUERYUNION-RESOURCE %w: %s", ErrReBACReadGrantInvalid, resource)
	}
	identifiers := uniqueReBACStrings(grants.Identifiers)
	if len(identifiers) == 0 {
		return grammar.LogicalExpression{}, semanticObjectCoverage{}, false, nil
	}
	return rebacIdentifierFormula(field, identifiers), rebacFullCoverage(resource), true, nil
}

func rebacSubmodelDescriptorGrantFormulas(grants ReBACReadGrantSet) (*grammar.LogicalExpression, *grammar.LogicalExpression, error) {
	if len(grants.Elements) > 0 {
		return nil, nil, fmt.Errorf("REBAC-QUERYUNION-SMDESCELEMENT %w", ErrReBACReadGrantInvalid)
	}
	var standalone *grammar.LogicalExpression
	if identifiers := uniqueReBACStrings(grants.Identifiers); len(identifiers) > 0 {
		formula := rebacIdentifierFormula("$smdesc#id", identifiers)
		standalone = &formula
	}
	embeddedGrants := uniqueReBACEmbeddedSubmodelDescriptorReadGrants(grants.EmbeddedSubmodelDescriptors)
	if len(embeddedGrants) == 0 {
		return standalone, nil, nil
	}
	expressions := make([]grammar.LogicalExpression, 0, len(embeddedGrants))
	for _, embedded := range embeddedGrants {
		aasField := grammar.ModelStringPattern("$aasdesc#id")
		aasValue := grammar.StandardString(embedded.AASDescriptorID)
		submodelField := grammar.ModelStringPattern("$smdesc#id")
		submodelValue := grammar.StandardString(embedded.SubmodelID)
		expressions = append(expressions, grammar.LogicalExpression{And: []grammar.LogicalExpression{
			{Eq: grammar.ComparisonItems{{Field: &aasField}, {StrVal: &aasValue}}},
			{Eq: grammar.ComparisonItems{{Field: &submodelField}, {StrVal: &submodelValue}}},
		}})
	}
	formula := rebacOrFormula(expressions)
	return standalone, &formula, nil
}

func uniqueReBACEmbeddedSubmodelDescriptorReadGrants(values []ReBACEmbeddedSubmodelDescriptorReadGrant) []ReBACEmbeddedSubmodelDescriptorReadGrant {
	seen := make(map[string]ReBACEmbeddedSubmodelDescriptorReadGrant, len(values))
	for _, value := range values {
		value.AASDescriptorID = strings.TrimSpace(value.AASDescriptorID)
		value.SubmodelID = strings.TrimSpace(value.SubmodelID)
		if value.AASDescriptorID != "" && value.SubmodelID != "" {
			seen[value.AASDescriptorID+"\x00"+value.SubmodelID] = value
		}
	}
	result := make([]ReBACEmbeddedSubmodelDescriptorReadGrant, 0, len(seen))
	for _, value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(first, second int) bool {
		if result[first].AASDescriptorID == result[second].AASDescriptorID {
			return result[first].SubmodelID < result[second].SubmodelID
		}
		return result[first].AASDescriptorID < result[second].AASDescriptorID
	})
	return result
}

func rebacResourceIdentifierField(resource SemanticResourceKind) (grammar.ModelStringPattern, bool) {
	switch resource {
	case SemanticResourceAAS:
		return "$aas#id", true
	case SemanticResourceSM:
		return "$sm#id", true
	case SemanticResourceAASDesc:
		return "$aasdesc#id", true
	case SemanticResourceSMDesc:
		return "$smdesc#id", true
	case SemanticResourceCD:
		return "$cd#id", true
	case SemanticResourceBD:
		return "$bd#aasId", true
	default:
		return "", false
	}
}

func rebacElementReadGrantFormula(grants ReBACReadGrantSet) (grammar.LogicalExpression, semanticObjectCoverage, bool, error) {
	if len(grants.Identifiers) > 0 {
		return grammar.LogicalExpression{}, semanticObjectCoverage{}, false, fmt.Errorf("REBAC-QUERYUNION-SMEIDENTIFIER %w", ErrReBACReadGrantInvalid)
	}
	elements := uniqueReBACElementReadGrants(grants.Elements)
	if len(elements) == 0 {
		return grammar.LogicalExpression{}, semanticObjectCoverage{}, false, nil
	}
	expressions := make([]grammar.LogicalExpression, 0, len(elements))
	for _, element := range elements {
		expressions = append(expressions, rebacElementFormula(element))
	}
	coverage := semanticObjectCoverage{
		resource:           SemanticResourceSME,
		representation:     semanticRouteFull,
		rebacElementGrants: append([]ReBACElementReadGrant(nil), elements...),
	}
	return rebacOrFormula(expressions), coverage, true, nil
}

func rebacElementFormula(grant ReBACElementReadGrant) grammar.LogicalExpression {
	pathField := grammar.ModelStringPattern("$sme." + grant.ElementPath + "#idShort")
	pathValue := grammar.StandardString(rebacElementPathBasename(grant.ElementPath))
	submodelField := grammar.ModelStringPattern("$sm#id")
	submodelValue := grammar.StandardString(grant.SubmodelID)
	return grammar.LogicalExpression{And: []grammar.LogicalExpression{
		{Eq: grammar.ComparisonItems{{Field: &pathField}, {StrVal: &pathValue}}},
		{Eq: grammar.ComparisonItems{{Field: &submodelField}, {StrVal: &submodelValue}}},
	}}
}

func uniqueReBACElementReadGrants(values []ReBACElementReadGrant) []ReBACElementReadGrant {
	seen := make(map[string]ReBACElementReadGrant, len(values))
	for _, value := range values {
		value.SubmodelID = strings.TrimSpace(value.SubmodelID)
		value.ElementPath = strings.TrimSpace(value.ElementPath)
		if value.SubmodelID == "" || !rebacElementPathValid(value.ElementPath) {
			continue
		}
		seen[value.SubmodelID+"\x00"+value.ElementPath] = value
	}
	result := make([]ReBACElementReadGrant, 0, len(seen))
	for _, value := range seen {
		result = append(result, value)
	}
	sort.Slice(result, func(first, second int) bool {
		if result[first].SubmodelID == result[second].SubmodelID {
			return result[first].ElementPath < result[second].ElementPath
		}
		return result[first].SubmodelID < result[second].SubmodelID
	})
	return result
}

func rebacElementPathValid(path string) bool {
	if path == "" || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") || strings.Contains(path, "..") {
		return false
	}
	field := grammar.ModelStringPattern("$sme." + path + "#idShort")
	_, err := grammar.ResolveScalarFieldToSQL(&field)
	return err == nil
}

func rebacElementPathBasename(path string) string {
	segment := path[strings.LastIndex(path, ".")+1:]
	return strings.SplitN(segment, "[", 2)[0]
}

func rebacIdentifierFormula(field grammar.ModelStringPattern, identifiers []string) grammar.LogicalExpression {
	expressions := make([]grammar.LogicalExpression, 0, len(identifiers))
	for _, identifier := range identifiers {
		fieldCopy := field
		value := grammar.StandardString(identifier)
		expressions = append(expressions, grammar.LogicalExpression{Eq: grammar.ComparisonItems{{Field: &fieldCopy}, {StrVal: &value}}})
	}
	return rebacOrFormula(expressions)
}

func rebacOrFormula(expressions []grammar.LogicalExpression) grammar.LogicalExpression {
	if len(expressions) == 1 {
		return expressions[0]
	}
	return grammar.LogicalExpression{Or: expressions}
}

func rebacFullCoverage(resource SemanticResourceKind) semanticObjectCoverage {
	coverage := semanticObjectCoverage{resource: resource, representation: semanticRouteFull}
	if resource == SemanticResourceSM || resource == SemanticResourceSME {
		coverage.allSubmodelIDs = true
	}
	if resource == SemanticResourceSME {
		coverage.allSubmodelElements = true
	}
	return coverage
}

func uniqueReBACStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
