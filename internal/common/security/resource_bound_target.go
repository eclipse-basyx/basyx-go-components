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
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

type boundTarget struct {
	Kind     string
	AAS      string
	Submodel string
	Path     string
	Access   bool
	Suffix   string
}

func parseBoundTarget(path, basePath string) (boundTarget, error) {
	path = strings.TrimPrefix(path, strings.TrimRight(basePath, "/"))
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range parts {
		decoded, err := url.PathUnescape(part)
		if err != nil {
			return boundTarget{}, fmt.Errorf("REBAC-TARGET-ESCAPE %w", err)
		}
		parts[i] = decoded
	}
	target, index, err := parseBoundRoot(parts)
	if err != nil {
		return target, err
	}
	target, index, err = parseBoundChildren(target, parts, index)
	if err != nil {
		return target, err
	}
	if index < len(parts) && parts[index] == "$access" {
		target.Access = true
		index++
	}
	target.Suffix = strings.Join(parts[index:], "/")
	return target, nil
}

func parseBoundRoot(parts []string) (boundTarget, int, error) {
	var target boundTarget
	switch parts[0] {
	case "shells":
		target.Kind = "aas"
	case "submodels":
		target.Kind = "submodel"
	default:
		return target, 0, fmt.Errorf("REBAC-TARGET-ROUTE unsupported route")
	}
	if len(parts) == 1 || strings.HasPrefix(parts[1], "$") {
		target.Kind = parts[0]
		return target, 1, nil
	}
	id, err := common.DecodeString(parts[1])
	if err != nil {
		return target, 0, fmt.Errorf("REBAC-TARGET-IDENTIFIER %w", err)
	}
	if target.Kind == "aas" {
		target.AAS = id
	} else {
		target.Submodel = id
	}
	return target, 2, nil
}

func parseBoundChildren(target boundTarget, parts []string, index int) (boundTarget, int, error) {
	if target.Kind == "aas" && index < len(parts) && parts[index] == "submodels" {
		if index+1 >= len(parts) {
			return target, index, fmt.Errorf("REBAC-TARGET-SUBMODEL missing identifier")
		}
		id, err := common.DecodeString(parts[index+1])
		if err != nil {
			return target, index, fmt.Errorf("REBAC-TARGET-IDENTIFIER %w", err)
		}
		target.Kind = "submodel"
		target.Submodel = id
		index += 2
	}
	if target.Kind == "submodel" && index < len(parts) && parts[index] == "submodel-elements" {
		index++
		if index < len(parts) && !strings.HasPrefix(parts[index], "$") {
			target.Kind = "sme"
			target.Path = parts[index]
			index++
		}
	}
	return target, index, nil
}

func boundTargetFromObject(object grammar.ObjectItem) (boundTarget, error) {
	if _, err := ResourceBoundKey(object); err != nil {
		return boundTarget{}, err
	}
	switch object.Kind {
	case grammar.Route:
		return parseBoundTarget(object.Route.Route, "")
	case grammar.Identifiable:
		if object.Identifiable.Scope == "$aas" {
			return boundTarget{Kind: "aas", AAS: object.Identifiable.ID.ID}, nil
		}
		return boundTarget{Kind: "submodel", Submodel: object.Identifiable.ID.ID}, nil
	case grammar.Referable:
		return boundTarget{Kind: "sme", Submodel: object.Referable.ID.ID, Path: object.Referable.IDShortPath}, nil
	}
	return boundTarget{}, fmt.Errorf("REBAC-TARGET-OBJECT unsupported object")
}

func (target boundTarget) object() grammar.ObjectItem {
	if target.Kind == "shells" || target.Kind == "submodels" {
		return grammar.ObjectItem{Kind: grammar.Route, Route: &grammar.RouteValue{Route: "/" + target.Kind}}
	}
	if target.Kind == "sme" {
		return grammar.ObjectItem{Kind: grammar.Referable, Referable: &grammar.ReferableValue{Scope: "$sme", ID: grammar.Identifier{ID: target.Submodel}, IDShortPath: target.Path}}
	}
	scope, id := "$sm", target.Submodel
	if target.Kind == "aas" {
		scope, id = "$aas", target.AAS
	}
	return grammar.ObjectItem{Kind: grammar.Identifiable, Identifiable: &grammar.IdentifiableValue{Scope: scope, ID: grammar.Identifier{ID: id}}}
}

func (target boundTarget) condition() (exp.Expression, error) {
	dialect := goqu.Dialect("postgres")
	switch target.Kind {
	case "shells", "submodels":
		return goqu.C("collection").Eq("/" + target.Kind), nil
	case "aas":
		return goqu.C("aas_id").Eq(dialect.From("aas").Select("id").Where(goqu.Ex{"aas_id": target.AAS})), nil
	case "submodel":
		return goqu.C("submodel_id").Eq(dialect.From("submodel").Select("id").Where(goqu.Ex{"submodel_identifier": target.Submodel})), nil
	case "sme":
		ds := dialect.From(goqu.T("submodel_element").As("sme")).Join(goqu.T("submodel").As("sm"), goqu.On(goqu.I("sm.id").Eq(goqu.I("sme.submodel_id")))).Select(goqu.I("sme.id")).Where(goqu.Ex{"sm.submodel_identifier": target.Submodel, "sme.idshort_path": target.Path})
		return goqu.C("sme_id").Eq(ds), nil
	}
	return nil, fmt.Errorf("REBAC-TARGET-KIND unsupported resource kind")
}

