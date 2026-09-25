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
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/evidence"
	"github.com/stretchr/testify/require"
)

func TestArchiveVerificationRejectsTamperingAndWrongKeys(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer := rsaManifestSigner{key: key, keyID: "current"}
	event := normalizeEvent(Event{Actor: "user:a", Outcome: "allowed", Payload: Payload{Action: "read"}})
	hash, payload, err := hashEvent("", 1, event)
	require.NoError(t, err)
	record := archiveRecord{EventID: event.ID, Stream: "authorization", Sequence: 1, ContentHash: hash, Event: payload}
	artifact, err := (Archiver{Signer: signer}).artifact(t.Context(), record)
	require.NoError(t, err)
	runtime := &Runtime{verificationKey: &key.PublicKey, signingRequired: true}
	row := archiveVerificationRow{eventID: event.ID, sequence: 1, hash: hash}
	require.NoError(t, runtime.verifyEnvelope(t.Context(), artifact.Data, "authorization", row))
	require.Error(t, runtime.verifyEnvelope(t.Context(), artifact.Data, "other", row))
	var wrapper map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(artifact.Data, &wrapper))
	wrapper["event"] = json.RawMessage(`{"Sequence":2}`)
	modified, err := json.Marshal(wrapper)
	require.NoError(t, err)
	require.ErrorContains(t, runtime.verifyEnvelope(t.Context(), modified, "authorization", row), "SIGNATURE")
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	runtime.verificationKey = &wrongKey.PublicKey
	require.ErrorContains(t, runtime.verifyEnvelope(t.Context(), artifact.Data, "authorization", row), "SIGNATURE")
	unsigned, err := (Archiver{}).artifact(t.Context(), record)
	require.NoError(t, err)
	require.ErrorContains(t, runtime.verifyEnvelope(t.Context(), unsigned.Data, "authorization", row), "UNSIGNED")
}

type verificationStore struct {
	object                    *evidence.Object
	missing, retentionFailure bool
}

func (store verificationStore) PutArtifact(context.Context, evidence.Artifact) (*evidence.Receipt, error) {
	return nil, fmt.Errorf("TEST-UNSUPPORTED")
}
func (store verificationStore) GetArtifact(context.Context, evidence.Reference) (*evidence.Object, error) {
	if store.missing {
		return nil, fmt.Errorf("TEST-MISSING")
	}
	return store.object, nil
}
func (store verificationStore) VerifyArtifact(context.Context, evidence.Reference, string) (*evidence.Receipt, error) {
	return nil, fmt.Errorf("TEST-UNSUPPORTED")
}
func (store verificationStore) VerifyArtifactRetention(context.Context, evidence.Reference, evidence.Receipt) error {
	if store.retentionFailure {
		return fmt.Errorf("TEST-RETENTION")
	}
	return nil
}

func TestArchiveVerificationDistinguishesPendingAndMissingDeliveries(t *testing.T) {
	runtime := &Runtime{Repository: Repository{WORM: true}}
	result := ArchiveVerification{}
	row := archiveVerificationRow{deliveryID: sql.NullString{String: "event", Valid: true}}
	require.NoError(t, runtime.verifyCatalogObject(t.Context(), "authorization", row, &result))
	require.Equal(t, int64(1), result.Pending)
	require.ErrorContains(t, runtime.verifyCatalogObject(t.Context(), "authorization", archiveVerificationRow{}, &result), "DELIVERY")
}

