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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/eventfeed"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentUploadProducesAASAssetSubmodelAndPCNEvents(t *testing.T) {
	received := testenv.SubscribeMQTT(t, mqttBrokerURL, "basyx/#")
	kafkaEvents := testenv.SubscribeKafka(t)
	amqpEvents := testenv.SubscribeAMQP(t)
	stamp := time.Now().UnixNano()
	aasID := fmt.Sprintf("urn:example:event-feed:upload:aas:%d", stamp)
	smID := fmt.Sprintf("urn:example:event-feed:upload:sm:%d", stamp)
	assetID := fmt.Sprintf("urn:example:event-feed:upload:asset:%d", stamp)
	payload := fmt.Sprintf(`{
		"assetAdministrationShells": [{
			"modelType":"AssetAdministrationShell", "id":%q, "idShort":"FeedUpload",
			"assetInformation":{"assetKind":"Instance", "globalAssetId":%q},
			"submodels":[{"type":"ModelReference", "keys":[{"type":"Submodel", "value":%q}]}]
		}],
		"submodels":[{
			"modelType":"Submodel", "id":%q, "idShort":"UploadPCN",
			"semanticId":{"type":"ExternalReference", "keys":[{"type":"GlobalReference", "value":%q}]},
			"submodelElements":[{"modelType":"SubmodelElementCollection", "idShort":"Records", "value":[
				{"modelType":"SubmodelElementCollection", "idShort":"CN1", "value":[
					{"modelType":"Property", "idShort":"ManufacturerChangeID", "valueType":"xs:string", "value":"CN1"}
				]}
			]}]
		}],
		"conceptDescriptions":[]
	}`, aasID, assetID, smID, smID, eventfeed.SemanticIDPCN)
	path := filepath.Join(t.TempDir(), "environment.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	runMultipartUploadAction(t, testenv.JSONSuiteStep{
		Method: http.MethodPost, Endpoint: aasEnvEventFeedBaseURL + "/upload", Data: path,
		Headers: map[string]string{uploadHeaderPartContentType: "application/json"}, ExpectedStatus: http.StatusOK,
	})
	filter := fmt.Sprintf("rsql:event.subject=in=(%s,%s,%s)", aasID, assetID, smID)
	client := &http.Client{Timeout: 10 * time.Second}
	var feed eventfeed.FeedResponse
	require.Eventually(t, func() bool {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, aasEnvEventFeedBaseURL+"/events?"+url.Values{"filter": {filter}, "limit": {"100"}}.Encode(), nil)
		require.NoError(t, err)
		response := doHTTPIntegrationRequest(t, client, request)
		defer func() { _ = response.Body.Close() }()
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.NoError(t, json.NewDecoder(response.Body).Decode(&feed))
		return len(feed.Records) >= 4
	}, 5*time.Second, 50*time.Millisecond)
	require.Len(t, feed.Records, 4)
	mqttEvents := map[string]eventfeed.FeedRecord{}
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for len(mqttEvents) < 4 {
		select {
		case message := <-received:
			event := message.Event
			if event.Subject == aasID || event.Subject == assetID || event.Subject == smID {
				mqttEvents[event.ID] = event
			}
		case <-deadline.C:
			t.Fatal("MQTT upload events missing")
		}
	}
	for _, event := range feed.Records {
		require.Equal(t, event, mqttEvents[event.ID])
		key := "aas_history:" + aasID
		if event.Subject == smID {
			key = "submodel_history:" + smID
		}
		kafkaEvents.AssertEvent(t, event, key)
		amqpEvents.AssertEvent(t, event)
	}
	types := map[string]int{}
	for _, event := range feed.Records {
		types[event.Type]++
		if event.Type == eventfeed.TypePCN {
			record, ok := event.Data["record"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "CN1", record["ManufacturerChangeID"])
		}
	}
	require.Equal(t, map[string]int{eventfeed.TypeAASCreated: 1, eventfeed.TypeAssetCreated: 1, eventfeed.TypeSubmodelCreated: 1, eventfeed.TypePCN: 1}, types)
}
