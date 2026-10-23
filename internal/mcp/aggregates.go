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

package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/aol"
)

// Aggregates executes fixed GOQU aggregations over bounded REST-authorized projections.
// Base tables are never exposed to this layer; policy translation remains in the API.
type Aggregates struct {
	Reader  aol.Reader
	DB      *sql.DB
	Timeout time.Duration
}
type modelParams struct {
	SemanticID string          `json:"semanticId"`
	Condition  json.RawMessage `json:"condition,omitempty"`
}

func readModelCatalogue() any {
	return map[string]any{"models": []any{
		map[string]any{"name": "value_statistics_by_semantic_id", "description": "Count, min, max, average and sum of visible numeric Property values, grouped by explicit unit; unknown units are separate.", "parameters": map[string]any{"semanticId": "required string", "condition": "optional AASQL condition object"}, "example": map[string]any{"semanticId": "urn:example:rated-power"}},
		map[string]any{"name": "count_submodels_by_semantic_id", "description": "Count distinct visible submodels by their visible semantic ID.", "parameters": map[string]any{"condition": "optional AASQL condition object"}, "example": map[string]any{}},
	}}
}

// Run rejects unknown models and incomplete scans rather than reporting partial statistics.
func (a Aggregates) Run(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	if name != "value_statistics_by_semantic_id" && name != "count_submodels_by_semantic_id" {
		return nil, aol.Failure("MCP-READMODEL-UNKNOWN", "unknown read model", "Call list_read_models.")
	}
	params, err := readModelParams(raw, name)
	if err != nil {
		return nil, err
	}
	query := map[string]any{"$condition": map[string]any{"$boolean": true}}
	if len(params.Condition) > 0 {
		var condition any
		if err = json.Unmarshal(params.Condition, &condition); err != nil {
			return nil, err
		}
		query["$condition"] = condition
	}
	encoded, _ := json.Marshal(query)
	if err = aol.ValidateQuery("submodels", encoded); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()
	models, err := a.Reader.Scan(ctx, "/query/submodels", query)
	if err != nil {
		return nil, err
	}
	rows := projectAuthorized(models, name, params.SemanticID)
	return a.aggregate(ctx, name, rows)
}
func readModelParams(raw json.RawMessage, name string) (modelParams, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return modelParams{}, aol.Failure("MCP-READMODEL-PARAMTYPE", "parameters must be an object", "Use the published model parameter types.")
	}
	for key := range fields {
		if key != "condition" && (key != "semanticId" || name != "value_statistics_by_semantic_id") {
			return modelParams{}, aol.Failure("MCP-READMODEL-PARAMUNKNOWN", "unknown parameter "+key, "Use list_read_models for valid parameters.")
		}
	}
	var params modelParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, aol.Failure("MCP-READMODEL-PARAMTYPE", err.Error(), "Use typed model parameters.")
	}
	if name == "value_statistics_by_semantic_id" && params.SemanticID == "" {
		return params, aol.Failure("MCP-READMODEL-PARAMREQUIRED", "semanticId is required", "Supply the numeric property's semantic ID.")
	}
	return params, nil
}
func projectAuthorized(models []map[string]any, name, semantic string) []map[string]string {
	result := []map[string]string{}
	seen := map[string]bool{}
	for _, sm := range models {
		id, _ := sm["id"].(string)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if name == "count_submodels_by_semantic_id" {
			if sem := semanticID(sm); sem != "" {
				result = append(result, map[string]string{"semantic_id": sem})
			}
			continue
		}
		elements, _ := sm["submodelElements"].([]any)
		collectNumeric(elements, semantic, &result)
	}
	return result
}
func semanticID(value map[string]any) string {
	ref, _ := value["semanticId"].(map[string]any)
	keys, _ := ref["keys"].([]any)
	if len(keys) != 1 {
		return ""
	}
	key, _ := keys[0].(map[string]any)
	text, _ := key["value"].(string)
	return text
}
func collectNumeric(elements []any, semantic string, result *[]map[string]string) {
	for _, raw := range elements {
		element, _ := raw.(map[string]any)
		model, _ := element["modelType"].(string)
		if model == "Property" && semanticID(element) == semantic && numericType(element["valueType"]) {
			value, _ := element["value"].(string)
			number, err := strconv.ParseFloat(value, 64)
			if err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
				*result = append(*result, map[string]string{"value": value, "unit": unit(element)})
			}
		}
		if model == "SubmodelElementCollection" || model == "SubmodelElementList" {
			nested, _ := element["value"].([]any)
			collectNumeric(nested, semantic, result)
		}
		if model == "Entity" {
			nested, _ := element["statements"].([]any)
			collectNumeric(nested, semantic, result)
		}
	}
}
func unit(element map[string]any) string {
	specs, _ := element["embeddedDataSpecifications"].([]any)
	for _, raw := range specs {
		spec, _ := raw.(map[string]any)
		content, _ := spec["dataSpecificationContent"].(map[string]any)
		u, _ := content["unit"].(string)
		if u != "" {
			return u
		}
	}
	return ""
}
func (a Aggregates) aggregate(ctx context.Context, name string, rows []map[string]string) (any, error) {
	raw, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	field := func(key string) exp.SQLFunctionExpression {
		return goqu.Func("jsonb_extract_path_text", goqu.I("authorized.value"), goqu.V(key))
	}
	base := goqu.Dialect("postgres").From(goqu.Func("jsonb_array_elements", goqu.Cast(goqu.V(string(raw)), "jsonb")).As("authorized"))
	groupKey := "semantic_id"
	if name == "value_statistics_by_semantic_id" {
		groupKey = "unit"
	}
	query := base.Select(field(groupKey).As("group_key"), goqu.COUNT(goqu.Star())).GroupBy(goqu.I("group_key"))
	if name == "value_statistics_by_semantic_id" {
		value := goqu.Cast(field("value"), "numeric")
		query = query.SelectAppend(goqu.MIN(value), goqu.MAX(value), goqu.AVG(value), goqu.SUM(value))
	}
	statement, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("MCP-READMODEL-BUILD: %w", err)
	}
	tx, err := a.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("MCP-READMODEL-BEGIN: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	timeoutSQL, timeoutArgs, err := goqu.Dialect("postgres").Select(goqu.Func("set_config", goqu.V("statement_timeout"), goqu.V(strconv.FormatInt(a.Timeout.Milliseconds(), 10)), goqu.V(true))).Prepared(true).ToSQL()
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, timeoutSQL, timeoutArgs...); err != nil {
		return nil, fmt.Errorf("MCP-READMODEL-TIMEOUT: %w", err)
	}
	result, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, aol.Failure("MCP-READMODEL-EXECUTE", "aggregation failed or exceeded its execution deadline", "Narrow the condition and check database availability.")
	}
	defer func() { _ = result.Close() }()
	values := []any{}
	for result.Next() {
		row, scanErr := scanAggregateRow(result, name)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, row)
	}
	if err = result.Err(); err != nil {
		return nil, err
	}
	if err = result.Close(); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]any{"model": name, "result": values, "complete": true, "source": "/query/submodels", "untrusted": true}, nil
}

func scanAggregateRow(result *sql.Rows, name string) (map[string]any, error) {
	var group string
	var count int64
	if name != "value_statistics_by_semantic_id" {
		if err := result.Scan(&group, &count); err != nil {
			return nil, err
		}
		return map[string]any{"semanticId": group, "count": count}, nil
	}
	var minimum, maximum, average, sum sql.NullString
	if err := result.Scan(&group, &count, &minimum, &maximum, &average, &sum); err != nil {
		return nil, err
	}
	return map[string]any{"unit": group, "count": count, "min": minimum.String, "max": maximum.String, "avg": average.String, "sum": sum.String}, nil
}

func numericType(raw any) bool {
	value, _ := raw.(string)
	switch value {
	case "xs:double", "xs:float", "xs:decimal", "xs:integer", "xs:int", "xs:long", "xs:short", "xs:byte", "xs:unsignedLong", "xs:unsignedInt", "xs:unsignedShort", "xs:unsignedByte", "xs:positiveInteger", "xs:negativeInteger", "xs:nonPositiveInteger", "xs:nonNegativeInteger":
		return true
	}
	return false
}
