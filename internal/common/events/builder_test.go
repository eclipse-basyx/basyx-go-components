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

package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuilderAASCreatedPayloads(t *testing.T) {
	b := NewBuilder(Config{SourceBaseURL: "https://example.com/api", SchemaBaseURL: "https://schemas.example.com"})
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	b.now = func() time.Time { return now }
	event, err := b.AASCreated("aas-1", "asset-1", []SubmodelRef{{SubmodelID: "sm-1", SemanticID: "sem-1"}, {SubmodelID: "sm-2"}})
	require.NoError(t, err)
	require.Equal(t, TypeAASCreated, event.Type)
	require.Equal(t, "aas-1", event.Subject)
	require.Equal(t, "https://example.com/api/shells", event.Source)
	require.Equal(t, now, event.Time)
	require.Equal(t, "https://schemas.example.com/metamodel-aasChangeEvent.v1.schema.json", event.DataSchemaFull)
	require.JSONEq(t, `{"aasId":"aas-1","globalAssetId":"asset-1","submodels":[
 {"type":"ModelReference","keys":[{"type":"Submodel","value":"sm-1"}],"referredSemanticId":{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"sem-1"}]}},
 {"type":"ModelReference","keys":[{"type":"Submodel","value":"sm-2"}]}
 ]}`, event.DataFull)
	require.JSONEq(t, `{"aasId":"aas-1"}`, event.DataCompact)
}
func TestBuilderSubmodelUpdated(t *testing.T) {
	event, err := NewBuilder(DefaultConfig()).SubmodelUpdated("sm-1", "https://semantic", []string{"asset-1"})
	require.NoError(t, err)
	require.Equal(t, TypeSubmodelUpdated, event.Type)
	require.JSONEq(t, `{"submodelId":"sm-1","globalAssetIds":["asset-1"],"semanticId":{"type":"ExternalReference","keys":[{"type":"GlobalReference","value":"https://semantic"}]}}`, event.DataFull)
}
func TestBuilderDeletedAndPCN(t *testing.T) {
	b := NewBuilder(DefaultConfig())
	aas, err := b.AASDeleted("aas-1", "asset-1", []SubmodelRef{{SubmodelID: "sm-1"}})
	require.NoError(t, err)
	require.Equal(t, TypeAASDeleted, aas.Type)
	asset, err := b.AssetDeleted("asset-1", "aas-1", nil)
	require.NoError(t, err)
	require.Equal(t, TypeAssetDeleted, asset.Type)
	sm, err := b.SubmodelDeleted("sm-1", SemanticIDPCN, nil)
	require.NoError(t, err)
	require.Equal(t, TypeSubmodelDeleted, sm.Type)
	pcn, err := b.PCN("sm-1", []string{"asset-1"}, map[string]any{"ManufacturerChangeID": "CN123456"})
	require.NoError(t, err)
	require.Equal(t, TypePCN, pcn.Type)
	require.JSONEq(t, `{"submodelId":"sm-1","globalAssetIds":["asset-1"],"record":{"ManufacturerChangeID":"CN123456"}}`, pcn.DataFull)
	require.True(t, IsPCNSemanticID(SemanticIDPCN))
	require.False(t, IsPCNSemanticID("other"))
	require.True(t, IsPCNSemanticID("0173-1#01-AHE582#005"))
	require.False(t, IsPCNSemanticID("0173-1#01-AHE581#003"))
}
