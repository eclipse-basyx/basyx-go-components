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
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/go-chi/chi/v5"
)

type managementRoute struct {
	method  string
	pattern string
	handler func(*Repository) http.HandlerFunc
}

var managementRoutes = []managementRoute{
	{method: http.MethodGet, pattern: "/", handler: listVersionsHandler},
	{method: http.MethodPost, pattern: "/", handler: importPolicyHandler},
	{method: http.MethodGet, pattern: "/{versionID}", handler: getVersionHandler},
	{method: http.MethodPost, pattern: "/{versionID}/clone", handler: cloneVersionHandler},
	{method: http.MethodPost, pattern: "/{versionID}/validate", handler: validateVersionHandler},
	{method: http.MethodPost, pattern: "/{versionID}/activate", handler: activateVersionHandler},
	{method: http.MethodPost, pattern: "/{versionID}/reject", handler: rejectVersionHandler},
	{method: http.MethodGet, pattern: "/{versionID}/rules", handler: listRulesHandler},
	{method: http.MethodPost, pattern: "/{versionID}/rules", handler: createRuleHandler},
	{method: http.MethodGet, pattern: "/{versionID}/rules/{ruleIndex}", handler: getRuleHandler},
	{method: http.MethodPut, pattern: "/{versionID}/rules/{ruleIndex}", handler: replaceRuleHandler},
	{method: http.MethodPatch, pattern: "/{versionID}/rules/{ruleIndex}", handler: patchRuleHandler},
	{method: http.MethodDelete, pattern: "/{versionID}/rules/{ruleIndex}", handler: deleteRuleHandler},
	{method: http.MethodPost, pattern: "/{versionID}/rules/{ruleIndex}/duplicate", handler: duplicateRuleHandler},
	{method: http.MethodPost, pattern: "/{versionID}/rules/{ruleIndex}/move", handler: moveRuleHandler},
	{method: http.MethodPut, pattern: "/{versionID}/rules/{ruleIndex}/enabled", handler: setRuleEnabledHandler},
}

// SetupSecurityWithABACRepository imports or loads the active PostgreSQL ABAC
// policy and installs OIDC/ABAC middleware. Callers mount management routes
// after every service middleware has been registered.
func SetupSecurityWithABACRepository(
	ctx context.Context,
	cfg *common.Config,
	r *chi.Mux,
	db *sql.DB,
	serviceScope string,
	claimsMiddleware ...func(http.Handler) http.Handler,
) (*Repository, error) {
	if cfg == nil || !cfg.ABAC.Enabled {
		return nil, nil
	}
	repo, err := NewRepository(db, serviceScope, r, cfg.Server.ContextPath)
	if err != nil {
		return nil, err
	}
	mode, err := resolvePolicyFileImportMode(cfg.ABAC.PolicyFileImport, serviceScope)
	if err != nil {
		return nil, err
	}
	if err = initializeRepository(ctx, repo, cfg.ABAC.ModelPath, serviceScope, mode); err != nil {
		return nil, err
	}
	if err = auth.SetupSecurityWithAccessModelProvider(ctx, cfg, r, repo, claimsMiddleware...); err != nil {
		return nil, err
	}
	return repo, nil
}

// ManagementAPIAllowed reports whether a service scope may expose the ABAC
// management API. Digital Twin Registry deliberately keeps its preconfigured
// access-rule file as the only policy source of truth.
func ManagementAPIAllowed(serviceScope string) bool {
	return !strings.EqualFold(strings.TrimSpace(serviceScope), "digitaltwinregistryservice")
}

// ManagementRoutesEnabled reports whether this service should expose ABAC
// policy management routes.
func ManagementRoutesEnabled(cfg *common.Config, serviceScope string) bool {
	return cfg != nil && cfg.ABAC.Enabled && cfg.ABAC.ManagementAPI.Enabled && ManagementAPIAllowed(serviceScope)
}

