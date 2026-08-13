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

package persistence

import (
	"database/sql"
	"regexp"
	"strconv"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/stretchr/testify/require"
)

func TestAASReconciliationPlanDetectsTableGroups(t *testing.T) {
	previous := reconciliationAAS("aas", "global")
	target, err := cloneAssetAdministrationShell(previous)
	require.NoError(t, err)
	category := "updated"
	target.SetCategory(&category)
	target.SetDescription([]types.ILangStringTextType{types.NewLangStringTextType("en", "updated")})
	assetType := "asset-type"
	target.AssetInformation().SetAssetType(&assetType)

	plan, err := buildAASReconciliationPlan(previous, target, aasReconciliationOptions{})
	require.NoError(t, err)
	require.True(t, plan.Metadata.CoreChanged)
	require.True(t, plan.Metadata.PayloadChanged)
	require.True(t, plan.Metadata.AssetInformationChanged)
	require.False(t, plan.Metadata.ThumbnailChanged)
	require.Empty(t, plan.SpecificUpdates)
	require.Empty(t, plan.ReferenceUpdates)
}

func TestAASReconciliationPlanDetectsIdenticalAASAsNoOp(t *testing.T) {
	previous := reconciliationAAS("aas", "global")
	target, err := cloneAssetAdministrationShell(previous)
	require.NoError(t, err)

	plan, err := buildAASReconciliationPlan(previous, target, aasReconciliationOptions{})
	require.NoError(t, err)
	require.False(t, plan.hasLiveMutation())
}

func TestAASReconciliationPlanIsTableAware(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(types.IAssetAdministrationShell)
		assert func(*testing.T, aasReconciliationPlan)
	}{
		{
			name: "core only",
			mutate: func(aas types.IAssetAdministrationShell) {
				idShort := "updated"
				aas.SetIDShort(&idShort)
			},
			assert: func(t *testing.T, plan aasReconciliationPlan) {
				require.True(t, plan.Metadata.CoreChanged)
				require.False(t, plan.Metadata.PayloadChanged)
				require.False(t, plan.Metadata.AssetInformationChanged)
				require.False(t, plan.Metadata.ThumbnailChanged)
			},
		},
		{
			name: "payload only",
			mutate: func(aas types.IAssetAdministrationShell) {
				aas.SetDescription([]types.ILangStringTextType{types.NewLangStringTextType("en", "updated")})
			},
			assert: func(t *testing.T, plan aasReconciliationPlan) {
				require.False(t, plan.Metadata.CoreChanged)
				require.True(t, plan.Metadata.PayloadChanged)
				require.False(t, plan.Metadata.AssetInformationChanged)
				require.False(t, plan.Metadata.ThumbnailChanged)
			},
		},
		{
			name: "asset information only",
			mutate: func(aas types.IAssetAdministrationShell) {
				assetType := "updated"
				aas.AssetInformation().SetAssetType(&assetType)
			},
			assert: func(t *testing.T, plan aasReconciliationPlan) {
				require.False(t, plan.Metadata.CoreChanged)
				require.False(t, plan.Metadata.PayloadChanged)
				require.True(t, plan.Metadata.AssetInformationChanged)
				require.False(t, plan.Metadata.ThumbnailChanged)
			},
		},
		{
			name: "thumbnail only",
			mutate: func(aas types.IAssetAdministrationShell) {
				aas.AssetInformation().SetDefaultThumbnail(types.NewResource("https://example.com/thumbnail.png"))
			},
			assert: func(t *testing.T, plan aasReconciliationPlan) {
				require.False(t, plan.Metadata.CoreChanged)
				require.False(t, plan.Metadata.PayloadChanged)
				require.False(t, plan.Metadata.AssetInformationChanged)
				require.True(t, plan.Metadata.ThumbnailChanged)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			previous := reconciliationAAS("aas", "global")
			target, err := cloneAssetAdministrationShell(previous)
			require.NoError(t, err)
			testCase.mutate(target)

			plan, err := buildAASReconciliationPlan(previous, target, aasReconciliationOptions{})
			require.NoError(t, err)
			testCase.assert(t, plan)
			require.Empty(t, plan.SpecificUpdates)
			require.Empty(t, plan.ReferenceUpdates)
		})
	}
}

