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
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/conditional"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const submodelRevisionKind = "submodel"

func TestConditionalRequests(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ConfigPath: "etag_it_config.json",
		StepName: func(step testenv.JSONSuiteStep, stepNumber int) string {
			return fmt.Sprintf("Step_(%s)_%d_%s", step.Context, stepNumber, step.Method)
		},
	})
}

type conditionalSubmodel struct {
	t        *testing.T
	db       *sql.DB
	id       string
	endpoint string
}

func newConditionalSubmodel(t *testing.T, name string) conditionalSubmodel {
	t.Helper()
	db, err := common.NewDatabaseConnection(submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	id := fmt.Sprintf("urn:etag:%s:%d", name, time.Now().UnixNano())
	submodel := conditionalSubmodel{t: t, db: db, id: id, endpoint: submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(id)}
	body := mustJSON(t, map[string]any{
		"modelType": "Submodel",
		"id":        id,
		"idShort":   "ConditionalSubmodel",
		"submodelElements": []any{
			map[string]any{"modelType": "Property", "idShort": "Prop", "valueType": "xs:string", "value": "initial"},
		},
	})
	created := testenv.DoHTTP(t, http.MethodPost, submodelRepositoryBaseURL+"/submodels", body, nil)
	require.Equal(t, http.StatusCreated, created.Status, string(created.Body))
	t.Cleanup(func() { testenv.DoHTTP(t, http.MethodDelete, submodel.endpoint, nil, nil) })
	return submodel
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	return body
}

func (s conditionalSubmodel) etag() string {
	s.t.Helper()
	response := testenv.DoHTTP(s.t, http.MethodGet, s.endpoint, nil, nil)
	require.Equal(s.t, http.StatusOK, response.Status, string(response.Body))
	etag := response.Header.Get("ETag")
	require.NotEmpty(s.t, etag)
	return etag
}

func (s conditionalSubmodel) patchValue(value string, headers map[string]string) testenv.HTTPResult {
	return testenv.DoHTTP(s.t, http.MethodPatch, s.endpoint+"/submodel-elements/Prop/$value", mustJSON(s.t, value), headers)
}

func (s conditionalSubmodel) value() string {
	s.t.Helper()
	response := testenv.DoHTTP(s.t, http.MethodGet, s.endpoint+"/submodel-elements/Prop/$value", nil, nil)
	require.Equal(s.t, http.StatusOK, response.Status, string(response.Body))
	var value string
	require.NoError(s.t, json.Unmarshal(response.Body, &value))
	return value
}

func (s conditionalSubmodel) historyRows() int {
	return testenv.CountRows(s.t, s.db, "submodel_history", goqu.Ex{"identifier": s.id})
}

// inOrder starts the requests one after another while a test transaction
// holds the Submodel's revision row, waits until each of them queues for a
// lock, then releases the row, so they commit in the given order.
func (s conditionalSubmodel) inOrder(requests ...func() testenv.HTTPResult) []testenv.HTTPResult {
	s.t.Helper()
	release := testenv.LockResourceRevision(s.t, s.db, submodelRevisionKind, s.id)
	defer release()
	results := make([]testenv.HTTPResult, len(requests))
	var wg sync.WaitGroup
	for index, request := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[index] = request()
		}()
		testenv.WaitForLockWaiters(s.t, s.db, index+1)
	}
	release()
	wg.Wait()
	return results
}

func TestConditionalUpdateLosesAgainstConcurrentUnconditionalUpdate(t *testing.T) {
	submodel := newConditionalSubmodel(t, "race-update")
	etag := submodel.etag()

	results := submodel.inOrder(
		func() testenv.HTTPResult { return submodel.patchValue("unconditional", nil) },
		func() testenv.HTTPResult {
			return submodel.patchValue("conditional", map[string]string{"If-Match": etag})
		},
	)

	require.Equal(t, http.StatusNoContent, results[0].Status, string(results[0].Body))
	require.Equal(t, http.StatusPreconditionFailed, results[1].Status, string(results[1].Body))
	require.Equal(t, "unconditional", submodel.value())
}

