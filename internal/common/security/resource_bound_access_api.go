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
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/google/uuid"
)

type boundHTTPError struct {
	status  int
	message string
}

func (err *boundHTTPError) Error() string {
	if err.status == http.StatusForbidden {
		return "403 Denied: " + err.message
	}
	return err.message
}
func boundError(status int, message string) error {
	return &boundHTTPError{status: status, message: "REBAC-ACCESS-" + message}
}
func boundETag(revision int64, access, effective *boundAccess) string {
	effectiveID, effectiveRevision := int64(0), int64(0)
	if effective != nil {
		effectiveID, effectiveRevision = effective.ID, effective.Revision
	}
	key, _ := ResourceBoundKey(access.Resource)
	state := fmt.Sprintf("%d:%d:%d:%d:%s", access.ID, revision, effectiveID, effectiveRevision, key)
	return fmt.Sprintf("\"%x\"", sha256.Sum256([]byte(state)))
}

func boundActor(ctx context.Context) (common.AccessPrincipal, error) {
	claims := ClaimsFromContext(ctx)
	issuer, _ := claims.GetString("iss")
	subject, _ := claims.GetString("sub")
	if strings.TrimSpace(issuer) == "" || strings.TrimSpace(subject) == "" {
		return common.AccessPrincipal{}, boundError(http.StatusUnauthorized, "IDENTITY authenticated issuer and subject required")
	}
	return common.AccessPrincipal{Type: common.AccessPrincipalUser, Issuer: issuer, Subject: subject}, nil
}
func containsBoundPrincipal(principals []common.AccessPrincipal, principal common.AccessPrincipal) bool {
	for _, candidate := range principals {
		if candidate.NormalizedType() == principal.NormalizedType() && candidate.Issuer == principal.Issuer && candidate.Subject == principal.Subject {
			return true
		}
	}
	return false
}

func (repo *resourceBoundRepository) serveAccess(w http.ResponseWriter, r *http.Request, target boundTarget) {
	status, value, tag, err := repo.accessRequest(r, target)
	if err != nil {
		writeBoundError(w, err)
		return
	}
	if tag != "" {
		w.Header().Set("ETag", tag)
	}
	if status == http.StatusCreated {
		if grant, ok := value.(boundGrant); ok {
			w.Header().Set("Location", strings.TrimRight(r.URL.Path, "/")+"/"+grant.ID)
		}
	}
	if value != nil {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	if value != nil {
		if err = json.NewEncoder(w).Encode(value); err != nil {
			return
		}
	}
}

func writeBoundError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var apiErr *boundHTTPError
	if errors.As(err, &apiErr) {
		status = apiErr.status
	} else if errors.Is(err, sql.ErrNoRows) {
		status = http.StatusNotFound
	}
	_ = common.WriteErrorResponse(w, err, status, "Security", "ResourceBound", "Access")
}

func (repo *resourceBoundRepository) accessRequest(r *http.Request, target boundTarget) (int, any, string, error) {
	actor, err := boundActor(r.Context())
	if err != nil {
		return 0, nil, "", err
	}
	write := r.Method != http.MethodGet
	tx, err := repo.db.BeginTx(r.Context(), nil)
	if err != nil {
		return 0, nil, "", fmt.Errorf("REBAC-ACCESS-BEGIN %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	revision, err := repo.lock(r.Context(), tx, write)
	if err != nil {
		return 0, nil, "", err
	}
	access, err := repo.load(r.Context(), tx, target)
	if err != nil {
		return 0, nil, "", err
	}
	effective, err := repo.effective(r.Context(), tx, target)
	if err != nil {
		return 0, nil, "", err
	}
	actors := repo.boundRequestPrincipals(r.Context())
	owner := containsAnyBoundPrincipal(access.Owners, actors)
	manager := effective != nil && containsAnyBoundPrincipal(effective.Managers, actors)
	if !owner && !manager {
		return 0, nil, "", boundError(http.StatusForbidden, "DENIED access administration required")
	}
	if !write {
		status, value, err := boundAccessRead(target, access, effective)
		return status, value, boundETag(revision, access, effective), err
	}
	if r.Header.Get("If-Match") == "" {
		return 0, nil, "", boundError(http.StatusPreconditionRequired, "PRECONDITION If-Match required")
	}
	if r.Header.Get("If-Match") != boundETag(revision, access, effective) {
		return 0, nil, "", boundError(http.StatusPreconditionFailed, "REVISION stale access revision")
	}
	status, value, err := mutateBoundAccess(r, tx, target, access, owner)
	if err != nil {
		return 0, nil, "", err
	}
	etag, err := repo.commitAccess(r, tx, target, access, actor, revision+1)
	return status, value, etag, err
}

func containsAnyBoundPrincipal(allowed, actors []common.AccessPrincipal) bool {
	for _, actor := range actors {
		if containsBoundPrincipal(allowed, actor) {
			return true
		}
	}
	return false
}

func (repo *resourceBoundRepository) commitAccess(r *http.Request, tx *sql.Tx, target boundTarget, access *boundAccess, actor common.AccessPrincipal, revision int64) (string, error) {
	if err := repo.save(r.Context(), tx, access, actor); err != nil {
		return "", err
	}
	effective, err := repo.effective(r.Context(), tx, target)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", fmt.Errorf("REBAC-ACCESS-COMMIT %w", err)
	}
	return boundETag(revision, access, effective), nil
}

func boundAccessRead(target boundTarget, access, effective *boundAccess) (int, any, error) {
	switch target.Suffix {
	case "":
		var policy *ResourceBoundPolicy
		if effective != nil {
			policy = effective.Policy
		}
		return http.StatusOK, struct {
			*boundAccess
			EffectivePolicy *ResourceBoundPolicy `json:"effectivePolicy"`
		}{access, policy}, nil
	case "policy":
		if access.Policy == nil {
			return 0, nil, boundError(http.StatusNotFound, "NOPOLICY no directly bound policy")
		}
		return http.StatusOK, access.Policy, nil
	}
	return 0, nil, boundError(http.StatusNotFound, "ROUTE unknown access endpoint")
}

func decodeBoundBody(r *http.Request, value any) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil {
		return boundError(http.StatusBadRequest, "BODY unreadable body")
	}
	if len(data) > 1<<20 {
		return boundError(http.StatusRequestEntityTooLarge, "BODY request exceeds 1 MiB")
	}
	if err = common.UnmarshalAndDisallowUnknownFields(data, value); err != nil {
		return boundError(http.StatusBadRequest, "BODY "+err.Error())
	}
	return nil
}

