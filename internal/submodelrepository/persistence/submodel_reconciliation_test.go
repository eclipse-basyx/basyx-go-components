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
// Author: Jannik Fried (Fraunhofer IESE)

package persistence

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	submodelelements "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/submodelElements"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestSubmodelReconciliationQueryHasConstantSingleStatementShape(t *testing.T) {
	oldSubmodel := readReconciliationFixture(t, "../integration_tests/bodies/post/postSubmodel.json")
	newSubmodel := readReconciliationFixture(t, "../integration_tests/bodies/put/putSubmodelUpdate.json")
	sut := &SubmodelDatabase{}
	plan, err := sut.buildSubmodelReconciliationPlan(oldSubmodel, newSubmodel)
	require.NoError(t, err)
	require.True(t, plan.hasLiveMutation())

	planJSON, err := plan.marshal()
	require.NoError(t, err)
	query, args, err := newReconciliationQueryBuilder().build(planJSON, newSubmodel.ID())
	require.NoError(t, err)
	require.NotContains(t, strings.TrimSuffix(query, ";"), ";")
	require.Contains(t, query, "WITH reconciliation_plan")
	require.Contains(t, query, "jsonb_to_recordset")
	require.Contains(t, query, "updated_element_rows AS (UPDATE")
	require.Contains(t, query, "inserted_element_rows AS (INSERT")
	require.Contains(t, query, "deleted_element_rows AS (DELETE")
	require.Len(t, args, 2)
}

func TestExecuteSubmodelReconciliationUsesOneQueryRowStatement(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	plan := submodelReconciliationPlan{
		Updates: []submodelelements.ReconciliationElementRow{{Path: "Property"}},
	}
	mock.ExpectQuery(regexp.QuoteMeta("WITH reconciliation_plan")).
		WillReturnRows(sqlmock.NewRows([]string{"updated_count", "inserted_count", "deleted_count"}).AddRow(1, 0, 0))

	result, err := executeSubmodelReconciliationStatement(t.Context(), tx, "sm", plan)
	require.NoError(t, err)
	require.Equal(t, 1, result.UpdatedElements)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSubmodelCreatePlanInsertsCompleteElementGraph(t *testing.T) {
	submodel := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"Group","modelType":"SubmodelElementCollection","value":[{"idShort":"Value","modelType":"Property","valueType":"xs:string","value":"created"}]}]}`)

	plan, err := (&SubmodelDatabase{}).buildSubmodelElementCreatePlan(submodel.SubmodelElements())

	require.NoError(t, err)
	require.Len(t, plan.Inserts, 2)
	require.Empty(t, plan.Updates)
	require.Empty(t, plan.Deletes)
	require.Equal(t, "Group", plan.Inserts[0].Path)
	require.Equal(t, "Group.Value", plan.Inserts[1].Path)
	for _, row := range plan.Inserts {
		require.Equal(t, submodelelements.AllReconciliationElementChanges(), row.Changes)
	}
}

func TestSubmodelCreatePlanRejectsDuplicateElementPaths(t *testing.T) {
	submodel := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"Duplicate","modelType":"Property","valueType":"xs:string"},{"idShort":"Duplicate","modelType":"Property","valueType":"xs:string"}]}`)

	_, err := (&SubmodelDatabase{}).buildSubmodelElementCreatePlan(submodel.SubmodelElements())

	require.Error(t, err)
	require.Truef(t, common.IsErrConflict(err), "got %v", err)
	require.Contains(t, err.Error(), "SMREPO-RECON-DUPLICATEPATH")
}

