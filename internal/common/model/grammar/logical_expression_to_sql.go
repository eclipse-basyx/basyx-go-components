/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// Package grammar defines the data structures for representing logical expressions in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE ), Martin Stemmer ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

type existsJoinRule struct {
	Alias string
	Deps  []string
	Apply func(ds *goqu.SelectDataset) *goqu.SelectDataset
}

type existsJoinPlan struct {
	BaseAlias       string
	BaseTable       string
	RequiredAliases map[string]struct{}
	ExpandedAliases []string
	Rules           map[string]existsJoinRule
}

type JoinPlanConfig struct {
	PreferredBase   string
	BaseAliases     []string
	Rules           map[string]existsJoinRule
	TableForAlias   func(string) (string, bool)
	GroupKeyForBase func(string) (exp.IdentifierExpression, error)
	Correlatable    func(string) bool
}

func NewResolvedFieldPathCollectorForRoot(root string, cteAlias string) (*ResolvedFieldPathCollector, error) {
	if strings.TrimSpace(cteAlias) == "" {
		return nil, fmt.Errorf("cteAlias must be provided")
	}
	cfg, err := joinPlanConfigForRoot(root)
	if err != nil {
		return nil, err
	}
	return NewResolvedFieldPathCollectorWithConfig(cteAlias, &cfg), nil
}

func joinPlanConfigForRoot(root string) (JoinPlanConfig, error) {
	switch normalizeRoot(root) {
	case "aasdesc", "smdesc":
		return defaultJoinPlanConfig(), nil
	case "sm":
		return joinPlanConfigForSM(), nil
	case "sme":
		return joinPlanConfigForSME(), nil
	default:
		return JoinPlanConfig{}, fmt.Errorf("unsupported collector root %q", root)
	}
}

func normalizeRoot(root string) string {
	r := strings.TrimSpace(root)
	if after, ok := strings.CutPrefix(r, "$"); ok {
		r = after
	}
	if idx := strings.Index(r, "."); idx >= 0 {
		r = r[:idx]
	}
	return r
}

func joinPlanConfigForSM() JoinPlanConfig {
	return JoinPlanConfig{
		PreferredBase: "s",
		BaseAliases:   []string{"s", "semantic_id_reference", "semantic_id_reference_key"},
		Rules: map[string]existsJoinRule{
			"s": {
				Alias: "s",
				Deps:  nil,
				Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
					return ds
				},
			},
			"semantic_id_reference": {
				Alias: "semantic_id_reference",
				Deps:  []string{"s"},
				Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
					return ds.Join(
						goqu.T("reference").As("semantic_id_reference"),
						goqu.On(goqu.I("semantic_id_reference.id").Eq(goqu.I("s.semantic_id"))),
					)
				},
			},
			"semantic_id_reference_key": {
				Alias: "semantic_id_reference_key",
				Deps:  []string{"semantic_id_reference"},
				Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
					return ds.Join(
						goqu.T("reference_key").As("semantic_id_reference_key"),
						goqu.On(goqu.I("semantic_id_reference_key.reference_id").Eq(goqu.I("semantic_id_reference.id"))),
					)
				},
			},
		},
		TableForAlias: func(alias string) (string, bool) {
			switch alias {
			case "s":
				return "submodel", true
			case "semantic_id_reference":
				return "reference", true
			case "semantic_id_reference_key":
				return "reference_key", true
			default:
				return "", false
			}
		},
		GroupKeyForBase: func(base string) (exp.IdentifierExpression, error) {
			if base == "s" {
				return goqu.I("s.id"), nil
			}
			return nil, fmt.Errorf("unsupported SM base alias %q", base)
		},
		Correlatable: func(alias string) bool {
			return alias == "s"
		},
	}
}

func joinPlanConfigForSME() JoinPlanConfig {
	return JoinPlanConfig{
		PreferredBase: "submodel_element",
		BaseAliases:   []string{"submodel_element", "property_element", "semantic_id_reference", "semantic_id_reference_key"},
		Rules: map[string]existsJoinRule{
			"submodel_element": {
				Alias: "submodel_element",
				Deps:  nil,
				Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
					return ds
				},
			},
			"property_element": {
				Alias: "property_element",
				Deps:  []string{"submodel_element"},
				Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
					return ds.Join(
						goqu.T("property_element").As("property_element"),
						goqu.On(goqu.I("property_element.id").Eq(goqu.I("submodel_element.id"))),
					)
				},
			},
			"semantic_id_reference": {
				Alias: "semantic_id_reference",
				Deps:  []string{"submodel_element"},
				Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
					return ds.Join(
						goqu.T("reference").As("semantic_id_reference"),
						goqu.On(goqu.I("semantic_id_reference.id").Eq(goqu.I("submodel_element.semantic_id"))),
					)
				},
			},
			"semantic_id_reference_key": {
				Alias: "semantic_id_reference_key",
				Deps:  []string{"semantic_id_reference"},
				Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
					return ds.Join(
						goqu.T("reference_key").As("semantic_id_reference_key"),
						goqu.On(goqu.I("semantic_id_reference_key.reference_id").Eq(goqu.I("semantic_id_reference.id"))),
					)
				},
			},
		},
		TableForAlias: func(alias string) (string, bool) {
			switch alias {
			case "submodel_element":
				return "submodel_element", true
			case "property_element":
				return "property_element", true
			case "semantic_id_reference":
				return "reference", true
			case "semantic_id_reference_key":
				return "reference_key", true
			default:
				return "", false
			}
		},
		GroupKeyForBase: func(base string) (exp.IdentifierExpression, error) {
			if base == "submodel_element" {
				return goqu.I("submodel_element.id"), nil
			}
			return nil, fmt.Errorf("unsupported SME base alias %q", base)
		},
		Correlatable: func(alias string) bool {
			return alias == "submodel_element"
		},
	}
}

// ResolvedFieldPathFlag ties a resolved field path set to the boolean flag alias that
// will be emitted in a precomputed CTE.
type ResolvedFieldPathFlag struct {
	Alias     string
	Resolved  []ResolvedFieldPath
	Predicate exp.Expression
}

// ResolvedFieldPathCollector collects resolved field path predicates and assigns
// unique flag aliases that can be referenced in WHERE clauses.
type ResolvedFieldPathCollector struct {
	CTEAlias              string
	nextID                int
	nextGroupID           int
	keyToAlias            map[string]string
	groupKeyToAlias       map[string]string
	flagAliasToGroupAlias map[string]string
	entries               []ResolvedFieldPathFlag
	joinConfig            *JoinPlanConfig
}

// NewResolvedFieldPathCollector creates a collector with the provided CTE alias.
// When cteAlias is empty, "descriptor_flags" is used.
func NewResolvedFieldPathCollectorWithConfig(cteAlias string, config *JoinPlanConfig) *ResolvedFieldPathCollector {
	if strings.TrimSpace(cteAlias) == "" {
		cteAlias = "descriptor_flags"
	}
	return &ResolvedFieldPathCollector{
		CTEAlias:              cteAlias,
		keyToAlias:            map[string]string{},
		groupKeyToAlias:       map[string]string{},
		flagAliasToGroupAlias: map[string]string{},
		joinConfig:            config,
	}
}