func TestAASSpecificAssetIDsMatchPositionallyAndOnlyMarkChangedSection(t *testing.T) {
	previous := reconciliationAAS("aas", "global")
	first := types.NewSpecificAssetID("serial", "one")
	second := types.NewSpecificAssetID("batch", "two")
	previous.AssetInformation().SetSpecificAssetIDs([]types.ISpecificAssetID{first, second})
	target, err := cloneAssetAdministrationShell(previous)
	require.NoError(t, err)
	target.AssetInformation().SpecificAssetIDs()[1].SetValue("changed")

	plan, err := buildAASReconciliationPlan(previous, target, aasReconciliationOptions{})
	require.NoError(t, err)
	require.Len(t, plan.SpecificUpdates, 1)
	require.Equal(t, 1, plan.SpecificUpdates[0].MatchPosition)
	require.True(t, plan.SpecificUpdates[0].Changes.Core)
	require.False(t, plan.SpecificUpdates[0].Changes.Payload)
	require.False(t, plan.SpecificUpdates[0].Changes.External)
	require.False(t, plan.SpecificUpdates[0].Changes.SupplementalID)
}

func TestAASSubmodelReferencesMatchSemanticIdentityAcrossReordering(t *testing.T) {
	previous := reconciliationAAS("aas", "global")
	previous.SetSubmodels([]types.IReference{reconciliationSubmodelReference("one"), reconciliationSubmodelReference("two")})
	target, err := cloneAssetAdministrationShell(previous)
	require.NoError(t, err)
	target.SetSubmodels([]types.IReference{target.Submodels()[1], target.Submodels()[0]})

	plan, err := buildAASReconciliationPlan(previous, target, aasReconciliationOptions{})
	require.NoError(t, err)
	require.Len(t, plan.ReferenceUpdates, 2)
	require.Empty(t, plan.ReferenceInserts)
	require.Empty(t, plan.ReferenceDeletes)
	for _, row := range plan.ReferenceUpdates {
		require.NotNil(t, row.MatchIdentity)
		require.True(t, row.Changes.Core)
		require.False(t, row.Changes.Payload)
		require.False(t, row.Changes.Keys)
	}
}

func TestAASSubmodelReferenceMatchingReservesSemanticRowsBeforePositionFallback(t *testing.T) {
	previous := reconciliationAAS("aas", "global")
	previous.SetSubmodels([]types.IReference{reconciliationSubmodelReference("one"), reconciliationSubmodelReference("two")})
	target, err := cloneAssetAdministrationShell(previous)
	require.NoError(t, err)
	target.SetSubmodels([]types.IReference{reconciliationSubmodelReference("new"), target.Submodels()[0]})

	plan, err := buildAASReconciliationPlan(previous, target, aasReconciliationOptions{})
	require.NoError(t, err)
	require.Len(t, plan.ReferenceUpdates, 1)
	require.Equal(t, "one", *plan.ReferenceUpdates[0].MatchIdentity)
	require.Len(t, plan.ReferenceInserts, 1)
	require.Len(t, plan.ReferenceDeletes, 1)
}

func TestAASSubmodelReferencesWithoutSemanticIdentityMatchPositionally(t *testing.T) {
	previous := reconciliationAAS("aas", "global")
	previous.SetSubmodels([]types.IReference{reconciliationGlobalReference("one"), reconciliationGlobalReference("two")})
	target, err := cloneAssetAdministrationShell(previous)
	require.NoError(t, err)
	target.SetSubmodels([]types.IReference{target.Submodels()[1], target.Submodels()[0]})

	plan, err := buildAASReconciliationPlan(previous, target, aasReconciliationOptions{})
	require.NoError(t, err)
	require.Len(t, plan.ReferenceUpdates, 2)
	for position, row := range plan.ReferenceUpdates {
		require.Nil(t, row.MatchIdentity)
		require.Equal(t, position, row.MatchPosition)
		require.True(t, row.Changes.Payload)
		require.True(t, row.Changes.Keys)
	}
}

