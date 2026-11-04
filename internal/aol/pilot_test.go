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

package aol

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func pilotReader(t *testing.T, serve func(http.ResponseWriter, *http.Request)) Reader {
	t.Helper()
	return Reader{Dispatch: Dispatcher{Router: http.HandlerFunc(serve), MaxBytes: 4096}, PageSize: 2, MaxScan: 100}
}
func writePilotJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func pilotShell() map[string]any {
	return map[string]any{"id": "urn:motor", "idShort": "M4711", "assetInformation": map[string]any{"globalAssetId": "M-4711"}, "submodels": []any{map[string]any{"keys": []any{map[string]any{"value": "urn:technical"}}}}}
}
func pilotTemperature(name string) map[string]any {
	return map[string]any{"idShort": name, "modelType": "Property", "valueType": "xs:double", "semanticId": reference("urn:temperature"), "embeddedDataSpecifications": []any{map[string]any{"dataSpecificationContent": map[string]any{"unit": "degC"}}}}
}

func TestPilotAssetSearchShrinksRESTPagesAndReturnsCompactResults(t *testing.T) {
	requests := 0
	reader := pilotReader(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit > 2 {
			_, _ = w.Write([]byte(strings.Repeat("x", 5000)))
			return
		}
		shell := pilotShell()
		shell["irrelevantPayload"] = strings.Repeat("x", 2000)
		writePilotJSON(w, map[string]any{"result": []any{shell}, "paging_metadata": map[string]any{"cursor": "next"}})
	})
	reader.PageSize = 20
	value, err := reader.FindAsset(t.Context(), "M4711", "")
	require.NoError(t, err)
	raw, _ := json.Marshal(value)
	require.NotContains(t, string(raw), "irrelevantPayload")
	require.NotContains(t, string(raw), `"submodels"`)
	require.Contains(t, string(raw), "urn:motor")
	require.Greater(t, requests, 1)
	require.Less(t, len(raw), 1000)
}
func TestPilotDescriptionPagesSubmodelsWithoutReadingFullSubmodels(t *testing.T) {
	reader := pilotReader(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shells/" + EncodeID("urn:motor"):
			writePilotJSON(w, pilotShell())
		case "/shells/" + EncodeID("urn:motor") + "/submodel-refs":
			writePilotJSON(w, map[string]any{"result": []any{map[string]any{"keys": []any{map[string]any{"value": "urn:technical"}}}}, "paging_metadata": map[string]any{"cursor": "next"}})
		case "/submodels/" + EncodeID("urn:technical") + "/$metadata":
			writePilotJSON(w, map[string]any{"id": "urn:technical", "idShort": "TechnicalData"})
		case "/submodels/" + EncodeID("urn:technical") + "/submodel-elements/$metadata":
			element := pilotTemperature("Temperature")
			element["value"] = "50"
			writePilotJSON(w, map[string]any{"result": []any{element}, "paging_metadata": map[string]any{}})
		case "/submodels/" + EncodeID("urn:technical") + "/submodel-elements/Temperature/$value":
			writePilotJSON(w, 50)
		case "/concept-descriptions/" + EncodeID("urn:temperature"):
			w.WriteHeader(404)
		default:
			t.Errorf("unexpected full or unrelated read %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	value, err := reader.DescribePage(t.Context(), "urn:motor", "")
	require.NoError(t, err)
	raw, _ := json.Marshal(value)
	require.Contains(t, string(raw), "Temperature")
	require.Contains(t, string(raw), "degC")
	require.Contains(t, string(raw), "next")
}
func measurementFixture(t *testing.T, ambiguous bool, valueCalls *int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer caller", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/shells/" + EncodeID("M-4711"):
			w.WriteHeader(404)
		case "/shells":
			writePilotJSON(w, map[string]any{"result": []any{pilotShell()}, "paging_metadata": map[string]any{}})
		case "/shells/" + EncodeID("urn:motor"):
			writePilotJSON(w, pilotShell())
		case "/shells/" + EncodeID("urn:motor") + "/submodel-refs":
			writePilotJSON(w, map[string]any{"result": pilotShell()["submodels"], "paging_metadata": map[string]any{}})
		case "/submodels/" + EncodeID("urn:technical") + "/submodel-elements/$metadata":
			elements := []any{pilotTemperature("Temperature")}
			if ambiguous {
				elements = append(elements, pilotTemperature("TemperatureOutside"))
			}
			writePilotJSON(w, map[string]any{"result": elements, "paging_metadata": map[string]any{}})
		case "/submodels/" + EncodeID("urn:technical") + "/submodel-elements/Temperature/$value":
			(*valueCalls)++
			w.Header().Set("ETag", `"revision-2"`)
			writePilotJSON(w, 50)
		case "/submodels/" + EncodeID("urn:technical") + "/submodel-elements/Temperature/$metadata":
			writePilotJSON(w, pilotTemperature("Temperature"))
		default:
			t.Errorf("unexpected read %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}
}
func TestPilotMeasurementLookupResolvesAliasAndReportsAmbiguity(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(strconv.FormatBool(ambiguous), func(t *testing.T) {
			valueCalls := 0
			reader := pilotReader(t, measurementFixture(t, ambiguous, &valueCalls))
			ctx := ContextWithIdentity(t.Context(), Identity{Token: "Bearer caller"})
			value, err := reader.Measurement(ctx, "M-4711", "temperature", "")
			require.NoError(t, err)
			raw, _ := json.Marshal(value)
			if ambiguous {
				require.Contains(t, string(raw), "ambiguous")
				require.Zero(t, valueCalls)
			} else {
				require.Contains(t, string(raw), `"data":50`)
				require.Contains(t, string(raw), `"unit":"degC"`)
				require.Contains(t, string(raw), `"measurement_time":null`)
				require.Contains(t, string(raw), `"retrieved_at"`)
				require.Equal(t, 1, valueCalls)
			}
		})
	}
}
func TestPilotElementReportsRetrievalTimeAndRevision(t *testing.T) {
	reader := pilotReader(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"r2"`)
		if strings.HasSuffix(r.URL.Path, "/$metadata") {
			writePilotJSON(w, pilotTemperature("Temperature"))
			return
		}
		writePilotJSON(w, 50)
	})
	result, err := reader.Element(t.Context(), "urn:technical", "Temperature")
	require.NoError(t, err)
	require.Equal(t, `"r2"`, result.(map[string]any)["etag"])
	require.NotEmpty(t, result.(map[string]any)["retrieved_at"])
}
func TestPilotMeasurementRejectsUnboundedSearch(t *testing.T) {
	reader := pilotReader(t, func(_ http.ResponseWriter, _ *http.Request) { t.Fatal("empty measurement search dispatched") })
	_, err := reader.Measurement(t.Context(), "urn:motor", "", "")
	require.ErrorContains(t, err, "MCP-MEASUREMENT-QUERY")
}

func TestPilotDiscoveryPagesFitEncodedBudgetWithoutLosingMatches(t *testing.T) {
	reader := pilotReader(t, func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		end := min(4, start+limit)
		items := []any{}
		for i := start; i < end; i++ {
			items = append(items, map[string]any{"id": strings.Repeat("x", 200) + strconv.Itoa(i), "idShort": "Motor" + strconv.Itoa(i), "assetInformation": map[string]any{"globalAssetId": strings.Repeat("a", 200)}})
		}
		cursor := ""
		if end < 4 {
			cursor = strconv.Itoa(end)
		}
		writePilotJSON(w, map[string]any{"result": items, "paging_metadata": map[string]any{"cursor": cursor}})
	})
	reader.PageSize = 20
	count := 0
	cursor := ""
	for {
		value, err := reader.FindAsset(t.Context(), "Motor", cursor)
		require.NoError(t, err)
		raw, _ := json.Marshal(value)
		quoted, _ := json.Marshal(string(raw))
		require.LessOrEqual(t, len(raw)+len(quoted)+256, reader.Dispatch.MaxBytes)
		result := value.(map[string]any)
		count += len(result["result"].([]map[string]any))
		paging, _ := json.Marshal(result["paging_metadata"])
		var next struct {
			Cursor string `json:"cursor"`
		}
		require.NoError(t, json.Unmarshal(paging, &next))
		cursor = next.Cursor
		if cursor == "" {
			break
		}
	}
	require.Equal(t, 4, count)
}

func TestPilotValuePagesIncludeMCPEncodingOverhead(t *testing.T) {
	reader := pilotReader(t, func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		values := []any{strings.Repeat("x", 1000), strings.Repeat("y", 1000)}
		if limit < 2 {
			values = values[:1]
		}
		writePilotJSON(w, map[string]any{"result": values, "paging_metadata": map[string]any{"cursor": "continue"}})
	})
	value, err := reader.ReadPaged(t.Context(), "/values", "")
	require.NoError(t, err)
	require.True(t, fitsResponse(value, reader.Dispatch.MaxBytes))
	require.Contains(t, value.(map[string]any)["data"], "paging_metadata")
}

func TestPilotDescriptionReadsNestedValuesThroughTheirAuthorizedPaths(t *testing.T) {
	reader := pilotReader(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submodels/" + EncodeID("urn:nested") + "/$metadata":
			writePilotJSON(w, map[string]any{"id": "urn:nested"})
		case "/submodels/" + EncodeID("urn:nested") + "/submodel-elements/$metadata":
			writePilotJSON(w, map[string]any{"result": []any{map[string]any{"modelType": "SubmodelElementCollection", "idShort": "Telemetry", "value": []any{pilotTemperature("Temperature")}}}, "paging_metadata": map[string]any{}})
		case "/submodels/" + EncodeID("urn:nested") + "/submodel-elements/Telemetry.Temperature/$value":
			writePilotJSON(w, 50)
		case "/concept-descriptions/" + EncodeID("urn:temperature"):
			writePilotJSON(w, map[string]any{"id": "urn:temperature", "idShort": "TemperatureConcept"})
		default:
			t.Errorf("unexpected read %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	value, err := reader.describeSubmodel(t.Context(), "urn:nested")
	require.NoError(t, err)
	raw, _ := json.Marshal(value)
	require.Contains(t, string(raw), `"path":"Telemetry.Temperature"`)
	require.Contains(t, string(raw), `"value":50`)
	require.Contains(t, string(raw), "TemperatureConcept")
}

func TestPilotDescriptionListChildrenUseIndices(t *testing.T) {
	elements := []any{map[string]any{"idShort": "Samples", "modelType": "SubmodelElementList", "value": []any{pilotTemperature("IgnoredName")}}}
	values := []map[string]any{}
	flattenElements(elements, "", "/submodels/test", &values)
	require.Len(t, values, 1)
	require.Equal(t, "Samples[0]", values[0]["path"])
}