func TestReconciliationPlanMatchesNamedChildrenAndListsPositionally(t *testing.T) {
	oldNamed := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"A","modelType":"Property","valueType":"xs:string","value":"a"},{"idShort":"B","modelType":"Property","valueType":"xs:string","value":"b"}]}`)
	newNamed := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"B","modelType":"Property","valueType":"xs:string","value":"b"},{"idShort":"A","modelType":"Property","valueType":"xs:string","value":"a"}]}`)
	plan := buildReconciliationPlanForTest(t, oldNamed, newNamed)
	require.Len(t, plan.Updates, 2)
	require.Empty(t, plan.Inserts)
	require.Empty(t, plan.Deletes)
	for _, row := range plan.Updates {
		require.True(t, row.Changes.Core)
		require.False(t, row.Changes.TypeData)
	}

	oldList := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"L","modelType":"SubmodelElementList","typeValueListElement":"Property","value":[{"modelType":"Property","valueType":"xs:string","value":"a"},{"modelType":"Property","valueType":"xs:string","value":"b"}]}]}`)
	newList := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"L","modelType":"SubmodelElementList","typeValueListElement":"Property","value":[{"modelType":"Property","valueType":"xs:string","value":"b"},{"modelType":"Property","valueType":"xs:string","value":"a"}]}]}`)
	plan = buildReconciliationPlanForTest(t, oldList, newList)
	require.Len(t, plan.Updates, 2)
	require.Empty(t, plan.Inserts)
	require.Empty(t, plan.Deletes)
	require.Equal(t, "L[0]", plan.Updates[0].Path)
	require.Equal(t, "L[1]", plan.Updates[1].Path)
}

