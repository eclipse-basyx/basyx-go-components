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
	"fmt"
	"slices"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/google/uuid"
)

type reBACGrantsContextKey struct{}

// ReBACGrantSet records the resources that a relationship-based authorization
// decision allows for one request. Every entry is bound to its own resource
// kind and rights, so a grant never widens another table touched by the same
// request. Build the set completely before publishing it with WithReBACGrants.
type ReBACGrantSet struct {
	defaultRights []grammar.RightsEnum
	entries       []reBACGrantEntry
}

type reBACGrantEntry struct {
	resource    SemanticResourceKind
	authUUIDs   []string
	allOfKind   bool
	elementPath string
	// query selects granted authorization UUIDs, or for SubmodelElements
	// (submodel_uuid, element_path) pairs. Datasets are immutable.
	query  *goqu.SelectDataset
	rights []grammar.RightsEnum
}

// NewReBACGrantSet creates an empty grant set. defaultRights are the rights of
// the matched route; they apply whenever a backend did not select a specific
// right with SelectFormulaForRight.
func NewReBACGrantSet(defaultRights ...grammar.RightsEnum) *ReBACGrantSet {
	return &ReBACGrantSet{defaultRights: slices.Clone(defaultRights)}
}

// AllowResources grants rights on concrete identifiables of one kind.
// Supported kinds are SemanticResourceAAS, SemanticResourceSM and
// SemanticResourceCD. Invalid UUIDs are rejected so they can never be
// interpolated into SQL.
func (s *ReBACGrantSet) AllowResources(resource SemanticResourceKind, authUUIDs []string, rights ...grammar.RightsEnum) error {
	if !isReBACIdentifiableKind(resource) {
		return fmt.Errorf("AUTH-REBACGRANT-KIND unsupported resource kind %q", resource)
	}
	normalized, err := normalizeReBACUUIDs(authUUIDs)
	if err != nil {
		return err
	}
	if len(normalized) == 0 || len(rights) == 0 {
		return nil
	}
	s.entries = append(s.entries, reBACGrantEntry{resource: resource, authUUIDs: normalized, rights: slices.Clone(rights)})
	return nil
}

// AllowAllOfKind grants rights on every identifiable of one kind. It is used
// for repository-level relations such as creator or administrator.
func (s *ReBACGrantSet) AllowAllOfKind(resource SemanticResourceKind, rights ...grammar.RightsEnum) error {
	if !isReBACIdentifiableKind(resource) {
		return fmt.Errorf("AUTH-REBACGRANT-KIND unsupported resource kind %q", resource)
	}
	if len(rights) == 0 {
		return nil
	}
	s.entries = append(s.entries, reBACGrantEntry{resource: resource, allOfKind: true, rights: slices.Clone(rights)})
	return nil
}

// AllowSubmodelElement grants rights on one SubmodelElement subtree, identified
// by the Submodel authorization UUID and the idShortPath of its root. The
// grant covers the element and its descendants, never its ancestors, siblings
// or the containing Submodel.
func (s *ReBACGrantSet) AllowSubmodelElement(submodelAuthUUID string, idShortPath string, rights ...grammar.RightsEnum) error {
	normalized, err := normalizeReBACUUIDs([]string{submodelAuthUUID})
	if err != nil {
		return err
	}
	idShortPath = strings.TrimSpace(idShortPath)
	if idShortPath == "" {
		return fmt.Errorf("AUTH-REBACGRANT-ELEMENTPATH idShortPath must not be empty")
	}
	if len(rights) == 0 {
		return nil
	}
	s.entries = append(s.entries, reBACGrantEntry{
		resource:    SemanticResourceSME,
		authUUIDs:   normalized,
		elementPath: idShortPath,
		rights:      slices.Clone(rights),
	})
	return nil
}

// AllowQueriedResources grants rights on the identifiables of one kind whose
// authorization UUIDs the query selects in its only column. The query is
// embedded into the backend SQL, so lists need no materialized allowlist.
func (s *ReBACGrantSet) AllowQueriedResources(resource SemanticResourceKind, query *goqu.SelectDataset, rights ...grammar.RightsEnum) error {
	if !isReBACIdentifiableKind(resource) {
		return fmt.Errorf("AUTH-REBACGRANT-KIND unsupported resource kind %q", resource)
	}
	if query == nil || len(rights) == 0 {
		return nil
	}
	s.entries = append(s.entries, reBACGrantEntry{resource: resource, query: query, rights: slices.Clone(rights)})
	return nil
}