// Entries returns a shallow copy of the collected flag definitions.
func (c *ResolvedFieldPathCollector) Entries() []ResolvedFieldPathFlag {
	if c == nil {
		return nil
	}
	out := make([]ResolvedFieldPathFlag, len(c.entries))
	copy(out, c.entries)
	return out
}

// Register adds a new resolved predicate or returns the existing alias if it was already registered.
func (c *ResolvedFieldPathCollector) Register(resolved []ResolvedFieldPath, predicate exp.Expression) (string, error) {
	if c == nil {
		return "", fmt.Errorf("resolved field path collector is nil")
	}
	key, err := c.signature(resolved, predicate)
	if err != nil {
		return "", err
	}
	if alias, ok := c.keyToAlias[key]; ok {
		return alias, nil
	}

	c.nextID++
	alias := fmt.Sprintf("rfp_%d", c.nextID)
	c.keyToAlias[key] = alias
	c.entries = append(c.entries, ResolvedFieldPathFlag{
		Alias:     alias,
		Resolved:  resolved,
		Predicate: predicate,
	})
	groupAlias, err := c.groupAliasForResolved(resolved)
	if err != nil {
		return "", err
	}
	c.flagAliasToGroupAlias[alias] = groupAlias
	return alias, nil
}

func (c *ResolvedFieldPathCollector) signature(resolved []ResolvedFieldPath, predicate exp.Expression) (string, error) {
	resolvedJSON, err := json.Marshal(resolved)
	if err != nil {
		return "", err
	}
	predicateJSON, err := predicateSignature(predicate)
	if err != nil {
		return "", err
	}
	return string(resolvedJSON) + "|" + predicateJSON, nil
}

func predicateSignature(expr exp.Expression) (string, error) {
	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T("descriptor").As("descriptor")).Select(goqu.V(1))
	if expr != nil {
		ds = ds.Where(expr)
	}
	ds = ds.Prepared(true)
	sql, args, err := ds.ToSQL()
	if err != nil {
		return "", err
	}
	payload := struct {
		SQL  string        `json:"sql"`
		Args []interface{} `json:"args"`
	}{
		SQL:  sql,
		Args: args,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *ResolvedFieldPathCollector) qualifiedAlias(alias string) string {
	if c == nil {
		return alias
	}
	if groupAlias, ok := c.flagAliasToGroupAlias[alias]; ok && strings.TrimSpace(groupAlias) != "" {
		return groupAlias + "." + alias
	}
	if strings.TrimSpace(c.CTEAlias) == "" {
		return alias
	}
	return c.CTEAlias + "." + alias
}

func (c *ResolvedFieldPathCollector) groupAliasForResolved(resolved []ResolvedFieldPath) (string, error) {
	if c == nil {
		return "", fmt.Errorf("resolved field path collector is nil")
	}
	plan, err := buildJoinPlanForResolvedWithConfig(resolved, c.effectiveJoinConfig())
	if err != nil {
		return "", err
	}
	key := joinPlanSignature(plan)
	if alias, ok := c.groupKeyToAlias[key]; ok {
		return alias, nil
	}
	c.nextGroupID++
	alias := fmt.Sprintf("%s_%d", c.CTEAlias, c.nextGroupID)
	c.groupKeyToAlias[key] = alias
	return alias, nil
}

// ResolvedFieldPathFlagCTE groups multiple flag expressions that share the same join graph.
type ResolvedFieldPathFlagCTE struct {
	Alias   string
	Dataset *goqu.SelectDataset
	Flags   []ResolvedFieldPathFlag
}

// BuildResolvedFieldPathFlagCTEs builds one or more CTE datasets for the provided entries.
// Entries that share the same join graph are grouped into a single CTE with multiple flag columns.
//
// Join planning is root-specific via JoinPlanConfig; use NewResolvedFieldPathCollectorForRoot
// (or a custom config) to target $sm/$sme/$smdesc.
func BuildResolvedFieldPathFlagCTEs(cteAlias string, entries []ResolvedFieldPathFlag) ([]ResolvedFieldPathFlagCTE, error) {
	return BuildResolvedFieldPathFlagCTEsWithWhere(cteAlias, entries, nil)
}

// BuildResolvedFieldPathFlagCTEsWithWhere builds one or more CTE datasets for the provided entries
// and applies an optional WHERE clause to each CTE (e.g., root key filters).
func BuildResolvedFieldPathFlagCTEsWithWhere(cteAlias string, entries []ResolvedFieldPathFlag, where exp.Expression) ([]ResolvedFieldPathFlagCTE, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(cteAlias) == "" {
		cteAlias = "descriptor_flags"
	}

	type cteGroup struct {
		plan    existsJoinPlan
		entries []ResolvedFieldPathFlag
	}

	grouped := map[string]*cteGroup{}
	order := make([]string, 0, len(entries))

	for _, entry := range entries {
		plan, err := buildJoinPlanForResolvedWithConfig(entry.Resolved, defaultJoinPlanConfig())
		if err != nil {
			return nil, err
		}
		key := joinPlanSignature(plan)
		group, ok := grouped[key]
		if !ok {
			group = &cteGroup{plan: plan}
			grouped[key] = group
			order = append(order, key)
		}
		group.entries = append(group.entries, entry)
	}

	ctes := make([]ResolvedFieldPathFlagCTE, 0, len(grouped))
	for idx, key := range order {
		group := grouped[key]
		alias := cteAlias
		if len(grouped) > 1 {
			alias = fmt.Sprintf("%s_%d", cteAlias, idx+1)
		}
		ds, err := buildFlagCTEDataset(group.plan, group.entries, where, defaultJoinPlanConfig())
		if err != nil {
			return nil, err
		}
		ctes = append(ctes, ResolvedFieldPathFlagCTE{
			Alias:   alias,
			Dataset: ds,
			Flags:   group.entries,
		})
	}

	return ctes, nil
}

// BuildResolvedFieldPathFlagCTEsWithCollector builds one or more CTE datasets for the provided entries
// and uses the collector's join-group aliases to name the CTEs.
func BuildResolvedFieldPathFlagCTEsWithCollector(collector *ResolvedFieldPathCollector, entries []ResolvedFieldPathFlag, where exp.Expression) ([]ResolvedFieldPathFlagCTE, error) {
	if collector == nil {
		return nil, fmt.Errorf("resolved field path collector is nil")
	}
	if len(entries) == 0 {
		return nil, nil
	}

	type cteGroup struct {
		plan    existsJoinPlan
		entries []ResolvedFieldPathFlag
		alias   string
	}

	grouped := map[string]*cteGroup{}
	order := make([]string, 0, len(entries))

	for _, entry := range entries {
		plan, err := buildJoinPlanForResolvedWithConfig(entry.Resolved, collector.effectiveJoinConfig())
		if err != nil {
			return nil, err
		}
		key := joinPlanSignature(plan)
		groupAlias, err := collector.groupAliasForResolved(entry.Resolved)
		if err != nil {
			return nil, err
		}
		group, ok := grouped[key]
		if !ok {
			group = &cteGroup{plan: plan, alias: groupAlias}
			grouped[key] = group
			order = append(order, key)
		}
		group.entries = append(group.entries, entry)
	}

	ctes := make([]ResolvedFieldPathFlagCTE, 0, len(grouped))
	for _, key := range order {
		group := grouped[key]
		ds, err := buildFlagCTEDataset(group.plan, group.entries, where, collector.effectiveJoinConfig())
		if err != nil {
			return nil, err
		}
		ctes = append(ctes, ResolvedFieldPathFlagCTE{
			Alias:   group.alias,
			Dataset: ds,
			Flags:   group.entries,
		})
	}

	return ctes, nil
}