func TestReconciliationPlanRejectsDuplicateElementPaths(t *testing.T) {
	testCases := []struct {
		name     string
		previous string
		target   string
	}{
		{
			name:     "duplicate existing root path",
			previous: `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"old"}]}`,
			target:   `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"first"},{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"second"}]}`,
		},
		{
			name:     "duplicate new nested path",
			previous: `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"Group","modelType":"SubmodelElementCollection","value":[]}]}`,
			target:   `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"Group","modelType":"SubmodelElementCollection","value":[{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"first"},{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"second"}]}]}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			previous := readReconciliationJSON(t, testCase.previous)
			target := readReconciliationJSON(t, testCase.target)

			_, err := (&SubmodelDatabase{}).buildSubmodelReconciliationPlan(previous, target)

			require.Error(t, err)
			require.Truef(t, common.IsErrConflict(err), "got %v", err)
			require.Contains(t, err.Error(), "SMREPO-RECON-DUPLICATEPATH")
		})
	}
}

func TestReconcileSubmodelElementRowsRejectsDuplicateSiblingPositions(t *testing.T) {
	rows := []submodelelements.ReconciliationElementRow{
		{Path: "Group.A", ParentPath: "Group", Position: 0},
		{Path: "Group.B", ParentPath: "Group", Position: 0},
	}

	_, _, _, err := reconcileSubmodelElementRows(nil, rows)

	require.Error(t, err)
	require.Truef(t, common.IsErrConflict(err), "got %v", err)
	require.Contains(t, err.Error(), "SMREPO-RECON-DUPLICATEPOSITION")
	_, _, _, err = reconcileSubmodelElementRows(nil, []submodelelements.ReconciliationElementRow{
		{Path: "First.A", ParentPath: "First", Position: 0},
		{Path: "Second.B", ParentPath: "Second", Position: 0},
	})
	require.NoError(t, err)
}

func TestReconciliationPlanDeletesOmittedSubtreeAndCompactsSiblingPosition(t *testing.T) {
	previous := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"Group","modelType":"SubmodelElementCollection","value":[{"idShort":"Removed","modelType":"SubmodelElementCollection","value":[{"idShort":"Descendant","modelType":"Property","valueType":"xs:string","value":"delete"}]},{"idShort":"Keep","modelType":"Property","valueType":"xs:string","value":"same"}]}]}`)
	target := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"Group","modelType":"SubmodelElementCollection","value":[{"idShort":"Keep","modelType":"Property","valueType":"xs:string","value":"same"}]}]}`)

	plan := buildReconciliationPlanForTest(t, previous, target)

	require.Equal(t, []string{"Group.Removed"}, plan.Deletes)
	require.Equal(t, 2, plan.ExpectedDeletedElements)
	require.Len(t, plan.Updates, 1)
	require.Equal(t, "Group.Keep", plan.Updates[0].Path)
	require.Equal(t, 0, plan.Updates[0].Position)
	require.True(t, plan.Updates[0].Changes.Core)
	require.Empty(t, plan.Inserts)
}

func TestReconciliationPlanCompactsAllSubmodelReferencePositions(t *testing.T) {
	previous := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","semanticId":{"type":"ModelReference","keys":[{"type":"Submodel","value":"root"},{"type":"Property","value":"semantic-survivor"}]},"supplementalSemanticIds":[{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"removed-reference"}]},{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"supplemental-survivor"},{"type":"FragmentReference","value":"supplemental-fragment"}]}],"submodelElements":[{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"same","semanticId":{"type":"ModelReference","keys":[{"type":"Submodel","value":"root"},{"type":"Property","value":"element-semantic-survivor"}]},"supplementalSemanticIds":[{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"element-removed-reference"}]},{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"element-supplemental-survivor"},{"type":"FragmentReference","value":"element-supplemental-fragment"}]}]}]}`)
	target := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","semanticId":{"type":"ModelReference","keys":[{"type":"Property","value":"semantic-survivor"}]},"supplementalSemanticIds":[{"type":"ExternalReference","keys":[{"type":"FragmentReference","value":"supplemental-fragment"}]}],"submodelElements":[{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"same","semanticId":{"type":"ModelReference","keys":[{"type":"Property","value":"element-semantic-survivor"}]},"supplementalSemanticIds":[{"type":"ExternalReference","keys":[{"type":"FragmentReference","value":"element-supplemental-fragment"}]}]}]}`)

	plan := buildReconciliationPlanForTest(t, previous, target)

	require.True(t, plan.Metadata.SemanticIDChanged)
	require.Equal(t, 0, plan.Metadata.SemanticID.Keys[0].Position)
	require.Equal(t, "semantic-survivor", plan.Metadata.SemanticID.Keys[0].Value)
	require.True(t, plan.Metadata.SupplementalChanged)
	require.Len(t, plan.Metadata.SupplementalRefs, 1)
	require.Equal(t, 0, plan.Metadata.SupplementalRefs[0].Position)
	require.Equal(t, 0, plan.Metadata.SupplementalRefs[0].Keys[0].Position)
	require.Equal(t, "supplemental-fragment", plan.Metadata.SupplementalRefs[0].Keys[0].Value)
	require.Len(t, plan.Updates, 1)
	require.Equal(t, 0, plan.Updates[0].SemanticID.Keys[0].Position)
	require.Equal(t, "element-semantic-survivor", plan.Updates[0].SemanticID.Keys[0].Value)
	require.Len(t, plan.Updates[0].SupplementalSemanticIDs, 1)
	require.Equal(t, 0, plan.Updates[0].SupplementalSemanticIDs[0].Position)
	require.Equal(t, 0, plan.Updates[0].SupplementalSemanticIDs[0].Keys[0].Position)
	require.Equal(t, "element-supplemental-fragment", plan.Updates[0].SupplementalSemanticIDs[0].Keys[0].Value)
}

func TestSubmodelReconciliationMapsDeferredUniqueViolationToConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	plan := submodelReconciliationPlan{
		Inserts: []submodelelements.ReconciliationElementRow{{Path: "P"}},
	}
	mock.ExpectExec(regexp.QuoteMeta("SET CONSTRAINTS uq_sibling_idshort, uq_sibling_pos DEFERRED")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("WITH reconciliation_plan")).
		WillReturnRows(sqlmock.NewRows([]string{"updated_count", "inserted_count", "deleted_count"}).AddRow(0, 1, 0))
	mock.ExpectExec(regexp.QuoteMeta("SET CONSTRAINTS uq_sibling_idshort, uq_sibling_pos IMMEDIATE")).
		WillReturnError(&pgconn.PgError{Code: "23505"})

	err = (&SubmodelDatabase{}).executeSubmodelReconciliationTx(t.Context(), tx, "sm", plan)

	require.Error(t, err)
	require.Truef(t, common.IsErrConflict(err), "got %v", err)
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReconciliationPlanOnlyMarksChangedPersistenceSection(t *testing.T) {
	oldSubmodel := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"old"}]}`)
	newSubmodel := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"new"}]}`)
	plan := buildReconciliationPlanForTest(t, oldSubmodel, newSubmodel)
	require.Len(t, plan.Updates, 1)
	changes := plan.Updates[0].Changes
	require.True(t, changes.TypeData)
	require.False(t, changes.Core)
	require.False(t, changes.Payload)
	require.False(t, changes.SemanticID)
	require.False(t, changes.SupplementalID)
	require.False(t, changes.LanguageValues)
	require.False(t, changes.ValueID)
}

