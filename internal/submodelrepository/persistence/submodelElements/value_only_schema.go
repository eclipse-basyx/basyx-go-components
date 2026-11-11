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
	"context"
	"database/sql"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type valueOnlySchemaRow struct {
	node     *gen.ValueOnlySchemaNode
	parentID sql.NullInt64
	idShort  sql.NullString
	position sql.NullInt64
}

// ValueOnlySchemaRequest names the stored elements a ValueOnly update needs: the addressed root elements
// and the paths below them that the payload can address. With TopLevel set, roots are top-level idShorts.
type ValueOnlySchemaRequest struct {
	SubmodelDatabaseID int64
	RootPaths          []string
	AddressedPaths     []string
	TopLevel           bool
}

// LoadValueOnlySchemaTx loads the stored schema of the elements a ValueOnly update addresses with one
// indexed query. Elements of the stored subtree that the payload does not address are not loaded.
// The result maps every requested root path that exists to its schema node.
func LoadValueOnlySchemaTx(ctx context.Context, tx *sql.Tx, request ValueOnlySchemaRequest) (map[string]*gen.ValueOnlySchemaNode, error) {
	if len(request.RootPaths) == 0 {
		return map[string]*gen.ValueOnlySchemaNode{}, nil
	}
	query, args, err := buildValueOnlySchemaQuery(request)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-LOADVALONLYSCHEMA-BUILDQ " + err.Error())
	}
	rows, err := tx.QueryContext(ctx, query, args...)
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
	return linkValueOnlySchemaRows(schemaRows, request), nil
}

func buildValueOnlySchemaQuery(request ValueOnlySchemaRequest) (string, []any, error) {
	rootCondition := []exp.Expression{common.PostgreSQLTextArrayContains(goqu.I("sme.idshort_path"), request.RootPaths)}
	if request.TopLevel {
		rootCondition = append(rootCondition, goqu.I("sme.parent_sme_id").IsNull())
	}
	pathCondition := goqu.And(rootCondition...)
	if len(request.AddressedPaths) > 0 {
		pathCondition = goqu.Or(pathCondition, common.PostgreSQLTextArrayContains(goqu.I("sme.idshort_path"), request.AddressedPaths))
	}
	return goqu.Dialect("postgres").
		From(goqu.T("submodel_element").As("sme")).
		LeftJoin(goqu.T("property_element").As("pe"), goqu.On(goqu.I("pe.id").Eq(goqu.I("sme.id")))).
		LeftJoin(goqu.T("range_element").As("re"), goqu.On(goqu.I("re.id").Eq(goqu.I("sme.id")))).
		Select(
			goqu.I("sme.id"),
			goqu.I("sme.parent_sme_id"),
			goqu.I("sme.id_short"),
			goqu.I("sme.model_type"),
			goqu.I("sme.idshort_path"),
			goqu.I("sme.position"),
			goqu.COALESCE(goqu.I("pe.value_type"), goqu.I("re.value_type"), 0).As("value_type"),
		).
		Where(goqu.I("sme.submodel_id").Eq(request.SubmodelDatabaseID), pathCondition).
		Order(
			goqu.I("sme.parent_sme_id").Asc().NullsFirst(),
			goqu.I("sme.position").Asc().NullsLast(),
			goqu.I("sme.id").Asc(),
		).
		Prepared(true).
		ToSQL()
}

func scanValueOnlySchemaRows(rows *sql.Rows) ([]valueOnlySchemaRow, error) {
	var result []valueOnlySchemaRow
	for rows.Next() {
		var modelType, valueType int64
		node := &gen.ValueOnlySchemaNode{}
		row := valueOnlySchemaRow{node: node}
		if err := rows.Scan(&node.ElementID, &row.parentID, &row.idShort, &modelType, &node.IDShortPath, &row.position, &valueType); err != nil {
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

// linkValueOnlySchemaRows builds the schema trees of the requested roots. Rows whose parent was not
// loaded cannot be reached from a root and are ignored.
func linkValueOnlySchemaRows(rows []valueOnlySchemaRow, request ValueOnlySchemaRequest) map[string]*gen.ValueOnlySchemaNode {
	nodes := make(map[int64]*gen.ValueOnlySchemaNode, len(rows))
	for _, row := range rows {
		nodes[row.node.ElementID] = row.node
	}
	rootPaths := make(map[string]struct{}, len(request.RootPaths))
	for _, path := range request.RootPaths {
		rootPaths[path] = struct{}{}
	}
	roots := make(map[string]*gen.ValueOnlySchemaNode)
	for _, row := range rows {
		if isRequestedValueOnlyRoot(row, rootPaths, request.TopLevel) {
			roots[row.node.IDShortPath] = row.node
			continue
		}
		if parent, ok := nodes[row.parentID.Int64]; ok && row.parentID.Valid {
			attachValueOnlySchemaChild(parent, row)
		}
	}
	return roots
}

func isRequestedValueOnlyRoot(row valueOnlySchemaRow, rootPaths map[string]struct{}, topLevel bool) bool {
	if topLevel && row.parentID.Valid {
		return false
	}
	_, requested := rootPaths[row.node.IDShortPath]
	return requested
}

func attachValueOnlySchemaChild(parent *gen.ValueOnlySchemaNode, row valueOnlySchemaRow) {
	if parent.ModelType == types.ModelTypeSubmodelElementList {
		attachValueOnlyListItem(parent, row)
		return
	}
	if parent.Children == nil {
		parent.Children = make(map[string]*gen.ValueOnlySchemaNode)
	}
	parent.Children[row.idShort.String] = row.node
}

// attachValueOnlyListItem places a list item at its stored position, which is also the index in its idShort path.
func attachValueOnlyListItem(parent *gen.ValueOnlySchemaNode, row valueOnlySchemaRow) {
	if !row.position.Valid || row.position.Int64 < 0 {
		return
	}
	position := int(row.position.Int64)
	for len(parent.Items) <= position {
		parent.Items = append(parent.Items, nil)
	}
	parent.Items[position] = row.node
}