func TestAASReconciliationCompactsAllPositionedRowsAfterDeletion(t *testing.T) {
	previous := reconciliationAAS("aas", "global")
	firstSpecific := types.NewSpecificAssetID("serial", "one")
	secondSpecific := types.NewSpecificAssetID("batch", "two")
	secondSpecific.SetExternalSubjectID(types.NewReference(types.ReferenceTypesExternalReference, []types.IKey{
		types.NewKey(types.KeyTypesGlobalReference, "removed-external-key"),
		types.NewKey(types.KeyTypesFragmentReference, "surviving-external-key"),
	}))
	secondSpecific.SetSupplementalSemanticIDs([]types.IReference{
		reconciliationGlobalReference("removed-specific-reference"),
		types.NewReference(types.ReferenceTypesExternalReference, []types.IKey{
			types.NewKey(types.KeyTypesGlobalReference, "removed-supplemental-key"),
			types.NewKey(types.KeyTypesFragmentReference, "surviving-supplemental-key"),
		}),
	})
	previous.AssetInformation().SetSpecificAssetIDs([]types.ISpecificAssetID{firstSpecific, secondSpecific})
	removedReference := reconciliationSubmodelReference("removed")
	survivingReference := types.NewReference(types.ReferenceTypesModelReference, []types.IKey{
		types.NewKey(types.KeyTypesAssetAdministrationShell, "aas"),
		types.NewKey(types.KeyTypesSubmodel, "surviving"),
	})
	previous.SetSubmodels([]types.IReference{removedReference, survivingReference})

	target, err := cloneAssetAdministrationShell(previous)
	require.NoError(t, err)
	targetSpecific := target.AssetInformation().SpecificAssetIDs()[1]
	targetSpecific.SetExternalSubjectID(types.NewReference(types.ReferenceTypesExternalReference, []types.IKey{
		types.NewKey(types.KeyTypesFragmentReference, "surviving-external-key"),
	}))
	targetSpecific.SetSupplementalSemanticIDs([]types.IReference{
		types.NewReference(types.ReferenceTypesExternalReference, []types.IKey{
			types.NewKey(types.KeyTypesFragmentReference, "surviving-supplemental-key"),
		}),
	})
	target.AssetInformation().SetSpecificAssetIDs([]types.ISpecificAssetID{targetSpecific})
	target.SetSubmodels([]types.IReference{types.NewReference(types.ReferenceTypesModelReference, []types.IKey{
		types.NewKey(types.KeyTypesSubmodel, "surviving"),
	})})

	plan, err := buildAASReconciliationPlan(previous, target, aasReconciliationOptions{})
	require.NoError(t, err)

	require.Len(t, plan.SpecificUpdates, 1)
	require.Equal(t, 0, plan.SpecificUpdates[0].Position)
	require.Equal(t, "two", plan.SpecificUpdates[0].Value)
	require.Equal(t, 0, plan.SpecificUpdates[0].ExternalSubjectID.Keys[0].Position)
	require.Equal(t, "surviving-external-key", plan.SpecificUpdates[0].ExternalSubjectID.Keys[0].Value)
	require.Len(t, plan.SpecificUpdates[0].SupplementalSemanticIDs, 1)
	require.Equal(t, 0, plan.SpecificUpdates[0].SupplementalSemanticIDs[0].Position)
	require.Equal(t, 0, plan.SpecificUpdates[0].SupplementalSemanticIDs[0].Keys[0].Position)
	require.Equal(t, "surviving-supplemental-key", plan.SpecificUpdates[0].SupplementalSemanticIDs[0].Keys[0].Value)
	require.Len(t, plan.SpecificDeletes, 1)
	require.Equal(t, 1, plan.SpecificDeletes[0].MatchPosition)
	require.Len(t, plan.ReferenceUpdates, 1)
	require.Equal(t, 1, plan.ReferenceUpdates[0].MatchPosition)
	require.Equal(t, 0, plan.ReferenceUpdates[0].Position)
	require.Len(t, plan.ReferenceUpdates[0].Keys, 1)
	require.Equal(t, 0, plan.ReferenceUpdates[0].Keys[0].Position)
	require.Equal(t, "surviving", plan.ReferenceUpdates[0].Keys[0].Value)
	require.Len(t, plan.ReferenceDeletes, 1)
}