func TestReconciliationPlanDetectsIdenticalSubmodelsAsNoOp(t *testing.T) {
	payload := `{"id":"sm","idShort":"same","modelType":"Submodel","submodelElements":[{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"same"}]}`
	previous := readReconciliationJSON(t, payload)
	target := readReconciliationJSON(t, payload)

	plan := buildReconciliationPlanForTest(t, previous, target)
	require.False(t, plan.hasLiveMutation())
	require.Empty(t, plan.Updates)
	require.Empty(t, plan.Inserts)
	require.Empty(t, plan.Deletes)
}

func TestReconciliationMetadataTreatsMissingAndEmptyIDShortAsEqual(t *testing.T) {
	previous := types.NewSubmodel("sm")
	emptyIDShort := ""
	previous.SetIDShort(&emptyIDShort)
	target := types.NewSubmodel("sm")

	metadata, err := buildSubmodelReconciliationMetadata(previous, target)
	require.NoError(t, err)
	require.False(t, metadata.CoreChanged)
}

func TestReconciliationPlanDetectsMetadataOnlyChange(t *testing.T) {
	previous := readReconciliationJSON(t, `{"id":"sm","idShort":"old","modelType":"Submodel","submodelElements":[{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"same"}]}`)
	target := readReconciliationJSON(t, `{"id":"sm","idShort":"new","modelType":"Submodel","submodelElements":[{"idShort":"P","modelType":"Property","valueType":"xs:string","value":"same"}]}`)

	plan := buildReconciliationPlanForTest(t, previous, target)
	require.True(t, plan.Metadata.CoreChanged)
	require.False(t, plan.Metadata.PayloadChanged)
	require.False(t, plan.Metadata.SemanticIDChanged)
	require.False(t, plan.Metadata.SupplementalChanged)
	require.Empty(t, plan.Updates)
	require.Empty(t, plan.Inserts)
	require.Empty(t, plan.Deletes)
}

func TestReconciliationPlanOnlyUpdatesChangedNestedElement(t *testing.T) {
	previous := readReconciliationJSON(t, `{"id":"sm","category":"old","modelType":"Submodel","submodelElements":[{"idShort":"Group","modelType":"SubmodelElementCollection","value":[{"idShort":"P0","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P1","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P2","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P3","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P4","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P5","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P6","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P7","modelType":"Property","valueType":"xs:string","value":"old"},{"idShort":"P8","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P9","modelType":"Property","valueType":"xs:string","value":"same"}]}]}`)
	target := readReconciliationJSON(t, `{"id":"sm","category":"new","modelType":"Submodel","submodelElements":[{"idShort":"Group","modelType":"SubmodelElementCollection","value":[{"idShort":"P0","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P1","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P2","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P3","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P4","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P5","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P6","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P7","modelType":"Property","valueType":"xs:string","value":"new"},{"idShort":"P8","modelType":"Property","valueType":"xs:string","value":"same"},{"idShort":"P9","modelType":"Property","valueType":"xs:string","value":"same"}]}]}`)

	plan := buildReconciliationPlanForTest(t, previous, target)
	require.True(t, plan.Metadata.CoreChanged)
	require.Len(t, plan.Updates, 1)
	require.Equal(t, "Group.P7", plan.Updates[0].Path)
	require.True(t, plan.Updates[0].Changes.TypeData)
	require.Empty(t, plan.Inserts)
	require.Empty(t, plan.Deletes)
}

