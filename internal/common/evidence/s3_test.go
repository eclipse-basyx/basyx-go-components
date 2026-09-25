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

package evidence

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
)

func TestS3StoreReceiptAppliesPrefixAndRetention(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	store := &S3EvidenceStore{cfg: S3EvidenceStoreConfig{Bucket: "evidence", Prefix: "tenant-a", RetentionMode: "governance", RetentionDays: 7}, now: func() time.Time { return now }}

	key := store.objectKey("manifests/one.json")
	receipt := store.receiptForArtifact(key, Artifact{ArtifactType: ArtifactManifest, ContentType: "application/json", Data: []byte(`{"ok":true}`)})

	require.Equal(t, "tenant-a/manifests/one.json", receipt.Reference.ObjectKey)
	require.Equal(t, "governance", receipt.RetentionMode)
	require.Equal(t, now.AddDate(0, 0, 7), *receipt.RetainUntil)
	require.Equal(t, SHA256Hex([]byte(`{"ok":true}`)), receipt.SHA256)
}

func TestS3StoreRejectsWritesWithoutRetention(t *testing.T) {
	store := &S3EvidenceStore{client: &s3.Client{}, cfg: S3EvidenceStoreConfig{Bucket: "evidence", Region: "us-east-1"}, now: time.Now}

	_, err := store.PutArtifact(t.Context(), Artifact{ArtifactType: ArtifactHistoryEvent, ObjectKey: "events/one.json", ContentType: "application/json", Data: []byte(`{}`)})

	require.ErrorContains(t, err, "HISTORY-EVIDENCE-S3-RETENTION")
}

func TestS3StoreRejectsInvalidConfiguration(t *testing.T) {
	_, err := normalizeS3EvidenceStoreConfig(S3EvidenceStoreConfig{Bucket: " evidence ", Region: "us-east-1", RetentionMode: "invalid"})
	require.ErrorContains(t, err, "HISTORY-EVIDENCE-S3-RETENTIONMODE")

	normalized, err := normalizeS3EvidenceStoreConfig(S3EvidenceStoreConfig{Bucket: " evidence ", Prefix: "/audit/", Region: " us-east-1 ", RetentionMode: " GOVERNANCE "})
	require.NoError(t, err)
	require.Equal(t, "evidence", normalized.Bucket)
	require.Equal(t, "audit", normalized.Prefix)
	require.Equal(t, "governance", normalized.RetentionMode)
	require.False(t, strings.Contains(normalized.Prefix, "/"))
}

func TestS3StoreRequiresVersionIDAfterWrite(t *testing.T) {
	client := s3.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""), HTTPClient: httpClientFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})})
	store := &S3EvidenceStore{client: client, cfg: S3EvidenceStoreConfig{Bucket: "evidence", RetentionMode: "governance", RetentionDays: 7}, now: time.Now}

	receipt, err := store.PutArtifact(t.Context(), Artifact{ArtifactType: ArtifactHistoryEvent, ObjectKey: "events/one.json", ContentType: "application/json", Data: []byte(`{}`)})

	require.Nil(t, receipt)
	require.ErrorContains(t, err, "HISTORY-EVIDENCE-S3-VERSIONID")
}

func TestS3StoreVerifiesRetentionState(t *testing.T) {
	retainUntil := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	client := s3.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""), HTTPClient: httpClientFunc(func(request *http.Request) (*http.Response, error) {
		body := `<LegalHold><Status>OFF</Status></LegalHold>`
		if strings.Contains(request.URL.RawQuery, "retention") {
			body = `<Retention><Mode>GOVERNANCE</Mode><RetainUntilDate>2026-06-12T12:00:00Z</RetainUntilDate></Retention>`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	store := &S3EvidenceStore{client: client, cfg: S3EvidenceStoreConfig{Bucket: "evidence"}}

	err := store.VerifyArtifactRetention(t.Context(), Reference{Provider: ProviderS3, Bucket: "evidence", ObjectKey: "history-events/aas_history/aas-1/1-row.json", VersionID: "version-1"}, Receipt{RetentionMode: "governance", RetainUntil: &retainUntil})

	require.NoError(t, err)
}

type httpClientFunc func(*http.Request) (*http.Response, error)

func (fn httpClientFunc) Do(request *http.Request) (*http.Response, error) { return fn(request) }

func TestValidateS3BucketRequiresVersioningAndObjectLock(t *testing.T) {
	for _, test := range []struct {
		name, versioning, lock string
		valid                  bool
	}{
		{"protected", "Enabled", "Enabled", true},
		{"suspended", "Suspended", "Enabled", false},
		{"unlocked", "Enabled", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := s3.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""), HTTPClient: httpClientFunc(func(request *http.Request) (*http.Response, error) {
				body := ""
				if request.URL.Query().Has("versioning") {
					body = "<VersioningConfiguration><Status>" + test.versioning + "</Status></VersioningConfiguration>"
				}
				if request.URL.Query().Has("object-lock") {
					body = "<ObjectLockConfiguration><ObjectLockEnabled>" + test.lock + "</ObjectLockEnabled></ObjectLockConfiguration>"
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})})
			store := &S3EvidenceStore{client: client, cfg: S3EvidenceStoreConfig{Bucket: "evidence", RetentionMode: "compliance", RetentionDays: 1}}
			err := store.Validate(t.Context())
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
