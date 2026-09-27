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

package rebacintegration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/rebac"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	"github.com/stretchr/testify/require"
)

// asyncCall sends a request from another goroutine, so a test can hold
// database locks meanwhile. It never calls t, which is not goroutine-safe.
func asyncCall(bearer string, method string, url string, body io.Reader, contentType string, headers map[string]string) <-chan response {
	return asyncCallAfter(nil, bearer, method, url, body, contentType, headers)
}

// asyncCallAfter sends the request once start is closed; a nil start sends
// it right away.
func asyncCallAfter(start <-chan struct{}, bearer string, method string, url string, body io.Reader, contentType string, headers map[string]string) <-chan response {
	done := make(chan response, 1)
	go func() {
		if start != nil {
			<-start
		}
		result := response{status: -1}
		defer func() { done <- result }()
		request, err := http.NewRequestWithContext(context.Background(), method, url, body)
		if err != nil {
			return
		}
		request.Header.Set("Authorization", "Bearer "+bearer)
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		reply, err := testenv.HTTPClient().Do(request)
		if err != nil {
			return
		}
		defer func() { _ = reply.Body.Close() }()
		payload, _ := io.ReadAll(reply.Body)
		result = response{status: reply.StatusCode, body: payload, header: reply.Header}
	}()
	return done
}

func jsonBody(t *testing.T, value any) io.Reader {
	t.Helper()
	payload, err := json.Marshal(value)
	require.NoError(t, err)
	return bytes.NewReader(payload)
}

// awaitLockWaiter waits until a session waits for a row lock of
// rebac_object_revision or until the request completed without waiting.
func awaitLockWaiter(t *testing.T, db *sql.DB, done <-chan response) (response, bool) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("pg_stat_activity").Select(goqu.COUNT("*")).Where(
		goqu.C("wait_event_type").Eq("Lock"),
		goqu.L("query ILIKE ?", "%rebac_object_revision%"),
	).ToSQL()
	require.NoError(t, err)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case result := <-done:
			return result, true
		default:
		}
		var waiting int
		require.NoError(t, db.QueryRowContext(t.Context(), query, args...).Scan(&waiting))
		if waiting > 0 {
			return response{}, false
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the request neither waited for the access lock nor completed")
	return response{}, false
}

// revokeWhileLocked locks the access revision of a Submodel, removes the
// grants of user in the same transaction and returns the open transaction.
func revokeWhileLocked(t *testing.T, db *sql.DB, identifier string, user string) *sql.Tx {
	t.Helper()
	authUUID, found, err := rebac.LookupAuthUUID(t.Context(), db, rebac.KindSubmodel, identifier)
	require.NoError(t, err)
	require.True(t, found)
	objectKey := rebac.ResourceKey(rebac.TypeSubmodel, authUUID)
	dialect := goqu.Dialect("postgres")
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	lock, args, err := dialect.From("rebac_object_revision").Select("revision").
		Where(goqu.C("object_key").Eq(objectKey)).ForUpdate(goqu.Wait).ToSQL()
	require.NoError(t, err)
	var revision int64
	require.NoError(t, tx.QueryRowContext(t.Context(), lock, args...).Scan(&revision))
	revoke, args, err := dialect.Delete("rebac_grant").Where(
		goqu.C("object_key").Eq(objectKey), goqu.C("subject_key").Eq(rebac.UserKey(issuer, subject(t, user))),
	).ToSQL()
	require.NoError(t, err)
	_, err = tx.ExecContext(t.Context(), revoke, args...)
	require.NoError(t, err)
	return tx
}

func TestRevokedManagersCannotCompleteStaleWrites(t *testing.T) {
	db := openDB(t)
	identifier := createSubmodel(t, submodelURL, "alice", "stale-manager")
	accessURL := submodelAccess(submodelURL, identifier)
	expiresAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	for name, send := range map[string]func() <-chan response{
		"grants with If-Match *": func() <-chan response {
			body := jsonBody(t, map[string]any{"grants": []grant{userGrant(t, "owner", "bob"), userGrant(t, "viewer", "eve")}})
			return asyncCall(token(t, "bob"), http.MethodPut, accessURL+"/grants", body, "application/json", map[string]string{"If-Match": "*"})
		},
		"invitation": func() <-chan response {
			body := jsonBody(t, map[string]any{"relation": "viewer", "expiresAt": expiresAt})
			return asyncCall(token(t, "bob"), http.MethodPost, accessURL+"/invitations", body, "application/json", nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			setGrants(t, "alice", accessURL, []grant{userGrant(t, "owner", "alice"), userGrant(t, "owner", "bob")})
			tx := revokeWhileLocked(t, db, identifier, "bob")
			done := send()
			result, completed := awaitLockWaiter(t, db, done)
			require.NoError(t, tx.Commit())
			if !completed {
				result = <-done
			}
			expectStatus(t, http.StatusNotFound, result, "a manager revoked while the change waited must not complete it")
			grants, _ := readAccess(t, "alice", accessURL)
			require.Equal(t, []grant{userGrant(t, "owner", "alice")}, grants)
			invitations := call(t, "alice", http.MethodGet, accessURL+"/invitations", nil, nil)
			require.JSONEq(t, `{"invitations":[]}`, string(invitations.body))
		})
	}
}