// AllowQueriedSubmodelElements grants rights on the SubmodelElement subtrees
// whose (submodel_uuid, element_path) pairs the query selects.
func (s *ReBACGrantSet) AllowQueriedSubmodelElements(query *goqu.SelectDataset, rights ...grammar.RightsEnum) error {
	if query == nil || len(rights) == 0 {
		return nil
	}
	s.entries = append(s.entries, reBACGrantEntry{resource: SemanticResourceSME, query: query, rights: slices.Clone(rights)})
	return nil
}

// IsEmpty reports whether the set grants nothing.
func (s *ReBACGrantSet) IsEmpty() bool {
	return s == nil || len(s.entries) == 0
}

// DefaultRights returns the route rights the set was created for.
func (s *ReBACGrantSet) DefaultRights() []grammar.RightsEnum {
	if s == nil {
		return nil
	}
	return slices.Clone(s.defaultRights)
}

func (s *ReBACGrantSet) clone() *ReBACGrantSet {
	if s == nil {
		return nil
	}
	cloned := &ReBACGrantSet{
		defaultRights: slices.Clone(s.defaultRights),
		entries:       make([]reBACGrantEntry, len(s.entries)),
	}
	for index, entry := range s.entries {
		entry.authUUIDs = slices.Clone(entry.authUUIDs)
		entry.rights = slices.Clone(entry.rights)
		cloned.entries[index] = entry
	}
	return cloned
}

// WithReBACGrants publishes an immutable copy of grants for the request. An
// empty set leaves ctx unchanged so ABAC-only requests stay byte-identical.
func WithReBACGrants(ctx context.Context, grants *ReBACGrantSet) context.Context {
	if grants.IsEmpty() {
		return ctx
	}
	return context.WithValue(ctx, reBACGrantsContextKey{}, grants.clone())
}

// ReBACGrantsFromContext returns the published grant set or nil.
func ReBACGrantsFromContext(ctx context.Context) *ReBACGrantSet {
	if ctx == nil {
		return nil
	}
	grants, _ := ctx.Value(reBACGrantsContextKey{}).(*ReBACGrantSet)
	return grants
}

func isReBACIdentifiableKind(resource SemanticResourceKind) bool {
	return resource == SemanticResourceAAS || resource == SemanticResourceSM || resource == SemanticResourceCD
}

func normalizeReBACUUIDs(values []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		parsed, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("AUTH-REBACGRANT-UUID invalid authorization UUID: %w", err)
		}
		normalized = append(normalized, parsed.String())
	}
	slices.Sort(normalized)
	return slices.Compact(normalized), nil
}

func activeReBACRights(ctx context.Context, grants *ReBACGrantSet) []grammar.RightsEnum {
	if right, selected := SelectedFormulaRight(ctx); selected {
		return []grammar.RightsEnum{right}
	}
	return grants.defaultRights
}

func (e reBACGrantEntry) grantsAll(rights []grammar.RightsEnum) bool {
	if len(rights) == 0 {
		return false
	}
	for _, right := range rights {
		if !slices.Contains(e.rights, right) {
			return false
		}
	}
	return true
}

func (s *ReBACGrantSet) entriesFor(resource SemanticResourceKind, rights []grammar.RightsEnum) []reBACGrantEntry {
	var matched []reBACGrantEntry
	for _, entry := range s.entries {
		if entry.resource == resource && entry.grantsAll(rights) {
			matched = append(matched, entry)
		}
	}
	return matched
}

// reBACGrantPredicate returns the SQL predicate that selects the collector's
// root rows granted by ReBAC for the active right. The predicate only ever
// references the collector's own root, so grants cannot leak into related
// resources of the same query.
func reBACGrantPredicate(ctx context.Context, collector *grammar.ResolvedFieldPathCollector) (exp.Expression, bool) {
	grants := ReBACGrantsFromContext(ctx)
	if grants.IsEmpty() {
		return nil, false
	}
	root, rootKey, rootAlias, ok := collector.AuthorizationRoot()
	if !ok {
		return nil, false
	}
	rights := activeReBACRights(ctx, grants)
	switch root {
	case grammar.CollectorRootAAS:
		return identifiableGrantPredicate(rootKey, "aas", grants.entriesFor(SemanticResourceAAS, rights))
	case grammar.CollectorRootSM:
		return identifiableGrantPredicate(rootKey, "submodel", grants.entriesFor(SemanticResourceSM, rights))
	case grammar.CollectorRootCD:
		return identifiableGrantPredicate(rootKey, "concept_description", grants.entriesFor(SemanticResourceCD, rights))
	case grammar.CollectorRootSME:
		return submodelElementGrantPredicate(rootAlias, grants.entriesFor(SemanticResourceSM, rights), grants.entriesFor(SemanticResourceSME, rights))
	default:
		return nil, false
	}
}

