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

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/google/uuid"
)

const (
	shareLinkDefaultLifetime = time.Hour
	shareLinkMaximumLifetime = 7 * 24 * time.Hour
	shareLinkRedeemPath      = "/security/rebac/share-links/redeem"
)

type shareLinkInput struct {
	Rights            []grammar.RightsEnum    `json:"rights"`
	ExpiresInSeconds  int64                   `json:"expiresInSeconds,omitempty"`
	ExpectedPrincipal *common.AccessPrincipal `json:"expectedPrincipal,omitempty"`
}

type shareLinkResponse struct {
	ID        string    `json:"id"`
	ShareLink string    `json:"shareLink"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (repo *resourceBoundRepository) mutateShareLink(r *http.Request, tx *sql.Tx, access *boundAccess, id string) (int, any, error) {
	if access.Policy == nil {
		return 0, nil, boundError(http.StatusConflict, "INHERITED create a local policy explicitly first")
	}
	if id != "" {
		return repo.revokeShareLink(r, tx, access.ID, id)
	}
	if r.Method != http.MethodPost {
		return 0, nil, boundError(http.StatusMethodNotAllowed, "METHOD expected POST")
	}
	var input shareLinkInput
	if err := decodeBoundBody(r, &input); err != nil {
		return 0, nil, err
	}
	if err := validateBoundRights(input.Rights); err != nil {
		return 0, nil, err
	}
	if _, err := boundPrincipalRule(common.AccessPrincipal{Issuer: "validation", Subject: "validation"}, input.Rights); err != nil {
		return 0, nil, boundError(http.StatusBadRequest, "RIGHTS "+err.Error())
	}
	if input.ExpectedPrincipal != nil {
		input.ExpectedPrincipal.Type = input.ExpectedPrincipal.NormalizedType()
		if input.ExpectedPrincipal.Type != common.AccessPrincipalUser {
			return 0, nil, boundError(http.StatusBadRequest, "RECIPIENT expected principal must be a user")
		}
		if err := validateBoundPrincipals([]common.AccessPrincipal{*input.ExpectedPrincipal}, false); err != nil {
			return 0, nil, err
		}
	}
	lifetime := shareLinkDefaultLifetime
	if input.ExpiresInSeconds != 0 {
		if input.ExpiresInSeconds < 1 || input.ExpiresInSeconds > int64(shareLinkMaximumLifetime/time.Second) {
			return 0, nil, boundError(http.StatusBadRequest, "EXPIRY must be between 1 second and 7 days")
		}
		lifetime = time.Duration(input.ExpiresInSeconds) * time.Second
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return 0, nil, fmt.Errorf("REBAC-SHARE-GENERATE %w", err)
	}
	encodedToken := base64.RawURLEncoding.EncodeToString(token)
	tokenHash := sha256.Sum256([]byte(encodedToken))
	rights, err := json.Marshal(input.Rights)
	if err != nil {
		return 0, nil, fmt.Errorf("REBAC-SHARE-RIGHTS %w", err)
	}
	actor, err := boundActor(r.Context())
	if err != nil {
		return 0, nil, err
	}
	id = uuid.NewString()
	expiresAt := time.Now().UTC().Add(lifetime)
	record := goqu.Record{
		"id": id, "scope": repo.scope, "access_id": access.ID, "issued_access_revision": access.Revision,
		"token_hash": tokenHash[:], "rights": string(rights), "created_by_issuer": actor.Issuer,
		"created_by_subject": actor.Subject, "expires_at": expiresAt,
	}
	if input.ExpectedPrincipal != nil {
		record["expected_issuer"] = input.ExpectedPrincipal.Issuer
		record["expected_subject"] = input.ExpectedPrincipal.Subject
	}
	if err = boundExec(r.Context(), tx, goqu.Dialect("postgres").Insert("rebac_share_invitation").Rows(record).Prepared(true)); err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, shareLinkResponse{ID: id, ShareLink: "#/share-access?token=" + encodedToken, ExpiresAt: expiresAt}, nil
}

func (repo *resourceBoundRepository) revokeShareLink(r *http.Request, tx *sql.Tx, accessID int64, id string) (int, any, error) {
	if r.Method != http.MethodDelete {
		return 0, nil, boundError(http.StatusMethodNotAllowed, "METHOD expected DELETE")
	}
	if _, err := uuid.Parse(id); err != nil {
		return 0, nil, boundError(http.StatusBadRequest, "SHARE invalid invitation ID")
	}
	actor, err := boundActor(r.Context())
	if err != nil {
		return 0, nil, err
	}
	ds := goqu.Dialect("postgres").Update("rebac_share_invitation").Set(goqu.Record{
		"revoked_at":         goqu.L("CURRENT_TIMESTAMP"),
		"revoked_by_issuer":  actor.Issuer,
		"revoked_by_subject": actor.Subject,
	}).Where(goqu.Ex{"id": id, "scope": repo.scope, "access_id": accessID, "revoked_at": nil, "redeemed_at": nil})
	query, args, err := ds.Prepared(true).ToSQL()
	if err != nil {
		return 0, nil, fmt.Errorf("REBAC-SHARE-REVOKEBUILD %w", err)
	}
	result, err := tx.ExecContext(r.Context(), query, args...)
	if err != nil {
		return 0, nil, fmt.Errorf("REBAC-SHARE-REVOKE %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return 0, nil, boundError(http.StatusNotFound, "SHARE invitation not found")
	}
	return http.StatusNoContent, nil, nil
}

func (repo *resourceBoundRepository) serveShareLinkRedemption(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodPost {
		writeBoundError(w, boundError(http.StatusMethodNotAllowed, "METHOD expected POST"))
		return
	}
	actor, err := boundActor(r.Context())
	if err != nil {
		writeBoundError(w, err)
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	if err = decodeBoundBody(r, &input); err != nil {
		writeBoundError(w, err)
		return
	}
	if strings.TrimSpace(input.Token) == "" {
		writeBoundError(w, boundError(http.StatusBadRequest, "TOKEN token required"))
		return
	}
	decodedToken, decodeErr := base64.RawURLEncoding.DecodeString(input.Token)
	if decodeErr != nil || len(decodedToken) != 32 {
		writeBoundError(w, boundError(http.StatusNotFound, "SHARE invitation not found"))
		return
	}
	grantID, err := repo.redeemShareLink(r, actor, input.Token)
	if err != nil {
		writeBoundError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"grantId": grantID})
}

func (repo *resourceBoundRepository) redeemShareLink(r *http.Request, actor common.AccessPrincipal, token string) (string, error) {
	tx, err := repo.db.BeginTx(r.Context(), nil)
	if err != nil {
		return "", fmt.Errorf("REBAC-SHARE-BEGIN %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = repo.lock(r.Context(), tx, true); err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(token))
	invitation := goqu.Dialect("postgres").From("rebac_share_invitation").Select("id", "access_id", "issued_access_revision", "rights", "expected_issuer", "expected_subject").Where(
		goqu.Ex{"scope": repo.scope, "token_hash": hash[:], "redeemed_at": nil, "revoked_at": nil},
		goqu.C("expires_at").Gt(goqu.L("CURRENT_TIMESTAMP")),
	).ForUpdate(goqu.Wait).Prepared(true)
	query, args, buildErr := invitation.ToSQL()
	if buildErr != nil {
		return "", fmt.Errorf("REBAC-SHARE-LOOKUPBUILD %w", buildErr)
	}
	var invitationID string
	var accessID, issuedRevision int64
	var rightsJSON []byte
	var expectedIssuer, expectedSubject sql.NullString
	if err = tx.QueryRowContext(r.Context(), query, args...).Scan(&invitationID, &accessID, &issuedRevision, &rightsJSON, &expectedIssuer, &expectedSubject); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", boundError(http.StatusNotFound, "SHARE invitation not found")
		}
		return "", fmt.Errorf("REBAC-SHARE-LOOKUP %w", err)
	}
	if expectedIssuer.Valid && (expectedIssuer.String != actor.Issuer || expectedSubject.String != actor.Subject) {
		return "", boundError(http.StatusNotFound, "SHARE invitation not found")
	}
	access, err := loadShareAccess(r, tx, accessID, issuedRevision)
	if err != nil {
		return "", err
	}
	var rights []grammar.RightsEnum
	if err = json.Unmarshal(rightsJSON, &rights); err != nil {
		return "", fmt.Errorf("REBAC-SHARE-RIGHTSDECODE %w", err)
	}
	grantID := uuid.NewString()
	rule, err := boundManagedGrantRule(actor, rights, grantID)
	if err != nil {
		return "", err
	}
	encodedRights, _ := json.Marshal(rights)
	grant := goqu.Record{"id": grantID, "access_id": access.ID, "principal_type": common.AccessPrincipalUser, "issuer": actor.Issuer, "subject": actor.Subject, "rights": string(encodedRights), "rule": string(rule)}
	if err = boundExec(r.Context(), tx, goqu.Dialect("postgres").Insert("rebac_grant").Rows(grant).Prepared(true)); err != nil {
		return "", err
	}
	access.Policy.Rules = append(access.Policy.Rules, rule)
	if err = repo.save(r.Context(), tx, access, actor); err != nil {
		return "", err
	}
	consume := goqu.Dialect("postgres").Update("rebac_share_invitation").Set(goqu.Record{"redeemed_at": goqu.L("CURRENT_TIMESTAMP"), "redeemed_by_issuer": actor.Issuer, "redeemed_by_subject": actor.Subject}).Where(goqu.Ex{"id": invitationID, "redeemed_at": nil}).Prepared(true)
	if err = boundExec(r.Context(), tx, consume); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("REBAC-SHARE-COMMIT %w", err)
	}
	return grantID, nil
}

func loadShareAccess(r *http.Request, tx *sql.Tx, accessID, issuedRevision int64) (*boundAccess, error) {
	ds := goqu.Dialect("postgres").From("rebac_access").Select("revision", "policy").Where(goqu.Ex{"id": accessID, "revision": issuedRevision}).ForUpdate(goqu.Wait).Prepared(true)
	query, args, err := ds.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-SHARE-ACCESSBUILD %w", err)
	}
	access := &boundAccess{ID: accessID}
	var raw []byte
	if err = tx.QueryRowContext(r.Context(), query, args...).Scan(&access.Revision, &raw); errors.Is(err, sql.ErrNoRows) {
		return nil, boundError(http.StatusNotFound, "SHARE invitation not found")
	}
	if err != nil {
		return nil, fmt.Errorf("REBAC-SHARE-ACCESS %w", err)
	}
	if len(raw) == 0 {
		return nil, boundError(http.StatusNotFound, "SHARE invitation not found")
	}
	access.Policy, err = decodeBoundPolicy(raw)
	if err != nil {
		return nil, err
	}
	return access, nil
}
