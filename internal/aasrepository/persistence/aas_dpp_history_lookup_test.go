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
	"strings"
	"testing"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestHistoricalIdentifierCandidatesDatasetUsesIndexedJSONPathPredicates(t *testing.T) {
	at := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	dataset, err := buildHistoricalIdentifierCandidatesDataset(history.TableSubmodel, `https://example.org/dpp/"quoted"`, at)
	require.NoError(t, err)

	query, args, err := dataset.Prepared(true).ToSQL()
	require.NoError(t, err)
	require.Contains(t, query, `SELECT DISTINCT "history"."identifier"`)
	require.Contains(t, query, `"payload"."snapshot" @@ CAST($2 AS jsonpath)`)
	require.Contains(t, query, `"payload"."diff" @@ CAST($3 AS jsonpath)`)
	require.NotContains(t, query, "COALESCE")
	require.True(t, strings.HasSuffix(query, "LIMIT $4"))
	require.Equal(t, at, args[0])
	require.Equal(t, `$.** == "https://example.org/dpp/\"quoted\""`, args[1])
	require.Equal(t, args[1], args[2])
	require.Equal(t, int64(maxHistoricalDPPIdentityCandidates+1), args[3])
}

func TestHistoricalIdentifierCandidatesDatasetRejectsUnsupportedTable(t *testing.T) {
	_, err := buildHistoricalIdentifierCandidatesDataset("unknown_history", "value", time.Now())
	require.ErrorContains(t, err, "AASREPO-HISTORY-DPP-BADTABLE")
}

func TestHistoricalSubmodelMatchesExactDPPIdentityAndSemanticID(t *testing.T) {
	metadata := historicalMetadataSubmodel("metadata-1", "dpp-1", "semantic-1")

	require.True(t, historicalSubmodelMatchesDPP(metadata, "dpp-1", []string{"semantic-1"}))
	require.False(t, historicalSubmodelMatchesDPP(metadata, "dpp-2", []string{"semantic-1"}))
	require.False(t, historicalSubmodelMatchesDPP(metadata, "dpp-1", []string{"semantic-2"}))
}

func TestHistoricalAASReferenceMatchingUsesExactKeyValue(t *testing.T) {
	aas := types.NewAssetAdministrationShell("aas-1", types.NewAssetInformation(types.AssetKindInstance))
	aas.SetSubmodels([]types.IReference{
		types.NewReference(types.ReferenceTypesModelReference, []types.IKey{types.NewKey(types.KeyTypesSubmodel, "metadata-10")}),
	})

	require.True(t, aasReferencesSubmodel(aas, "metadata-10"))
	require.False(t, aasReferencesSubmodel(aas, "metadata-1"))
}

func TestHistoricalDPPResolutionFailsClosedBeforeDatabaseLookup(t *testing.T) {
	falseValue := false
	ctx := auth.WithQueryFilter(t.Context(), &auth.QueryFilter{
		Formula: &grammar.LogicalExpression{Boolean: &falseValue},
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumREAD: {Boolean: &falseValue},
		},
	})
	repository := &AssetAdministrationShellDatabase{}

	_, err := repository.GetAssetAdministrationShellByDPPIDAndDate(
		ctx,
		"dpp-1",
		[]string{"semantic-1"},
		time.Now(),
	)

	require.Error(t, err)
	require.True(t, common.IsErrNotFound(err))
	require.True(t, strings.Contains(err.Error(), "AASREPO-HISTORY-DPP-FILTERED"))
}

func TestHistoricalDPPIdentityBudgetRejectsTooManyUniqueCandidates(t *testing.T) {
	budget := historicalDPPIdentityBudget{seen: make(map[string]struct{})}
	identifiers := make([]string, maxHistoricalDPPIdentityCandidates+1)
	for index := range identifiers {
		identifiers[index] = string(rune(index + 1))
	}

	err := budget.consume(history.TableAAS, identifiers)

	require.ErrorContains(t, err, "AASREPO-HISTORY-DPP-TOOBROAD")
	require.True(t, common.IsErrConflict(err))
}

func historicalMetadataSubmodel(identifier string, dppID string, semanticID string) types.ISubmodel {
	metadata := types.NewSubmodel(identifier)
	metadata.SetSemanticID(types.NewReference(types.ReferenceTypesExternalReference, []types.IKey{
		types.NewKey(types.KeyTypesGlobalReference, semanticID),
	}))
	property := types.NewProperty(types.DataTypeDefXSDString)
	idShort := dppIdentifierPropertyIDShort
	property.SetIDShort(&idShort)
	property.SetValue(&dppID)
	metadata.SetSubmodelElements([]types.ISubmodelElement{property})
	return metadata
}