func TestConditionalDeleteLosesAgainstConcurrentUpdate(t *testing.T) {
	submodel := newConditionalSubmodel(t, "race-delete")
	etag := submodel.etag()

	results := submodel.inOrder(
		func() testenv.HTTPResult { return submodel.patchValue("changed", nil) },
		func() testenv.HTTPResult {
			return testenv.DoHTTP(t, http.MethodDelete, submodel.endpoint, nil, map[string]string{"If-Match": etag})
		},
	)

	require.Equal(t, http.StatusNoContent, results[0].Status, string(results[0].Body))
	require.Equal(t, http.StatusPreconditionFailed, results[1].Status, string(results[1].Body))
	require.Equal(t, "changed", submodel.value())
}

func TestConditionalUpdateAfterDeleteDoesNotRecreate(t *testing.T) {
	submodel := newConditionalSubmodel(t, "delete-first")
	etag := submodel.etag()

	require.Equal(t, http.StatusNoContent, testenv.DoHTTP(t, http.MethodDelete, submodel.endpoint, nil, nil).Status)
	late := submodel.patchValue("late", map[string]string{"If-Match": etag})

	require.Contains(t, []int{http.StatusNotFound, http.StatusPreconditionFailed}, late.Status, string(late.Body))
	require.Equal(t, http.StatusNotFound, testenv.DoHTTP(t, http.MethodGet, submodel.endpoint, nil, nil).Status)
}

func TestConcurrentFirstConditionalWritesWithoutRevisionRow(t *testing.T) {
	submodel := newConditionalSubmodel(t, "first-write")
	testenv.DeleteResourceRevision(t, submodel.db, submodelRevisionKind, submodel.id)
	etag := submodel.etag()
	require.True(t, strings.HasPrefix(etag, `"0-`), etag)

	statuses := concurrently(2, func(index int) int {
		return submodel.patchValue(fmt.Sprintf("writer-%d", index), map[string]string{"If-Match": etag}).Status
	})

	require.ElementsMatch(t, []int{http.StatusNoContent, http.StatusPreconditionFailed}, statuses)
}

func TestConcurrentCreateOnlyPutsCreateOnce(t *testing.T) {
	db, err := common.NewDatabaseConnection(submodelRepositoryIntegrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	id := fmt.Sprintf("urn:etag:create-only:%d", time.Now().UnixNano())
	endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(id)
	t.Cleanup(func() { testenv.DoHTTP(t, http.MethodDelete, endpoint, nil, nil) })

	statuses := concurrently(2, func(index int) int {
		body := mustJSON(t, map[string]any{"modelType": "Submodel", "id": id, "idShort": fmt.Sprintf("Writer%d", index)})
		return testenv.DoHTTP(t, http.MethodPut, endpoint, body, map[string]string{"If-None-Match": "*"}).Status
	})

	require.ElementsMatch(t, []int{http.StatusCreated, http.StatusPreconditionFailed}, statuses)
}

func TestFailedPreconditionLeavesNoTraces(t *testing.T) {
	submodel := newConditionalSubmodel(t, "no-traces")
	etag := submodel.etag()
	revision, _ := testenv.ResourceRevision(t, submodel.db, submodelRevisionKind, submodel.id)
	historyRows := submodel.historyRows()
	stale := `"1-00000000"`

	require.Equal(t, http.StatusPreconditionFailed, submodel.patchValue("stale", map[string]string{"If-Match": stale}).Status)
	require.Equal(t, http.StatusPreconditionFailed, testenv.DoHTTP(t, http.MethodDelete, submodel.endpoint, nil, map[string]string{"If-Match": stale}).Status)
	require.Equal(t, http.StatusPreconditionFailed, testenv.DoHTTP(t, http.MethodPost, submodel.endpoint+"/submodel-elements",
		mustJSON(t, map[string]any{"modelType": "Property", "idShort": "Other", "valueType": "xs:string"}), map[string]string{"If-Match": stale}).Status)

	require.Equal(t, "initial", submodel.value())
	require.Equal(t, etag, submodel.etag())
	require.Equal(t, historyRows, submodel.historyRows())
	current, _ := testenv.ResourceRevision(t, submodel.db, submodelRevisionKind, submodel.id)
	require.Equal(t, revision, current)
}

func TestSuperpathAndElementReadsShareTheSubmodelRevision(t *testing.T) {
	submodel := newConditionalSubmodel(t, "shared-revision")
	submodelETag := submodel.etag()
	element := testenv.DoHTTP(t, http.MethodGet, submodel.endpoint+"/submodel-elements/Prop", nil, nil)
	require.Equal(t, http.StatusOK, element.Status)

	require.Equal(t, concurrencyPrefix(submodelETag), concurrencyPrefix(element.Header.Get("ETag")))
	require.NotEqual(t, submodelETag, element.Header.Get("ETag"))
	require.Equal(t, http.StatusNoContent, submodel.patchValue("by-element-etag", map[string]string{"If-Match": element.Header.Get("ETag")}).Status)
}

func TestInvokeChecksIfMatchBeforeDispatch(t *testing.T) {
	var calls atomic.Int32
	// #nosec G102 -- the delegation target must be reachable from the repository container.
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	delegationURL := fmt.Sprintf("http://host.docker.internal:%d/delegate", listener.Addr().(*net.TCPAddr).Port)

	id := fmt.Sprintf("urn:etag:invoke:%d", time.Now().UnixNano())
	endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(id)
	created := testenv.DoHTTP(t, http.MethodPost, submodelRepositoryBaseURL+"/submodels", mustJSON(t, map[string]any{
		"modelType": "Submodel", "id": id, "idShort": "Invoke",
		"submodelElements": []any{map[string]any{
			"modelType": "Operation", "idShort": "Op",
			"qualifiers": []any{map[string]any{"type": "invocationDelegation", "valueType": "xs:string", "value": delegationURL}},
		}},
	}), nil)
	require.Equal(t, http.StatusCreated, created.Status, string(created.Body))
	t.Cleanup(func() { testenv.DoHTTP(t, http.MethodDelete, endpoint, nil, nil) })

	invoke := testenv.DoHTTP(t, http.MethodPost, endpoint+"/submodel-elements/Op/invoke",
		mustJSON(t, map[string]any{"clientTimeoutDuration": "PT10S"}), map[string]string{"If-Match": `"1-00000000"`})
	require.Equal(t, http.StatusPreconditionFailed, invoke.Status, string(invoke.Body))
	require.Zero(t, calls.Load())
}

func concurrencyPrefix(etag string) string {
	parts := strings.Split(strings.Trim(etag, `"`), "-")
	if len(parts) < 2 {
		return etag
	}
	return parts[0] + "-" + parts[1]
}

func concurrently(count int, request func(index int) int) []int {
	statuses := make([]int, count)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			statuses[index] = request(index)
		}()
	}
	close(start)
	wg.Wait()
	return statuses
}