func TestAASReconciliationPreservesUnchangedManagedThumbnail(t *testing.T) {
	contentType := "image/png"
	previous := reconciliationAAS("aas", "global")
	thumbnail := types.NewResource("/shells/aas/asset-information/thumbnail/managed")
	thumbnail.SetContentType(&contentType)
	previous.AssetInformation().SetDefaultThumbnail(thumbnail)
	target, err := cloneAssetAdministrationShell(previous)
	require.NoError(t, err)
	targetThumbnail := types.NewResource(thumbnail.Path())
	target.AssetInformation().SetDefaultThumbnail(targetThumbnail)

	plan, err := buildAASReconciliationPlan(previous, target, aasReconciliationOptions{PreserveExistingManagedThumbnail: true})
	require.NoError(t, err)
	require.False(t, plan.Metadata.ThumbnailChanged)
	require.Equal(t, &contentType, plan.Metadata.Thumbnail.ContentType)
}

func TestEffectiveAssetInformationPutPreservesOmittedPartialFields(t *testing.T) {
	contentType := "image/png"
	assetType := "current-type"
	previous := reconciliationAAS("aas", "global")
	previous.AssetInformation().SetAssetType(&assetType)
	previous.AssetInformation().SetSpecificAssetIDs([]types.ISpecificAssetID{types.NewSpecificAssetID("serial", "one")})
	currentThumbnail := types.NewResource("https://example.com/thumbnail.png")
	currentThumbnail.SetContentType(&contentType)
	previous.AssetInformation().SetDefaultThumbnail(currentThumbnail)
	submitted := types.NewAssetInformation(0)
	submitted.SetDefaultThumbnail(types.NewResource(currentThumbnail.Path()))

	target := buildEffectiveAssetInformation(previous.AssetInformation(), submitted)
	plan, err := buildAssetInformationReconciliationPlan(previous.ID(), previous.AssetInformation(), submitted)
	require.NoError(t, err)
	require.False(t, plan.hasLiveMutation())
	require.Equal(t, previous.AssetInformation().AssetKind(), target.AssetKind())
	require.Equal(t, previous.AssetInformation().GlobalAssetID(), target.GlobalAssetID())
	require.Equal(t, previous.AssetInformation().AssetType(), target.AssetType())
	require.Len(t, target.SpecificAssetIDs(), 1)
	require.Equal(t, &contentType, target.DefaultThumbnail().ContentType())
}

func TestEffectiveAssetInformationPutClearsOmittedThumbnailOnly(t *testing.T) {
	previous := reconciliationAAS("aas", "global")
	previous.AssetInformation().SetDefaultThumbnail(types.NewResource("https://example.com/thumbnail.png"))

	submitted := types.NewAssetInformation(0)
	target := buildEffectiveAssetInformation(previous.AssetInformation(), submitted)
	plan, err := buildAssetInformationReconciliationPlan(previous.ID(), previous.AssetInformation(), submitted)
	require.NoError(t, err)
	require.True(t, plan.Metadata.ThumbnailChanged)
	require.Nil(t, plan.Metadata.Thumbnail)
	require.Nil(t, target.DefaultThumbnail())
	require.False(t, plan.Metadata.AssetInformationChanged)
	require.Empty(t, plan.SpecificUpdates)
	require.Empty(t, plan.SpecificInserts)
	require.Empty(t, plan.SpecificDeletes)
}

func TestAASSpecificAssetIDNestedReferencesRemainPositional(t *testing.T) {
	previous := reconciliationAAS("aas", "global")
	specificAssetID := types.NewSpecificAssetID("serial", "one")
	specificAssetID.SetExternalSubjectID(reconciliationGlobalReference("external"))
	specificAssetID.SetSupplementalSemanticIDs([]types.IReference{
		reconciliationGlobalReference("duplicate"),
		reconciliationGlobalReference("duplicate"),
	})
	previous.AssetInformation().SetSpecificAssetIDs([]types.ISpecificAssetID{specificAssetID})
	target, err := cloneAssetAdministrationShell(previous)
	require.NoError(t, err)
	targetSpecific := target.AssetInformation().SpecificAssetIDs()[0]
	targetSpecific.SetExternalSubjectID(reconciliationGlobalReference("external-updated"))
	targetSpecific.SetSupplementalSemanticIDs([]types.IReference{
		reconciliationGlobalReference("duplicate"),
		reconciliationGlobalReference("updated-at-position-one"),
	})

	plan, err := buildAASReconciliationPlan(previous, target, aasReconciliationOptions{})
	require.NoError(t, err)
	require.Len(t, plan.SpecificUpdates, 1)
	row := plan.SpecificUpdates[0]
	require.Equal(t, 0, row.MatchPosition)
	require.True(t, row.Changes.External)
	require.True(t, row.Changes.SupplementalID)
	require.Equal(t, 0, row.SupplementalSemanticIDs[0].Position)
	require.Equal(t, 1, row.SupplementalSemanticIDs[1].Position)
}