func mutateBoundAccess(r *http.Request, tx *sql.Tx, target boundTarget, access *boundAccess, owner bool) (int, any, error) {
	switch target.Suffix {
	case "policy":
		return mutateBoundPolicy(r, tx, target, access)
	case "owners", "managers":
		return mutateBoundPrincipals(r, tx, target.Suffix, access, owner)
	case "grants":
		if r.Method != http.MethodPost {
			return 0, nil, boundError(http.StatusMethodNotAllowed, "METHOD expected POST")
		}
		return mutateBoundGrant(r, tx, access, "")
	default:
		if strings.HasPrefix(target.Suffix, "grants/") && !strings.Contains(strings.TrimPrefix(target.Suffix, "grants/"), "/") {
			return mutateBoundGrant(r, tx, access, strings.TrimPrefix(target.Suffix, "grants/"))
		}
	}
	return 0, nil, boundError(http.StatusNotFound, "ROUTE unknown access endpoint")
}

func mutateBoundPolicy(r *http.Request, tx *sql.Tx, target boundTarget, access *boundAccess) (int, any, error) {
	switch r.Method {
	case http.MethodDelete:
		if access.Policy == nil {
			return 0, nil, boundError(http.StatusNotFound, "NOPOLICY no directly bound policy")
		}
		if err := clearBoundDelegation(r.Context(), tx, access.ID); err != nil {
			return 0, nil, err
		}
		access.Policy = nil
		access.Managers = nil
		access.Grants = nil
		return http.StatusNoContent, nil, nil
	case http.MethodPut:
		var policy ResourceBoundPolicy
		if err := decodeBoundBody(r, &policy); err != nil {
			return 0, nil, err
		}
		if !boundObjectsEqual(policy.Resource, target.object()) {
			return 0, nil, boundError(http.StatusBadRequest, "RESOURCE binding differs from addressed resource")
		}
		if _, err := CompileResourceBoundPolicy(policy, nil, ""); err != nil {
			return 0, nil, boundError(http.StatusBadRequest, "POLICY "+err.Error())
		}
		if err := checkBoundManagerRules(policy, access.Managers); err != nil {
			return 0, nil, err
		}
		if err := reconcileBoundGrants(r.Context(), tx, access, policy); err != nil {
			return 0, nil, err
		}
		access.Policy = &policy
		return http.StatusOK, policy, nil
	}
	return 0, nil, boundError(http.StatusMethodNotAllowed, "METHOD expected PUT or DELETE")
}