func nestedCollection(value string) map[string]any {
	return map[string]any{
		"modelType": "SubmodelElementCollection", "idShort": "Outer",
		"value": []any{map[string]any{
			"modelType": "SubmodelElementCollection", "idShort": "Inner",
			"value": []any{map[string]any{"modelType": "Property", "idShort": "Prop", "valueType": "xs:string", "value": value}},
		}},
	}
}

func TestConditionalWritesToNestedCollectionsUseTheSubmodelRevision(t *testing.T) {
	id := fmt.Sprintf("urn:etag:nested-collection:%d", time.Now().UnixNano())
	endpoint := submodelRepositoryBaseURL + "/submodels/" + common.EncodeString(id)
	created := testenv.DoHTTP(t, http.MethodPost, submodelRepositoryBaseURL+"/submodels", mustJSON(t, map[string]any{
		"modelType": "Submodel", "id": id, "idShort": "Nested", "submodelElements": []any{nestedCollection("initial")},
	}), nil)
	require.Equal(t, http.StatusCreated, created.Status, string(created.Body))
	t.Cleanup(func() { testenv.DoHTTP(t, http.MethodDelete, endpoint, nil, nil) })
	outerURL := endpoint + "/submodel-elements/Outer"
	propValueURL := endpoint + "/submodel-elements/Outer.Inner.Prop/$value"
	readProp := func() string {
		response := testenv.DoHTTP(t, http.MethodGet, propValueURL, nil, nil)
		require.Equal(t, http.StatusOK, response.Status, string(response.Body))
		var value string
		require.NoError(t, json.Unmarshal(response.Body, &value))
		return value
	}

	submodelETag := testenv.DoHTTP(t, http.MethodGet, endpoint, nil, nil).Header.Get("ETag")
	outer := testenv.DoHTTP(t, http.MethodGet, outerURL, nil, nil)
	require.Equal(t, http.StatusOK, outer.Status)
	require.Equal(t, concurrencyPrefix(submodelETag), concurrencyPrefix(outer.Header.Get("ETag")))

	stale := testenv.DoHTTP(t, http.MethodPatch, outerURL, mustJSON(t, nestedCollection("stale")), map[string]string{"If-Match": `"1-00000000"`})
	require.Equal(t, http.StatusPreconditionFailed, stale.Status, string(stale.Body))
	require.Equal(t, "initial", readProp())

	patched := testenv.DoHTTP(t, http.MethodPatch, outerURL, mustJSON(t, nestedCollection("collection")), map[string]string{"If-Match": submodelETag})
	require.Equal(t, http.StatusNoContent, patched.Status, string(patched.Body))
	require.Equal(t, "collection", readProp())

	nestedValue := testenv.DoHTTP(t, http.MethodPatch, propValueURL, mustJSON(t, "nested"), map[string]string{"If-Match": submodelETag})
	require.Equal(t, http.StatusPreconditionFailed, nestedValue.Status, "the collection update must change the Submodel revision")
	require.Equal(t, "collection", readProp())

	current := testenv.DoHTTP(t, http.MethodGet, outerURL, nil, nil).Header.Get("ETag")
	nestedValue = testenv.DoHTTP(t, http.MethodPatch, propValueURL, mustJSON(t, "nested"), map[string]string{"If-Match": current})
	require.Equal(t, http.StatusNoContent, nestedValue.Status, string(nestedValue.Body))
	require.Equal(t, "nested", readProp())

	child := mustJSON(t, map[string]any{"modelType": "Property", "idShort": "Child", "valueType": "xs:string", "value": "x"})
	require.Equal(t, http.StatusPreconditionFailed,
		testenv.DoHTTP(t, http.MethodPost, outerURL+".Inner", child, map[string]string{"If-Match": current}).Status,
		"the nested value update must change the Submodel revision")
	latest := testenv.DoHTTP(t, http.MethodGet, endpoint, nil, nil).Header.Get("ETag")
	added := testenv.DoHTTP(t, http.MethodPost, outerURL+".Inner", child, map[string]string{"If-Match": latest})
	require.Equal(t, http.StatusCreated, added.Status, string(added.Body))
	require.NotEqual(t, concurrencyPrefix(latest), concurrencyPrefix(added.Header.Get("ETag")))
}

