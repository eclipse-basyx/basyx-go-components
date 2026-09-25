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
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/rebac/model"
	fgasdk "github.com/openfga/go-sdk"
	"github.com/stretchr/testify/require"
)

func embeddedActivation(t *testing.T) Activation {
	t.Helper()
	hash, err := EmbeddedModelHash()
	require.NoError(t, err)
	return Activation{Scope: "it", StoreID: "01STORE", ModelID: "01MODEL", ModelHash: hash}
}

func TestStartupAcceptsOnlyTheModelOfThisRelease(t *testing.T) {
	t.Parallel()

	activation := embeddedActivation(t)
	require.NoError(t, verifyModel(t.Context(), &fakeClient{modelJSON: storedModel(t, nil)}, activation))

	changed := storedModel(t, func(model map[string]any) {
		definitions := model["type_definitions"].([]any)
		model["type_definitions"] = definitions[:len(definitions)-1]
	})
	err := verifyModel(t.Context(), &fakeClient{modelJSON: changed}, activation)
	require.ErrorContains(t, err, "REBAC-SETUP-MODELHASH")

	stale := activation
	stale.ModelHash = "stale"
	require.ErrorContains(t, verifyModel(t.Context(), &fakeClient{modelJSON: storedModel(t, nil)}, stale), "REBAC-SETUP-MODELHASH")

	require.ErrorContains(t, verifyModel(t.Context(), &fakeClient{err: errors.New("unreachable")}, activation), "REBAC-SETUP-MODELUNREACHABLE")
}

// storedModel returns the embedded model as OpenFGA returns it, with a
// store-assigned ID, optionally mutated.
func storedModel(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(model.JSON(), &parsed))
	parsed["id"] = "01MODEL"
	if mutate != nil {
		mutate(parsed)
	}
	encoded, err := json.Marshal(parsed)
	require.NoError(t, err)
	return encoded
}

func TestStartupRejectsForeignBindings(t *testing.T) {
	t.Parallel()

	activation := embeddedActivation(t)
	config := common.ReBACConfig{Scope: "it"}
	require.NoError(t, checkActivation(config, activation))

	config.Scope = "other"
	require.ErrorContains(t, checkActivation(config, activation), "REBAC-SETUP-SCOPEMISMATCH")
	config = common.ReBACConfig{Scope: "it", OpenFGA: common.ReBACOpenFGAConfig{StoreID: "01OTHER"}}
	require.ErrorContains(t, checkActivation(config, activation), "REBAC-SETUP-STOREMISMATCH")
	config = common.ReBACConfig{Scope: "it", OpenFGA: common.ReBACOpenFGAConfig{AuthorizationModelID: "01OTHER"}}
	require.ErrorContains(t, checkActivation(config, activation), "REBAC-SETUP-MODELMISMATCH")
}

func TestStartupRequiresABACAndOIDC(t *testing.T) {
	t.Parallel()

	cfg := &common.Config{}
	require.ErrorContains(t, validateServiceRequirements(cfg), "REBAC-SETUP-ABACREQUIRED")
	cfg.ABAC.Enabled = true
	require.ErrorContains(t, validateServiceRequirements(cfg), "REBAC-SETUP-OIDCREQUIRED")
	cfg.OIDC.TrustlistPath = t.TempDir() + "/missing.json"
	require.ErrorContains(t, validateServiceRequirements(cfg), "REBAC-SETUP-OIDCREQUIRED")
}

func TestProjectorBatchesNeverTouchATupleTwice(t *testing.T) {
	t.Parallel()

	tuple := Tuple{User: "user:a", Relation: RelationViewer, Object: "aas:1"}
	other := Tuple{User: "user:b", Relation: RelationViewer, Object: "aas:1"}
	rows := []OutboxRow{
		{Seq: 1, Operation: OutboxOperation{Tuple: tuple}},
		{Seq: 2, Operation: OutboxOperation{Tuple: other}},
		{Seq: 3, Operation: OutboxOperation{Delete: true, Tuple: tuple}},
		{Seq: 4, Operation: OutboxOperation{Tuple: other}},
	}
	batch := conflictFreePrefix(rows)
	require.Len(t, batch, 2, "a write and a later delete of one tuple must not share a transaction")
	writes, deletes := splitOperations(batch)
	require.Equal(t, []Tuple{tuple, other}, writes)
	require.Empty(t, deletes)
}

func TestProjectorBackoffIsBounded(t *testing.T) {
	t.Parallel()

	require.Equal(t, projectorBaseBackoff, projectorBackoff(0))
	require.Equal(t, 2*projectorBaseBackoff, projectorBackoff(1))
	require.Equal(t, projectorMaxBackoff, projectorBackoff(100))
	require.Less(t, projectorBackoff(3), 30*time.Second)
}

func TestWritesAreChunkedToOpenFGALimits(t *testing.T) {
	t.Parallel()

	writes := make([]Tuple, 150)
	deletes := make([]Tuple, 30)
	chunkWrites, chunkDeletes := splitWriteChunk(writes, deletes)
	require.Len(t, chunkWrites, maxWriteTuples)
	require.Empty(t, chunkDeletes)
	chunkWrites, chunkDeletes = splitWriteChunk(writes[100:], deletes)
	require.Len(t, chunkWrites, 50)
	require.Len(t, chunkDeletes, 30)
}

func TestModelReadThroughTheSDKHashesEqualToTheEmbeddedModel(t *testing.T) {
	t.Parallel()

	stored, err := os.ReadFile("model/testdata/openfga_1_18_stored_model.json")
	require.NoError(t, err)
	var sdkModel fgasdk.AuthorizationModel
	require.NoError(t, json.Unmarshal(stored, &sdkModel))
	marshaled, err := json.Marshal(sdkModel)
	require.NoError(t, err)
	storedHash, err := ModelContentHash(marshaled)
	require.NoError(t, err)
	embeddedHash, err := EmbeddedModelHash()
	require.NoError(t, err)
	require.Equal(t, embeddedHash, storedHash)
}
