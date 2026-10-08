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

package integration_tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

const dppRepresentationETagPattern = `^"\d+-[0-9a-f]{16}-[0-9a-f]{16}"$`

// assertDPPConditionalRequests checks that a DPP's entity tag covers its
// shell and all its Submodels, including changes made outside the DPP API.
func assertDPPConditionalRequests(t *testing.T, baseURL string, aasBaseURL string, encodedDPPID string, elementPath string, submodelID string) {
	t.Helper()
	dppURL := baseURL + "/v1/dpps/" + encodedDPPID
	read := testenv.DoHTTP(t, http.MethodGet, dppURL, nil, nil)
	require.Equal(t, http.StatusOK, read.Status)
	etag := read.Header.Get("ETag")
	require.Regexp(t, dppRepresentationETagPattern, etag)
	require.Equal(t, http.StatusNotModified, testenv.DoHTTP(t, http.MethodGet, dppURL, nil, map[string]string{"If-None-Match": etag}).Status)

	element := testenv.DoHTTP(t, http.MethodGet, dppURL+"/elements/"+elementPath, nil, nil)
	require.Equal(t, http.StatusOK, element.Status)
	require.Equal(t, dppValidator(etag), dppValidator(element.Header.Get("ETag")))

	patch := mustMarshal(t, map[string]any{"facilityId": "facility-etag"})
	require.Equal(t, http.StatusPreconditionFailed, testenv.DoHTTP(t, http.MethodPatch, dppURL, patch, map[string]string{"If-Match": `"1-0000000000000000"`}).Status)
	require.Equal(t, etag, testenv.DoHTTP(t, http.MethodGet, dppURL, nil, nil).Header.Get("ETag"))
	patched := testenv.DoHTTP(t, http.MethodPatch, dppURL, patch, map[string]string{"If-Match": etag})
	require.Equal(t, http.StatusOK, patched.Status, string(patched.Body))
	require.Regexp(t, `^"\d+-[0-9a-f]{16}"$`, patched.Header.Get("ETag"))
	afterPatch := testenv.DoHTTP(t, http.MethodGet, dppURL, nil, nil).Header.Get("ETag")
	require.NotEqual(t, etag, afterPatch)
	require.Equal(t, dppValidator(patched.Header.Get("ETag")), dppValidator(afterPatch))

	standalone := testenv.DoHTTP(t, http.MethodPatch,
		aasBaseURL+"/submodels/"+common.EncodeString(submodelID)+"/submodel-elements/energyClass/$value", mustMarshal(t, "A"), nil)
	require.Equal(t, http.StatusNoContent, standalone.Status, string(standalone.Body))
	afterStandalone := testenv.DoHTTP(t, http.MethodGet, dppURL, nil, nil).Header.Get("ETag")
	require.NotEqual(t, afterPatch, afterStandalone, "a change of a member Submodel must change the DPP entity tag")
	staleElement := testenv.DoHTTP(t, http.MethodPatch, dppURL+"/elements/"+elementPath, mustMarshal(t, 122), map[string]string{"If-Match": afterPatch})
	require.Equal(t, http.StatusPreconditionFailed, staleElement.Status, string(staleElement.Body))
}

func dppValidator(etag string) string {
	parts := strings.Split(strings.Trim(etag, `"`), "-")
	if len(parts) < 2 {
		return etag
	}
	return parts[0] + "-" + parts[1]
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	return body
}

// testDPPUpdateAfterConcurrentDelete forces a conditional element update to
// run after a concurrent delete of the same DPP, whose revisions are still
// 0 as for data that existed before revisions were introduced. The update
// must not recreate parts of the deleted DPP.
func testDPPUpdateAfterConcurrentDelete(t *testing.T, baseURL string, databasePort int, idSuffix string, now time.Time) {
	t.Helper()
	dppID := "https://www.example.org/dpp/race/" + idSuffix
	created := testenv.DoHTTP(t, http.MethodPost, baseURL+"/v1/dpps", mustMarshal(t, lifecycleDPPDocument(dppID, "https://www.example.org/race/"+idSuffix, now)), nil)
	require.Equal(t, http.StatusCreated, created.Status, string(created.Body))
	db := openDPPIntegrationDatabase(t, databasePort)
	defer func() { _ = db.Close() }()
	executeDataset(t, db, goqu.Dialect("postgres").Delete("resource_revision").Where(goqu.Or(
		goqu.C("identifier").Eq(dppID), goqu.C("identifier").Like(dppID+"/%"),
	)))

	dppURL := baseURL + "/v1/dpps/" + encodedPathParam(dppID)
	etag := testenv.DoHTTP(t, http.MethodGet, dppURL, nil, nil).Header.Get("ETag")
	require.True(t, strings.HasPrefix(etag, `"0-`), etag)
	elementURL := dppURL + "/elements/" + encodedPathParam(dppElementJSONPath(lifecycleTechnicalDataSpec, "energyClass"))

	barrier, err := db.BeginTx(context.TODO(), nil)
	require.NoError(t, err)
	lockShell, args, err := goqu.Dialect("postgres").From("aas").Select("id").Where(goqu.C("aas_id").Eq(dppID)).ForUpdate(exp.Wait).Prepared(true).ToSQL()
	require.NoError(t, err)
	var shellID int64
	require.NoError(t, barrier.QueryRow(lockShell, args...).Scan(&shellID))

	var wg sync.WaitGroup
	var deleted, updated testenv.HTTPResult
	wg.Add(2)
	go func() { defer wg.Done(); deleted = testenv.DoHTTP(t, http.MethodDelete, dppURL, nil, nil) }()
	testenv.WaitForLockWaiters(t, db, 1)
	go func() {
		defer wg.Done()
		updated = testenv.DoHTTP(t, http.MethodPatch, elementURL, mustMarshal(t, "C"), map[string]string{"If-Match": etag})
	}()
	testenv.WaitForLockWaiters(t, db, 2)
	require.NoError(t, barrier.Commit())
	wg.Wait()

	require.Equal(t, http.StatusNoContent, deleted.Status, string(deleted.Body))
	require.Equal(t, http.StatusNotFound, updated.Status, string(updated.Body))
	assertSubmodelIdentifierExistsInDatabase(t, db, dppID+"/submodels/DppMetadata", false)
	assertAASIdentifierExists(t, databasePort, dppID, false)
}

func executeDataset(t *testing.T, db interface {
	Exec(query string, args ...any) (sql.Result, error)
}, dataset *goqu.DeleteDataset) {
	t.Helper()
	query, args, err := dataset.Prepared(true).ToSQL()
	require.NoError(t, err)
	_, err = db.Exec(query, args...)
	require.NoError(t, err)
}
