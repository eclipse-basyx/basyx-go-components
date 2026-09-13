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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

type referenceSelectorsKey struct{}

// ReferenceSelector is a typed API filter, independent of public query-language operands.
type ReferenceSelector struct {
	Field     grammar.ModelStringPattern
	Reference types.IReference
}

type referenceKey struct {
	kind  int
	value string
}
type referencePredicate struct {
	field        grammar.ModelStringPattern
	kind         int
	keys         []referenceKey
	json         string
	referredJSON string
}

// WithAuthorizedReferenceSelectors snapshots typed predicates under the caller's access view.
func WithAuthorizedReferenceSelectors(ctx context.Context, resource SemanticResourceKind, selectors ...ReferenceSelector) (context.Context, error) {
	ctx, err := WithAuthorizedQuery(ctx, resource, grammar.Query{})
	if err != nil {
		return ctx, err
	}
	existing, _ := ctx.Value(referenceSelectorsKey{}).([]referencePredicate)
	predicates := append([]referencePredicate(nil), existing...)
	for _, selector := range selectors {
		if selector.Reference == nil {
			continue
		}
		predicate, err := snapshotReferencePredicate(selector)
		if err != nil {
			return ctx, err
		}
		predicates = append(predicates, predicate)
	}
	return context.WithValue(ctx, referenceSelectorsKey{}, predicates), nil
}

func snapshotReferencePredicate(selector ReferenceSelector) (referencePredicate, error) {
	object, err := jsonization.ToJsonable(selector.Reference)
	if err != nil {
		return referencePredicate{}, fmt.Errorf("AUTH-REFSELECTOR-SERIALIZE: %w", err)
	}
	data, err := json.Marshal(object)
	if err != nil {
		return referencePredicate{}, fmt.Errorf("AUTH-REFSELECTOR-MARSHAL: %w", err)
	}
	referred, err := json.Marshal(object["referredSemanticId"])
	if err != nil {
		return referencePredicate{}, fmt.Errorf("AUTH-REFSELECTOR-REFERRED: %w", err)
	}
	predicate := referencePredicate{field: selector.Field, kind: int(selector.Reference.Type()), json: string(data), referredJSON: string(referred)}
	for _, key := range selector.Reference.Keys() {
		predicate.keys = append(predicate.keys, referenceKey{kind: int(key.Type()), value: key.Value()})
	}
	return predicate, nil
}

// AddReferenceSelectorQuery applies full-reference predicates before the backend page limit.
func AddReferenceSelectorQuery(ctx context.Context, ds *goqu.SelectDataset, resource SemanticResourceKind) (*goqu.SelectDataset, error) {
	predicates, _ := ctx.Value(referenceSelectorsKey{}).([]referencePredicate)
	for _, predicate := range predicates {
		if semanticResourceFromField(predicate.field) != resource {
			continue
		}
		expression, err := compileReferencePredicate(ctx, predicate)
		if err != nil {
			return nil, fmt.Errorf("AUTH-REFSELECTOR-COMPILE: %w", err)
		}
		ds = ds.Where(expression)
	}
	return ds, nil
}

func compileReferencePredicate(ctx context.Context, predicate referencePredicate) (exp.Expression, error) {
	switch predicate.field {
	case "$sm#semanticId":
		primary, err := submodelReferencePredicate(ctx, predicate, false)
		if err != nil {
			return nil, err
		}
		supplemental, err := submodelReferencePredicate(ctx, predicate, true)
		if err != nil {
			return nil, err
		}
		return goqu.Or(primary, supplemental), nil
	case "$cd#isCaseOf", "$cd#embeddedDataSpecifications.dataSpecification":
		return conceptDescriptionReferencePredicate(ctx, predicate)
	default:
		return nil, common.NewInternalServerError("AUTH-REFSELECTOR-FIELD unsupported reference selector")
	}
}

type referenceSQLSpec struct {
	table, alias, ownerColumn, fragment string
	fullPayload                         bool
}

