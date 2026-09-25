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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/audit"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/evidence"
)

func TestAuditRecordArchivesToObjectLock(t *testing.T) {
	db := auditTestDatabase(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	store := archiveTestStore(ctx, t)
	repository := audit.Repository{DB: db, WORM: true, BacklogLimit: 1000000}
	receipt, err := repository.Record(ctx, auditStream(), auditEvent("archive"))
	if err != nil {
		t.Fatal(err)
	}
	archiver := audit.Archiver{DB: db, Store: store}
	for attempt := 0; attempt < 100; attempt++ {
		if _, err = archiver.ArchiveNext(ctx); err != nil {
			t.Fatal(err)
		}
		stored, ready := archivedReceipt(ctx, t, db, receipt.EventID)
		if ready {
			if err = store.VerifyArtifactRetention(ctx, stored.Reference, stored); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("created audit delivery was not archived")
}

func archiveTestStore(ctx context.Context, t *testing.T) *evidence.S3EvidenceStore {
	t.Helper()
	endpoint, access, secret := os.Getenv("TEST_S3_ENDPOINT"), os.Getenv("TEST_S3_ACCESS_KEY"), os.Getenv("TEST_S3_SECRET_KEY")
	if endpoint == "" || access == "" || secret == "" {
		t.Skip("TEST_S3_ENDPOINT, TEST_S3_ACCESS_KEY, and TEST_S3_SECRET_KEY are required")
	}
	bucket := fmt.Sprintf("basyx-audit-it-%d", time.Now().UnixNano())
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"), awsconfig.WithBaseEndpoint(endpoint), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(access, secret, "")))
	if err != nil {
		t.Fatal(err)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) { options.UsePathStyle = true })
	if _, err = client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket), ObjectLockEnabledForBucket: aws.Bool(true)}); err != nil {
		t.Fatal(err)
	}
	store, err := evidence.NewS3EvidenceStore(ctx, evidence.S3EvidenceStoreConfig{Bucket: bucket, Prefix: "audit", Region: "us-east-1", Endpoint: endpoint, AccessKeyID: access, SecretAccessKey: secret, UsePathStyle: true, RetentionMode: "compliance", RetentionDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func archivedReceipt(ctx context.Context, t *testing.T, db *sql.DB, eventID string) (evidence.Receipt, bool) {
	t.Helper()
	query, args, err := goqu.Dialect("postgres").From("audit_delivery").Select("archived_at", "receipt").Where(goqu.C("event_id").Eq(eventID)).Prepared(true).ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	var archivedAt sql.NullTime
	var raw []byte
	if err = db.QueryRowContext(ctx, query, args...).Scan(&archivedAt, &raw); err != nil {
		t.Fatal(err)
	}
	var stored evidence.Receipt
	if !archivedAt.Valid {
		return stored, false
	}
	if err = json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	return stored, true
}
