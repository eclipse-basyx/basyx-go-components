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

package testenv

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // registers the postgres dialect
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/stretchr/testify/require"
)

var conditionalDialect = goqu.Dialect("postgres")

// HTTPResult is the outcome of an HTTP request in integration tests.
type HTTPResult struct {
	Status int
	Header http.Header
	Body   []byte
}

// DoHTTP sends a request with optional JSON body and headers.
func DoHTTP(t *testing.T, method string, endpoint string, body []byte, headers map[string]string) HTTPResult {
	t.Helper()
	var reader io.Reader
	contentType := ""
	if body != nil {
		reader = bytes.NewReader(body)
		contentType = "application/json"
	}
	return doRequest(t, method, endpoint, reader, contentType, headers)
}

// DoMultipartUpload sends content as the "file" part of a multipart PUT, as
// used by attachment and thumbnail uploads.
func DoMultipartUpload(t *testing.T, endpoint string, fileName string, content []byte, headers map[string]string) HTTPResult {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return doRequest(t, http.MethodPut, endpoint, body, writer.FormDataContentType(), headers)
}

func doRequest(t *testing.T, method string, endpoint string, body io.Reader, contentType string, headers map[string]string) HTTPResult {
	t.Helper()
	request, err := http.NewRequest(method, endpoint, body)
	require.NoError(t, err)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	// #nosec G107 G704 -- integration tests call local test endpoints.
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return HTTPResult{Status: response.StatusCode, Header: response.Header.Clone(), Body: responseBody}
}

// LockResourceRevision holds the revision row of a resource locked until the
// returned function is called, so concurrent writes queue at commit in a
// known order.
func LockResourceRevision(t *testing.T, db *sql.DB, kind string, identifier string) func() {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	query, args, err := conditionalDialect.From("resource_revision").Select(goqu.C("revision")).
		Where(goqu.Ex{"kind": kind, "identifier": identifier}).ForUpdate(exp.Wait).Prepared(true).ToSQL()
	require.NoError(t, err)
	var revision int64
	require.NoError(t, tx.QueryRow(query, args...).Scan(&revision), "resource %s %s has no revision row", kind, identifier)
	released := false
	return func() {
		if !released {
			released = true
			require.NoError(t, tx.Commit())
		}
	}
}

// WaitForLockWaiters waits until at least count database sessions wait for
// a lock.
func WaitForLockWaiters(t *testing.T, db *sql.DB, count int) {
	t.Helper()
	query, args, err := conditionalDialect.From("pg_stat_activity").Select(goqu.COUNT(goqu.Star())).
		Where(goqu.C("wait_event_type").Eq("Lock")).Prepared(true).ToSQL()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var waiting int
		if scanErr := db.QueryRow(query, args...).Scan(&waiting); scanErr != nil {
			return false
		}
		return waiting >= count
	}, 20*time.Second, 20*time.Millisecond, "expected %d sessions waiting for a lock", count)
}

// ResourceRevision returns the stored revision of a resource and whether a
// revision row exists.
func ResourceRevision(t *testing.T, db *sql.DB, kind string, identifier string) (int64, bool) {
	t.Helper()
	query, args, err := conditionalDialect.From("resource_revision").Select(goqu.C("revision")).
		Where(goqu.Ex{"kind": kind, "identifier": identifier}).Prepared(true).ToSQL()
	require.NoError(t, err)
	var revision int64
	err = db.QueryRow(query, args...).Scan(&revision)
	if err == sql.ErrNoRows {
		return 0, false
	}
	require.NoError(t, err)
	return revision, true
}

// DeleteResourceRevision removes the revision row of a resource, as for
// resources that existed before revisions were introduced.
func DeleteResourceRevision(t *testing.T, db *sql.DB, kind string, identifier string) {
	t.Helper()
	query, args, err := conditionalDialect.Delete("resource_revision").
		Where(goqu.Ex{"kind": kind, "identifier": identifier}).Prepared(true).ToSQL()
	require.NoError(t, err)
	_, err = db.Exec(query, args...)
	require.NoError(t, err)
}

// CountRows counts the rows of a table matching where.
func CountRows(t *testing.T, db *sql.DB, table string, where goqu.Ex) int {
	t.Helper()
	query, args, err := conditionalDialect.From(table).Select(goqu.COUNT(goqu.Star())).Where(where).Prepared(true).ToSQL()
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRow(query, args...).Scan(&count))
	return count
}