func TestAASReconciliationQueryHasConstantSingleStatementShape(t *testing.T) {
	previous := reconciliationAAS("aas", "global")
	target, err := cloneAssetAdministrationShell(previous)
	require.NoError(t, err)
	category := "updated"
	target.SetCategory(&category)
	plan, err := buildAASReconciliationPlan(previous, target, aasReconciliationOptions{})
	require.NoError(t, err)
	planJSON, err := plan.marshal()
	require.NoError(t, err)

	query, args, err := newAASReconciliationQueryBuilder().build(planJSON, target.ID())
	require.NoError(t, err)
	require.Len(t, args, 2)
	require.Contains(t, query, "WITH aas_reconciliation_plan")
	require.Contains(t, query, "updated_aas_metadata AS (UPDATE")
	require.Contains(t, query, "updated_specific_rows AS (UPDATE")
	require.Contains(t, query, "inserted_reference_rows AS (INSERT")
	require.Contains(t, query, "updated_specific_external_payload AS (UPDATE")
	require.Contains(t, query, "inserted_specific_external_payload AS (INSERT")
	require.Contains(t, query, "updated_specific_supplemental_payload AS (UPDATE")
	require.Contains(t, query, "inserted_specific_supplemental_payload AS (INSERT")
	externalPayloadCTE := aasReconciliationCTESQL(t, query, "inserted_specific_external_payload", "deleted_specific_external_keys")
	require.NotContains(t, externalPayloadCTE, "ON CONFLICT")
	supplementalPayloadCTE := aasReconciliationCTESQL(t, query, "inserted_specific_supplemental_payload", "deleted_specific_supplemental_keys")
	require.NotContains(t, supplementalPayloadCTE, "ON CONFLICT")
	require.NotContains(t, strings.ToLower(query), `delete from "aas"`)
	require.NotContains(t, strings.ToLower(query), `insert into "aas"`)
	require.NotContains(t, strings.TrimSuffix(query, ";"), ";")
}

func aasReconciliationCTESQL(t *testing.T, query string, name string, nextName string) string {
	t.Helper()
	startMarker := name + " AS ("
	endMarker := "), " + nextName + " AS ("
	start := strings.Index(query, startMarker)
	require.NotEqual(t, -1, start)
	end := strings.Index(query[start:], endMarker)
	require.NotEqual(t, -1, end)
	return query[start : start+end]
}

func TestAASReconciliationQueryParameterCountIsConstantAtScale(t *testing.T) {
	emptyJSON, err := (aasReconciliationPlan{}).marshal()
	require.NoError(t, err)
	emptyQuery, emptyArgs, err := newAASReconciliationQueryBuilder().build(emptyJSON, "aas")
	require.NoError(t, err)

	largePlan := aasReconciliationPlan{}
	for position := range 2500 {
		identity := "submodel-" + strconv.Itoa(position)
		largePlan.SpecificInserts = append(largePlan.SpecificInserts, aasSpecificAssetIDRow{
			Position: position,
			Name:     "name",
			Value:    strconv.Itoa(position),
			Changes:  allAASSpecificAssetIDChanges(),
		})
		largePlan.ReferenceInserts = append(largePlan.ReferenceInserts, aasSubmodelReferenceRow{
			Position: position,
			Identity: &identity,
			Type:     int(types.ReferenceTypesModelReference),
			Changes:  allAASSubmodelReferenceChanges(),
		})
	}
	largeJSON, err := largePlan.marshal()
	require.NoError(t, err)
	largeQuery, largeArgs, err := newAASReconciliationQueryBuilder().build(largeJSON, "aas")
	require.NoError(t, err)

	require.Equal(t, emptyQuery, largeQuery)
	require.Len(t, emptyArgs, 2)
	require.Len(t, largeArgs, 2)
}

