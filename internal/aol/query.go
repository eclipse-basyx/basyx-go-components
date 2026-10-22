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

package aol

import (
	"encoding/json"
	"fmt"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// ValidateQuery reuses the API expression decoder and field resolver without execution.
func ValidateQuery(target string, raw json.RawMessage) error {
	collector, err := queryCollector(target)
	if err != nil {
		return err
	}
	var query grammar.Query
	if err = json.Unmarshal(raw, &query); err != nil {
		return queryFailure(err)
	}
	if err = grammar.AssertQueryRequired(query); err != nil {
		return queryFailure(err)
	}
	if err = grammar.AssertQueryConstraints(query); err != nil {
		return queryFailure(err)
	}
	if _, _, err = query.Condition.EvaluateToExpression(collector); err != nil {
		return queryFailure(err)
	}
	for _, filter := range query.FilterConditions {
		if _, _, err = filter.Condition.EvaluateToExpression(collector); err != nil {
			return queryFailure(err)
		}
	}
	return nil
}
func queryCollector(target string) (*grammar.ResolvedFieldPathCollector, error) {
	if target == "shells" {
		return grammar.NewResolvedFieldPathCollectorForAAS(true), nil
	}
	if target == "submodels" {
		return grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
	}
	return nil, Failure("MCP-QUERY-TARGET", "unsupported query target", "Select shells or submodels.")
}
func queryFailure(err error) *Error {
	result := Failure("MCP-QUERY-VALIDATE", err.Error(), "Use aasql://grammar and aasql://fields; correct the indicated JSON expression.")
	result.Location = "/aasql"
	if syntax, ok := err.(*json.SyntaxError); ok {
		result.Location = fmt.Sprintf("/aasql (byte %d)", syntax.Offset)
	}
	return result
}
