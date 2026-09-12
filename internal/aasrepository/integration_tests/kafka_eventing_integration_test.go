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
	"encoding/base64"
	"fmt"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/events"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"
)

func TestKafkaOnlyPublishesAASAndAsset(t *testing.T) {
	base := "http://" + testenv.KafkaAddress("BASYX_IT_KAFKA_API_PORT")
	require.NoError(t, testenv.WaitHealthyURL(base+"/health", 150*time.Second))
	received := testenv.SubscribeKafka(t)
	id := fmt.Sprintf("urn:kafka:aas:%d", time.Now().UnixNano())
	asset := id + ":asset"
	endpoint := base + "/shells/" + base64.RawURLEncoding.EncodeToString([]byte(id))
	status, err := postResponseStatus(base+"/shells", fmt.Sprintf(`{"modelType":"AssetAdministrationShell","id":%q,"assetInformation":{"assetKind":"Instance","globalAssetId":%q}}`, id, asset))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	received.AwaitSubject(t, id, events.TypeAASCreated)
	received.AwaitSubject(t, asset, events.TypeAssetCreated)
	client := &http.Client{Timeout: 10 * time.Second}
	for path, want := range map[string]int{"/events": http.StatusNotFound, events.SchemaPath + "/metamodel-aasChangeEvent.v1.schema.json": http.StatusOK} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+path, nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, want, resp.StatusCode)
	}
	status, err = deleteResponseStatus(endpoint)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, status)
	received.AwaitSubject(t, id, events.TypeAASDeleted)
	received.AwaitSubject(t, asset, events.TypeAssetDeleted)
}
