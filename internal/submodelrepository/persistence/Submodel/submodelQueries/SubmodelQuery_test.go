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

package submodelQueries

import (
	"strings"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestAddPaginationToQuery(t *testing.T) {
	dialect := goqu.Dialect("postgres")

	tests := []struct {
		name     string
		limit    int64
		cursor   string
		wantSQL  string
		wantArgs []interface{}
	}{
		{
			name:    "no pagination - but still orders",
			limit:   0,
			cursor:  "",
			wantSQL: `SELECT "s"."id" FROM "submodel" AS "s" ORDER BY "s"."id" ASC`,
		},
		{
			name:    "limit only",
			limit:   10,
			cursor:  "",
			wantSQL: `SELECT "s"."id" FROM "submodel" AS "s" ORDER BY "s"."id" ASC LIMIT 11`,
		},
		{
			name:    "cursor only",
			limit:   0,
			cursor:  "submodel-123",
			wantSQL: `SELECT "s"."id" FROM "submodel" AS "s" WHERE ("s"."id" > 'submodel-123') ORDER BY "s"."id" ASC`,
		},
		{
			name:    "limit and cursor",
			limit:   5,
			cursor:  "submodel-456",
			wantSQL: `SELECT "s"."id" FROM "submodel" AS "s" WHERE ("s"."id" > 'submodel-456') ORDER BY "s"."id" ASC LIMIT 6`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a simple query
			query := dialect.From(goqu.T("submodel").As("s")).Select(goqu.I("s.id"))

			// Apply pagination
			result := addPaginationToQuery(query, tt.limit, tt.cursor)

			// Generate SQL
			sql, _, err := result.ToSQL()
			if err != nil {
				t.Fatalf("Failed to generate SQL: %v", err)
			}

			if sql != tt.wantSQL {
				t.Errorf("Expected SQL: %s\nGot SQL: %s", tt.wantSQL, sql)
			}
		})
	}
}

func TestGetQueryWithGoquPagination(t *testing.T) {
	tests := []struct {
		name        string
		submodelID  string
		limit       int64
		cursor      string
		aasQuery    *grammar.QueryWrapper
		shouldError bool
		sqlContains []string
	}{
		{
			name:       "basic pagination",
			submodelID: "",
			limit:      10,
			cursor:     "",
			aasQuery:   nil,
			sqlContains: []string{
				"ORDER BY",
				`"s"."id" ASC`,
				"LIMIT 11",
			},
		},
		{
			name:       "with cursor",
			submodelID: "",
			limit:      5,
			cursor:     "test-submodel",
			aasQuery:   nil,
			sqlContains: []string{
				"ORDER BY",
				`"s"."id" ASC`,
				"LIMIT 6",
				`"s"."id" > 'test-submodel'`,
			},
		},
		{
			name:        "no pagination",
			submodelID:  "",
			limit:       0,
			cursor:      "",
			aasQuery:    nil,
			sqlContains: []string{
				// Should NOT contain ORDER BY when no pagination is requested
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, err := GetQueryWithGoqu(tt.submodelID, tt.limit, tt.cursor, tt.aasQuery)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Check that the SQL contains expected elements
			for _, contains := range tt.sqlContains {
				if contains != "" && !containsIgnoreCase(sql, contains) {
					t.Errorf("Expected SQL to contain '%s', but it didn't.\nSQL: %s", contains, sql)
				}
			}

			// For no pagination test, ensure no ORDER BY is present
			if tt.name == "no pagination" {
				if containsIgnoreCase(sql, "ORDER BY") {
					t.Errorf("Expected SQL to NOT contain ORDER BY for no pagination case, but it did.\nSQL: %s", sql)
				}
			}
		})
	}
}

// Helper function to check if a string contains another string (case insensitive)
func containsIgnoreCase(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}
