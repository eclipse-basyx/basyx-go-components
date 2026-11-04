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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package auth

import (
	"context"
	"database/sql"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/stretchr/testify/require"
)

type creatorState struct {
	creator types.IReference
}

func (creatorState) ResourceCreated(context.Context, *sql.Tx, SemanticResourceKind, string) error {
	return nil
}

func (creatorState) ResourceDeleted(context.Context, *sql.Tx, SemanticResourceKind, string) error {
	return nil
}

func (creatorState) SubmodelReferencesRemoved(context.Context, *sql.Tx, string, []string) error {
	return nil
}

func (creatorState) SubmodelCreatedWithShell(context.Context, *sql.Tx, string, string) error {
	return nil
}

func (s creatorState) CreatorReference(context.Context) types.IReference {
	return s.creator
}

func testCreator(value string) types.IReference {
	return types.NewReference(types.ReferenceTypesExternalReference, []types.IKey{types.NewKey(types.KeyTypesGlobalReference, value)})
}

func creatorValue(administration types.IAdministrativeInformation) string {
	if administration == nil || administration.Creator() == nil {
		return ""
	}
	return administration.Creator().Keys()[0].Value()
}

func TestFillReBACCreatorFillsOnlyMissingCreators(t *testing.T) {
	ctx := WithReBACState(context.Background(), creatorState{creator: testCreator("issuer|alice")})

	submodel := types.NewSubmodel("urn:submodel")
	FillReBACCreator(ctx, submodel)
	require.Equal(t, "issuer|alice", creatorValue(submodel.Administration()), "a missing administration is created")

	versioned := types.NewAdministrativeInformation()
	version := "2"
	versioned.SetVersion(&version)
	submodel.SetAdministration(versioned)
	FillReBACCreator(ctx, submodel)
	require.Equal(t, "issuer|alice", creatorValue(submodel.Administration()))
	require.Equal(t, "2", *submodel.Administration().Version(), "other administrative information is kept")

	provided := types.NewAdministrativeInformation()
	provided.SetCreator(testCreator("urn:provided"))
	submodel.SetAdministration(provided)
	FillReBACCreator(ctx, submodel)
	require.Equal(t, "urn:provided", creatorValue(submodel.Administration()), "a provided creator is kept")
}

func TestReBACCreatorReferenceIsAbsentWithoutReBACPrincipal(t *testing.T) {
	require.Nil(t, ReBACCreatorReference(context.Background()), "ReBAC disabled")

	anonymous := WithReBACState(context.Background(), creatorState{})
	require.Nil(t, ReBACCreatorReference(anonymous), "anonymous caller")

	authenticated := WithReBACState(context.Background(), creatorState{creator: testCreator("issuer|alice")})
	derived := WithReBACSource(authenticated, SemanticResourceAAS, "")
	require.Nil(t, ReBACCreatorReference(derived), "derived resources mirror their source")

	submodel := types.NewSubmodel("urn:submodel")
	FillReBACCreator(derived, submodel)
	require.Nil(t, submodel.Administration(), "derived resources stay unchanged")
}