func identifiableGrantPredicate(rootKey exp.IdentifierExpression, table string, entries []reBACGrantEntry) (exp.Expression, bool) {
	if len(entries) == 0 {
		return nil, false
	}
	var authUUIDs []string
	alternatives := make([]exp.Expression, 0, len(entries))
	for _, entry := range entries {
		switch {
		case entry.allOfKind:
			return goqu.L("TRUE"), true
		case entry.query != nil:
			alternatives = append(alternatives, rootKey.In(queriedRowIDs(table, entry.query)))
		default:
			authUUIDs = append(authUUIDs, entry.authUUIDs...)
		}
	}
	if len(authUUIDs) > 0 {
		alternatives = append(alternatives, rootKey.In(authUUIDRowIDs(table, authUUIDs)))
	}
	return orAlternatives(alternatives)
}

func submodelElementGrantPredicate(
	rowAlias string,
	submodelEntries []reBACGrantEntry,
	elementEntries []reBACGrantEntry,
) (exp.Expression, bool) {
	submodelColumn := goqu.I(rowAlias + ".submodel_id")
	pathColumn := goqu.I(rowAlias + ".idshort_path")
	alternatives := make([]exp.Expression, 0, 1+len(elementEntries))
	if predicate, ok := identifiableGrantPredicate(submodelColumn, "submodel", submodelEntries); ok {
		alternatives = append(alternatives, predicate)
	}
	for _, entry := range elementEntries {
		if entry.query != nil {
			alternatives = append(alternatives, queriedElementCondition(submodelColumn, pathColumn, entry.query))
			continue
		}
		alternatives = append(alternatives, goqu.And(
			submodelColumn.In(authUUIDRowIDs("submodel", entry.authUUIDs)),
			submodelElementPathSubtreeCondition(pathColumn, entry.elementPath),
		))
	}
	return orAlternatives(alternatives)
}

func orAlternatives(alternatives []exp.Expression) (exp.Expression, bool) {
	switch len(alternatives) {
	case 0:
		return nil, false
	case 1:
		return alternatives[0], true
	default:
		return goqu.Or(alternatives...), true
	}
}

func authUUIDRowIDs(table string, authUUIDs []string) *goqu.SelectDataset {
	alias := "rebac_granted_" + table
	return goqu.Dialect("postgres").
		From(goqu.T(table).As(alias)).
		Select(goqu.I(alias + ".id")).
		Where(goqu.L("? = ANY(?::uuid[])", goqu.I(alias+".auth_uuid"), "{"+strings.Join(authUUIDs, ",")+"}"))
}

func queriedRowIDs(table string, query *goqu.SelectDataset) *goqu.SelectDataset {
	alias := "rebac_granted_" + table
	return goqu.Dialect("postgres").
		From(goqu.T(table).As(alias)).
		Select(goqu.I(alias + ".id")).
		Where(goqu.I(alias + ".auth_uuid").In(query))
}

// queriedElementCondition matches rows below any granted element path of
// their Submodel. Paths from the database are escaped for LIKE in SQL.
func queriedElementCondition(submodelColumn exp.IdentifierExpression, pathColumn exp.IdentifierExpression, query *goqu.SelectDataset) exp.Expression {
	const grant = "rebac_granted_element"
	grantPath := goqu.I(grant + ".element_path")
	escaped := goqu.L("replace(replace(replace(?, '!', '!!'), '%', '!%'), '_', '!_')", grantPath)
	granted := goqu.Dialect("postgres").
		From(query.As(grant)).
		InnerJoin(goqu.T("submodel").As("rebac_granted_submodel"),
			goqu.On(goqu.I("rebac_granted_submodel.auth_uuid").Eq(goqu.I(grant+".submodel_uuid")))).
		Select(goqu.L("1")).
		Where(
			goqu.I("rebac_granted_submodel.id").Eq(submodelColumn),
			goqu.Or(
				pathColumn.Eq(grantPath),
				goqu.L("? LIKE (? || '.%') ESCAPE '!'", pathColumn, escaped),
				goqu.L("? LIKE (? || '[%]%') ESCAPE '!'", pathColumn, escaped),
			),
		)
	return goqu.L("EXISTS (?)", granted)
}

// orReBACGrant widens a security condition by the ReBAC grant of the
// collector's root. A nil condition means unrestricted and stays nil.
func orReBACGrant(condition exp.Expression, grant exp.Expression, hasGrant bool) exp.Expression {
	if condition == nil || !hasGrant {
		return condition
	}
	return goqu.Or(condition, grant)
}
