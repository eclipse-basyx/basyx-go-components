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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/evidence"
)

func TestS3ObjectLockEnforcesComplianceRetention(t *testing.T) {
	endpoint, accessKey, secretKey, bucketPrefix := objectLockEnvironment(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	bucket := fmt.Sprintf("%s-%d", bucketPrefix, time.Now().UnixNano())
	client := objectLockClient(ctx, t, endpoint, accessKey, secretKey)
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket), ObjectLockEnabledForBucket: aws.Bool(true)})
	if err != nil {
		t.Fatalf("create object-lock bucket: %v", err)
	}
	_, err = client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{Bucket: aws.String(bucket), VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled}})
	if err != nil {
		t.Fatalf("enable bucket versioning: %v", err)
	}
	store, err := evidence.NewS3EvidenceStore(ctx, evidence.S3EvidenceStoreConfig{Bucket: bucket, Prefix: "integration", Region: "us-east-1", Endpoint: endpoint, AccessKeyID: accessKey, SecretAccessKey: secretKey, UsePathStyle: true, RetentionMode: "compliance", RetentionDays: 1})
	if err != nil {
		t.Fatalf("create evidence store: %v", err)
	}
	first, err := store.PutArtifact(ctx, evidence.Artifact{ArtifactType: evidence.ArtifactHistoryEvent, ObjectKey: "event.json", ContentType: "application/json", Data: []byte(`{"version":1}`)})
	if err != nil {
		t.Fatalf("put locked artifact: %v", err)
	}
	if err = store.VerifyArtifactRetention(ctx, first.Reference, *first); err != nil {
		t.Fatalf("verify compliance retention: %v", err)
	}
	if _, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(first.Reference.ObjectKey), VersionId: aws.String(first.Reference.VersionID)}); err == nil {
		t.Fatal("Object Lock allowed deletion of a compliance-retained version")
	}
	second, err := store.PutArtifact(ctx, evidence.Artifact{ArtifactType: evidence.ArtifactHistoryEvent, ObjectKey: "event.json", ContentType: "application/json", Data: []byte(`{"version":2}`)})
	if err != nil {
		t.Fatalf("put second version: %v", err)
	}
	if first.Reference.VersionID == second.Reference.VersionID {
		t.Fatal("expected a distinct S3 version")
	}
	if _, err = store.VerifyArtifact(ctx, first.Reference, first.SHA256); err != nil {
		t.Fatalf("verify first immutable version after second write: %v", err)
	}
	badRetention := *first
	longerRetention := time.Now().Add(48 * time.Hour)
	badRetention.RetainUntil = &longerRetention
	if err = store.VerifyArtifactRetention(ctx, first.Reference, badRetention); err == nil {
		t.Fatal("retention verification accepted insufficient storage retention")
	}
}

func objectLockEnvironment(t *testing.T) (string, string, string, string) {
	t.Helper()
	endpoint, accessKey, secretKey := os.Getenv("TEST_S3_ENDPOINT"), os.Getenv("TEST_S3_ACCESS_KEY"), os.Getenv("TEST_S3_SECRET_KEY")
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Skip("TEST_S3_ENDPOINT, TEST_S3_ACCESS_KEY, and TEST_S3_SECRET_KEY are required")
	}
	prefix := os.Getenv("TEST_S3_BUCKET")
	if prefix == "" {
		prefix = "basyx-evidence-it"
	}
	return endpoint, accessKey, secretKey, strings.ToLower(prefix)
}

func objectLockClient(ctx context.Context, t *testing.T, endpoint string, accessKey string, secretKey string) *s3.Client {
	t.Helper()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"), awsconfig.WithBaseEndpoint(endpoint), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")))
	if err != nil {
		t.Fatalf("load S3 client configuration: %v", err)
	}
	return s3.NewFromConfig(awsCfg, func(options *s3.Options) { options.UsePathStyle = true })
}
