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

package historyevidenceverifier

import (
	"context"
	"fmt"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
)

func newS3EvidenceStore(ctx context.Context, cfg *common.Config) (*history.S3EvidenceStore, error) {
	if strings.ToLower(strings.TrimSpace(cfg.History.Evidence.Provider)) != history.EvidenceProviderS3 {
		return nil, fmt.Errorf("HISTORY-EVIDENCE-CLI-PROVIDER history.evidence.provider must be s3")
	}
	return history.NewS3EvidenceStore(ctx, history.S3EvidenceStoreConfig{
		Bucket:          cfg.History.Evidence.Bucket,
		Prefix:          cfg.History.Evidence.Prefix,
		Region:          cfg.History.Evidence.Region,
		Endpoint:        cfg.History.Evidence.Endpoint,
		AccessKeyID:     cfg.History.Evidence.AccessKeyID,
		SecretAccessKey: cfg.History.Evidence.SecretAccessKey,
		UsePathStyle:    cfg.History.Evidence.UsePathStyle,
		RetentionMode:   cfg.History.Evidence.RetentionMode,
		RetentionDays:   cfg.History.Evidence.RetentionDays,
	})
}

func optionalS3EvidenceStore(ctx context.Context, cfg *common.Config) (*history.S3EvidenceStore, error) {
	if strings.ToLower(strings.TrimSpace(cfg.History.Evidence.Provider)) != history.EvidenceProviderS3 {
		return nil, nil
	}
	if strings.TrimSpace(cfg.History.Evidence.Bucket) == "" {
		return nil, nil
	}
	return newS3EvidenceStore(ctx, cfg)
}

func newManifestSigner(cfg *common.Config, keyID string) (*history.ManifestJWSSigner, error) {
	keyPath := strings.TrimSpace(cfg.History.Evidence.Signing.PrivateKeyPath)
	if keyPath == "" {
		keyPath = strings.TrimSpace(cfg.JWS.PrivateKeyPath)
	}
	if keyPath == "" {
		return nil, nil
	}
	return history.NewManifestJWSSignerFromKeyFile(keyPath, keyID)
}

func newManifestVerifier(cfg *common.Config) (*history.ManifestJWSVerifier, error) {
	keyPath := strings.TrimSpace(cfg.History.Evidence.Signing.PublicKeyPath)
	if keyPath == "" {
		return nil, nil
	}
	return history.NewManifestJWSVerifierFromKeyFile(keyPath)
}