func TestArchiveVerificationChecksStoredHashAndRetention(t *testing.T) {
	event := normalizeEvent(Event{Actor: "user:a", Outcome: "allowed", Payload: Payload{Action: "read"}})
	hash, payload, err := hashEvent("", 1, event)
	require.NoError(t, err)
	artifact, err := (Archiver{}).artifact(t.Context(), archiveRecord{EventID: event.ID, Stream: "authorization", Sequence: 1, ContentHash: hash, Event: payload})
	require.NoError(t, err)
	receipt, err := json.Marshal(evidence.Receipt{Reference: evidence.Reference{VersionID: "v1"}, SHA256: evidence.SHA256Hex(artifact.Data)})
	require.NoError(t, err)
	row := archiveVerificationRow{eventID: event.ID, sequence: 1, hash: hash, deliveryID: sql.NullString{Valid: true}, archived: sql.NullTime{Time: time.Now(), Valid: true}, receipt: receipt}
	for _, test := range []struct {
		name  string
		store verificationStore
		valid bool
	}{
		{"intact", verificationStore{object: &evidence.Object{Data: artifact.Data}}, true},
		{"missing", verificationStore{missing: true}, false},
		{"changed", verificationStore{object: &evidence.Object{Data: []byte("changed")}}, false},
		{"retention", verificationStore{object: &evidence.Object{Data: artifact.Data}, retentionFailure: true}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := &Runtime{archiver: &Archiver{Store: test.store}}
			result := ArchiveVerification{}
			err := runtime.verifyCatalogObject(t.Context(), "authorization", row, &result)
			if test.valid {
				require.NoError(t, err)
				require.Equal(t, int64(1), result.Archived)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestVerifyArchiveReportsPendingAndDetectsCatalogGaps(t *testing.T) {
	for _, missing := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		event := testEvent()
		hash, payload, err := hashEvent("", 1, event)
		require.NoError(t, err)
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT .* FROM "audit_record"`).WillReturnRows(sqlmock.NewRows([]string{"sequence", "event", "previous_hash", "content_hash"}).AddRow(1, payload, "", hash))
		mock.ExpectQuery(`SELECT .* FROM "audit_stream"`).WillReturnRows(sqlmock.NewRows([]string{"last_sequence", "last_hash"}).AddRow(1, hash))
		mock.ExpectCommit()
		deliveries := sqlmock.NewRows([]string{"event_id", "sequence", "content_hash", "delivery_id", "archived_at", "receipt"})
		if !missing {
			deliveries.AddRow(event.ID, 1, hash, event.ID, nil, nil)
		}
		mock.ExpectQuery(`SELECT .* FROM "audit_record" LEFT JOIN`).WillReturnRows(deliveries)
		runtime := &Runtime{Repository: Repository{DB: db, WORM: true}}
		result, err := runtime.VerifyArchive(t.Context(), "access")
		require.NoError(t, err)
		require.Equal(t, !missing, result.Valid)
		if !missing {
			require.Equal(t, int64(1), result.Pending)
			require.Zero(t, result.Archived)
		}
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

func TestAuditVerificationPreservesRetainedSigningKeysAfterRotation(t *testing.T) {
	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	currentKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer := rsaManifestSigner{key: oldKey, keyID: publicKeyIdentifier(&oldKey.PublicKey)}
	event := normalizeEvent(Event{Actor: "user:creator", Outcome: "allowed", Payload: Payload{Action: "read"}})
	hash, payload, err := hashEvent("", 1, event)
	require.NoError(t, err)
	record := archiveRecord{EventID: event.ID, Stream: "authorization", Sequence: 1, ContentHash: hash, Event: payload}
	artifact, err := (Archiver{Signer: signer}).artifact(t.Context(), record)
	require.NoError(t, err)
	runtime := &Runtime{verificationKey: &currentKey.PublicKey, verificationKeys: map[string]*rsa.PublicKey{publicKeyIdentifier(&oldKey.PublicKey): &oldKey.PublicKey, publicKeyIdentifier(&currentKey.PublicKey): &currentKey.PublicKey}, signingRequired: true}
	row := archiveVerificationRow{eventID: event.ID, sequence: 1, hash: hash}
	require.NoError(t, runtime.verifyEnvelope(t.Context(), artifact.Data, "authorization", row))
	delete(runtime.verificationKeys, publicKeyIdentifier(&oldKey.PublicKey))
	require.ErrorContains(t, runtime.verifyEnvelope(t.Context(), artifact.Data, "authorization", row), "unknown signing key")
}