// ConditionalLifecycle describes a resource for RunConditionalLifecycle.
type ConditionalLifecycle struct {
	// ResourceURL addresses the resource.
	ResourceURL string
	// Create creates the resource and returns the response.
	Create func(t *testing.T) HTTPResult
	// CreatedStatus is the expected status of Create.
	CreatedStatus int
	// UpdateMethod and UpdateBody replace or change the resource.
	UpdateMethod string
	UpdateBody   []byte
	// UpdatedStatus is the expected status of a successful update.
	UpdatedStatus int
	// DeletedStatus is the expected status of a successful delete.
	DeletedStatus int
	// Headers are sent with every request, for example authorization.
	Headers map[string]string
}

const (
	staleETag                = `"1-00000000"`
	representationETagRegexp = `^"\d+-[0-9a-f]{8}(-[0-9a-f]{16})?"$`
)

// RunConditionalLifecycle checks the conditional request contract of a
// resource: ETags on GET, 304, 412 for stale or create-only writes, new
// ETags after writes, conditional delete and a new ETag after recreation.
func RunConditionalLifecycle(t *testing.T, spec ConditionalLifecycle) {
	t.Helper()
	created := spec.Create(t)
	require.Equal(t, spec.CreatedStatus, created.Status, string(created.Body))
	t.Cleanup(func() { DoHTTP(t, http.MethodDelete, spec.ResourceURL, nil, spec.Headers) })

	first := spec.requireETag(t)
	require.Equal(t, http.StatusNotModified, spec.do(t, http.MethodGet, nil, map[string]string{"If-None-Match": first}).Status)
	require.Equal(t, http.StatusPreconditionFailed, spec.do(t, http.MethodGet, nil, map[string]string{"If-Match": staleETag}).Status)

	stale := spec.do(t, spec.UpdateMethod, spec.UpdateBody, map[string]string{"If-Match": staleETag})
	require.Equal(t, http.StatusPreconditionFailed, stale.Status, string(stale.Body))
	require.Equal(t, first, spec.requireETag(t), "a failed precondition must not change the resource")
	if spec.UpdateMethod == http.MethodPut {
		createOnly := spec.do(t, http.MethodPut, spec.UpdateBody, map[string]string{"If-None-Match": "*"})
		require.Equal(t, http.StatusPreconditionFailed, createOnly.Status, string(createOnly.Body))
	}

	updated := spec.do(t, spec.UpdateMethod, spec.UpdateBody, map[string]string{"If-Match": first})
	require.Equal(t, spec.UpdatedStatus, updated.Status, string(updated.Body))
	second := spec.requireETag(t)
	require.NotEqual(t, first, second)

	require.Equal(t, http.StatusPreconditionFailed, spec.do(t, http.MethodDelete, nil, map[string]string{"If-Match": first}).Status)
	deleted := spec.do(t, http.MethodDelete, nil, map[string]string{"If-Match": second})
	require.Equal(t, spec.DeletedStatus, deleted.Status, string(deleted.Body))
	require.Equal(t, http.StatusNotFound, spec.do(t, http.MethodGet, nil, nil).Status)

	recreated := spec.Create(t)
	require.Equal(t, spec.CreatedStatus, recreated.Status, string(recreated.Body))
	require.Equal(t, http.StatusPreconditionFailed, spec.do(t, spec.UpdateMethod, spec.UpdateBody, map[string]string{"If-Match": second}).Status,
		"an ETag of a deleted resource must not match its recreation")
}

func (spec ConditionalLifecycle) do(t *testing.T, method string, body []byte, headers map[string]string) HTTPResult {
	t.Helper()
	merged := map[string]string{}
	for key, value := range spec.Headers {
		merged[key] = value
	}
	for key, value := range headers {
		merged[key] = value
	}
	return DoHTTP(t, method, spec.ResourceURL, body, merged)
}

func (spec ConditionalLifecycle) requireETag(t *testing.T) string {
	t.Helper()
	response := spec.do(t, http.MethodGet, nil, nil)
	require.Equal(t, http.StatusOK, response.Status, string(response.Body))
	etag := response.Header.Get("ETag")
	require.Regexp(t, representationETagRegexp, etag)
	return etag
}

// MaxResourceRevision returns the highest revision of a resource kind.
func MaxResourceRevision(t *testing.T, db *sql.DB, kind string) int64 {
	t.Helper()
	query, args, err := conditionalDialect.From("resource_revision").Select(goqu.COALESCE(goqu.MAX("revision"), 0)).
		Where(goqu.Ex{"kind": kind}).Prepared(true).ToSQL()
	require.NoError(t, err)
	var revision int64
	require.NoError(t, db.QueryRow(query, args...).Scan(&revision))
	return revision
}