func TestExecuteAASReconciliationRejectsAffectedCountMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	plan := aasReconciliationPlan{SpecificUpdates: []aasSpecificAssetIDRow{{MatchPosition: 0}}}
	mock.ExpectQuery(regexp.QuoteMeta("WITH aas_reconciliation_plan")).WillReturnRows(sqlmock.NewRows([]string{
		"updated_specific", "inserted_specific", "deleted_specific",
		"updated_references", "inserted_references", "deleted_references",
	}).AddRow(0, 0, 0, 0, 0, 0))

	_, err = executeAASReconciliationStatement(t.Context(), tx, "aas", plan)
	require.Error(t, err)
	require.Contains(t, err.Error(), "AASREPO-RECON-COUNT")
	mock.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAssetInformationNoOpStillAppendsHistory(t *testing.T) {
	previousHistoryConfig := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeAPI})
	t.Cleanup(func() { history.Configure(previousHistoryConfig) })

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	tx := beginMockTransaction(t, db, mock)
	aasID := "urn:basyx:test:asset-information-no-op-history"
	globalAssetID := "urn:basyx:test:asset"
	mock.ExpectQuery(`SELECT "id" FROM "aas".*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	expectFocusedAssetInformationRead(mock, globalAssetID)
	mock.ExpectQuery(`SELECT "id" FROM "aas" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	expectExistingAASRead(mock, aasID, globalAssetID, "")
	mock.ExpectExec(`SELECT .*pg_advisory_xact_lock`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT "row_hash" FROM "aas_history"`).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`INSERT INTO "aas_history"`).
		WillReturnRows(sqlmock.NewRows([]string{"history_id"}).AddRow(int64(1)))
	mock.ExpectExec(`INSERT INTO "aas_history_payload"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	assetInformation := types.NewAssetInformation(types.AssetKindInstance)
	assetInformation.SetGlobalAssetID(&globalAssetID)
	repository := &AssetAdministrationShellDatabase{}
	require.NoError(t, repository.PutAssetInformationByAASIDInTransaction(
		contextWithConfig(), tx, aasID, assetInformation,
	))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAssetInformationPutDoesNotLoadUnrelatedAASCollections(t *testing.T) {
	previousHistoryConfig := history.ActiveConfig()
	history.Configure(history.Config{Mode: history.ModeOff})
	t.Cleanup(func() { history.Configure(previousHistoryConfig) })

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	tx := beginMockTransaction(t, db, mock)
	aasID := "urn:basyx:test:asset-information-focused-read"
	globalAssetID := "urn:basyx:test:asset"
	mock.ExpectQuery(`SELECT "id" FROM "aas".*FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))
	expectFocusedAssetInformationRead(mock, globalAssetID)
	mock.ExpectRollback()

	assetInformation := types.NewAssetInformation(types.AssetKindInstance)
	assetInformation.SetGlobalAssetID(&globalAssetID)
	repository := &AssetAdministrationShellDatabase{}
	require.NoError(t, repository.PutAssetInformationByAASIDInTransaction(
		contextWithConfig(), tx, aasID, assetInformation,
	))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectFocusedAssetInformationRead(mock sqlmock.Sqlmock, globalAssetID string) {
	mock.ExpectQuery(`SELECT .*FROM "asset_information" AS "asset_information"`).
		WillReturnRows(sqlmock.NewRows([]string{
			"asset_kind", "global_asset_id", "asset_type", "thumbnail_value", "thumbnail_content_type",
		}).AddRow(int(types.AssetKindInstance), globalAssetID, nil, nil, nil))
}

func reconciliationAAS(id string, globalAssetID string) types.IAssetAdministrationShell {
	assetInformation := types.NewAssetInformation(types.AssetKindInstance)
	assetInformation.SetGlobalAssetID(&globalAssetID)
	return types.NewAssetAdministrationShell(id, assetInformation)
}

func reconciliationSubmodelReference(id string) types.IReference {
	return types.NewReference(types.ReferenceTypesModelReference, []types.IKey{types.NewKey(types.KeyTypesSubmodel, id)})
}

func reconciliationGlobalReference(id string) types.IReference {
	return types.NewReference(types.ReferenceTypesExternalReference, []types.IKey{types.NewKey(types.KeyTypesGlobalReference, id)})
}
