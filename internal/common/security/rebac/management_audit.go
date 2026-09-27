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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
)

// handleListAudit pages through the audit trail for administrators, newest
// events first unless afterId is given.
func (c *Coordinator) handleListAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := c.administrator(w, r); !ok {
		return
	}
	query, found, err := c.auditQuery(r.Context(), r.URL.Query())
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, AuditPage{Events: []AuditEvent{}})
		return
	}
	page, err := ListAuditEvents(r.Context(), c.db, query)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// handleVerifyAudit verifies a range of the hash chain and, with history
// evidence enabled, the archived events. Clients continue from the returned
// head until the range is complete.
func (c *Coordinator) handleVerifyAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := c.administrator(w, r); !ok {
		return
	}
	rng, err := auditRange(r.URL.Query())
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	report, err := VerifyAuditRange(r.Context(), c.db, history.ActiveConfig().EvidenceStore, rng)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// auditQuery parses the page, direction and filters of an audit listing.
// found is false when the filtered resource does not exist.
func (c *Coordinator) auditQuery(ctx context.Context, values url.Values) (AuditQuery, bool, error) {
	var query AuditQuery
	var err error
	if query.Limit, err = boundedCount(values.Get("limit"), defaultAuditPage, maxAuditPage, "REBAC-LISTAUDIT-LIMIT"); err != nil {
		return AuditQuery{}, false, err
	}
	if query.Ascending, query.AfterID, query.BeforeID, err = auditCursor(values); err != nil {
		return AuditQuery{}, false, err
	}
	if query.Actor, err = auditActor(values); err != nil {
		return AuditQuery{}, false, err
	}
	object, found, err := c.auditObject(ctx, values)
	query.Object = object
	return query, found, err
}

// auditCursor returns the direction and position of a page: afterId pages
// forwards, beforeId continues backwards from the newest events.
func auditCursor(values url.Values) (bool, int64, int64, error) {
	after, before := values.Get("afterId"), values.Get("beforeId")
	if after != "" && before != "" {
		return false, 0, 0, common.NewErrBadRequest("REBAC-LISTAUDIT-CURSOR afterId and beforeId must not be combined")
	}
	if after != "" {
		id, err := auditEventID(after, "REBAC-LISTAUDIT-AFTERID afterId")
		return true, id, 0, err
	}
	if before != "" {
		id, err := auditEventID(before, "REBAC-LISTAUDIT-BEFOREID beforeId")
		return false, 0, id, err
	}
	return false, 0, 0, nil
}

// auditActor returns the actor key of a filter given as key or as issuer
// and user ID.
func auditActor(values url.Values) (string, error) {
	actor := strings.TrimSpace(values.Get("actor"))
	issuer := strings.TrimSpace(values.Get("actorIssuer"))
	subject := strings.TrimSpace(values.Get("actorSubject"))
	switch {
	case actor != "" && (issuer != "" || subject != ""):
		return "", common.NewErrBadRequest("REBAC-LISTAUDIT-ACTOR actor must not be combined with actorIssuer or actorSubject")
	case actor != "":
		return actor, nil
	case subject == "" && issuer == "":
		return "", nil
	case subject == "" || issuer == "":
		return "", common.NewErrBadRequest("REBAC-LISTAUDIT-ACTOR actorIssuer and actorSubject must be given together")
	default:
		return UserKey(issuer, subject), nil
	}
}

// auditObject returns the object key of a filter given as key or as object
// type with identifier. found is false for resources that do not exist.
func (c *Coordinator) auditObject(ctx context.Context, values url.Values) (string, bool, error) {
	object := strings.TrimSpace(values.Get("object"))
	objectType := strings.TrimSpace(values.Get("objectType"))
	objectID := strings.TrimSpace(values.Get("objectId"))
	path := strings.TrimSpace(values.Get("idShortPath"))
	switch {
	case object != "" && (objectType != "" || objectID != "" || path != ""):
		return "", false, common.NewErrBadRequest("REBAC-LISTAUDIT-OBJECT object must not be combined with objectType, objectId or idShortPath")
	case object != "":
		return object, true, nil
	case objectType == "" && objectID == "" && path == "":
		return "", true, nil
	case objectType == "" || objectID == "":
		return "", false, common.NewErrBadRequest("REBAC-LISTAUDIT-OBJECT objectType and objectId must be given together")
	case (objectType == TypeElement) != (path != ""):
		return "", false, common.NewErrBadRequest("REBAC-LISTAUDIT-OBJECT idShortPath is required for and only allowed with objectType element")
	}
	return c.auditObjectKey(ctx, objectType, objectID, path)
}

// auditObjectKey resolves the object key of a resource, element path or
// repository family.
func (c *Coordinator) auditObjectKey(ctx context.Context, objectType string, objectID string, path string) (string, bool, error) {
	if objectType == TypeRepository {
		if _, covered := KindForObjectType(objectID); !covered {
			return "", false, common.NewErrBadRequest("REBAC-LISTAUDIT-OBJECT unknown repository " + objectID)
		}
		return RepositoryKey(objectID), true, nil
	}
	kindType := objectType
	if objectType == TypeElement {
		kindType = TypeSubmodel
	}
	kind, covered := KindForObjectType(kindType)
	if !covered {
		return "", false, common.NewErrBadRequest("REBAC-LISTAUDIT-OBJECT unknown objectType " + objectType)
	}
	authUUID, found, err := LookupAuthUUID(ctx, c.db, kind, objectID)
	if err != nil || !found {
		return "", false, err
	}
	if objectType == TypeElement {
		return ElementKey(authUUID, path), true, nil
	}
	return ResourceKey(objectType, authUUID), true, nil
}

// auditRange parses the checkpoint, size and expected head of a
// verification range.
func auditRange(values url.Values) (AuditRange, error) {
	rng := AuditRange{
		AfterHash:    strings.ToLower(strings.TrimSpace(values.Get("afterHash"))),
		ExpectedHead: strings.ToLower(strings.TrimSpace(values.Get("expectedHead"))),
	}
	var err error
	if rng.Limit, err = boundedCount(values.Get("limit"), defaultAuditVerifyRange, maxAuditVerifyRange, "REBAC-VERIFYAUDIT-LIMIT"); err != nil {
		return AuditRange{}, err
	}
	if raw := values.Get("afterId"); raw != "" {
		if rng.AfterID, err = auditEventID(raw, "REBAC-VERIFYAUDIT-AFTERID afterId"); err != nil {
			return AuditRange{}, err
		}
	}
	if (rng.AfterID > 0) != (rng.AfterHash != "") {
		return AuditRange{}, common.NewErrBadRequest("REBAC-VERIFYAUDIT-CHECKPOINT afterId and afterHash must be given together")
	}
	return rng, nil
}

func auditEventID(raw string, description string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 0 {
		return 0, common.NewErrBadRequest(description + " must be a non-negative integer")
	}
	return id, nil
}

// boundedCount parses an optional count between 1 and maximum.
func boundedCount(raw string, fallback uint, maximum uint, code string) (uint, error) {
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || parsed == 0 || uint(parsed) > maximum {
		return 0, common.NewErrBadRequest(fmt.Sprintf("%s limit must be between 1 and %d", code, maximum))
	}
	return uint(parsed), nil
}