func joinPlanSignature(plan existsJoinPlan) string {
	aliases := make([]string, 0, len(plan.ExpandedAliases))
	aliases = append(aliases, plan.ExpandedAliases...)
	sort.Strings(aliases)
	return plan.BaseAlias + "|" + strings.Join(aliases, ",")
}

func buildFlagCTEDataset(plan existsJoinPlan, entries []ResolvedFieldPathFlag, where exp.Expression, config JoinPlanConfig) (*goqu.SelectDataset, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("cannot build flag CTE dataset with no entries")
	}

	d := goqu.Dialect("postgres")
	ds := d.From(goqu.T(plan.BaseTable).As(plan.BaseAlias))

	applied := map[string]struct{}{plan.BaseAlias: {}}
	visiting := map[string]struct{}{}

	var ensure func(alias string) error
	ensure = func(alias string) error {
		if alias == "" {
			return nil
		}
		if alias == plan.BaseAlias {
			return nil
		}
		if _, ok := applied[alias]; ok {
			return nil
		}
		if _, ok := visiting[alias]; ok {
			return fmt.Errorf("cyclic CTE join dependency for alias %q", alias)
		}
		rule, ok := plan.Rules[alias]
		if !ok {
			return fmt.Errorf("no CTE join rule registered for alias %q", alias)
		}
		visiting[alias] = struct{}{}
		for _, dep := range rule.Deps {
			if err := ensure(dep); err != nil {
				return err
			}
		}
		delete(visiting, alias)
		ds = rule.Apply(ds)
		applied[alias] = struct{}{}
		return nil
	}

	for _, alias := range plan.ExpandedAliases {
		if err := ensure(alias); err != nil {
			return nil, err
		}
	}

	descriptorExpr, err := config.GroupKeyForBase(plan.BaseAlias)
	if err != nil {
		return nil, err
	}

	// Keep a stable column name; this is the root key used by ApplyResolvedFieldPathCTEs.
	selects := []interface{}{descriptorExpr.As("root_id")}
	for _, entry := range entries {
		flagExpr := andBindingsForResolvedFieldPaths(entry.Resolved, entry.Predicate)
		selects = append(selects, goqu.L("COALESCE(BOOL_OR(?), false)", flagExpr).As(entry.Alias))
	}

	ds = ds.Select(selects...).GroupBy(descriptorExpr)
	if where != nil {
		ds = ds.Where(where)
	}
	return ds, nil
}

func defaultJoinPlanConfig() JoinPlanConfig {
	return JoinPlanConfig{
		PreferredBase:   "aas_descriptor",
		BaseAliases:     []string{"specific_asset_id", "aas_descriptor_endpoint", "submodel_descriptor", "aas_descriptor"},
		Rules:           existsJoinRulesForAASDescriptors(),
		TableForAlias:   existsTableForAlias,
		GroupKeyForBase: descriptorIDForBaseAlias,
		Correlatable: func(alias string) bool {
			return existsCorrelationForAlias(alias) != nil
		},
	}
}

func (c *ResolvedFieldPathCollector) effectiveJoinConfig() JoinPlanConfig {
	if c == nil || c.joinConfig == nil {
		return defaultJoinPlanConfig()
	}
	cfg := *c.joinConfig
	if cfg.Rules == nil {
		cfg.Rules = existsJoinRulesForAASDescriptors()
	}
	if cfg.TableForAlias == nil {
		cfg.TableForAlias = existsTableForAlias
	}
	if cfg.GroupKeyForBase == nil {
		cfg.GroupKeyForBase = descriptorIDForBaseAlias
	}
	if cfg.BaseAliases == nil {
		cfg.BaseAliases = []string{"specific_asset_id", "aas_descriptor_endpoint", "submodel_descriptor", "aas_descriptor"}
	}
	if strings.TrimSpace(cfg.PreferredBase) == "" {
		cfg.PreferredBase = "aas_descriptor"
	}
	if cfg.Correlatable == nil {
		cfg.Correlatable = func(alias string) bool {
			return existsCorrelationForAlias(alias) != nil
		}
	}
	return cfg
}

func descriptorIDForBaseAlias(base string) (exp.IdentifierExpression, error) {
	switch base {
	case "aas_descriptor":
		return goqu.I("aas_descriptor.descriptor_id"), nil
	case "specific_asset_id":
		return goqu.I("specific_asset_id.descriptor_id"), nil
	case "aas_descriptor_endpoint":
		return goqu.I("aas_descriptor_endpoint.descriptor_id"), nil
	case "submodel_descriptor":
		return goqu.I("submodel_descriptor.aas_descriptor_id"), nil
	default:
		return nil, fmt.Errorf("unsupported base alias for descriptor id selection: %q", base)
	}
}

// extractFieldOperandAndCast walks through cast wrappers to find the underlying field operand
// and returns the outermost cast target type (if any).
func extractFieldOperandAndCast(v *Value) (*Value, string) {
	cur := v
	castType := ""
	for cur != nil {
		// Record only the outermost cast.
		if castType == "" {
			switch {
			case cur.StrCast != nil:
				castType = "text"
			case cur.NumCast != nil:
				castType = "double precision"
			case cur.BoolCast != nil:
				castType = "boolean"
			case cur.TimeCast != nil:
				castType = "time"
			case cur.DateTimeCast != nil:
				castType = "timestamptz"
			case cur.HexCast != nil:
				castType = "text"
			}
		}

		if cur.Field != nil {
			return cur, castType
		}
		switch {
		case cur.StrCast != nil:
			cur = cur.StrCast
		case cur.NumCast != nil:
			cur = cur.NumCast
		case cur.BoolCast != nil:
			cur = cur.BoolCast
		case cur.TimeCast != nil:
			cur = cur.TimeCast
		case cur.DateTimeCast != nil:
			cur = cur.DateTimeCast
		case cur.HexCast != nil:
			cur = cur.HexCast
		default:
			return nil, ""
		}
	}
	return nil, ""
}

