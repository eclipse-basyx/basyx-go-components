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

package rebac

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	invitationTokenBytes  = 32
	maxInvitationUses     = 1000
	maxInvitationLifetime = 90 * 24 * time.Hour
)

// Invitation is a pending share link. Its token is only returned once.
type Invitation struct {
	ID        string    `json:"id"`
	Relation  string    `json:"relation"`
	ExpiresAt time.Time `json:"expiresAt"`
	MaxUses   int       `json:"maxUses"`
	UsedCount int       `json:"usedCount"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	Token     string    `json:"token,omitempty"`
	// Restricted reports whether only one expected user may redeem.
	Restricted bool `json:"restricted"`
}

type invitationRequest struct {
	Relation          string             `json:"relation"`
	ExpiresAt         time.Time          `json:"expiresAt"`
	MaxUses           *int               `json:"maxUses,omitempty"`
	ExpectedPrincipal *expectedPrincipal `json:"expectedPrincipal,omitempty"`
}

// expectedPrincipal restricts an invitation to one verified user.
type expectedPrincipal struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}

type acceptRequest struct {
	Token string `json:"token"`
}

type acceptedInvitation struct {
	Object   accessObject `json:"object"`
	Relation string       `json:"relation"`
}

// redeemedInvitation is the target of a redeemed invitation.
type redeemedInvitation struct {
	id          string
	objectType  string
	objectUUID  string
	elementPath string
	relation    string
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func newInvitationToken() (string, error) {
	raw := make([]byte, invitationTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("REBAC-INVITATION-TOKEN: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func validateInvitation(target accessTarget, input invitationRequest) (int, error) {
	switch input.Relation {
	case RelationViewer, RelationEditor:
	case RelationExecutor:
		if err := ValidateRelation(target.objectType(), RelationExecutor); err != nil {
			return 0, common.NewErrBadRequest(err.Error())
		}
	default:
		return 0, common.NewErrBadRequest("REBAC-INVITATION-RELATION relation must be viewer, editor or executor")
	}
	now := time.Now()
	if !input.ExpiresAt.After(now) || input.ExpiresAt.After(now.Add(maxInvitationLifetime)) {
		return 0, common.NewErrBadRequest("REBAC-INVITATION-EXPIRY expiresAt must be in the future and within 90 days")
	}
	if expected := input.ExpectedPrincipal; expected != nil &&
		(strings.TrimSpace(expected.Issuer) == "" || strings.TrimSpace(expected.Subject) == "") {
		return 0, common.NewErrBadRequest("REBAC-INVITATION-EXPECTEDPRINCIPAL expectedPrincipal needs issuer and subject")
	}
	maxUses := 1
	if input.MaxUses != nil {
		maxUses = *input.MaxUses
	}
	if maxUses < 1 || maxUses > maxInvitationUses {
		return 0, common.NewErrBadRequest(fmt.Sprintf("REBAC-INVITATION-MAXUSES maxUses must be between 1 and %d", maxInvitationUses))
	}
	return maxUses, nil
}

func (c *Coordinator) handleCreateInvitation(w http.ResponseWriter, r *http.Request, request accessRequest) {
	var input invitationRequest
	if err := decodeBody(r, &input); err != nil {
		writeManagementError(w, r, err)
		return
	}
	maxUses, err := validateInvitation(request.target, input)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	token, err := newInvitationToken()
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	invitation := Invitation{
		ID: uuid.NewString(), Relation: input.Relation, ExpiresAt: input.ExpiresAt.UTC(), MaxUses: maxUses,
		CreatedBy: request.principal.UserKey(), CreatedAt: time.Now().UTC(), Token: token,
	}
	record := goqu.Record{
		"id": goqu.L("?::uuid", invitation.ID), "token_hash": hashToken(token), "object_key": request.target.objectKey(),
		"object_type": request.target.objectType(), "object_uuid": goqu.L("?::uuid", request.target.authUUID),
		"relation": invitation.Relation, "expires_at": invitation.ExpiresAt, "max_uses": maxUses,
		"created_by": invitation.CreatedBy,
	}
	if request.target.elementPath != "" {
		record["element_path"] = request.target.elementPath
	}
	if input.ExpectedPrincipal != nil {
		record["expected_subject_key"] = UserKey(strings.TrimSpace(input.ExpectedPrincipal.Issuer), strings.TrimSpace(input.ExpectedPrincipal.Subject))
		invitation.Restricted = true
	}
	details := map[string]any{
		"invitation": invitation.ID, "relation": invitation.Relation, "expiresAt": invitation.ExpiresAt.Format(time.RFC3339Nano),
		"maxUses": maxUses, "restricted": invitation.Restricted,
	}
	err = common.ExecuteInTransaction(c.db, "REBAC-CREATEINVITATION-STARTTX", "REBAC-CREATEINVITATION-COMMIT", func(tx *sql.Tx) error {
		if _, txErr := execDataset(r.Context(), tx, "REBAC-CREATEINVITATION", dialect.Insert(invitationTable).Rows(record).Prepared(true)); txErr != nil {
			return txErr
		}
		return c.auditTarget(r.Context(), tx, AuditInvitationCreated, request.target, details)
	})
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "ReBAC invitation created", "event.code", "REBAC-INVITATION-CREATED",
		"rebac.invitation_id", invitation.ID, "rebac.object", request.target.objectKey(), "rebac.relation", invitation.Relation)
	writeJSON(w, http.StatusCreated, invitation)
}

func (c *Coordinator) handleListInvitations(w http.ResponseWriter, r *http.Request, request accessRequest) {
	ds := dialect.From(goqu.T(invitationTable)).Select(
		goqu.L("id::text"), goqu.C("relation"), goqu.C("expires_at"), goqu.C("max_uses"), goqu.C("used_count"),
		goqu.C("created_by"), goqu.C("created_at"), goqu.L("expected_subject_key IS NOT NULL"),
	).Where(
		goqu.C("object_key").Eq(request.target.objectKey()), goqu.C("revoked_at").IsNull(),
		goqu.C("expires_at").Gt(goqu.L("clock_timestamp()")), goqu.C("used_count").Lt(goqu.C("max_uses")),
	).Order(goqu.C("created_at").Asc())
	rows, err := queryDataset(r.Context(), c.db, "REBAC-LISTINVITATIONS", ds)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()
	invitations := []Invitation{}
	for rows.Next() {
		var invitation Invitation
		if err = rows.Scan(&invitation.ID, &invitation.Relation, &invitation.ExpiresAt, &invitation.MaxUses,
			&invitation.UsedCount, &invitation.CreatedBy, &invitation.CreatedAt, &invitation.Restricted); err != nil {
			writeManagementError(w, r, fmt.Errorf("REBAC-LISTINVITATIONS-SCAN: %w", err))
			return
		}
		invitations = append(invitations, invitation)
	}
	writeJSON(w, http.StatusOK, map[string][]Invitation{"invitations": invitations})
}

// handleRevokeInvitation revokes a pending invitation. Grants that were
// already redeemed stay until they are removed individually.
func (c *Coordinator) handleRevokeInvitation(w http.ResponseWriter, r *http.Request, request accessRequest) {
	invitationID := chi.URLParam(r, paramInvitation)
	if _, err := uuid.Parse(invitationID); err != nil {
		writeNotFound(w)
		return
	}
	ds := dialect.Update(invitationTable).Set(goqu.Record{"revoked_at": goqu.L("clock_timestamp()")}).Where(
		goqu.C("id").Eq(goqu.L("?::uuid", invitationID)), goqu.C("object_key").Eq(request.target.objectKey()),
		goqu.C("revoked_at").IsNull(),
	).Prepared(true)
	err := common.ExecuteInTransaction(c.db, "REBAC-REVOKEINVITATION-STARTTX", "REBAC-REVOKEINVITATION-COMMIT", func(tx *sql.Tx) error {
		result, txErr := execDataset(r.Context(), tx, "REBAC-REVOKEINVITATION", ds)
		if txErr != nil {
			return txErr
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return errTargetGone
		}
		return c.auditTarget(r.Context(), tx, AuditInvitationRevoked, request.target, map[string]any{"invitation": invitationID})
	})
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "ReBAC invitation revoked", "event.code", "REBAC-INVITATION-REVOKED", "rebac.invitation_id", invitationID)
	w.WriteHeader(http.StatusNoContent)
}

// handleAcceptInvitation redeems an invitation for the authenticated caller.
// The token only creates a normal direct grant for the caller's own issuer
// and subject; it never authorizes data access by itself.
func (c *Coordinator) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	principal, ok := c.managementPrincipal(w, r)
	if !ok {
		return
	}
	var input acceptRequest
	if err := decodeBody(r, &input); err != nil || strings.TrimSpace(input.Token) == "" {
		writeNotFound(w)
		return
	}
	var accepted acceptedInvitation
	err := common.ExecuteInTransaction(c.db, "REBAC-ACCEPTINVITATION-STARTTX", "REBAC-ACCEPTINVITATION-COMMIT", func(tx *sql.Tx) error {
		var txErr error
		accepted, txErr = c.redeemInvitation(r.Context(), tx, principal, strings.TrimSpace(input.Token))
		return txErr
	})
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "ReBAC invitation redeemed", "event.code", "REBAC-INVITATION-REDEEMED",
		"rebac.object_type", accepted.Object.Type, "rebac.relation", accepted.Relation)
	writeJSON(w, http.StatusOK, accepted)
}

func (c *Coordinator) redeemInvitation(ctx context.Context, tx *sql.Tx, principal Principal, token string) (acceptedInvitation, error) {
	redeemed, found, err := consumeInvitation(ctx, tx, token, principal)
	if err != nil {
		return acceptedInvitation{}, err
	}
	if !found {
		return acceptedInvitation{}, errTargetGone
	}
	kind := KindSubmodel
	if redeemed.objectType != TypeElement {
		kind, _ = KindForObjectType(redeemed.objectType)
	}
	identifier, exists, err := IdentifierByAuthUUID(ctx, tx, kind, redeemed.objectUUID)
	if err != nil || !exists {
		return acceptedInvitation{}, firstError(err, errTargetGone)
	}
	target := accessTarget{kind: kind, identifier: identifier, authUUID: redeemed.objectUUID, elementPath: redeemed.elementPath}
	if _, err = LockObjectRevision(ctx, tx, target.objectKey()); err != nil {
		return acceptedInvitation{}, err
	}
	grant, err := buildGrant(accessRequest{principal: principal, target: target}, grantInput{
		Relation: redeemed.relation, SubjectType: TypeUser, Issuer: principal.Issuer, Subject: principal.Subject,
	})
	if err != nil {
		return acceptedInvitation{}, err
	}
	current, err := ListGrants(ctx, tx, target.objectKey())
	if err != nil {
		return acceptedInvitation{}, err
	}
	if err = c.applyGrantDiff(ctx, tx, target, current, append(current, grant)); err != nil {
		return acceptedInvitation{}, err
	}
	err = c.auditTarget(ctx, tx, AuditInvitationRedeemed, target, map[string]any{"invitation": redeemed.id, "relation": redeemed.relation})
	accepted := acceptedInvitation{Object: accessObject{Type: target.objectType(), ID: identifier, IDShortPath: target.elementPath}, Relation: redeemed.relation}
	return accepted, err
}

// consumeInvitation atomically counts one use of a valid invitation, so
// concurrent redemptions never exceed maxUses. Invalid, expired, revoked,
// exhausted and foreign invitations are indistinguishable.
func consumeInvitation(ctx context.Context, tx *sql.Tx, token string, principal Principal) (redeemedInvitation, bool, error) {
	ds := dialect.Update(invitationTable).Set(goqu.Record{"used_count": goqu.L("used_count + 1")}).Where(
		goqu.C("token_hash").Eq(hashToken(token)), goqu.C("revoked_at").IsNull(),
		goqu.C("expires_at").Gt(goqu.L("clock_timestamp()")), goqu.C("used_count").Lt(goqu.C("max_uses")),
		goqu.Or(goqu.C("expected_subject_key").IsNull(), goqu.C("expected_subject_key").Eq(principal.UserKey())),
	).Returning(goqu.L("id::text"), goqu.C("object_type"), goqu.L("object_uuid::text"), goqu.L("COALESCE(element_path, '')"), goqu.C("relation")).Prepared(true)
	var redeemed redeemedInvitation
	found, err := queryRowDataset(ctx, tx, "REBAC-CONSUMEINVITATION", ds,
		&redeemed.id, &redeemed.objectType, &redeemed.objectUUID, &redeemed.elementPath, &redeemed.relation)
	return redeemed, found, err
}
