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
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/audit"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
)

func (s *rebacSecurity) serveManagement(w http.ResponseWriter, r *http.Request, request *rebacRequest) {
	if request.route.Kind == ReBACRouteKindManagement {
		s.serveAudit(w, r, request)
		return
	}
	var resource rebac.StoredResource
	err := s.runtime.State.WithApplied(r.Context(), func(tx *sql.Tx) error {
		var resolveErr error
		resource, _, resolveErr = s.resolveTarget(r.Context(), tx, request.route)
		return resolveErr
	})
	if err != nil {
		writeManagementError(w, err)
		return
	}
	if request.route.Management == "$access/effective" && r.Method == http.MethodGet {
		s.serveEffective(w, r, request, resource)
		return
	}
	service := s.grantService()
	switch {
	case request.route.Management == "$access" && r.Method == http.MethodGet:
		snapshot, err := service.Snapshot(r.Context(), request.actor, resource)
		if err != nil {
			writeManagementError(w, err)
			return
		}
		if err = s.recordDecision(r.Context(), resource.Kind+":"+resource.Identifier, "inspect_grants", "rebac", "allowed"); err != nil {
			writeManagementError(w, err)
			return
		}
		w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(snapshot.Revision, 10)))
		writeReBACJSON(w, snapshot, http.StatusOK)
	case request.route.Management == "$access/inheritance" && r.Method == http.MethodPut && resource.Kind == "submodel":
		s.replaceInheritance(w, r, request, resource)
	case request.route.Management == "$access/grants" && r.Method == http.MethodPut:
		s.replaceGrants(w, r, request, resource, service)
	default:
		writeReBACError(w, fmt.Errorf("REBAC-ACCESS-METHOD unsupported access operation"), http.StatusMethodNotAllowed)
	}
}

func (s *rebacSecurity) grantService() *rebac.GrantService {
	return &rebac.GrantService{State: s.runtime.State, Checker: s.runtime.Client, Audit: func(ctx context.Context, tx *sql.Tx, _ rebac.GrantActor, object string, writes, deletes []rebac.Tuple, revision int64) error {
		if !s.audit.Enabled {
			return nil
		}
		event := s.auditEvent(ctx, object, "grants_replace", "rebac", "pending")
		changes, _ := json.Marshal(struct{ Writes, Deletes []rebac.Tuple }{writes, deletes})
		event.Payload.Details["changes"] = string(changes)
		event.Payload.Details["revision"] = fmt.Sprint(revision)
		_, err := s.audit.Repository.Append(ctx, tx, "authorization/"+s.cfg.ReBAC.Scope, event)
		return err
	}}
}