func toSQLResolvedFieldOrValue(operand *Value, explicitCastType string, position string) (interface{}, *ResolvedFieldPath, error) {
	fieldOperand, _ := extractFieldOperandAndCast(operand)
	if fieldOperand == nil || fieldOperand.Field == nil {
		val, err := toSQLComponent(operand, position)
		return val, nil, err
	}
	fieldStr := string(*fieldOperand.Field)
	f := ModelStringPattern(fieldStr)
	resolved, err := ResolveScalarFieldToSQL(&f)
	if err != nil {
		return nil, nil, err
	}
	ident := goqu.I(resolved.Column)
	if explicitCastType != "" {
		return safeCastSQLValue(ident, explicitCastType), &resolved, nil
	}
	return ident, &resolved, nil
}

func anyResolvedHasBindings(resolved []ResolvedFieldPath) bool {
	for _, r := range resolved {
		if len(r.ArrayBindings) > 0 {
			return true
		}
	}
	return false
}

func resolvedNeedsCTE(resolved []ResolvedFieldPath) bool {
	if anyResolvedHasBindings(resolved) {
		return true
	}
	for _, r := range resolved {
		if strings.TrimSpace(r.Column) == "" {
			continue
		}
		alias, ok := leadingAlias(r.Column)
		if !ok {
			continue
		}
		if alias != "aas_descriptor" {
			return true
		}
	}
	return false
}

func collectResolvedFieldPaths(a, b *ResolvedFieldPath) []ResolvedFieldPath {
	var out []ResolvedFieldPath
	if a != nil {
		out = append(out, *a)
	}
	if b != nil {
		out = append(out, *b)
	}
	return out
}

func sqlTypeForOperand(v *Value) string {
	if v == nil {
		return ""
	}
	switch {
	case v.StrVal != nil:
		return "text"
	case v.NumVal != nil:
		return "double precision"
	case v.Boolean != nil:
		return "boolean"
	case v.TimeVal != nil:
		return "time"
	case v.DateTimeVal != nil:
		return "timestamptz"
	default:
		return ""
	}
}

func leadingAlias(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", false
	}
	idx := strings.Index(expr, ".")
	if idx <= 0 {
		return "", false
	}
	return expr[:idx], true
}

func requiredAliasesFromResolved(resolved []ResolvedFieldPath) (map[string]struct{}, error) {
	return requiredAliasesFromResolvedWithConfig(resolved, defaultJoinPlanConfig())
}

func requiredAliasesFromResolvedWithConfig(resolved []ResolvedFieldPath, config JoinPlanConfig) (map[string]struct{}, error) {
	req := map[string]struct{}{}
	for _, r := range resolved {
		if strings.TrimSpace(r.Column) != "" {
			a, ok := leadingAlias(r.Column)
			if ok {
				if _, exists := config.Rules[a]; exists {
					req[a] = struct{}{}
					continue
				}
			}
			found := false
			for alias := range config.Rules {
				if strings.Contains(r.Column, alias+".") {
					req[alias] = struct{}{}
					found = true
				}
			}
			if !found {
				return nil, fmt.Errorf("cannot extract alias from column %q", r.Column)
			}
		}
		for _, b := range r.ArrayBindings {
			a, ok := leadingAlias(b.Alias)
			if !ok {
				return nil, fmt.Errorf("cannot extract alias from binding alias %q", b.Alias)
			}
			req[a] = struct{}{}
		}
	}
	return req, nil
}

func expandAliasesWithDeps(aliases map[string]struct{}, rules map[string]existsJoinRule) map[string]struct{} {
	out := map[string]struct{}{}
	var visit func(a string)
	visit = func(a string) {
		if a == "" {
			return
		}
		if _, ok := out[a]; ok {
			return
		}
		out[a] = struct{}{}
		r, ok := rules[a]
		if !ok {
			return
		}
		for _, dep := range r.Deps {
			visit(dep)
		}
	}
	for a := range aliases {
		visit(a)
	}
	return out
}

func buildJoinPlanForResolved(resolved []ResolvedFieldPath) (existsJoinPlan, error) {
	return buildJoinPlanForResolvedWithConfig(resolved, defaultJoinPlanConfig())
}

func buildJoinPlanForResolvedWithConfig(resolved []ResolvedFieldPath, config JoinPlanConfig) (existsJoinPlan, error) {
	required, err := requiredAliasesFromResolvedWithConfig(resolved, config)
	if err != nil {
		return existsJoinPlan{}, err
	}

	expanded := expandAliasesWithDeps(required, config.Rules)

	// Choose a base alias that can be correlated to the outer root key.
	// Important: required aliases might be leaf tables (e.g. reference_key), and we
	// still need to include their dependency chain to reach a correlatable base.
	base := ""
	if config.PreferredBase != "" {
		if _, ok := expanded[config.PreferredBase]; ok && len(expanded) > 1 {
			base = config.PreferredBase
		}
	}
	if base == "" {
		for _, cand := range config.BaseAliases {
			if _, ok := expanded[cand]; ok {
				base = cand
				break
			}
		}
	}
	if base == "" && config.Correlatable != nil {
		for a := range expanded {
			if config.Correlatable(a) {
				base = a
				break
			}
		}
	}
	if base == "" {
		for a := range expanded {
			base = a
			break
		}
	}
	if base == "" {
		return existsJoinPlan{}, fmt.Errorf("cannot build join plan: no correlatable base alias found")
	}
	baseTable, ok := config.TableForAlias(base)
	if !ok {
		return existsJoinPlan{}, fmt.Errorf("cannot build join plan: no table mapping for alias %q", base)
	}

	expandedAliases := make([]string, 0, len(expanded))
	for alias := range expanded {
		expandedAliases = append(expandedAliases, alias)
	}
	sort.Strings(expandedAliases)

	return existsJoinPlan{
		BaseAlias:       base,
		BaseTable:       baseTable,
		RequiredAliases: required,
		ExpandedAliases: expandedAliases,
		Rules:           config.Rules,
	}, nil
}

func andBindingsForResolvedFieldPaths(resolved []ResolvedFieldPath, predicate exp.Expression) exp.Expression {
	where := make([]exp.Expression, 0, 1)
	if predicate != nil {
		where = append(where, predicate)
	}
	for _, r := range resolved {
		for _, b := range r.ArrayBindings {
			if b.Index.intValue != nil {
				where = append(where, goqu.I(b.Alias).Eq(*b.Index.intValue))
			}
			if b.Index.stringValue != nil {
				where = append(where, goqu.I(b.Alias).Eq(*b.Index.stringValue))
			}
		}
	}
	if len(where) == 0 {
		return goqu.L("1=1")
	}
	return goqu.And(where...)
}

// wrapBindingsAsResolvedPath wraps bare array bindings into a minimal ResolvedFieldPath
// with an empty column. This is useful for fragment-only resolutions that produce only
// array index constraints without resolving a concrete SQL column.
func wrapBindingsAsResolvedPath(bindings []ArrayIndexBinding) ResolvedFieldPath {
	return ResolvedFieldPath{
		Column:        "", // Empty: this is bindings-only
		ArrayBindings: bindings,
	}
}