func (repo *resourceBoundRepository) effective(ctx context.Context, db boundQueryer, target boundTarget) (*boundAccess, error) {
	for depth := 0; depth < 256; depth++ {
		access, err := repo.load(ctx, db, target)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if access != nil && access.Policy != nil {
			return access, nil
		}
		parent, ok, err := target.parent(ctx, db)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		target = parent
	}
	return nil, fmt.Errorf("REBAC-EFFECTIVE-DEPTH resource hierarchy exceeds 256 levels")
}

func (target boundTarget) parent(ctx context.Context, db boundQueryer) (boundTarget, bool, error) {
	switch target.Kind {
	case "aas":
		return boundTarget{Kind: "shells"}, true, nil
	case "submodel":
		return target.aasParent(ctx, db)
	case "sme":
		return target.smeParent(ctx, db)
	}
	return boundTarget{}, false, nil
}

func (target boundTarget) aasParent(ctx context.Context, db boundQueryer) (boundTarget, bool, error) {
	ds := goqu.Dialect("postgres").From(goqu.T("aas_submodel_reference").As("r")).Join(goqu.T("aas_submodel_reference_key").As("k"), goqu.On(goqu.I("k.reference_id").Eq(goqu.I("r.id")))).Join(goqu.T("aas").As("a"), goqu.On(goqu.I("a.id").Eq(goqu.I("r.aas_id")))).Select(goqu.I("a.aas_id")).Distinct().Where(goqu.Ex{"k.value": target.Submodel, "k.position": 0, "k.type": 20, "r.type": 1}).Limit(2)
	extra := goqu.Dialect("postgres").From(goqu.T("aas_submodel_reference_key").As("extra")).Select(goqu.L("1")).Where(goqu.I("extra.reference_id").Eq(goqu.I("r.id")), goqu.I("extra.position").Neq(0))
	ds = ds.Where(goqu.L("NOT EXISTS ?", extra))
	if target.AAS != "" {
		ds = ds.Where(goqu.Ex{"a.aas_id": target.AAS})
	}
	query, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		return boundTarget{}, false, fmt.Errorf("REBAC-PARENT-BUILD %w", err)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return boundTarget{}, false, fmt.Errorf("REBAC-PARENT-QUERY %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return boundTarget{}, false, fmt.Errorf("REBAC-PARENT-SCAN %w", err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return boundTarget{}, false, fmt.Errorf("REBAC-PARENT-ROWS %w", err)
	}
	if len(ids) == 1 {
		return boundTarget{Kind: "aas", AAS: ids[0]}, true, nil
	}
	if len(ids) == 0 && target.AAS == "" {
		return boundTarget{Kind: "submodels"}, true, nil
	}
	return boundTarget{}, false, nil
}

func (target boundTarget) smeParent(ctx context.Context, db boundQueryer) (boundTarget, bool, error) {
	ds := goqu.Dialect("postgres").From(goqu.T("submodel_element").As("child")).Join(goqu.T("submodel").As("sm"), goqu.On(goqu.I("sm.id").Eq(goqu.I("child.submodel_id")))).LeftJoin(goqu.T("submodel_element").As("parent"), goqu.On(goqu.I("parent.id").Eq(goqu.I("child.parent_sme_id")))).Select(goqu.I("parent.idshort_path")).Where(goqu.Ex{"sm.submodel_identifier": target.Submodel, "child.idshort_path": target.Path})
	query, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		return boundTarget{}, false, fmt.Errorf("REBAC-PARENT-BUILD %w", err)
	}
	var path sql.NullString
	if err = db.QueryRowContext(ctx, query, args...).Scan(&path); err != nil {
		return boundTarget{}, false, fmt.Errorf("REBAC-PARENT-QUERY %w", err)
	}
	target.Kind = "submodel"
	target.Path = ""
	if path.Valid {
		target.Kind = "sme"
		target.Path = path.String
	}
	return target, true, nil
}

func boundObjectsEqual(left, right grammar.ObjectItem) bool {
	a, err := ResourceBoundKey(left)
	if err != nil {
		return false
	}
	b, err := ResourceBoundKey(right)
	return err == nil && a == b
}

func boundPrincipalRule(principal common.AccessPrincipal, rights []grammar.RightsEnum) (json.RawMessage, error) {
	value := map[string]any{"ACL": map[string]any{"ATTRIBUTES": []any{map[string]string{"CLAIM": "iss"}, map[string]string{"CLAIM": "sub"}}, "RIGHTS": rights, "ACCESS": "ALLOW"}, "FORMULA": map[string]any{"$and": []any{
		map[string]any{"$eq": []any{map[string]any{"$attribute": map[string]string{"CLAIM": "iss"}}, map[string]string{"$strVal": principal.Issuer}}},
		map[string]any{"$eq": []any{map[string]any{"$attribute": map[string]string{"CLAIM": "sub"}}, map[string]string{"$strVal": principal.Subject}}},
	}}}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("REBAC-GRANTRULE-ENCODE %w", err)
	}
	if _, err = compileBoundRule(data); err != nil {
		return nil, err
	}
	return data, nil
}

func validateBoundContext(ctx context.Context, db boundQueryer, target boundTarget) error {
	if target.AAS == "" || target.Submodel == "" {
		return nil
	}
	_, valid, err := target.aasParent(ctx, db)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("REBAC-TARGET-CONTEXT %w", sql.ErrNoRows)
	}
	return nil
}