func TestReconciliationPlanCanonicalizesDateTimePropertyValues(t *testing.T) {
	submitted := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"lastUpdate","modelType":"Property","valueType":"xs:dateTime","value":"2026-08-07T11:16:26.125183291Z"}]}`)
	persisted := readReconciliationJSON(t, `{"id":"sm","modelType":"Submodel","submodelElements":[{"idShort":"lastUpdate","modelType":"Property","valueType":"xs:dateTime","value":"2026-08-07T11:16:26.125183+00:00"}]}`)

	plan := buildReconciliationPlanForTest(t, persisted, submitted)
	require.False(t, plan.hasLiveMutation())
}

func TestReconciliationQueryParameterCountIsConstantAtScale(t *testing.T) {
	oldSubmodel := types.NewSubmodel("scale")
	newSubmodel := types.NewSubmodel("scale")
	oldElements := make([]types.ISubmodelElement, 2500)
	newElements := make([]types.ISubmodelElement, 2500)
	for index := range oldElements {
		idShort := "P" + strconv.Itoa(index)
		oldValue := "value"
		newValue := oldValue
		if index == len(oldElements)-1 {
			newValue = "changed"
		}
		oldProperty := types.NewProperty(types.DataTypeDefXSDString)
		oldProperty.SetIDShort(&idShort)
		oldProperty.SetValue(&oldValue)
		newProperty := types.NewProperty(types.DataTypeDefXSDString)
		newProperty.SetIDShort(&idShort)
		newProperty.SetValue(&newValue)
		oldElements[index] = oldProperty
		newElements[index] = newProperty
	}
	oldSubmodel.SetSubmodelElements(oldElements)
	newSubmodel.SetSubmodelElements(newElements)
	plan := buildReconciliationPlanForTest(t, oldSubmodel, newSubmodel)
	require.Len(t, plan.Updates, 1)
	planJSON, err := plan.marshal()
	require.NoError(t, err)
	_, args, err := newReconciliationQueryBuilder().build(planJSON, "scale")
	require.NoError(t, err)
	require.Len(t, args, 2)
}

func TestContextWithoutFragmentFiltersPreservesUpdateFormula(t *testing.T) {
	allowed := true
	formula := grammar.LogicalExpression{Boolean: &allowed}
	original := &auth.QueryFilter{
		Formula: &formula,
		Filters: auth.FragmentFilters{
			grammar.FragmentStringPattern("$sm#idShort"): {},
		},
	}
	ctx, err := contextWithoutFragmentFilters(auth.WithQueryFilter(t.Context(), original))
	require.NoError(t, err)
	filtered := auth.GetQueryFilter(ctx)
	require.NotNil(t, filtered)
	require.Equal(t, original.Formula, filtered.Formula)
	require.Nil(t, filtered.Filters)
	require.NotNil(t, original.Filters)
}

func buildReconciliationPlanForTest(t *testing.T, oldSubmodel types.ISubmodel, newSubmodel types.ISubmodel) submodelReconciliationPlan {
	t.Helper()
	plan, err := (&SubmodelDatabase{}).buildSubmodelReconciliationPlan(oldSubmodel, newSubmodel)
	require.NoError(t, err)
	return plan
}

func readReconciliationJSON(t *testing.T, payload string) types.ISubmodel {
	t.Helper()
	var jsonable map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &jsonable))
	submodel, err := jsonization.SubmodelFromJsonable(jsonable)
	require.NoError(t, err)
	return submodel
}

func readReconciliationFixture(t *testing.T, path string) types.ISubmodel {
	t.Helper()
	// #nosec G304 -- paths are repository-owned test fixtures selected by the test.
	payload, err := os.ReadFile(path)
	require.NoError(t, err)
	var jsonable map[string]any
	require.NoError(t, json.Unmarshal(payload, &jsonable))
	submodel, err := jsonization.SubmodelFromJsonable(jsonable)
	require.NoError(t, err)
	return submodel
}