func existsTableForAlias(alias string) (string, bool) {
	switch alias {
	case "aas_descriptor":
		return "aas_descriptor", true
	case "specific_asset_id":
		return "specific_asset_id", true
	case "external_subject_reference":
		return "reference", true
	case "external_subject_reference_key":
		return "reference_key", true
	case "aas_descriptor_endpoint":
		return "aas_descriptor_endpoint", true
	case "submodel_descriptor":
		return "submodel_descriptor", true
	case "submodel_descriptor_endpoint":
		return "aas_descriptor_endpoint", true
	case "aasdesc_submodel_descriptor_semantic_id_reference":
		return "reference", true
	case "aasdesc_submodel_descriptor_semantic_id_reference_key":
		return "reference_key", true
	default:
		return "", false
	}
}

func existsCorrelationForAlias(base string) exp.Expression {
	switch base {
	case "aas_descriptor":
		return goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id"))
	case "specific_asset_id":
		return goqu.I("specific_asset_id.descriptor_id").Eq(goqu.I("descriptor.id"))
	case "aas_descriptor_endpoint":
		return goqu.I("aas_descriptor_endpoint.descriptor_id").Eq(goqu.I("descriptor.id"))
	case "submodel_descriptor":
		// submodel_descriptor.aas_descriptor_id points to the AAS descriptor (descriptor.id)
		return goqu.I("submodel_descriptor.aas_descriptor_id").Eq(goqu.I("descriptor.id"))
	default:
		return nil
	}
}

func existsJoinRulesForAASDescriptors() map[string]existsJoinRule {
	return map[string]existsJoinRule{
		"specific_asset_id": {
			Alias: "specific_asset_id",
			Deps:  []string{"aas_descriptor"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("specific_asset_id").As("specific_asset_id"),
					goqu.On(goqu.I("specific_asset_id.descriptor_id").Eq(goqu.I("aas_descriptor.descriptor_id"))),
				)
			},
		},
		"aas_descriptor_endpoint": {
			Alias: "aas_descriptor_endpoint",
			Deps:  []string{"aas_descriptor"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("aas_descriptor_endpoint").As("aas_descriptor_endpoint"),
					goqu.On(goqu.I("aas_descriptor_endpoint.descriptor_id").Eq(goqu.I("aas_descriptor.descriptor_id"))),
				)
			},
		},
		"submodel_descriptor": {
			Alias: "submodel_descriptor",
			Deps:  []string{"aas_descriptor"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("submodel_descriptor").As("submodel_descriptor"),
					goqu.On(goqu.I("submodel_descriptor.aas_descriptor_id").Eq(goqu.I("aas_descriptor.descriptor_id"))),
				)
			},
		},
		"external_subject_reference": {
			Alias: "external_subject_reference",
			Deps:  []string{"specific_asset_id"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("reference").As("external_subject_reference"),
					goqu.On(goqu.I("external_subject_reference.id").Eq(goqu.I("specific_asset_id.external_subject_ref"))),
				)
			},
		},
		"external_subject_reference_key": {
			Alias: "external_subject_reference_key",
			Deps:  []string{"external_subject_reference"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("reference_key").As("external_subject_reference_key"),
					goqu.On(goqu.I("external_subject_reference_key.reference_id").Eq(goqu.I("external_subject_reference.id"))),
				)
			},
		},
		"submodel_descriptor_endpoint": {
			Alias: "submodel_descriptor_endpoint",
			Deps:  []string{"submodel_descriptor"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("aas_descriptor_endpoint").As("submodel_descriptor_endpoint"),
					goqu.On(goqu.I("submodel_descriptor_endpoint.descriptor_id").Eq(goqu.I("submodel_descriptor.descriptor_id"))),
				)
			},
		},
		"aasdesc_submodel_descriptor_semantic_id_reference": {
			Alias: "aasdesc_submodel_descriptor_semantic_id_reference",
			Deps:  []string{"submodel_descriptor"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("reference").As("aasdesc_submodel_descriptor_semantic_id_reference"),
					goqu.On(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference.id").Eq(goqu.I("submodel_descriptor.semantic_id"))),
				)
			},
		},
		"aasdesc_submodel_descriptor_semantic_id_reference_key": {
			Alias: "aasdesc_submodel_descriptor_semantic_id_reference_key",
			Deps:  []string{"aasdesc_submodel_descriptor_semantic_id_reference"},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.Join(
					goqu.T("reference_key").As("aasdesc_submodel_descriptor_semantic_id_reference_key"),
					goqu.On(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference_key.reference_id").Eq(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference.id"))),
				)
			},
		},
	}
}

func (le *LogicalExpression) evaluateFragmentToExpression(collector *ResolvedFieldPathCollector, fragment FragmentStringPattern) (exp.Expression, []ResolvedFieldPath, error) {
	bindings, err := ResolveFragmentFieldToSQL(&fragment)
	if err != nil {
		return nil, nil, err
	}

	// Wrap bindings as a minimal ResolvedFieldPath for consistency.
	// Since fragments may only produce array constraints without a concrete column,
	// we allow empty Column fields in this context.
	resolved := []ResolvedFieldPath{wrapBindingsAsResolvedPath(bindings)}

	// Fragments resolve only to array bindings. We translate these into a predicate
	// without injecting a literal TRUE into the SQL.
	var fragmentExpr exp.Expression
	if len(bindings) == 0 {
		// Wildcard fragment (e.g. endpoints[]) applies to all rows.
		fragmentExpr = goqu.L("1=1")
	} else {
		where := make([]exp.Expression, 0, len(bindings))
		for _, b := range bindings {
			if b.Index.intValue != nil {
				where = append(where, goqu.I(b.Alias).Eq(*b.Index.intValue))
			}
			if b.Index.stringValue != nil {
				where = append(where, goqu.I(b.Alias).Eq(*b.Index.stringValue))
			}
		}
		fragmentExpr = goqu.And(where...)
	}

	if collector != nil && resolvedNeedsCTE(resolved) {
		// Important: do NOT include binding constraints in the registered predicate.
		// The CTE builder (buildFlagCTEDataset) will always apply array bindings from
		// ResolvedFieldPath via andBindingsForResolvedFieldPaths; registering bindings
		// here would duplicate them (e.g. position = ? AND position = ?).
		alias, err := collector.Register(resolved, nil)
		if err != nil {
			return nil, nil, err
		}
		return goqu.I(collector.qualifiedAlias(alias)), resolved, nil
	}

	return fragmentExpr, resolved, nil
}