// stalledUpload starts a multipart upload whose body only completes when
// release is called, so the access decision is taken long before the
// package is persisted.
func stalledUpload(bearer string, method string, url string, fileName string, content []byte) (<-chan response, func()) {
	reader, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	release := make(chan struct{})
	go func() {
		part, err := form.CreateFormFile("file", fileName)
		if err == nil {
			<-release
			_, err = part.Write(content)
		}
		if err == nil {
			err = form.Close()
		}
		_ = writer.CloseWithError(err)
	}()
	return asyncCall(bearer, method, url, reader, form.FormDataContentType(), nil), func() { close(release) }
}

func TestCreatingPackageUploadsNeverOverwriteAnotherPackage(t *testing.T) {
	bootstrapCreators(t)
	content, err := os.ReadFile(samplePackage)
	require.NoError(t, err)
	packageURL := packagesURL + "/packages/" + enc(strings.ReplaceAll(unique("race"), ":", "-"))

	done, release := stalledUpload(token(t, "carol"), http.MethodPut, packageURL, "carol.aasx", content)
	time.Sleep(500 * time.Millisecond)
	expectStatus(t, http.StatusCreated, uploadFile(t, "alice", http.MethodPut, packageURL, "alice.aasx", content), "alice creates the package meanwhile")
	release()
	result := <-done
	expectStatus(t, http.StatusForbidden, result, "carol's creation right must not overwrite alice's package")

	stored := call(t, "alice", http.MethodGet, packageURL, nil, nil)
	expectStatus(t, http.StatusOK, stored, "alice still reads her package")
	require.Equal(t, "alice.aasx", stored.header.Get("X-FileName"), "the package keeps alice's content")
}

func TestAccessETagsAreBoundToTheirObject(t *testing.T) {
	first := createSubmodel(t, submodelURL, "alice", "etag-first")
	second := createSubmodel(t, submodelURL, "alice", "etag-second")
	_, firstTag := readAccess(t, "alice", submodelAccess(submodelURL, first))
	_, secondTag := readAccess(t, "alice", submodelAccess(submodelURL, second))
	require.NotEqual(t, firstTag, secondTag, "equal revisions of different objects have different ETags")
	body := map[string]any{"grants": []grant{userGrant(t, "owner", "alice"), userGrant(t, "viewer", "bob")}}
	stale := call(t, "alice", http.MethodPut, submodelAccess(submodelURL, second)+"/grants", body, map[string]string{"If-Match": firstTag})
	expectStatus(t, http.StatusPreconditionFailed, stale, "the ETag of another object is rejected")
}

func TestAcceptingAndRevokingAnInvitationConcurrentlyNeverFails(t *testing.T) {
	identifier := createSubmodel(t, submodelURL, "alice", "invitation-race")
	accessURL := submodelAccess(submodelURL, identifier)
	aliceBearer, bobBearer := token(t, "alice"), token(t, "bob")
	expiresAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	for attempt := 0; attempt < 30; attempt++ {
		created := call(t, "alice", http.MethodPost, accessURL+"/invitations", map[string]any{"relation": "viewer", "expiresAt": expiresAt}, nil)
		expectStatus(t, http.StatusCreated, created, "create invitation")
		invitation := created.json(t)
		start := make(chan struct{})
		accepted := asyncCallAfter(start, bobBearer, http.MethodPost, submodelURL+"/security/rebac/invitations/accept",
			jsonBody(t, map[string]any{"token": invitation["token"]}), "application/json", nil)
		revoked := asyncCallAfter(start, aliceBearer, http.MethodDelete, accessURL+"/invitations/"+invitation["id"].(string), nil, "", nil)
		close(start)
		acceptance, revocation := <-accepted, <-revoked
		require.Contains(t, []int{http.StatusOK, http.StatusNotFound}, acceptance.status, "attempt %d: accept: %s", attempt, acceptance.body)
		require.Equal(t, http.StatusNoContent, revocation.status, "attempt %d: revoke: %s", attempt, revocation.body)
	}
}

func TestChangingLinksWhileRemovingTheReferenceNeverFails(t *testing.T) {
	bootstrapCreators(t)
	aliceBearer := token(t, "alice")
	for attempt := 0; attempt < 20; attempt++ {
		submodelID := createSubmodel(t, environmentURL, "alice", "link-race")
		shellID := unique("aas")
		expectStatus(t, http.StatusCreated, call(t, "alice", http.MethodPost, environmentURL+"/shells", shell(shellID, submodelID), nil), "create shell")
		accessURL := submodelAccess(environmentURL, submodelID)
		_, etag := readAccess(t, "alice", accessURL)
		approved := call(t, "alice", http.MethodPut, accessURL+"/inheritance", map[string]any{"aasIds": []string{shellID}}, map[string]string{"If-Match": etag})
		expectStatus(t, http.StatusOK, approved, "approve link")

		start := make(chan struct{})
		unlinked := asyncCallAfter(start, aliceBearer, http.MethodPut, accessURL+"/inheritance",
			jsonBody(t, map[string]any{"aasIds": []string{}}), "application/json", map[string]string{"If-Match": approved.header.Get("ETag")})
		dereferenced := asyncCallAfter(start, aliceBearer, http.MethodDelete,
			environmentURL+"/shells/"+enc(shellID)+"/submodel-refs/"+enc(submodelID), nil, "", nil)
		close(start)
		links, reference := <-unlinked, <-dereferenced
		require.Contains(t, []int{http.StatusOK, http.StatusPreconditionFailed}, links.status, "attempt %d: links: %s", attempt, links.body)
		require.Equal(t, http.StatusNoContent, reference.status, "attempt %d: reference: %s", attempt, reference.body)
	}
}
