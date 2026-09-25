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

package rebac

import (
	"strings"
	"testing"

	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestElementPathChainFollowsSegmentsAndListIndexes(t *testing.T) {
	t.Parallel()

	for path, expected := range map[string][]string{
		"a":            {"a"},
		"a.b.c":        {"a", "a.b", "a.b.c"},
		"list[1]":      {"list", "list[1]"},
		"list[1][0]":   {"list", "list[1]", "list[1][0]"},
		"a.list[2].c":  {"a", "a.list", "a.list[2]", "a.list[2].c"},
		"a.list[10].c": {"a", "a.list", "a.list[10]", "a.list[10].c"},
	} {
		require.Equal(t, expected, ElementPathChain(path), path)
	}
	require.Equal(t, "", ParentElementPath("a"))
	require.Equal(t, "a.list[2]", ParentElementPath("a.list[2].c"))
}

func TestElementKeysArePathAndSubmodelSpecific(t *testing.T) {
	t.Parallel()

	const submodelUUID = "1c7d6b4f-7b90-4c75-8b1d-1e2e503b2f02"
	require.NotEqual(t, ElementKey(submodelUUID, "list[1]"), ElementKey(submodelUUID, "list[10]"))
	require.NotEqual(t, ElementKey(submodelUUID, "a"), ElementKey("2d8e7c50-8ca1-4d86-9c2e-2f3f614c3003", "a"))
}

func TestIdentitiesAreIssuerScopedAndBounded(t *testing.T) {
	t.Parallel()

	require.NotEqual(t, UserKey("https://a.example", "alice"), UserKey("https://b.example", "alice"))
	require.NotEqual(t, UserKey("https://a.example", "x.y"), UserKey("https://a.example.x", "y"))
	require.NotEqual(t, GroupKey("https://a.example", "ops"), UserKey("https://a.example", "ops"))
	long := strings.Repeat("s", 400)
	object := UserKey("https://a.example", long)
	require.LessOrEqual(t, len(object), maxIdentifierLength+len(TypeUser)+1)
	require.Equal(t, object, UserKey("https://a.example", long), "overlong identifiers must stay deterministic")
	require.NotEqual(t, object, UserKey("https://a.example", long+"t"))
	require.NotContains(t, strings.TrimPrefix(UserKey("https://a.example", "alice"), "user:"), ":")
	require.NotContains(t, UserKey("https://a.example", "alice#member"), "#")
}

func TestPrincipalRequiresIssuerAndSubject(t *testing.T) {
	t.Parallel()

	_, ok := PrincipalFromClaims(auth.Claims{}, "groups")
	require.False(t, ok, "anonymous callers are no principals")
	_, ok = PrincipalFromClaims(auth.Claims{"sub": "alice"}, "groups")
	require.False(t, ok, "subjects without issuer are no principals")

	principal, ok := PrincipalFromClaims(auth.Claims{
		"iss": "https://idp", "sub": "alice", "groups": []any{"ops", " ", "dev", "ops", 42},
	}, "groups")
	require.True(t, ok)
	require.Equal(t, []string{"dev", "ops"}, principal.Groups)
	require.Equal(t, []string{principal.UserKey(), GroupKey("https://idp", "dev"), GroupKey("https://idp", "ops")}, principal.SubjectKeys())

	single, _ := PrincipalFromClaims(auth.Claims{"iss": "https://idp", "sub": "bob", "role": "ops"}, "role")
	require.Equal(t, []string{"ops"}, single.Groups)
}

func TestOnlyResourceRolesCanBeGrantedDirectly(t *testing.T) {
	t.Parallel()

	for _, relation := range []string{RelationOwner, RelationEditor, RelationViewer, RelationExecutor} {
		require.NoError(t, ValidateRelation(TypeSubmodel, relation))
		require.NoError(t, ValidateRelation(TypeElement, relation))
	}
	require.Error(t, ValidateRelation(TypeConceptDescription, RelationExecutor))
	for _, relation := range []string{PermissionRead, RelationAdmin, RelationCreator, ""} {
		require.Error(t, ValidateRelation(TypeAAS, relation), relation)
	}
	require.NoError(t, ValidateRelation(TypeRepository, RelationCreator))
	require.NoError(t, ValidateRelation(TypeRepository, RelationAdmin))
	require.Error(t, ValidateRelation(TypeRepository, RelationViewer))
}
