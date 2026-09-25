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

package audit

import (
	"bytes"
	"context"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/evidence"
	commonjws "github.com/eclipse-basyx/basyx-go-components/internal/common/jws"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	jose "gopkg.in/go-jose/go-jose.v2"
)

// ArchiveVerification distinguishes intact database evidence from confirmed WORM protection.
type ArchiveVerification struct {
	Verification
	Archived, Pending, Unprotected int64
	WORMEnabled                    bool
	Failure                        string `json:",omitempty"`
}

// VerifyArchive checks the catalog chain and every archived immutable object version.
// The configured public-key bundle retains trusted historical signing keys.
func (runtime *Runtime) VerifyArchive(ctx context.Context, stream string) (ArchiveVerification, error) {
	ctx, span := otel.Tracer("basyx/audit").Start(ctx, "audit.verify")
	defer span.End()
	result, err := runtime.verifyArchive(ctx, stream)
	outcome := "valid"
	if err != nil || !result.Valid {
		outcome = "failed"
	}
	if counter, metricErr := otel.Meter("basyx/audit").Int64Counter("basyx.audit.verifications"); metricErr == nil {
		counter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
	}
	return result, err
}

func (runtime *Runtime) verifyArchive(ctx context.Context, stream string) (ArchiveVerification, error) {
	chain, err := runtime.Repository.Verify(ctx, stream)
	result := ArchiveVerification{Verification: chain, WORMEnabled: runtime.Repository.WORM}
	if err != nil || !chain.Valid {
		return result, err
	}
	query, args, err := dialect.From("audit_record").LeftJoin(goqu.T("audit_delivery"), goqu.On(goqu.I("audit_record.event_id").Eq(goqu.I("audit_delivery.event_id")))).Select(goqu.I("audit_record.event_id"), "sequence", "content_hash", goqu.I("audit_delivery.event_id"), "archived_at", "receipt").Where(goqu.C("stream").Eq(stream), goqu.C("sequence").Lte(chain.Records)).Order(goqu.C("sequence").Asc()).Prepared(true).ToSQL()
	if err != nil {
		return result, fmt.Errorf("AUDIT-ARCHIVEVERIFY-SQL: %w", err)
	}
	rows, err := runtime.Repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return result, fmt.Errorf("AUDIT-ARCHIVEVERIFY-QUERY: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		row, err := scanArchiveVerificationRow(rows)
		if err != nil {
			return result, err
		}
		if err = runtime.verifyCatalogObject(ctx, stream, row, &result); err != nil {
			result.Valid = false
			result.FailedSequence = row.sequence
			result.Failure = err.Error()
			return result, nil
		}
	}
	if err = rows.Err(); err != nil {
		return result, fmt.Errorf("AUDIT-ARCHIVEVERIFY-ROWS: %w", err)
	}
	if result.Archived+result.Pending+result.Unprotected != chain.Records {
		result.Valid = false
		result.Failure = "AUDIT-ARCHIVEVERIFY-GAP catalog changed during verification"
	}
	return result, nil
}

type archiveVerificationRow struct {
	eventID, hash string
	sequence      int64
	deliveryID    sql.NullString
	archived      sql.NullTime
	receipt       []byte
}

func scanArchiveVerificationRow(rows *sql.Rows) (archiveVerificationRow, error) {
	var row archiveVerificationRow
	err := rows.Scan(&row.eventID, &row.sequence, &row.hash, &row.deliveryID, &row.archived, &row.receipt)
	if err != nil {
		return row, fmt.Errorf("AUDIT-ARCHIVEVERIFY-SCAN: %w", err)
	}
	return row, nil
}