func sameBoundRule(left, right json.RawMessage) bool {
	var a, b any
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}
func hasBoundRule(policy ResourceBoundPolicy, rule json.RawMessage) bool {
	for _, candidate := range policy.Rules {
		if sameBoundRule(candidate, rule) {
			return true
		}
	}
	return false
}
func removeBoundRule(policy *ResourceBoundPolicy, rule json.RawMessage) {
	for index, candidate := range policy.Rules {
		if sameBoundRule(candidate, rule) {
			policy.Rules = append(policy.Rules[:index], policy.Rules[index+1:]...)
			return
		}
	}
}
func checkBoundManagerRules(policy ResourceBoundPolicy, managers []common.AccessPrincipal) error {
	for _, principal := range managers {
		rule, err := boundPrincipalRule(principal, []grammar.RightsEnum{grammar.RightsEnumALL})
		if err != nil {
			return err
		}
		if !hasBoundRule(policy, rule) {
			return boundError(http.StatusConflict, "MANAGERS change protected rules through /managers")
		}
	}
	return nil
}
func clearBoundDelegation(ctx context.Context, tx *sql.Tx, id int64) error {
	dialect := goqu.Dialect("postgres")
	if err := boundExec(ctx, tx, dialect.Delete("rebac_grant").Where(goqu.Ex{"access_id": id}).Prepared(true)); err != nil {
		return err
	}
	return boundExec(ctx, tx, dialect.Delete("rebac_principal").Where(goqu.Ex{"access_id": id, "relation": "manager"}).Prepared(true))
}
func reconcileBoundGrants(ctx context.Context, tx *sql.Tx, access *boundAccess, policy ResourceBoundPolicy) error {
	for _, grant := range access.Grants {
		if !hasBoundRule(policy, grant.Rule) {
			if err := boundExec(ctx, tx, goqu.Dialect("postgres").Delete("rebac_grant").Where(goqu.Ex{"access_id": access.ID, "id": grant.ID}).Prepared(true)); err != nil {
				return err
			}
		}
	}
	return nil
}

func mutateBoundPrincipals(r *http.Request, tx *sql.Tx, kind string, access *boundAccess, owner bool) (int, any, error) {
	if r.Method != http.MethodPut {
		return 0, nil, boundError(http.StatusMethodNotAllowed, "METHOD expected PUT")
	}
	if kind == "owners" && !owner {
		return 0, nil, boundError(http.StatusForbidden, "OWNERS only direct owners may change ownership")
	}
	if kind == "managers" && access.Policy == nil {
		return 0, nil, boundError(http.StatusConflict, "INHERITED create a local policy explicitly first")
	}
	var principals []common.AccessPrincipal
	if err := decodeBoundBody(r, &principals); err != nil {
		return 0, nil, err
	}
	if err := validateBoundPrincipals(principals, kind == "owners"); err != nil {
		return 0, nil, err
	}
	if kind == "managers" {
		if err := replaceBoundManagerRules(access, principals); err != nil {
			return 0, nil, err
		}
	}
	relation := strings.TrimSuffix(kind, "s")
	ds := goqu.Dialect("postgres").Delete("rebac_principal").Where(goqu.Ex{"access_id": access.ID, "relation": relation}).Prepared(true)
	if err := boundExec(r.Context(), tx, ds); err != nil {
		return 0, nil, err
	}
	for _, principal := range principals {
		record := boundPrincipalRecord(access.ID, principal, relation)
		if err := boundExec(r.Context(), tx, goqu.Dialect("postgres").Insert("rebac_principal").Rows(record).Prepared(true)); err != nil {
			return 0, nil, err
		}
	}
	return http.StatusOK, principals, nil
}
func validateBoundPrincipals(principals []common.AccessPrincipal, requireOwner bool) error {
	if principals == nil || (requireOwner && len(principals) == 0) {
		return boundError(http.StatusBadRequest, "PRINCIPALS array required; at least one owner must remain")
	}
	seen := map[common.AccessPrincipal]bool{}
	for index := range principals {
		principal := &principals[index]
		if strings.TrimSpace(principal.Issuer) == "" || strings.TrimSpace(principal.Subject) == "" {
			return boundError(http.StatusBadRequest, "PRINCIPALS issuer and subject must contain non-whitespace characters")
		}
		principal.Type = principal.NormalizedType()
		if principal.Type != common.AccessPrincipalUser && principal.Type != common.AccessPrincipalGroup {
			return boundError(http.StatusBadRequest, "PRINCIPALS type must be user or group")
		}
		if seen[*principal] {
			return boundError(http.StatusBadRequest, "PRINCIPALS duplicate principal")
		}
		seen[*principal] = true
	}
	return nil
}