// RegisterManagementRoutesIfEnabled mounts the ABAC policy management API when
// configuration allows it and a repository-backed ABAC policy is active.
func RegisterManagementRoutesIfEnabled(cfg *common.Config, r chi.Router, repo *Repository, serviceScope string) {
	if repo == nil || !ManagementRoutesEnabled(cfg, serviceScope) {
		return
	}
	RegisterManagementRoutes(r, repo)
}

// ExemptManagementMutationRoutesIfEnabled marks ABAC management mutations as
// outside AAS payload history. These endpoints write their own policy events and
// activation evidence instead.
func ExemptManagementMutationRoutesIfEnabled(cfg *common.Config, guard *history.MutationCoverageGuard, serviceScope string) {
	if guard == nil || !ManagementRoutesEnabled(cfg, serviceScope) {
		return
	}
	for _, route := range managementRoutes {
		if isManagementMutation(route.method) {
			guard.Exempt(route.method, route.fullPattern())
		}
	}
}

func resolvePolicyFileImportMode(configuredMode string, serviceScope string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(configuredMode))
	if mode == "" {
		return defaultPolicyFileImportMode(serviceScope), nil
	}
	switch mode {
	case common.ABACPolicyFileImportAlways, common.ABACPolicyFileImportIfMissing, common.ABACPolicyFileImportNever:
		return mode, nil
	default:
		return "", common.NewErrBadRequest("ABACPOLICY-STARTUP-IMPORTMODE unsupported abac.policyFileImport " + configuredMode)
	}
}

func defaultPolicyFileImportMode(serviceScope string) string {
	if strings.EqualFold(strings.TrimSpace(serviceScope), "digitaltwinregistryservice") {
		return common.ABACPolicyFileImportAlways
	}
	return common.ABACPolicyFileImportIfMissing
}

func initializeRepository(ctx context.Context, repo *Repository, modelPath string, serviceScope string, mode string) error {
	switch mode {
	case common.ABACPolicyFileImportAlways:
		return importStartupFile(ctx, repo, modelPath, serviceScope)
	case common.ABACPolicyFileImportIfMissing:
		return importStartupFileIfMissing(ctx, repo, modelPath, serviceScope)
	case common.ABACPolicyFileImportNever:
		return repo.RefreshActiveModel(ctx)
	default:
		return common.NewErrBadRequest("ABACPOLICY-STARTUP-IMPORTMODE unsupported abac.policyFileImport " + mode)
	}
}

func importStartupFileIfMissing(ctx context.Context, repo *Repository, modelPath string, serviceScope string) error {
	hasActive, err := repo.HasActivePolicy(ctx)
	if err != nil {
		return err
	}
	if hasActive {
		return repo.RefreshActiveModel(ctx)
	}
	return importStartupFile(ctx, repo, modelPath, serviceScope)
}

func importStartupFile(ctx context.Context, repo *Repository, modelPath string, serviceScope string) error {
	if strings.TrimSpace(modelPath) == "" {
		return common.NewErrBadRequest("ABACPOLICY-STARTUP-MODELPATH abac.modelPath is required for startup policy file import")
	}
	//nolint:gosec // abac.modelPath is trusted service configuration, not request input.
	data, err := os.ReadFile(modelPath)
	if err != nil {
		return common.NewInternalServerError("ABACPOLICY-STARTUP-READFILE " + err.Error())
	}
	systemCtx := history.ContextWithSystemAudit(ctx, history.SystemAuditOptions{
		ActorSubject: "system:abac-preconfiguration",
		ActorIssuer:  "basyx:" + serviceScope,
		ClientID:     serviceScope,
		Operation:    "ABACPreconfiguration",
		Endpoint:     "startup:abac-preconfiguration",
		HTTPMethod:   history.AuditHTTPMethodSystem,
		IDPrefix:     "abac-preconfiguration",
	})
	_, err = repo.ImportStartupPolicy(systemCtx, data, modelPath)
	return err
}