func (runtime *Runtime) verifyCatalogObject(ctx context.Context, stream string, row archiveVerificationRow, result *ArchiveVerification) error {
	if !row.deliveryID.Valid {
		result.Unprotected++
		if runtime.Repository.WORM {
			return fmt.Errorf("AUDIT-ARCHIVEVERIFY-DELIVERY missing delivery record")
		}
		return nil
	}
	if !row.archived.Valid {
		result.Pending++
		return nil
	}
	if runtime.archiver == nil || runtime.archiver.Store == nil {
		return fmt.Errorf("AUDIT-ARCHIVEVERIFY-STORE evidence store is unavailable")
	}
	var receipt evidence.Receipt
	if err := json.Unmarshal(row.receipt, &receipt); err != nil {
		return fmt.Errorf("AUDIT-ARCHIVEVERIFY-RECEIPT invalid receipt")
	}
	if receipt.Reference.VersionID == "" {
		return fmt.Errorf("AUDIT-ARCHIVEVERIFY-VERSION missing immutable object version")
	}
	object, err := runtime.archiver.Store.GetArtifact(ctx, receipt.Reference)
	if err != nil || object == nil {
		return fmt.Errorf("AUDIT-ARCHIVEVERIFY-OBJECT archived object unavailable")
	}
	if evidence.SHA256Hex(object.Data) != receipt.SHA256 {
		return fmt.Errorf("AUDIT-ARCHIVEVERIFY-HASH archived object hash mismatch")
	}
	retention, ok := runtime.archiver.Store.(evidence.RetentionVerifier)
	if !ok {
		return fmt.Errorf("AUDIT-ARCHIVEVERIFY-RETENTION retention verifier unavailable")
	}
	if err = retention.VerifyArtifactRetention(ctx, receipt.Reference, receipt); err != nil {
		return fmt.Errorf("AUDIT-ARCHIVEVERIFY-RETENTION object retention verification failed")
	}
	if err = runtime.verifyEnvelope(ctx, object.Data, stream, row); err != nil {
		return err
	}
	result.Archived++
	return nil
}

func (runtime *Runtime) verifyEnvelope(ctx context.Context, data []byte, stream string, row archiveVerificationRow) error {
	payload, err := runtime.verifiedPayload(ctx, data)
	if err != nil {
		return err
	}
	var record archiveRecord
	if err = json.Unmarshal(payload, &record); err != nil {
		return fmt.Errorf("AUDIT-ARCHIVEVERIFY-ENVELOPE invalid event envelope")
	}
	if record.Stream != stream || record.EventID != row.eventID || record.Sequence != row.sequence || record.ContentHash != row.hash {
		return fmt.Errorf("AUDIT-ARCHIVEVERIFY-BINDING archived evidence does not match catalog")
	}
	var event Event
	if err = json.Unmarshal(record.Event, &event); err != nil {
		return fmt.Errorf("AUDIT-ARCHIVEVERIFY-EVENT invalid event")
	}
	hash, _, err := hashEvent(record.PreviousHash, record.Sequence, event)
	if err != nil || hash != row.hash {
		return fmt.Errorf("AUDIT-ARCHIVEVERIFY-EVENTHASH archived event does not match evidence chain")
	}
	return nil
}

func (runtime *Runtime) verifiedPayload(ctx context.Context, data []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var envelope struct {
		Event     json.RawMessage `json:"event"`
		Signature []byte          `json:"signature"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("AUDIT-ARCHIVEVERIFY-JSON invalid archived JSON")
	}
	if len(envelope.Signature) == 0 {
		if runtime.signingRequired {
			return nil, fmt.Errorf("AUDIT-ARCHIVEVERIFY-UNSIGNED signed evidence required")
		}
		return data, nil
	}
	if runtime.verificationKey == nil {
		return nil, fmt.Errorf("AUDIT-ARCHIVEVERIFY-KEY no trusted verification key for signed evidence")
	}
	signed, err := jose.ParseSigned(string(envelope.Signature))
	if err != nil || len(signed.Signatures) != 1 || signed.Signatures[0].Header.Algorithm != string(jose.RS256) {
		return nil, fmt.Errorf("AUDIT-ARCHIVEVERIFY-SIGNATURE invalid signature format")
	}
	key := runtime.verificationKey
	if runtime.verificationKeys != nil {
		key = runtime.verificationKeys[signed.Signatures[0].Header.KeyID]
		if key == nil {
			return nil, fmt.Errorf("AUDIT-ARCHIVEVERIFY-KEY unknown signing key identifier")
		}
	}
	payload, err := signed.Verify(key)
	if err != nil || !bytes.Equal(payload, envelope.Event) {
		return nil, fmt.Errorf("AUDIT-ARCHIVEVERIFY-SIGNATURE signature verification failed")
	}
	return payload, nil
}

func configuredVerificationKey(cfg RuntimeConfig, signer ManifestSigner) (*rsa.PublicKey, error) {
	if cfg.SigningPublicKeyPath != "" {
		key, err := commonjws.LoadPublicKey(cfg.SigningPublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("AUDIT-ARCHIVEVERIFY-PUBLICKEY: %w", err)
		}
		return key, nil
	}
	if signing, ok := signer.(rsaManifestSigner); ok {
		return &signing.key.PublicKey, nil
	}
	return nil, nil
}