func (s *rebacSecurity) replaceGrants(w http.ResponseWriter, r *http.Request, request *rebacRequest, resource rebac.StoredResource, service *rebac.GrantService) {
	expected, err := parseReBACIfMatch(r.Header.Get("If-Match"))
	if err != nil {
		status := http.StatusBadRequest
		if r.Header.Get("If-Match") == "" {
			status = http.StatusPreconditionRequired
		}
		writeReBACError(w, err, status)
		return
	}
	var body struct {
		Grants []rebac.Grant `json:"grants"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&body); err != nil {
		writeReBACError(w, fmt.Errorf("REBAC-ACCESS-BODY invalid grant document"), http.StatusBadRequest)
		return
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		writeReBACError(w, fmt.Errorf("REBAC-ACCESS-BODY trailing content"), http.StatusBadRequest)
		return
	}
	if err = s.recordDecision(r.Context(), resource.Kind+":"+resource.Identifier, "grants_replace_intent", "rebac", "requested"); err != nil {
		writeManagementError(w, err)
		return
	}
	revision, err := service.Replace(r.Context(), request.actor, resource, expected, body.Grants)
	if err != nil {
		if auditErr := s.recordDecision(r.Context(), resource.Kind+":"+resource.Identifier, "grants_replace_result", "rebac", "failed"); auditErr != nil {
			err = auditErr
		}
		writeManagementError(w, err)
		return
	}
	location := strings.TrimSuffix(r.URL.Path, "/grants")
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(revision, 10)))
	w.Header().Set("Location", location)
	status := http.StatusAccepted
	if revision == expected {
		status = http.StatusOK
	}
	writeReBACJSON(w, map[string]any{"revision": revision, "statusURL": location}, status)
}

func parseReBACIfMatch(value string) (int64, error) {
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, fmt.Errorf("REBAC-ACCESS-IFMATCH a strong revision ETag is required")
	}
	revision, err := strconv.ParseInt(value[1:len(value)-1], 10, 64)
	if err != nil || revision < 0 {
		return 0, fmt.Errorf("REBAC-ACCESS-IFMATCH invalid revision")
	}
	return revision, nil
}

func (s *rebacSecurity) serveAudit(w http.ResponseWriter, r *http.Request, request *rebacRequest) {
	if request.route.Management != "audit" || !s.audit.Enabled {
		writeReBACError(w, fmt.Errorf("REBAC-AUDIT-NOTFOUND audit endpoint unavailable"), http.StatusNotFound)
		return
	}
	handler := audit.AdminHandler{Repository: s.audit.Repository, Stream: "authorization/" + s.cfg.ReBAC.Scope, Verify: s.audit.VerifyArchive, Authorize: func(*http.Request) (string, error) {
		if !request.actor.Administrator {
			return "", rebac.ErrForbidden
		}
		return request.actor.User, nil
	}}
	handler.ServeHTTP(w, r)
}

func (s *rebacSecurity) serveEffective(w http.ResponseWriter, r *http.Request, request *rebacRequest, resource rebac.StoredResource) {
	result := map[string]rebac.Decision{}
	err := s.runtime.State.WithApplied(r.Context(), func(_ *sql.Tx) error {
		object, err := rebac.ResourceObject(s.cfg.ReBAC.Scope, resource.Kind, resource.UUID)
		if err != nil {
			return err
		}
		path := strings.TrimSuffix(r.URL.Path, "/$access/effective")
		for _, operation := range effectiveReBACOperations(resource.Kind, path) {
			decision, err := s.effectiveDecision(r, request, object, operation)
			if err != nil {
				return err
			}
			result[operation.permission] = decision
		}
		return s.recordDecision(r.Context(), resource.Kind+":"+resource.Identifier, "effective_permissions", "coordinator", "allowed")
	})
	if err != nil {
		writeManagementError(w, err)
		return
	}
	writeReBACJSON(w, result, http.StatusOK)
}

func writeManagementError(w http.ResponseWriter, err error) {
	status := http.StatusServiceUnavailable
	switch {
	case errors.Is(err, rebac.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, rebac.ErrStaleRevision):
		status = http.StatusPreconditionFailed
	case errors.Is(err, rebac.ErrLastOwner):
		status = http.StatusConflict
	case errors.Is(err, sql.ErrNoRows):
		status = http.StatusNotFound
	}
	writeReBACError(w, err, status)
}
func writeReBACJSON(w http.ResponseWriter, body any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *rebacSecurity) replaceInheritance(w http.ResponseWriter, r *http.Request, request *rebacRequest, resource rebac.StoredResource) {
	expected, err := parseReBACIfMatch(r.Header.Get("If-Match"))
	if err != nil {
		status := http.StatusBadRequest
		if r.Header.Get("If-Match") == "" {
			status = http.StatusPreconditionRequired
		}
		writeReBACError(w, err, status)
		return
	}
	var body struct {
		AASIdentifiers []string `json:"aasIdentifiers"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&body); err != nil {
		writeReBACError(w, fmt.Errorf("REBAC-INHERITANCE-BODY invalid document"), http.StatusBadRequest)
		return
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		writeReBACError(w, fmt.Errorf("REBAC-INHERITANCE-BODY trailing content"), http.StatusBadRequest)
		return
	}
	if err = s.recordDecision(r.Context(), resource.Kind+":"+resource.Identifier, "inheritance_replace_intent", "rebac", "requested"); err != nil {
		writeManagementError(w, err)
		return
	}
	tx, err := s.runtime.State.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeManagementError(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	service := rebac.InheritanceService{State: s.runtime.State, Checker: s.runtime.Client, Audit: s.grantService().Audit}
	revision, err := service.ReplaceAASParents(r.Context(), tx, request.actor, resource, expected, body.AASIdentifiers)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		_ = tx.Rollback()
		if auditErr := s.recordDecision(r.Context(), resource.Kind+":"+resource.Identifier, "inheritance_replace_result", "rebac", "failed"); auditErr != nil {
			err = auditErr
		}
		writeManagementError(w, err)
		return
	}
	location := strings.TrimSuffix(r.URL.Path, "/inheritance")
	w.Header().Set("Location", location)
	writeReBACJSON(w, map[string]any{"revision": revision, "statusURL": location}, http.StatusAccepted)
}

type rebacEffectiveOperation struct{ permission, method, path string }

func effectiveReBACOperations(kind, path string) []rebacEffectiveOperation {
	operations := []rebacEffectiveOperation{{"read", "GET", path}, {"update", "PUT", path}, {"delete", "DELETE", path}, {"manage", "", path}}
	switch kind {
	case "repository":
		return []rebacEffectiveOperation{{"create", "POST", path}, {"manage", "", path}}
	case "submodel":
		operations = append(operations, rebacEffectiveOperation{"create_child", "POST", path + "/submodel-elements"})
	case "element":
		operations = append(operations, rebacEffectiveOperation{"create_child", "POST", path}, rebacEffectiveOperation{"execute", "POST", path + "/invoke"})
	}
	return operations
}

func (s *rebacSecurity) effectiveDecision(r *http.Request, request *rebacRequest, object string, operation rebacEffectiveOperation) (rebac.Decision, error) {
	allowed, err := s.runtime.Client.Check(r.Context(), request.actor.User, operation.permission, object, request.actor.Groups)
	if err != nil {
		return rebac.Decision{}, err
	}
	decision := rebac.Decision{Allowed: allowed, Source: "rebac"}
	if operation.permission == "manage" {
		if request.actor.Administrator {
			decision = rebac.Decision{Allowed: true, Source: "administrator"}
		}
		return decision, nil
	}
	if allowed {
		return decision, nil
	}
	model := activeAccessModel(s.settings)
	if model == nil {
		return rebac.Decision{}, rebac.ErrForbidden
	}
	options := grammar.DefaultSimplifyOptions()
	options.EnableImplicitCasts = s.settings.EnableImplicitCasts
	evaluation := model.AuthorizeWithFilterWithOptions(EvalInput{Method: operation.method, Path: operation.path, RoutePath: operation.path, Claims: ClaimsFromContext(r.Context())}, options)
	return rebac.Decision{Allowed: evaluation.Allowed, Source: "abac", Partial: evaluation.QueryFilter != nil}, nil
}