// EvaluateToExpression converts the logical expression tree into a goqu SQL expression.
//
// This method traverses the logical expression tree and constructs a corresponding SQL
// WHERE clause expression that can be used with the goqu query builder. It handles all
// supported comparison operations, logical operators (AND, OR, NOT), and nested expressions.
// When a collector is provided, comparisons that would otherwise require EXISTS joins are
// registered as flag predicates and replaced by references to the collector's aliases.
//
// The method supports special handling for AAS-specific fields, particularly semantic IDs,
// where additional constraints (like position = 0) may be added to the generated SQL.
//
// Supported operations:
//   - Comparison: $eq, $ne, $gt, $ge, $lt, $le
//   - Logical: $and (all true), $or (any true), $not (negation)
//   - Boolean: Direct boolean literal evaluation
//
// Returns:
//   - exp.Expression: A goqu expression that can be used in SQL WHERE clauses
//   - []ResolvedFieldPath: All resolved field paths discovered while evaluating the expression
//   - error: An error if the expression is invalid, has no valid operation, or if
//     evaluation of nested expressions fails
func (le *LogicalExpression) EvaluateToExpression(collector *ResolvedFieldPathCollector) (exp.Expression, []ResolvedFieldPath, error) {
	if le == nil {
		return nil, nil, fmt.Errorf("logical expression is nil")
	}
	// Handle comparison operations
	if len(le.Eq) > 0 {
		return le.evaluateComparison(le.Eq, "$eq", collector)
	}
	if len(le.Ne) > 0 {
		return le.evaluateComparison(le.Ne, "$ne", collector)
	}
	if len(le.Gt) > 0 {
		return le.evaluateComparison(le.Gt, "$gt", collector)
	}
	if len(le.Ge) > 0 {
		return le.evaluateComparison(le.Ge, "$ge", collector)
	}
	if len(le.Lt) > 0 {
		return le.evaluateComparison(le.Lt, "$lt", collector)
	}
	if len(le.Le) > 0 {
		return le.evaluateComparison(le.Le, "$le", collector)
	}

	// Handle string operations
	if len(le.Contains) > 0 {
		return le.evaluateStringOperationSQL(le.Contains, "$contains", collector)
	}
	if len(le.StartsWith) > 0 {
		return le.evaluateStringOperationSQL(le.StartsWith, "$starts-with", collector)
	}
	if len(le.EndsWith) > 0 {
		return le.evaluateStringOperationSQL(le.EndsWith, "$ends-with", collector)
	}
	if len(le.Regex) > 0 {
		return le.evaluateStringOperationSQL(le.Regex, "$regex", collector)
	}

	// Handle logical operations
	if len(le.And) > 0 {
		var expressions []exp.Expression
		var resolved []ResolvedFieldPath
		for i, nestedExpr := range le.And {
			expr, childResolved, err := nestedExpr.EvaluateToExpression(collector)
			if err != nil {
				return nil, nil, fmt.Errorf("error evaluating AND condition at index %d: %w", i, err)
			}
			expressions = append(expressions, expr)
			resolved = append(resolved, childResolved...)
		}
		return goqu.And(expressions...), resolved, nil
	}

	if len(le.Or) > 0 {
		var expressions []exp.Expression
		var resolved []ResolvedFieldPath
		for i, nestedExpr := range le.Or {
			expr, childResolved, err := nestedExpr.EvaluateToExpression(collector)
			if err != nil {
				return nil, nil, fmt.Errorf("error evaluating OR condition at index %d: %w", i, err)
			}
			expressions = append(expressions, expr)
			resolved = append(resolved, childResolved...)
		}
		return goqu.Or(expressions...), resolved, nil
	}

	if le.Not != nil {
		expr, resolved, err := le.Not.EvaluateToExpression(collector)
		if err != nil {
			return nil, nil, fmt.Errorf("error evaluating NOT condition: %w", err)
		}
		return goqu.L("NOT (?)", expr), resolved, nil
	}

	// Handle boolean literal
	if le.Boolean != nil {
		return goqu.L("?", *le.Boolean), nil, nil
	}

	return nil, nil, fmt.Errorf("logical expression has no valid operation")
}

// EvaluateToExpressionWithNegatedFragments evaluates the logical expression and then ORs it
// with an OR-group of NOT(fragmentExpr) expressions.
//
// This is primarily useful for fragment-scoped filtering: the main condition should only
// apply when the current row is part of the targeted fragment; for all other rows the
// predicate should evaluate to true.
//
// Semantics:
//
//	combined = mainExpr OR (OR_i NOT(fragmentExpr_i))
//
// The returned resolved field paths include both the main expression's resolved paths and
// those required for the fragment expressions.
func (le *LogicalExpression) EvaluateToExpressionWithNegatedFragments(
	collector *ResolvedFieldPathCollector,
	fragments []FragmentStringPattern,
) (exp.Expression, []ResolvedFieldPath, error) {
	if le == nil {
		return nil, nil, fmt.Errorf("logical expression is nil")
	}

	mainExpr, resolved, err := le.EvaluateToExpression(collector)
	if err != nil {
		return nil, nil, err
	}
	if len(fragments) == 0 {
		return mainExpr, resolved, nil
	}

	negated := make([]exp.Expression, 0, len(fragments))
	for i, f := range fragments {
		fragExpr, fragResolved, err := le.evaluateFragmentToExpression(collector, f)
		if err != nil {
			return nil, nil, fmt.Errorf("error evaluating fragment at index %d: %w", i, err)
		}
		// If the fragment has no bindings (wildcard like endpoints[]), it evaluates to 1=1.
		// NOT(1=1) is always false, so adding it to an OR-group is redundant.
		if !anyResolvedHasBindings(fragResolved) {
			continue
		}
		resolved = append(resolved, fragResolved...)
		negated = append(negated, goqu.L("NOT (?)", fragExpr))
	}
	if len(negated) == 0 {
		return mainExpr, resolved, nil
	}

	fragmentGuard := goqu.Or(negated...)
	return goqu.Or(mainExpr, fragmentGuard), resolved, nil
}

// evaluateStringOperationSQL builds SQL expressions for string operators like $contains, $starts-with, $ends-with, and $regex.
func (le *LogicalExpression) evaluateStringOperationSQL(items []StringValue, operation string, collector *ResolvedFieldPathCollector) (exp.Expression, []ResolvedFieldPath, error) {
	if len(items) != 2 {
		return nil, nil, fmt.Errorf("string operation %s requires exactly 2 operands, got %d", operation, len(items))
	}

	leftOperand := stringValueToValue(items[0])
	rightOperand := stringValueToValue(items[1])

	return HandleStringOperationWithCollector(&leftOperand, &rightOperand, operation, collector)
}

// evaluateComparison evaluates a comparison operation with the given operands
func (le *LogicalExpression) evaluateComparison(operands []Value, operation string, collector *ResolvedFieldPathCollector) (exp.Expression, []ResolvedFieldPath, error) {
	if len(operands) != 2 {
		return nil, nil, fmt.Errorf("comparison operation %s requires exactly 2 operands, got %d", operation, len(operands))
	}

	leftOperand := &operands[0]
	rightOperand := &operands[1]

	return HandleComparisonWithCollector(leftOperand, rightOperand, operation, collector)
}

