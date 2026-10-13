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

package submodelelements

import (
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

const valueOnlySchemaCTE = "value_only_schema"

type valueOnlySchemaRow struct {
	node     *gen.ValueOnlySchemaNode
	parentID sql.NullInt64
	idShort  sql.NullString
}

// LoadValueOnlySchemaTx loads the stored schema of the submodel elements addressed by a ValueOnly update
// with a single recursive query. With topLevel set, paths are top-level idShorts of the submodel.
// The result maps every requested path that exists to the root node of its stored subtree.
func LoadValueOnlySchemaTx(tx *sql.Tx, submodelDatabaseID int64, paths []string, topLevel bool) (map[string]*gen.ValueOnlySchemaNode, error) {
	if len(paths) == 0 {
		return map[string]*gen.ValueOnlySchemaNode{}, nil
	}
	query, args, err := buildValueOnlySchemaQuery(submodelDatabaseID, paths, topLevel)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-LOADVALONLYSCHEMA-BUILDQ " + err.Error())
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-LOADVALONLYSCHEMA-EXECQ " + err.Error())
	}
	defer func() {
		_ = rows.Close()
	}()
	schemaRows, err := scanValueOnlySchemaRows(rows)
	if err != nil {
		return nil, err
	}
	return linkValueOnlySchemaRows(schemaRows), nil
}

func buildValueOnlySchemaQuery(submodelDatabaseID int64, paths []string, topLevel bool) (string, []any, error) {
	dialect := goqu.Dialect("postgres")
	seedConditions := []exp.Expression{
		goqu.I("root.submodel_id").Eq(submodelDatabaseID),
		goqu.I("root.idshort_path").In(paths),
	}
	if topLevel {
		seedConditions = append(seedConditions, goqu.I("root.parent_sme_id").IsNull())
	}
	seed := dialect.From(goqu.T("submodel_element").As("root")).
		Select(valueOnlySchemaColumns("root")...).
		Where(seedConditions...)
	step := dialect.From(goqu.T("submodel_element").As("child")).
		InnerJoin(goqu.T(valueOnlySchemaCTE).As("parent"), goqu.On(goqu.I("child.parent_sme_id").Eq(goqu.I("parent.id")))).
		Select(valueOnlySchemaColumns("child")...).
		Where(goqu.I("child.submodel_id").Eq(submodelDatabaseID))
	return dialect.From(goqu.T(valueOnlySchemaCTE).As("tree")).
		WithRecursive(valueOnlySchemaCTE+"(id, parent_sme_id, id_short, position, model_type, idshort_path)", seed.UnionAll(step)).
		LeftJoin(goqu.T("property_element").As("pe"), goqu.On(goqu.I("pe.id").Eq(goqu.I("tree.id")))).
		LeftJoin(goqu.T("range_element").As("re"), goqu.On(goqu.I("re.id").Eq(goqu.I("tree.id")))).
		Select(
			goqu.I("tree.id"),
			goqu.I("tree.parent_sme_id"),
			goqu.I("tree.id_short"),
			goqu.I("tree.model_type"),
			goqu.I("tree.idshort_path"),
			goqu.COALESCE(goqu.I("pe.value_type"), goqu.I("re.value_type"), 0).As("value_type"),
		).
		Order(
			goqu.I("tree.parent_sme_id").Asc().NullsFirst(),
			goqu.I("tree.position").Asc().NullsLast(),
			goqu.I("tree.id").Asc(),
		).
		Prepared(true).
		ToSQL()
}

func valueOnlySchemaColumns(alias string) []any {
	return []any{
		goqu.I(alias + ".id"),
		goqu.I(alias + ".parent_sme_id"),
		goqu.I(alias + ".id_short"),
		goqu.I(alias + ".position"),
		goqu.I(alias + ".model_type"),
		goqu.I(alias + ".idshort_path"),
	}
}

func scanValueOnlySchemaRows(rows *sql.Rows) ([]valueOnlySchemaRow, error) {
	var result []valueOnlySchemaRow
	for rows.Next() {
		var modelType, valueType int64
		node := &gen.ValueOnlySchemaNode{}
		row := valueOnlySchemaRow{node: node}
		if err := rows.Scan(&node.ElementID, &row.parentID, &row.idShort, &modelType, &node.IDShortPath, &valueType); err != nil {
			return nil, common.NewInternalServerError("SMREPO-LOADVALONLYSCHEMA-SCAN " + err.Error())
		}
		node.ModelType = types.ModelType(modelType)
		node.ValueType = types.DataTypeDefXSD(valueType)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, common.NewInternalServerError("SMREPO-LOADVALONLYSCHEMA-ROWS " + err.Error())
	}
	return result, nil
}

func linkValueOnlySchemaRows(rows []valueOnlySchemaRow) map[string]*gen.ValueOnlySchemaNode {
	nodes := make(map[int64]*gen.ValueOnlySchemaNode, len(rows))
	for _, row := range rows {
		nodes[row.node.ElementID] = row.node
	}
	roots := make(map[string]*gen.ValueOnlySchemaNode)
	for _, row := range rows {
		parent, hasParent := nodes[row.parentID.Int64]
		if !row.parentID.Valid || !hasParent {
			roots[row.node.IDShortPath] = row.node
			continue
		}
		attachValueOnlySchemaChild(parent, row)
	}
	return roots
}

func attachValueOnlySchemaChild(parent *gen.ValueOnlySchemaNode, row valueOnlySchemaRow) {
	if parent.ModelType == types.ModelTypeSubmodelElementList {
		parent.Items = append(parent.Items, row.node)
		return
	}
	if parent.Children == nil {
		parent.Children = make(map[string]*gen.ValueOnlySchemaNode)
	}
	parent.Children[row.idShort.String] = row.node
}