func submodelReferencePredicate(ctx context.Context, predicate referencePredicate, supplemental bool) (exp.Expression, error) {
	spec := referenceSQLSpec{"submodel_semantic_id_reference", "semantic_id_reference", "id", "$sm#semanticId", true}
	if supplemental {
		spec = referenceSQLSpec{"submodel_supplemental_semantic_id_reference", "sm_supplemental_semantic_id_reference", "submodel_id", "$sm#supplementalSemanticIds[]", false}
	}
	d := goqu.Dialect(common.Dialect)
	ref := goqu.T(spec.alias)
	payload := goqu.T(spec.alias + "_payload")
	ds := d.From(goqu.T(spec.table).As(spec.alias)).
		LeftJoin(goqu.T(spec.table+"_payload").As(spec.alias+"_payload"), goqu.On(payload.Col("reference_id").Eq(ref.Col("id")))).
		Select(goqu.L("1")).Where(ref.Col(spec.ownerColumn).Eq(goqu.I("submodel.id")), ref.Col("type").Eq(predicate.kind))
	keys, err := visibleReferenceKeys(ctx, predicate, spec)
	if err != nil {
		return nil, err
	}
	allKeys := d.From(spec.table + "_key").Select(goqu.COUNT("*")).Where(goqu.C("reference_id").Eq(ref.Col("id")))
	ds = ds.Where(goqu.V(len(predicate.keys)).Eq(keys), goqu.V(len(predicate.keys)).Eq(allKeys))
	var referred exp.Expression = payload.Col("parent_reference_payload")
	if spec.fullPayload {
		referred = goqu.Func("jsonb_extract_path", referred, goqu.V("referredSemanticId"))
	}
	referred = goqu.Func("coalesce", goqu.Func("nullif", referred, goqu.Cast(goqu.V("{}"), "jsonb")), goqu.Cast(goqu.V("null"), "jsonb"))
	ds = ds.Where(goqu.Cast(goqu.V(predicate.referredJSON), "jsonb").Eq(referred))
	return goqu.Func("EXISTS", ds), nil
}

func visibleReferenceKeys(ctx context.Context, predicate referencePredicate, spec referenceSQLSpec) (*goqu.SelectDataset, error) {
	keyAlias := spec.alias + "_key"
	key := goqu.T(keyAlias)
	alternatives := make([]exp.Expression, 0, len(predicate.keys))
	for index, expected := range predicate.keys {
		alternatives = append(alternatives, goqu.And(key.Col("position").Eq(index), key.Col("type").Eq(expected.kind), key.Col("value").Eq(expected.value)))
	}
	ds := goqu.Dialect(common.Dialect).From(goqu.T(spec.table+"_key").As(keyAlias)).Select(goqu.COUNT("*")).
		Where(key.Col("reference_id").Eq(goqu.I(spec.alias+".id")), goqu.Or(alternatives...))
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
	if err != nil {
		return nil, err
	}
	collector.SetRootJoinKey("submodel", "id")
	collector.AllowInlineAliases("submodel", spec.alias, keyAlias)
	fragments := make([]grammar.FragmentStringPattern, 0, 6)
	if strings.HasSuffix(spec.fragment, "[]") {
		fragments = append(fragments, grammar.FragmentStringPattern(strings.TrimSuffix(spec.fragment, "[]")))
	}
	for _, suffix := range []string{"", ".type", ".keys[]", ".keys[].type", ".keys[].value"} {
		fragments = append(fragments, grammar.FragmentStringPattern(spec.fragment+suffix))
	}
	return AddFilterQueriesFromContext(ctx, ds, fragments, collector)
}

func conceptDescriptionReferencePredicate(ctx context.Context, predicate referencePredicate) (exp.Expression, error) {
	field := "isCaseOf"
	if predicate.field != "$cd#isCaseOf" {
		field = "embeddedDataSpecifications"
	}
	array := goqu.Func("coalesce", goqu.Func("jsonb_extract_path", goqu.I("concept_description.data"), goqu.V(field)), goqu.Cast(goqu.V("[]"), "jsonb"))
	var reference exp.Expression = goqu.I("api_reference.value")
	if field == "embeddedDataSpecifications" {
		reference = goqu.Func("jsonb_extract_path", reference, goqu.V("dataSpecification"))
	}
	ds := goqu.Dialect(common.Dialect).From(goqu.Func("jsonb_array_elements", array).As("api_reference")).
		Select(goqu.L("1")).Where(goqu.Cast(goqu.V(predicate.json), "jsonb").Eq(reference))
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootCD)
	if err != nil {
		return nil, err
	}
	ds, err = AddFilterQueryFromContext(ctx, ds, grammar.FragmentStringPattern(predicate.field), collector)
	if err != nil {
		return nil, err
	}
	return goqu.Func("EXISTS", ds), nil
}
