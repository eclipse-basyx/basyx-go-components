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

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

type persistedElementHierarchy struct {
	id       int64
	parentID sql.NullInt64
	rootID   int64
	depth    int
	position int
}

type elementReconciliationFixture struct {
	t          *testing.T
	db         *sql.DB
	submodelID string
	endpoint   string
}

func newElementReconciliationFixture(t *testing.T, name string, elements ...any) elementReconciliationFixture {
	t.Helper()
	submodelID := fmt.Sprintf("urn:basyx:integration:%s-%d", name, time.Now().UnixNano())
	endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(submodelID)
	status, body := sendReconciliationRequest(t, http.MethodPost, submodelRepositoryBaseURL+"/submodels", reconciliationTestSubmodel(submodelID, "Before", elements...))
	require.Equal(t, http.StatusCreated, status, "response=%s", string(body))
	t.Cleanup(func() { _, _ = sendReconciliationRequestWithoutFailure(http.MethodDelete, endpoint, nil) })
	db, err := sql.Open("pgx", submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return elementReconciliationFixture{t: t, db: db, submodelID: submodelID, endpoint: endpoint}
}

func (f elementReconciliationFixture) elementEndpoint(path string) string {
	return f.endpoint + "/submodel-elements/" + url.PathEscape(path)
}

func (f elementReconciliationFixture) send(method string, endpoint string, payload any) {
	f.t.Helper()
	status, body := sendReconciliationRequest(f.t, method, endpoint, payload)
	require.Equal(f.t, http.StatusNoContent, status, "%s %s response=%s", method, endpoint, string(body))
}

func (f elementReconciliationFixture) rows() map[string]persistedElementHierarchy {
	f.t.Helper()
	return reconciliationElementHierarchy(f.t, f.db, f.submodelID)
}

func (f elementReconciliationFixture) element(path string) map[string]any {
	f.t.Helper()
	status, body := sendReconciliationRequest(f.t, http.MethodGet, f.elementEndpoint(path), nil)
	require.Equal(f.t, http.StatusOK, status, "response=%s", string(body))
	var element map[string]any
	require.NoError(f.t, json.Unmarshal(body, &element))
	return element
}

func TestPatchSubmodelKeepsSubmodelAndUnchangedElementRows(t *testing.T) {
	fixture := newElementReconciliationFixture(t, "patch-reconciliation",
		reconciliationTestProperty("Unchanged", "same"),
		reconciliationTestProperty("Changed", "before"),
		reconciliationTestCollection("Group", reconciliationTestProperty("Child", "child")),
	)
	submodelDatabaseID := reconciliationSubmodelDatabaseID(t, fixture.db, fixture.submodelID)
	before := fixture.rows()

	fixture.send(http.MethodPatch, fixture.endpoint, reconciliationTestSubmodel(fixture.submodelID, "After",
		reconciliationTestProperty("Unchanged", "same"),
		reconciliationTestProperty("Changed", "after"),
		reconciliationTestCollection("Group", reconciliationTestProperty("Child", "child")),
		reconciliationTestProperty("Added", "new"),
	))

	require.Equal(t, submodelDatabaseID, reconciliationSubmodelDatabaseID(t, fixture.db, fixture.submodelID))
	after := fixture.rows()
	for _, path := range []string{"Unchanged", "Changed", "Group", "Group.Child"} {
		require.Equal(t, before[path].id, after[path].id, "path %s was recreated", path)
	}
	require.Contains(t, after, "Added")
	require.Equal(t, "after", fixture.element("Changed")["value"])

	status, body := sendReconciliationRequest(t, http.MethodGet, fixture.endpoint, nil)
	require.Equal(t, http.StatusOK, status, "response=%s", string(body))
	var submodel map[string]any
	require.NoError(t, json.Unmarshal(body, &submodel))
	require.Equal(t, "After", submodel["idShort"])
}

func TestPutSubmodelElementReconcilesCollectionChildren(t *testing.T) {
	fixture := newElementReconciliationFixture(t, "put-sme-collection",
		reconciliationTestCollection("Items",
			reconciliationTestProperty("A", "a"),
			reconciliationTestProperty("B", "b"),
			reconciliationTestCollection("C", reconciliationTestProperty("D", "d")),
		),
	)
	before := fixture.rows()

	fixture.send(http.MethodPut, fixture.elementEndpoint("Items"), reconciliationTestCollection("Items",
		reconciliationTestCollection("C", reconciliationTestProperty("D", "d")),
		reconciliationTestProperty("A", "changed"),
		reconciliationTestProperty("E", "e"),
	))

	after := fixture.rows()
	for _, path := range []string{"Items", "Items.A", "Items.C", "Items.C.D"} {
		require.Equal(t, before[path].id, after[path].id, "path %s was recreated", path)
	}
	require.NotContains(t, after, "Items.B")
	requireConsistentElementHierarchy(t, after, "Items", "Items.A", "Items.C", "Items.C.D", "Items.E")
	require.Equal(t, 0, after["Items.C"].position)
	require.Equal(t, 1, after["Items.A"].position)
	require.Equal(t, 2, after["Items.E"].position)
	require.Equal(t, "changed", fixture.element("Items.A")["value"])
}

func TestPutSubmodelElementReconcilesListItems(t *testing.T) {
	fixture := newElementReconciliationFixture(t, "put-sme-list",
		reconciliationTestList("Items", "first", "second", "third"),
	)
	before := fixture.rows()

	fixture.send(http.MethodPut, fixture.elementEndpoint("Items"), reconciliationTestList("Items", "first", "changed"))

	after := fixture.rows()
	for _, path := range []string{"Items", "Items[0]", "Items[1]"} {
		require.Equal(t, before[path].id, after[path].id, "path %s was recreated", path)
	}
	require.NotContains(t, after, "Items[2]")
	require.Equal(t, "changed", fixture.element("Items[1]")["value"])
}

func TestPutSubmodelElementReconcilesEntityStatementsAndAnnotations(t *testing.T) {
	fixture := newElementReconciliationFixture(t, "put-sme-entity",
		reconciliationTestEntity("Machine", reconciliationTestProperty("Serial", "1"), reconciliationTestProperty("Removed", "x")),
		reconciliationTestAnnotatedRelationship("Link", reconciliationTestProperty("Note", "n")),
	)
	before := fixture.rows()

	fixture.send(http.MethodPut, fixture.elementEndpoint("Machine"), reconciliationTestEntity("Machine", reconciliationTestProperty("Serial", "2")))
	fixture.send(http.MethodPut, fixture.elementEndpoint("Link"), reconciliationTestAnnotatedRelationship("Link",
		reconciliationTestProperty("Note", "n"), reconciliationTestProperty("Extra", "e"),
	))

	after := fixture.rows()
	for _, path := range []string{"Machine", "Machine.Serial", "Link", "Link.Note"} {
		require.Equal(t, before[path].id, after[path].id, "path %s was recreated", path)
	}
	require.NotContains(t, after, "Machine.Removed")
	requireConsistentElementHierarchy(t, after, "Machine", "Machine.Serial", "Link", "Link.Note", "Link.Extra")
	require.Equal(t, "2", fixture.element("Machine.Serial")["value"])
}

func TestPatchSubmodelElementValueReplacesCollectionChildren(t *testing.T) {
	fixture := newElementReconciliationFixture(t, "patch-sme-collection",
		reconciliationTestCollection("Items", reconciliationTestProperty("A", "a"), reconciliationTestProperty("B", "b")),
	)
	before := fixture.rows()

	fixture.send(http.MethodPatch, fixture.elementEndpoint("Items"), map[string]any{
		"modelType": "SubmodelElementCollection",
		"value":     []any{reconciliationTestProperty("A", "changed"), reconciliationTestProperty("C", "c")},
	})

	after := fixture.rows()
	require.Equal(t, before["Items"].id, after["Items"].id)
	require.Equal(t, before["Items.A"].id, after["Items.A"].id)
	require.NotContains(t, after, "Items.B")
	requireConsistentElementHierarchy(t, after, "Items", "Items.A", "Items.C")
	children := fixture.element("Items")["value"].([]any)
	require.Len(t, children, 2)
	require.Equal(t, "changed", children[0].(map[string]any)["value"])
	require.Equal(t, "C", children[1].(map[string]any)["idShort"])

	fixture.send(http.MethodPatch, fixture.elementEndpoint("Items"), map[string]any{
		"modelType": "SubmodelElementCollection",
		"value":     []any{},
	})
	cleared := fixture.rows()
	require.Equal(t, before["Items"].id, cleared["Items"].id)
	require.Len(t, cleared, 1, "an empty value removes all children")
}

func TestPatchSubmodelElementValueReplacesListItems(t *testing.T) {
	fixture := newElementReconciliationFixture(t, "patch-sme-list",
		reconciliationTestList("Items", "first", "second"),
	)
	before := fixture.rows()

	fixture.send(http.MethodPatch, fixture.elementEndpoint("Items"), reconciliationTestList("Items", "first", "changed", "third"))

	after := fixture.rows()
	require.Equal(t, before["Items[0]"].id, after["Items[0]"].id)
	require.Equal(t, before["Items[1]"].id, after["Items[1]"].id)
	requireConsistentElementHierarchy(t, after, "Items", "Items[0]", "Items[1]", "Items[2]")
	items := fixture.element("Items")["value"].([]any)
	require.Len(t, items, 3)
	require.Equal(t, "changed", items[1].(map[string]any)["value"])
}

func TestPatchSubmodelElementWithoutValueKeepsChildren(t *testing.T) {
	fixture := newElementReconciliationFixture(t, "patch-sme-metadata",
		reconciliationTestCollection("Items", reconciliationTestProperty("A", "a")),
	)
	before := fixture.rows()

	fixture.send(http.MethodPatch, fixture.elementEndpoint("Items"), map[string]any{
		"modelType": "SubmodelElementCollection",
		"category":  "PARAMETER",
	})
	fixture.send(http.MethodPatch, fixture.elementEndpoint("Items")+"/$metadata", map[string]any{
		"modelType": "SubmodelElementCollection",
		"category":  "VARIABLE",
	})

	after := fixture.rows()
	require.Equal(t, before["Items"].id, after["Items"].id)
	require.Equal(t, before["Items.A"].id, after["Items.A"].id)
	element := fixture.element("Items")
	require.Equal(t, "VARIABLE", element["category"])
	require.Len(t, element["value"].([]any), 1)
}

func TestPutSubmodelAfterDeletingNestedElementKeepsSiblingPositionsUnique(t *testing.T) {
	fixture := newElementReconciliationFixture(t, "put-after-delete",
		reconciliationTestCollection("Items",
			reconciliationTestProperty("A", "a"),
			reconciliationTestProperty("B", "b"),
			reconciliationTestProperty("C", "c"),
		),
	)
	status, body := sendReconciliationRequest(t, http.MethodDelete, fixture.elementEndpoint("Items.B"), nil)
	require.Equal(t, http.StatusNoContent, status, "response=%s", string(body))
	before := fixture.rows()

	fixture.send(http.MethodPut, fixture.endpoint, reconciliationTestSubmodel(fixture.submodelID, "Before",
		reconciliationTestCollection("Items",
			reconciliationTestProperty("A", "a"),
			reconciliationTestProperty("C", "c"),
			reconciliationTestProperty("D", "d"),
		),
	))

	after := fixture.rows()
	require.Equal(t, before["Items.C"].id, after["Items.C"].id)
	require.Equal(t, 0, after["Items.A"].position)
	require.Equal(t, 1, after["Items.C"].position)
	require.Equal(t, 2, after["Items.D"].position)
}

func requireConsistentElementHierarchy(t *testing.T, rows map[string]persistedElementHierarchy, paths ...string) {
	t.Helper()
	for _, path := range paths {
		row, exists := rows[path]
		require.True(t, exists, "missing path %s", path)
		parentPath, rootPath := reconciliationParentAndRootPath(path)
		require.Equal(t, rows[rootPath].id, row.rootID, "path %s has wrong root", path)
		if parentPath == "" {
			require.False(t, row.parentID.Valid, "path %s must not have a parent", path)
			require.Equal(t, 0, row.depth, "path %s has wrong depth", path)
			continue
		}
		require.Equal(t, rows[parentPath].id, row.parentID.Int64, "path %s has wrong parent", path)
		require.Equal(t, rows[parentPath].depth+1, row.depth, "path %s has wrong depth", path)
	}
}

func reconciliationParentAndRootPath(path string) (string, string) {
	parent := ""
	for index := len(path) - 1; index >= 0; index-- {
		if path[index] == '.' || path[index] == '[' {
			parent = path[:index]
			break
		}
	}
	for index, char := range path {
		if char == '.' || char == '[' {
			return parent, path[:index]
		}
	}
	return parent, path
}

func reconciliationElementHierarchy(t *testing.T, db *sql.DB, submodelID string) map[string]persistedElementHierarchy {
	t.Helper()
	query, args, err := goqu.Dialect(common.Dialect).
		From(goqu.T("submodel_element").As("element")).
		Join(goqu.T("submodel").As("submodel"), goqu.On(goqu.I("submodel.id").Eq(goqu.I("element.submodel_id")))).
		Select(
			goqu.I("element.idshort_path"), goqu.I("element.id"), goqu.I("element.parent_sme_id"),
			goqu.I("element.root_sme_id"), goqu.I("element.depth"), goqu.I("element.position"),
		).
		Where(goqu.I("submodel.submodel_identifier").Eq(submodelID)).
		Prepared(true).
		ToSQL()
	require.NoError(t, err)
	rows, err := db.QueryContext(t.Context(), query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	result := make(map[string]persistedElementHierarchy)
	for rows.Next() {
		var path string
		var row persistedElementHierarchy
		require.NoError(t, rows.Scan(&path, &row.id, &row.parentID, &row.rootID, &row.depth, &row.position))
		result[path] = row
	}
	require.NoError(t, rows.Err())
	return result
}

func reconciliationTestSubmodel(submodelID string, idShort string, elements ...any) map[string]any {
	return map[string]any{"id": submodelID, "idShort": idShort, "modelType": "Submodel", "submodelElements": elements}
}

func reconciliationTestProperty(idShort string, value string) map[string]any {
	return map[string]any{"idShort": idShort, "modelType": "Property", "valueType": "xs:string", "value": value}
}

func reconciliationTestCollection(idShort string, children ...any) map[string]any {
	return map[string]any{"idShort": idShort, "modelType": "SubmodelElementCollection", "value": children}
}

func reconciliationTestListItem(value string) map[string]any {
	return map[string]any{"modelType": "Property", "valueType": "xs:string", "value": value}
}

func reconciliationTestList(idShort string, values ...string) map[string]any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, reconciliationTestListItem(value))
	}
	return map[string]any{
		"idShort": idShort, "modelType": "SubmodelElementList",
		"typeValueListElement": "Property", "valueTypeListElement": "xs:string", "value": items,
	}
}

func reconciliationTestEntity(idShort string, statements ...any) map[string]any {
	return map[string]any{
		"idShort": idShort, "modelType": "Entity", "entityType": "SelfManagedEntity",
		"globalAssetId": "urn:basyx:integration:asset", "statements": statements,
	}
}

func reconciliationTestAnnotatedRelationship(idShort string, annotations ...any) map[string]any {
	return map[string]any{
		"idShort": idShort, "modelType": "AnnotatedRelationshipElement",
		"first":       reconciliationTestReference("urn:basyx:integration:first"),
		"second":      reconciliationTestReference("urn:basyx:integration:second"),
		"annotations": annotations,
	}
}

func reconciliationTestReference(value string) map[string]any {
	return map[string]any{
		"type": "ExternalReference",
		"keys": []any{map[string]any{"type": "GlobalReference", "value": value}},
	}
}