// RegisterManagementRoutes mounts the ABAC policy management API.
func RegisterManagementRoutes(r chi.Router, repo *Repository) {
	r.Route(managementBasePath, func(policyRouter chi.Router) {
		for _, route := range managementRoutes {
			policyRouter.Method(route.method, route.pattern, route.handler(repo))
		}
	})
}

func (route managementRoute) fullPattern() string {
	if strings.TrimSpace(route.pattern) == "/" {
		return managementBasePath
	}
	return managementBasePath + route.pattern
}

func isManagementMutation(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func listVersionsHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versions, err := repo.ListPolicyVersions(r.Context())
		writeResult(w, versions, err, http.StatusOK)
	}
}

func importPolicyHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request PolicyImportRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeError(w, err)
			return
		}
		if len(request.Policy) == 0 {
			writeError(w, common.NewErrBadRequest("ABACPOLICY-API-IMPORT-POLICY policy is required"))
			return
		}
		version, err := repo.ImportPolicy(r.Context(), request.Policy, request.SourceRef)
		if err == nil && request.Activate {
			version, err = repo.ActivatePolicy(r.Context(), version.VersionID)
		}
		writeResult(w, version, err, http.StatusCreated)
	}
}

func getVersionHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ok := pathInt64(w, r, "versionID")
		if !ok {
			return
		}
		version, err := repo.GetPolicyVersion(r.Context(), versionID)
		writeResult(w, version, err, http.StatusOK)
	}
}

func cloneVersionHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ok := pathInt64(w, r, "versionID")
		if !ok {
			return
		}
		version, err := repo.ClonePolicyVersion(r.Context(), versionID)
		writeResult(w, version, err, http.StatusCreated)
	}
}

func validateVersionHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ok := pathInt64(w, r, "versionID")
		if !ok {
			return
		}
		result, err := repo.ValidatePolicy(r.Context(), versionID)
		writeResult(w, result, err, http.StatusOK)
	}
}

func activateVersionHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ok := pathInt64(w, r, "versionID")
		if !ok {
			return
		}
		version, err := repo.ActivatePolicy(r.Context(), versionID)
		writeResult(w, version, err, http.StatusOK)
	}
}

func rejectVersionHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ok := pathInt64(w, r, "versionID")
		if !ok {
			return
		}
		version, err := repo.RejectPolicy(r.Context(), versionID)
		writeResult(w, version, err, http.StatusOK)
	}
}

func listRulesHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ok := pathInt64(w, r, "versionID")
		if !ok {
			return
		}
		rules, err := repo.ListRules(r.Context(), versionID)
		writeResult(w, rules, err, http.StatusOK)
	}
}

func getRuleHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ruleIndex, ok := versionAndRuleIndex(w, r)
		if !ok {
			return
		}
		rules, err := repo.ListRules(r.Context(), versionID)
		if err == nil {
			err = validateRuleIndex(ruleIndex, len(rules))
		}
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, rules[ruleIndex-1], http.StatusOK)
	}
}

func createRuleHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ok := pathInt64(w, r, "versionID")
		if !ok {
			return
		}
		request, err := decodeRuleMutation(r)
		if err != nil {
			writeError(w, err)
			return
		}
		version, err := repo.CreateRule(r.Context(), versionID, request)
		writeResult(w, version, err, http.StatusOK)
	}
}

func replaceRuleHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ruleIndex, ok := versionAndRuleIndex(w, r)
		if !ok {
			return
		}
		raw, err := readBody(r)
		if err != nil {
			writeError(w, err)
			return
		}
		version, err := repo.ReplaceRule(r.Context(), versionID, ruleIndex, raw)
		writeResult(w, version, err, http.StatusOK)
	}
}

func patchRuleHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ruleIndex, ok := versionAndRuleIndex(w, r)
		if !ok {
			return
		}
		raw, err := readBody(r)
		if err != nil {
			writeError(w, err)
			return
		}
		version, err := repo.PatchRule(r.Context(), versionID, ruleIndex, raw)
		writeResult(w, version, err, http.StatusOK)
	}
}

func deleteRuleHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ruleIndex, ok := versionAndRuleIndex(w, r)
		if !ok {
			return
		}
		version, err := repo.DeleteRule(r.Context(), versionID, ruleIndex)
		writeResult(w, version, err, http.StatusOK)
	}
}

func duplicateRuleHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ruleIndex, ok := versionAndRuleIndex(w, r)
		if !ok {
			return
		}
		var request MoveRuleRequest
		_ = decodeOptionalJSONBody(r, &request)
		version, err := repo.DuplicateRule(r.Context(), versionID, ruleIndex, request.Position)
		writeResult(w, version, err, http.StatusOK)
	}
}

func moveRuleHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ruleIndex, ok := versionAndRuleIndex(w, r)
		if !ok {
			return
		}
		var request MoveRuleRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeError(w, err)
			return
		}
		version, err := repo.MoveRule(r.Context(), versionID, ruleIndex, request.Position)
		writeResult(w, version, err, http.StatusOK)
	}
}

func setRuleEnabledHandler(repo *Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versionID, ruleIndex, ok := versionAndRuleIndex(w, r)
		if !ok {
			return
		}
		var request SetRuleEnabledRequest
		if err := decodeJSONBody(r, &request); err != nil {
			writeError(w, err)
			return
		}
		version, err := repo.SetRuleEnabled(r.Context(), versionID, ruleIndex, request.Enabled)
		writeResult(w, version, err, http.StatusOK)
	}
}

func versionAndRuleIndex(w http.ResponseWriter, r *http.Request) (int64, int, bool) {
	versionID, ok := pathInt64(w, r, "versionID")
	if !ok {
		return 0, 0, false
	}
	ruleIndex, ok := pathInt(w, r, "ruleIndex")
	return versionID, ruleIndex, ok
}

func pathInt64(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	value, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || value < 1 {
		writeError(w, common.NewErrBadRequest("ABACPOLICY-API-PATH invalid "+name))
		return 0, false
	}
	return value, true
}

func pathInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	value, err := strconv.Atoi(chi.URLParam(r, name))
	if err != nil || value < 1 {
		writeError(w, common.NewErrBadRequest("ABACPOLICY-API-PATH invalid "+name))
		return 0, false
	}
	return value, true
}

func decodeRuleMutation(r *http.Request) (RuleMutationRequest, error) {
	raw, err := readBody(r)
	if err != nil {
		return RuleMutationRequest{}, err
	}
	var wrapper RuleMutationRequest
	if err = json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Rule) > 0 {
		return wrapper, nil
	}
	return RuleMutationRequest{Rule: raw}, nil
}

func decodeJSONBody(r *http.Request, target any) error {
	raw, err := readBody(r)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, target); err != nil {
		return common.NewErrBadRequest("ABACPOLICY-API-DECODE " + err.Error())
	}
	return nil
}

func decodeOptionalJSONBody(r *http.Request, target any) error {
	raw, err := readBody(r)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	if err = json.Unmarshal(raw, target); err != nil {
		return common.NewErrBadRequest("ABACPOLICY-API-DECODE " + err.Error())
	}
	return nil
}

func readBody(r *http.Request) (json.RawMessage, error) {
	defer func() {
		_ = r.Body.Close()
	}()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		return nil, common.NewErrBadRequest("ABACPOLICY-API-READBODY " + err.Error())
	}
	return json.RawMessage(raw), nil
}

func writeResult(w http.ResponseWriter, body any, err error, status int) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, body, status)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case common.IsErrBadRequest(err):
		status = http.StatusBadRequest
	case common.IsErrConflict(err):
		status = http.StatusConflict
	case common.IsErrNotFound(err):
		status = http.StatusNotFound
	case common.IsErrDenied(err):
		status = http.StatusForbidden
	case common.IsErrServiceUnavailable(err):
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, map[string]any{"error": err.Error()}, status)
}

func writeJSON(w http.ResponseWriter, body any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(body); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		http.Error(w, "ABACPOLICY-API-ENCODE "+err.Error(), http.StatusInternalServerError)
	}
}