func validateBoundRights(rights []grammar.RightsEnum) error {
	if len(rights) == 0 {
		return boundError(http.StatusBadRequest, "RIGHTS at least one right required")
	}
	seen := make(map[grammar.RightsEnum]bool, len(rights))
	for _, right := range rights {
		if seen[right] {
			return boundError(http.StatusBadRequest, "RIGHTS duplicate right "+string(right))
		}
		seen[right] = true
	}
	return nil
}
func replaceBoundManagerRules(access *boundAccess, principals []common.AccessPrincipal) error {
	for _, principal := range access.Managers {
		rule, err := boundPrincipalRule(principal, []grammar.RightsEnum{grammar.RightsEnumALL})
		if err != nil {
			return err
		}
		removeBoundRule(access.Policy, rule)
	}
	for _, principal := range principals {
		rule, err := boundPrincipalRule(principal, []grammar.RightsEnum{grammar.RightsEnumALL})
		if err != nil {
			return err
		}
		access.Policy.Rules = append(access.Policy.Rules, rule)
	}
	access.Managers = principals
	return nil
}

func mutateBoundGrant(r *http.Request, tx *sql.Tx, access *boundAccess, id string) (int, any, error) {
	if access.Policy == nil {
		return 0, nil, boundError(http.StatusConflict, "INHERITED create a local policy explicitly first")
	}
	if err := prepareBoundGrant(r, tx, access, id); err != nil {
		return 0, nil, err
	}
	if r.Method == http.MethodDelete {
		return http.StatusNoContent, nil, nil
	}
	var input struct {
		Principal common.AccessPrincipal `json:"principal"`
		Rights    []grammar.RightsEnum   `json:"rights"`
	}
	if err := decodeBoundBody(r, &input); err != nil {
		return 0, nil, err
	}
	input.Principal.Type = input.Principal.NormalizedType()
	if err := validateBoundPrincipals([]common.AccessPrincipal{input.Principal}, false); err != nil {
		return 0, nil, err
	}
	if err := validateBoundRights(input.Rights); err != nil {
		return 0, nil, err
	}
	status := http.StatusOK
	if id == "" {
		id = uuid.NewString()
		status = http.StatusCreated
	}
	rule, err := boundManagedGrantRule(input.Principal, input.Rights, id)
	if err != nil {
		return 0, nil, boundError(http.StatusBadRequest, "RIGHTS "+err.Error())
	}
	grant := boundGrant{ID: id, Principal: input.Principal, Rights: input.Rights, Rule: rule}
	rights, err := json.Marshal(input.Rights)
	if err != nil {
		return 0, nil, fmt.Errorf("REBAC-GRANT-ENCODE %w", err)
	}
	record := goqu.Record{"id": id, "access_id": access.ID, "principal_type": input.Principal.NormalizedType(), "issuer": input.Principal.Issuer, "subject": input.Principal.Subject, "rights": string(rights), "rule": string(rule)}
	if err = boundExec(r.Context(), tx, goqu.Dialect("postgres").Insert("rebac_grant").Rows(record).Prepared(true)); err != nil {
		return 0, nil, err
	}
	access.Policy.Rules = append(access.Policy.Rules, rule)
	return status, grant, nil
}
func prepareBoundGrant(r *http.Request, tx *sql.Tx, access *boundAccess, id string) error {
	if id != "" {
		if _, err := uuid.Parse(id); err != nil {
			return boundError(http.StatusBadRequest, "GRANT invalid grant ID")
		}
		if r.Method != http.MethodPut && r.Method != http.MethodDelete {
			return boundError(http.StatusMethodNotAllowed, "METHOD expected PUT or DELETE")
		}
		if err := removeManagedBoundGrant(r.Context(), tx, access, id); err != nil {
			return err
		}
	}
	return nil
}

func removeManagedBoundGrant(ctx context.Context, tx *sql.Tx, access *boundAccess, id string) error {
	for _, grant := range access.Grants {
		if grant.ID == id {
			removeBoundRule(access.Policy, grant.Rule)
			return boundExec(ctx, tx, goqu.Dialect("postgres").Delete("rebac_grant").Where(goqu.Ex{"access_id": access.ID, "id": id}).Prepared(true))
		}
	}
	return boundError(http.StatusNotFound, "GRANT unknown managed grant")
}

func boundManagedGrantRule(principal common.AccessPrincipal, rights []grammar.RightsEnum, id string) (json.RawMessage, error) {
	raw, err := boundPrincipalRule(principal, rights)
	if err != nil {
		return nil, err
	}
	var rule map[string]any
	if err = json.Unmarshal(raw, &rule); err != nil {
		return nil, fmt.Errorf("REBAC-GRANTRULE-DECODE %w", err)
	}
	rule["FORMULA"] = map[string]any{"$and": []any{rule["FORMULA"], map[string]any{"$eq": []any{map[string]string{"$strVal": id}, map[string]string{"$strVal": id}}}}}
	raw, err = json.Marshal(rule)
	if err != nil {
		return nil, fmt.Errorf("REBAC-GRANTRULE-ENCODE %w", err)
	}
	return raw, nil
}
