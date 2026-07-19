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

package abacpolicy

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"path"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
)

const abacPolicyArtifactVersion = "basyx-abac-policy-version-v1"

type abacPolicyEvidenceDocument struct {
	ArtifactVersion        string          `json:"artifact_version"`
	ArtifactType           string          `json:"artifact_type"`
	VersionID              int64           `json:"version_id"`
	ServiceScope           string          `json:"service_scope"`
	PolicyID               string          `json:"policy_id"`
	Status                 string          `json:"status"`
	SourceType             string          `json:"source_type"`
	SourceRef              string          `json:"source_ref,omitempty"`
	ConfiguredPolicyJSON   json.RawMessage `json:"configured_policy_json"`
	ConfiguredPolicyHash   string          `json:"configured_policy_hash"`
	RawPolicyHash          string          `json:"raw_policy_hash,omitempty"`
	MaterializedPolicyJSON json.RawMessage `json:"materialized_policy_json"`
	MaterializedPolicyHash string          `json:"materialized_policy_hash"`
	Rules                  []PolicyRule    `json:"rules"`
	Activation             auditActor      `json:"activation"`
	CreatedAt              time.Time       `json:"created_at"`
	ActivatedAt            *time.Time      `json:"activated_at,omitempty"`
	ActivatedBySubject     string          `json:"activated_by_subject,omitempty"`
	ActivatedByIssuer      string          `json:"activated_by_issuer,omitempty"`
	ActivatedByClientID    string          `json:"activated_by_client_id,omitempty"`
	GeneratedAt            time.Time       `json:"generated_at"`
}

func writeActivationEvidenceTx(ctx context.Context, tx *sql.Tx, version PolicyVersion, rules []PolicyRule, actor auditActor) ([]byte, error) {
	cfg := history.ActiveConfig()
	if !cfg.EvidenceEnabled {
		return nil, nil
	}
	if cfg.EvidenceStore == nil {
		return nil, common.NewInternalServerError("ABACPOLICY-EVIDENCE-NILSTORE evidence store is not initialized")
	}
	artifact, err := buildActivationEvidenceArtifact(version, rules, actor)
	if err != nil {
		return nil, err
	}
	writeCtx := ctx
	cancel := func() {}
	if cfg.EvidenceWriteTimeout > 0 {
		writeCtx, cancel = context.WithTimeout(ctx, cfg.EvidenceWriteTimeout)
	}
	defer cancel()
	receipt, err := cfg.EvidenceStore.PutArtifact(writeCtx, artifact)
	if err != nil {
		log.Printf("ABACPOLICY-EVIDENCE-PUT evidence store write failed: %v", err)
		return nil, common.NewErrServiceUnavailable("ABACPOLICY-EVIDENCE-STORE policy evidence could not be stored")
	}
	if receipt == nil {
		return nil, common.NewInternalServerError("ABACPOLICY-EVIDENCE-NILRECEIPT evidence store returned nil receipt")
	}
	if err = insertABACPolicyEvidenceCatalogTx(ctx, tx, version, *receipt); err != nil {
		return nil, err
	}
	receiptJSON, err := common.CanonicalJSON(receipt)
	if err != nil {
		return nil, common.NewInternalServerError("ABACPOLICY-EVIDENCE-RECEIPTJSON " + err.Error())
	}
	return receiptJSON, nil
}

func buildActivationEvidenceArtifact(version PolicyVersion, rules []PolicyRule, actor auditActor) (history.EvidenceArtifact, error) {
	doc := abacPolicyEvidenceDocument{
		ArtifactVersion:        abacPolicyArtifactVersion,
		ArtifactType:           history.EvidenceArtifactABACPolicy,
		VersionID:              version.VersionID,
		ServiceScope:           version.ServiceScope,
		PolicyID:               version.PolicyID,
		Status:                 version.Status,
		SourceType:             version.SourceType,
		SourceRef:              version.SourceRef,
		ConfiguredPolicyJSON:   version.ConfiguredPolicyJSON,
		ConfiguredPolicyHash:   version.ConfiguredPolicyHash,
		RawPolicyHash:          version.RawPolicyHash,
		MaterializedPolicyJSON: version.MaterializedPolicyJSON,
		MaterializedPolicyHash: version.MaterializedPolicyHash,
		Rules:                  rules,
		Activation:             actor,
		CreatedAt:              version.CreatedAt.UTC(),
		ActivatedAt:            utcTimePtr(version.ActivatedAt),
		ActivatedBySubject:     version.ActivatedBySubject,
		ActivatedByIssuer:      version.ActivatedByIssuer,
		ActivatedByClientID:    version.ActivatedByClientID,
		GeneratedAt:            time.Now().UTC(),
	}
	data, err := common.CanonicalJSON(doc)
	if err != nil {
		return history.EvidenceArtifact{}, common.NewInternalServerError("ABACPOLICY-EVIDENCE-CANONICAL " + err.Error())
	}
	return history.EvidenceArtifact{
		ArtifactType: history.EvidenceArtifactABACPolicy,
		ObjectKey:    abacPolicyObjectKey(version),
		ContentType:  "application/json",
		Data:         data,
		Metadata: map[string]string{
			"artifact_type": history.EvidenceArtifactABACPolicy,
			"history_table": "abac_policy_versions",
			"identifier":    version.PolicyID,
			"version_id":    fmt.Sprintf("%d", version.VersionID),
			"service_scope": version.ServiceScope,
		},
	}, nil
}

func abacPolicyObjectKey(version PolicyVersion) string {
	return path.Join(
		"abac-policy-versions",
		url.PathEscape(version.ServiceScope),
		fmt.Sprintf("%d-%s.json", version.VersionID, version.PolicyID),
	)
}

func insertABACPolicyEvidenceCatalogTx(ctx context.Context, tx *sql.Tx, version PolicyVersion, receipt history.EvidenceReceipt) error {
	metadataJSON, err := common.CanonicalJSON(receipt.Metadata)
	if err != nil {
		return common.NewInternalServerError("ABACPOLICY-EVIDENCE-METADATA " + err.Error())
	}
	row := goqu.Record{
		"manifest_id":       nil,
		"artifact_type":     history.EvidenceArtifactABACPolicy,
		"history_table":     "abac_policy_versions",
		"identifier":        version.PolicyID,
		"history_id":        version.VersionID,
		"row_hash":          nullableString(version.MaterializedPolicyHash),
		"content_hash":      nullableString(version.ConfiguredPolicyHash),
		"provider":          receipt.Reference.Provider,
		"bucket":            nullableString(receipt.Reference.Bucket),
		"object_key":        receipt.Reference.ObjectKey,
		"object_version_id": nullableString(receipt.Reference.VersionID),
		"sha256":            receipt.SHA256,
		"size_bytes":        receipt.SizeBytes,
		"content_type":      receipt.ContentType,
		"retention_mode":    nullableString(receipt.RetentionMode),
		"retain_until":      nullableEvidenceTime(receipt.RetainUntil),
		"legal_hold":        receipt.LegalHold,
		"artifact_metadata": jsonbParam(metadataJSON),
	}
	query, args, err := goqu.Insert(history.TableHistoryEvidenceArtifacts).Rows(row).ToSQL()
	if err != nil {
		return common.NewInternalServerError("ABACPOLICY-EVIDENCE-BUILDCATALOG " + err.Error())
	}
	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return common.NewInternalServerError("ABACPOLICY-EVIDENCE-INSERTCATALOG " + err.Error())
	}
	return nil
}

func nullableEvidenceTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func utcTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}