// HandleComparison builds a SQL comparison expression from two Value operands.
//
// This function handles all combinations of operand types: field-to-field, field-to-value,
// value-to-field, and value-to-value comparisons. It validates that value-to-value comparisons
// have matching types and adds special constraints for AAS semantic ID fields, such as position
// constraints for specific key indices.
//
// Special handling for semantic IDs:
//   - Shorthand references ($sm#semanticId) add position = 0 constraint
//   - Specific key references ($sm#semanticId.keys[N].value) add position = N constraint
//   - Wildcard references ($sm#semanticId.keys[].value) match any position
//
// Parameters:
//   - leftOperand: The left side of the comparison (field or value)
//   - rightOperand: The right side of the comparison (field or value)
//   - operation: The comparison operator ($eq, $ne, $gt, $ge, $lt, $le)
//
// Returns:
//   - exp.Expression: A goqu expression representing the comparison with any necessary constraints
//   - error: An error if the operands are invalid, types don't match, or the operation is unsupported
func HandleComparison(leftOperand, rightOperand *Value, operation string) (exp.Expression, error) {
	expr, _, err := HandleComparisonWithCollector(leftOperand, rightOperand, operation, nil)
	return expr, err
}

// HandleComparisonWithCollector builds a SQL comparison expression from two Value operands
// and optionally registers resolved field paths in the collector.
func HandleComparisonWithCollector(leftOperand, rightOperand *Value, operation string, collector *ResolvedFieldPathCollector) (exp.Expression, []ResolvedFieldPath, error) {
	// Normalize shorthand semanticId / descriptor shorthand fields to explicit keys[0].value
	// (e.g. $aasdesc#specificAssetIds[].externalSubjectId ->
	//  $aasdesc#specificAssetIds[].externalSubjectId.keys[0].value)
	normalizeSemanticShorthand(leftOperand)
	normalizeSemanticShorthand(rightOperand)

	leftField, leftCastType := extractFieldOperandAndCast(leftOperand)
	rightField, rightCastType := extractFieldOperandAndCast(rightOperand)

	// Field-to-field comparisons are forbidden by the query language.
	// We can safely assume comparisons have either 0 or 1 field operands.
	if leftField != nil && rightField != nil {
		return nil, nil, fmt.Errorf("field-to-field comparisons are not supported")
	}

	// Fast-path: both are values (no FieldIdentifiers involved).
	if leftField == nil && rightField == nil {
		leftSQL, err := toSQLComponent(leftOperand, "left")
		if err != nil {
			return nil, nil, err
		}
		rightSQL, err := toSQLComponent(rightOperand, "right")
		if err != nil {
			return nil, nil, err
		}
		// has to be compatible
		_, err = leftOperand.IsComparableTo(*rightOperand)
		if err != nil {
			return nil, nil, err
		}
		expr, err := buildComparisonExpression(leftSQL, rightSQL, operation)
		return expr, nil, err
	}

	leftSQL, leftResolved, err := toSQLResolvedFieldOrValue(leftOperand, leftCastType, "left")
	if err != nil {
		return nil, nil, err
	}
	rightSQL, rightResolved, err := toSQLResolvedFieldOrValue(rightOperand, rightCastType, "right")
	if err != nil {
		return nil, nil, err
	}

	// Cast the field side to the non-field operand's type (unless already explicitly casted).
	if leftResolved != nil && rightResolved == nil && leftCastType == "" {
		if t := sqlTypeForOperand(rightOperand); t != "" {
			leftSQL = safeCastSQLValue(goqu.I(leftResolved.Column), t)
		}
	}
	if rightResolved != nil && leftResolved == nil && rightCastType == "" {
		if t := sqlTypeForOperand(leftOperand); t != "" {
			rightSQL = safeCastSQLValue(goqu.I(rightResolved.Column), t)
		}
	}

	// has to be compatible
	_, err = leftOperand.IsComparableTo(*rightOperand)
	if err != nil {
		return nil, nil, err
	}

	comparisonExpr, err := buildComparisonExpression(leftSQL, rightSQL, operation)
	if err != nil {
		return nil, nil, err
	}

	resolved := collectResolvedFieldPaths(leftResolved, rightResolved)
	// No resolved fields (should not happen due to earlier fast-path), fall back.
	if len(resolved) == 0 {
		return comparisonExpr, nil, nil
	}

	if collector != nil && resolvedNeedsCTE(resolved) {
		alias, err := collector.Register(resolved, comparisonExpr)
		if err != nil {
			return nil, nil, err
		}
		return goqu.I(collector.qualifiedAlias(alias)), resolved, nil
	}

	if collector != nil {
		return comparisonExpr, resolved, nil
	}
	if anyResolvedHasBindings(resolved) {
		return andBindingsForResolvedFieldPaths(resolved, comparisonExpr), resolved, nil
	}
	return comparisonExpr, resolved, nil
}

// HandleStringOperation builds SQL expressions for string-specific operators.
func HandleStringOperation(leftOperand, rightOperand *Value, operation string) (exp.Expression, error) {
	expr, _, err := HandleStringOperationWithCollector(leftOperand, rightOperand, operation, nil)
	return expr, err
}

// HandleStringOperationWithCollector builds SQL expressions for string-specific operators
// and optionally registers resolved field paths in the collector.
func HandleStringOperationWithCollector(leftOperand, rightOperand *Value, operation string, collector *ResolvedFieldPathCollector) (exp.Expression, []ResolvedFieldPath, error) {
	normalizeSemanticShorthand(leftOperand)
	normalizeSemanticShorthand(rightOperand)

	leftField, leftCastType := extractFieldOperandAndCast(leftOperand)
	rightField, rightCastType := extractFieldOperandAndCast(rightOperand)

	// Field-to-field string operations are forbidden by the query language.
	// We can safely assume string operations have either 0 or 1 field operands.
	if leftField != nil && rightField != nil {
		return nil, nil, fmt.Errorf("field-to-field string operations are not supported")
	}

	// Fast-path: no FieldIdentifiers involved.
	if leftField == nil && rightField == nil {
		leftSQL, err := toSQLComponent(leftOperand, "left")
		if err != nil {
			return nil, nil, err
		}
		rightSQL, err := toSQLComponent(rightOperand, "right")
		if err != nil {
			return nil, nil, err
		}
		expr, err := buildStringOperationExpression(leftSQL, rightSQL, operation)
		return expr, nil, err
	}

	leftSQL, leftResolved, err := toSQLResolvedFieldOrValue(leftOperand, leftCastType, "left")
	if err != nil {
		return nil, nil, err
	}
	rightSQL, rightResolved, err := toSQLResolvedFieldOrValue(rightOperand, rightCastType, "right")
	if err != nil {
		return nil, nil, err
	}

	if leftResolved != nil && rightResolved == nil && leftCastType == "" {
		if t := sqlTypeForOperand(rightOperand); t != "" {
			leftSQL = safeCastSQLValue(goqu.I(leftResolved.Column), t)
		}
	}
	if rightResolved != nil && leftResolved == nil && rightCastType == "" {
		if t := sqlTypeForOperand(leftOperand); t != "" {
			rightSQL = safeCastSQLValue(goqu.I(rightResolved.Column), t)
		}
	}

	stringExpr, err := buildStringOperationExpression(leftSQL, rightSQL, operation)
	if err != nil {
		return nil, nil, err
	}

	resolved := collectResolvedFieldPaths(leftResolved, rightResolved)
	if len(resolved) == 0 {
		return stringExpr, nil, nil
	}
	if collector != nil && resolvedNeedsCTE(resolved) {
		alias, err := collector.Register(resolved, stringExpr)
		if err != nil {
			return nil, nil, err
		}
		return goqu.I(collector.qualifiedAlias(alias)), resolved, nil
	}
	if collector != nil {
		return stringExpr, resolved, nil
	}
	if anyResolvedHasBindings(resolved) {
		return andBindingsForResolvedFieldPaths(resolved, stringExpr), resolved, nil
	}
	return stringExpr, resolved, nil
}

