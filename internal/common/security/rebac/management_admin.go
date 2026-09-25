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
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/go-chi/chi/v5"
)

type statusDocument struct {
	Ready      bool       `json:"ready"`
	Activation Activation `json:"activation"`
	State      ScopeState `json:"state"`
}

type operationDocument struct {
	OperationID string `json:"operationId"`
	Status      string `json:"status"`
}

type ownersDocument struct {
	Owners []grantInput `json:"owners"`
}

// administrator returns the caller when it is a configured administrator.
func (c *Coordinator) administrator(w http.ResponseWriter, r *http.Request) (Principal, bool) {
	principal, ok := c.managementPrincipal(w, r)
	if !ok {
		return Principal{}, false
	}
	if !c.isAdministrator(principal) {
		writeNotFound(w)
		return Principal{}, false
	}
	return principal, true
}

func (c *Coordinator) handleStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := c.administrator(w, r); !ok {
		return
	}
	activation, _, err := ReadActivation(r.Context(), c.db)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	state, err := ReadScopeState(r.Context(), c.db, c.scope)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, statusDocument{Ready: c.Ready(), Activation: activation, State: state})
}

// handleOperation reports whether an accepted access change reached OpenFGA.
// Operation IDs are random and only returned to the caller that made the
// change.
func (c *Coordinator) handleOperation(w http.ResponseWriter, r *http.Request) {
	if _, ok := c.managementPrincipal(w, r); !ok {
		return
	}
	operationID := chi.URLParam(r, paramOperation)
	found, applied, err := OperationApplied(r.Context(), c.db, operationID)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	if !found {
		writeNotFound(w)
		return
	}
	status := "pending"
	if applied {
		status = "applied"
	}
	writeJSON(w, http.StatusOK, operationDocument{OperationID: operationID, Status: status})
}

// repositoryRequest authorizes repository access management for configured
// administrators and repository admins.
func (c *Coordinator) repositoryRequest(w http.ResponseWriter, r *http.Request) (accessRequest, bool) {
	principal, ok := c.managementPrincipal(w, r)
	if !ok {
		return accessRequest{}, false
	}
	kind, covered := KindForObjectType(chi.URLParam(r, paramRepositoryKind))
	if !covered {
		writeNotFound(w)
		return accessRequest{}, false
	}
	request := accessRequest{principal: principal, admin: c.isAdministrator(principal),
		target: accessTarget{kind: kindRepository, identifier: kind.ObjectType}}
	allowed := request.admin
	if !allowed {
		resolution := &resolution{coordinator: c, principal: principal}
		var err error
		allowed, err = resolution.check(r.Context(), CheckItem{User: principal.UserObject(), Relation: RelationAdmin, Object: request.target.objectKey()})
		if err != nil {
			writeUnavailable(w, r, err)
			return accessRequest{}, false
		}
	}
	if !allowed {
		writeNotFound(w)
		return accessRequest{}, false
	}
	return request, true
}

func (c *Coordinator) handleGetRepositoryAccess(w http.ResponseWriter, r *http.Request) {
	if request, ok := c.repositoryRequest(w, r); ok {
		c.handleGetAccess(w, r, request)
	}
}

func (c *Coordinator) handlePutRepositoryGrants(w http.ResponseWriter, r *http.Request) {
	request, ok := c.repositoryRequest(w, r)
	if !ok {
		return
	}
	var body grantsDocument
	if err := decodeBody(r, &body); err != nil {
		writeManagementError(w, r, err)
		return
	}
	desired, err := buildGrants(request, body.Grants)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	if !request.admin && !hasRelation(desired, RelationAdmin) {
		writeManagementError(w, r, common.NewErrConflict("REBAC-PUTREPOSITORYGRANTS-LASTADMIN removing all repository admins requires an administrator"))
		return
	}
	c.replaceGrants(w, r, request, desired)
}

func hasRelation(grants []Grant, relation string) bool {
	for _, grant := range grants {
		if grant.Relation == relation {
			return true
		}
	}
	return false
}

func (c *Coordinator) handleReconcile(w http.ResponseWriter, r *http.Request) {
	principal, ok := c.administrator(w, r)
	if !ok {
		return
	}
	orphans, err := c.ReconcileOrphans(r.Context())
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	drift, err := c.RepairDrift(r.Context())
	if err != nil {
		writeUnavailable(w, r, err)
		return
	}
	orphans.MissingTuples, orphans.UnexpectedTuples = drift.MissingTuples, drift.UnexpectedTuples
	slog.InfoContext(r.Context(), "ReBAC reconciliation requested", "event.code", "REBAC-ADMIN-RECONCILE",
		"rebac.actor", principal.UserObject(), "rebac.unexpected_tuples", drift.UnexpectedTuples)
	writeJSON(w, http.StatusOK, orphans)
}

// handleRecoverOwners replaces the owners of an identifiable. It is the
// recovery path for resources without owner, for example resources created
// while ReBAC was disabled.
func (c *Coordinator) handleRecoverOwners(w http.ResponseWriter, r *http.Request) {
	principal, ok := c.administrator(w, r)
	if !ok {
		return
	}
	kind, covered := KindForObjectType(chi.URLParam(r, paramObjectType))
	if !covered {
		writeNotFound(w)
		return
	}
	target, found := c.resolveTarget(r, kind, paramIdentifier, "")
	if !found {
		writeNotFound(w)
		return
	}
	var body ownersDocument
	if err := decodeBody(r, &body); err != nil {
		writeManagementError(w, r, err)
		return
	}
	request := accessRequest{principal: principal, target: target, admin: true}
	var operationID string
	var document accessDocument
	err := common.ExecuteInTransaction(c.db, "REBAC-RECOVEROWNERS-STARTTX", "REBAC-RECOVEROWNERS-COMMIT", func(tx *sql.Tx) error {
		return c.recoverOwners(r, tx, request, body.Owners, &operationID, &document)
	})
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "ReBAC ownership recovered", "event.code", "REBAC-ADMIN-OWNERS",
		"rebac.actor", principal.UserObject(), "rebac.object", target.objectKey())
	c.respondAccessChange(w, r, operationID, document)
}

func (c *Coordinator) recoverOwners(r *http.Request, tx *sql.Tx, request accessRequest, owners []grantInput, operationID *string, document *accessDocument) error {
	if err := c.lockTarget(r, tx, request.target); err != nil {
		return err
	}
	current, err := ListGrants(r.Context(), tx, request.target.objectKey())
	if err != nil {
		return err
	}
	desired := make([]Grant, 0, len(current)+len(owners))
	for _, grant := range current {
		if grant.Relation != RelationOwner {
			desired = append(desired, grant)
		}
	}
	for _, owner := range owners {
		owner.Relation = RelationOwner
		grant, buildErr := buildGrant(request, owner)
		if buildErr != nil {
			return buildErr
		}
		desired = append(desired, grant)
	}
	if *operationID, err = c.applyGrantDiff(r.Context(), tx, request.target.objectKey(), current, desired); err != nil {
		return err
	}
	*document, err = c.accessDocument(r.Context(), tx, request.target)
	return err
}
