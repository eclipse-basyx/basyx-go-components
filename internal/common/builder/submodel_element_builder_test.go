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

package builder

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/stretchr/testify/require"
)

func TestBuildRelationshipElementAllowsNullOptionalReferences(t *testing.T) {
	t.Parallel()

	value := json.RawMessage(`{"first":null,"second":null}`)
	element, err := buildRelationshipElement(model.SubmodelElementRow{
		IDShort:   sql.NullString{String: "SparseRelationship", Valid: true},
		ModelType: int64(types.ModelTypeRelationshipElement),
		Value:     &value,
	})

	require.NoError(t, err)

	relationship, ok := element.(*types.RelationshipElement)
	require.True(t, ok)
	require.Nil(t, relationship.First())
	require.Nil(t, relationship.Second())
}

func TestBuildAnnotatedRelationshipElementAllowsNullOptionalReferences(t *testing.T) {
	t.Parallel()

	value := json.RawMessage(`{"first":null,"second":null}`)
	element, err := buildAnnotatedRelationshipElement(model.SubmodelElementRow{
		IDShort:   sql.NullString{String: "SparseAnnotatedRelationship", Valid: true},
		ModelType: int64(types.ModelTypeAnnotatedRelationshipElement),
		Value:     &value,
	})

	require.NoError(t, err)

	relationship, ok := element.(*types.AnnotatedRelationshipElement)
	require.True(t, ok)
	require.Nil(t, relationship.First())
	require.Nil(t, relationship.Second())
}