// stringValueToValue normalizes a StringValue into a Value so existing helpers can be reused.
func stringValueToValue(item StringValue) Value {
	switch {
	case item.Field != nil:
		return Value{Field: item.Field}
	case item.StrVal != nil:
		return Value{StrVal: item.StrVal}
	case item.Attribute != nil:
		return Value{Attribute: item.Attribute}
	case item.StrCast != nil:
		return Value{StrCast: item.StrCast}
	default:
		return Value{}
	}
}

// buildStringOperationExpression maps string operations to SQL expressions.
func buildStringOperationExpression(left interface{}, right interface{}, operation string) (exp.Expression, error) {
	switch operation {
	case "$contains":
		return goqu.L("? LIKE '%' || ? || '%'", left, right), nil
	case "$starts-with":
		return goqu.L("? LIKE ? || '%'", left, right), nil
	case "$ends-with":
		return goqu.L("? LIKE '%' || ?", left, right), nil
	case "$regex":
		// PostgreSQL regex match (case-sensitive). Use ~* if you need case-insensitive semantics.
		return goqu.L("? ~ ?", left, right), nil
	default:
		return nil, fmt.Errorf("unsupported string operation: %s", operation)
	}
}

// normalizeSemanticShorthand expands known shorthand fields to their explicit keys[0].value form.
func normalizeSemanticShorthand(operand *Value) {
	inner, _ := extractFieldOperandAndCast(operand)
	if inner == nil || inner.Field == nil {
		return
	}
	field := string(*inner.Field)
	// Already explicit -> nothing to do
	if strings.Contains(field, ".keys[") {
		return
	}
	if strings.HasSuffix(field, ".semanticId") || strings.HasSuffix(field, ".externalSubjectId") {
		field += ".keys[0].value"
		*inner.Field = ModelStringPattern(field)
	}

}

func toSQLComponent(operand *Value, position string) (interface{}, error) {
	if operand == nil {
		return nil, fmt.Errorf("%s operand is nil", position)
	}
	if operand.Attribute != nil {
		return nil, fmt.Errorf("attribute operands are not supported in SQL evaluation")
	}

	// Handle casts first so they take precedence over any accidentally set literal/field.
	if operand.StrCast != nil {
		return castOperandToSQLType(operand.StrCast, position, "text")
	}
	if operand.NumCast != nil {
		return castOperandToSQLType(operand.NumCast, position, "double precision")
	}
	if operand.BoolCast != nil {
		return castOperandToSQLType(operand.BoolCast, position, "boolean")
	}
	if operand.TimeCast != nil {
		return castOperandToSQLType(operand.TimeCast, position, "time")
	}
	if operand.DateTimeCast != nil {
		return castOperandToSQLType(operand.DateTimeCast, position, "timestamptz")
	}
	if operand.HexCast != nil {
		return castOperandToSQLType(operand.HexCast, position, "text")
	}

	if operand.IsField() {
		if operand.Field == nil {
			return nil, fmt.Errorf("%s operand is not a valid field", position)
		}
		fieldName := string(*operand.Field)
		f := ModelStringPattern(fieldName)
		resolved, err := ResolveScalarFieldToSQL(&f)
		if err != nil {
			return nil, err
		}
		return goqu.I(resolved.Column), nil
	}

	return goqu.V(normalizeLiteralForSQL(operand.GetValue())), nil
}

// buildComparisonExpression is a helper function to build comparison expressions
func buildComparisonExpression(left interface{}, right interface{}, operation string) (exp.Expression, error) {
	switch operation {
	case "$eq":
		return exp.NewLiteralExpression("? = ?", left, right), nil
	case "$ne":
		return exp.NewLiteralExpression("? != ?", left, right), nil
	case "$gt":
		return exp.NewLiteralExpression("? > ?", left, right), nil
	case "$ge":
		return exp.NewLiteralExpression("? >= ?", left, right), nil
	case "$lt":
		return exp.NewLiteralExpression("? < ?", left, right), nil
	case "$le":
		return exp.NewLiteralExpression("? <= ?", left, right), nil
	default:
		return nil, fmt.Errorf("unsupported comparison operation: %s", operation)
	}
}

// safeCastSQLValue applies a PostgreSQL cast to the provided SQL value.
//
// For types that can raise runtime errors (e.g. timestamptz, time, numeric, boolean), the cast is guarded
// so non-castable inputs yield NULL instead of a PostgreSQL cast error.
// This is critical for security rules: a failed cast should simply cause the predicate to not match.
func safeCastSQLValue(sqlValue interface{}, targetType string) exp.Expression {
	switch targetType {
	case "timestamptz":
		return goqu.L("CASE WHEN ?::text ~ ? THEN (?::timestamptz) END", sqlValue, `^[0-9]{4}-[0-9]{2}-[0-9]{2}T`, sqlValue)
	case "time":
		return goqu.L("CASE WHEN ?::text ~ ? THEN (?::time) END", sqlValue, `^[0-9]{2}:[0-9]{2}(:[0-9]{2})?$`, sqlValue)
	case "double precision":
		return goqu.L("CASE WHEN ?::text ~ ? THEN (?::double precision) END", sqlValue, `^\s*-?[0-9]+(\.[0-9]+)?\s*$`, sqlValue)
	case "boolean":
		return goqu.L("CASE WHEN lower(?::text) IN ('true','false','1','0','yes','no') THEN (?::boolean) END", sqlValue, sqlValue)
	default:
		// text/hex casts are always safe
		return goqu.L("?::"+targetType, sqlValue)
	}
}

// castOperandToSQLType recursively converts an operand to SQL and applies a PostgreSQL cast.
func castOperandToSQLType(inner *Value, position string, targetType string) (exp.Expression, error) {
	sqlValue, err := toSQLComponent(inner, position)
	if err != nil {
		return nil, err
	}
	return safeCastSQLValue(sqlValue, targetType), nil
}

// normalizeLiteralForSQL converts grammar literals to safe SQL encodable values.
func normalizeLiteralForSQL(v interface{}) interface{} {
	switch t := v.(type) {
	case DateTimeLiteralPattern:
		return normalizeTime(time.Time(t))
	case time.Time:
		return normalizeTime(t)
	default:
		return v
	}
}

func normalizeTime(t time.Time) time.Time {
	// Ensure location is set to avoid goqu encoding errors.
	if t.Location() == nil {
		return time.Unix(0, t.UnixNano()).UTC()
	}
	return t.UTC()
}