func TestWildcardsOfElementPutsReferToTheElement(t *testing.T) {
	submodel := newConditionalSubmodel(t, "element-put")
	elementURL := submodel.endpoint + "/submodel-elements/Added"
	element := mustJSON(t, map[string]any{"modelType": "Property", "idShort": "Added", "valueType": "xs:string", "value": "x"})

	require.Equal(t, http.StatusPreconditionFailed, testenv.DoHTTP(t, http.MethodPut, elementURL, element, map[string]string{"If-Match": "*"}).Status,
		"If-Match: * requires the element, not only its Submodel")
	created := testenv.DoHTTP(t, http.MethodPut, elementURL, element, map[string]string{"If-None-Match": "*"})
	require.Equal(t, http.StatusCreated, created.Status, string(created.Body))
	require.Equal(t, http.StatusPreconditionFailed, testenv.DoHTTP(t, http.MethodPut, elementURL, element, map[string]string{"If-None-Match": "*"}).Status)
	require.Equal(t, http.StatusNoContent, testenv.DoHTTP(t, http.MethodPut, elementURL, element, map[string]string{"If-Match": "*"}).Status)
}

func TestTombstoneCleanupRemovesOnlyRevisionsOfDeletedResources(t *testing.T) {
	deleted := newConditionalSubmodel(t, "tombstone-deleted")
	live := newConditionalSubmodel(t, "tombstone-live")
	require.Equal(t, http.StatusNoContent, testenv.DoHTTP(t, http.MethodDelete, deleted.endpoint, nil, nil).Status)
	_, tombstone := testenv.ResourceRevision(t, deleted.db, submodelRevisionKind, deleted.id)
	require.True(t, tombstone, "a delete keeps the revision as a tombstone")
	liveRevision, _ := testenv.ResourceRevision(t, live.db, submodelRevisionKind, live.id)

	_, err := conditional.RemoveTombstones(t.Context(), deleted.db)
	require.NoError(t, err)

	_, tombstone = testenv.ResourceRevision(t, deleted.db, submodelRevisionKind, deleted.id)
	require.False(t, tombstone)
	current, exists := testenv.ResourceRevision(t, live.db, submodelRevisionKind, live.id)
	require.True(t, exists)
	require.Equal(t, liveRevision, current)
}
