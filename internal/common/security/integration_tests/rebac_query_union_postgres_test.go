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

package integration_tests

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestReBACQueryUnionPostgreSQLScopesRows(t *testing.T) {
	db := rebacQueryUnionDatabase(t)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	prefix := "rebac-query-union-" + uuid.NewString()
	assertReBACSMESiblingIsolation(t, tx, prefix)
	assertReBACSubmodelDescriptorScopes(t, tx, prefix)
}

func rebacQueryUnionDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BASYX_REBAC_TEST_DSN")
	if dsn == "" {
		t.Skip("BASYX_REBAC_TEST_DSN is required for live PostgreSQL tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func assertReBACSMESiblingIsolation(t *testing.T, tx *sql.Tx, prefix string) {
	t.Helper()
	var submodelID int64
	if err := tx.QueryRowContext(t.Context(), `INSERT INTO submodel (submodel_identifier, id_short) VALUES ($1, $2) RETURNING id`, prefix+"-sm", "sm").Scan(&submodelID); err != nil {
		t.Fatal(err)
	}
	for _, element := range []struct{ idShort, path string }{{"temperature", "visible.temperature"}, {"sibling", "hidden.sibling"}} {
		if _, err := tx.ExecContext(t.Context(), `INSERT INTO submodel_element (submodel_id, model_type, id_short, idshort_path) VALUES ($1, $2, $3, $4)`, submodelID, 1, element.idShort, element.path); err != nil {
			t.Fatal(err)
		}
	}

	evaluation, err := auth.UnionAuthorizationEvaluationWithReBACReadGrants(
		auth.SemanticResourceSME,
		auth.AuthorizationEvaluation{},
		auth.ReBACReadGrantSet{Elements: []auth.ReBACElementReadGrant{{SubmodelID: prefix + "-sm", ElementPath: "visible.temperature"}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSME)
	if err != nil {
		t.Fatal(err)
	}
	query := goqu.Dialect("postgres").From(goqu.T("submodel_element").As("sme")).
		Select(goqu.I("sme.idshort_path")).
		Where(goqu.I("sme.submodel_id").Eq(submodelID))
	query, err = auth.AddSMEFormulaQueryFromContext(auth.WithQueryFilter(t.Context(), evaluation.QueryFilter), query, collector, "sme")
	if err != nil {
		t.Fatal(err)
	}
	paths := executeStringQuery(t, tx, query)
	if fmt.Sprint(paths) != "[visible.temperature]" {
		t.Fatalf("SME ReBAC grant exposed a sibling: %v", paths)
	}
}

func assertReBACSubmodelDescriptorScopes(t *testing.T, tx *sql.Tx, prefix string) {
	t.Helper()
	parentOne := insertAASDescriptor(t, tx, prefix+"-aas-one")
	parentTwo := insertAASDescriptor(t, tx, prefix+"-aas-two")
	insertSubmodelDescriptor(t, tx, nil, prefix+"-standalone")
	insertSubmodelDescriptor(t, tx, &parentOne, prefix+"-embedded")
	insertSubmodelDescriptor(t, tx, &parentTwo, prefix+"-embedded")

	evaluation, err := auth.UnionAuthorizationEvaluationWithReBACReadGrants(
		auth.SemanticResourceSMDesc,
		auth.AuthorizationEvaluation{},
		auth.ReBACReadGrantSet{
			Identifiers: []string{prefix + "-standalone"},
			EmbeddedSubmodelDescriptors: []auth.ReBACEmbeddedSubmodelDescriptorReadGrant{{
				AASDescriptorID: prefix + "-aas-one",
				SubmodelID:      prefix + "-embedded",
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithQueryFilter(t.Context(), evaluation.QueryFilter)
	standalone := executeStringQuery(t, tx, rebacStandaloneDescriptorQuery(ctx, t))
	if fmt.Sprint(standalone) != "["+prefix+"-standalone]" {
		t.Fatalf("standalone descriptor query crossed scopes: %v", standalone)
	}
	embedded := executeStringQuery(t, tx, rebacEmbeddedDescriptorQuery(ctx, t))
	if fmt.Sprint(embedded) != "["+prefix+"-embedded]" {
		t.Fatalf("embedded descriptor query crossed scopes: %v", embedded)
	}
}

func insertAASDescriptor(t *testing.T, tx *sql.Tx, identifier string) int64 {
	t.Helper()
	var descriptorID int64
	if err := tx.QueryRowContext(t.Context(), `INSERT INTO descriptor DEFAULT VALUES RETURNING id`).Scan(&descriptorID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(t.Context(), `INSERT INTO aas_descriptor (descriptor_id, id) VALUES ($1, $2)`, descriptorID, identifier); err != nil {
		t.Fatal(err)
	}
	return descriptorID
}

func insertSubmodelDescriptor(t *testing.T, tx *sql.Tx, aasDescriptorID *int64, identifier string) {
	t.Helper()
	var descriptorID int64
	if err := tx.QueryRowContext(t.Context(), `INSERT INTO descriptor DEFAULT VALUES RETURNING id`).Scan(&descriptorID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(t.Context(), `INSERT INTO submodel_descriptor (descriptor_id, position, aas_descriptor_id, id) VALUES ($1, $2, $3, $4)`, descriptorID, 0, aasDescriptorID, identifier); err != nil {
		t.Fatal(err)
	}
}

func rebacStandaloneDescriptorQuery(ctx context.Context, t *testing.T) *goqu.SelectDataset {
	t.Helper()
	collector, err := grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSMDesc)
	if err != nil {
		t.Fatal(err)
	}
	query := goqu.Dialect("postgres").From(goqu.T("submodel_descriptor").As("submodel_descriptor")).
		Select(goqu.I("submodel_descriptor.id")).
		Where(goqu.I("submodel_descriptor.aas_descriptor_id").IsNull())
	query, err = auth.AddSubmodelDescriptorFormulaQueryFromContext(ctx, query, collector, false)
	if err != nil {
		t.Fatal(err)
	}
	return query
}

func rebacEmbeddedDescriptorQuery(ctx context.Context, t *testing.T) *goqu.SelectDataset {
	t.Helper()
	collector, err := grammar.NewResolvedFieldPathCollectorForNestedSMDesc()
	if err != nil {
		t.Fatal(err)
	}
	query := goqu.Dialect("postgres").From(goqu.T("submodel_descriptor").As("submodel_descriptor")).
		Join(goqu.T("aas_descriptor").As("aas_descriptor"), goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("submodel_descriptor.aas_descriptor_id")))).
		Select(goqu.I("submodel_descriptor.id"))
	query, err = auth.AddSubmodelDescriptorFormulaQueryFromContext(ctx, query, collector, true)
	if err != nil {
		t.Fatal(err)
	}
	return query
}

func executeStringQuery(t *testing.T, tx *sql.Tx, query *goqu.SelectDataset) []string {
	t.Helper()
	sqlQuery, args, err := query.Prepared(true).ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := tx.QueryContext(t.Context(), sqlQuery, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]string, 0)
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		result = append(result, value)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(result)
	return result
}
