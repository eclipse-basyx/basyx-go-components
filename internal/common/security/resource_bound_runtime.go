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
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/go-chi/chi/v5"
)

// SetupResourceBoundSecurity installs resource-bound authorization with optional ABAC fallback.
func SetupResourceBoundSecurity(ctx context.Context, cfg *common.Config, router *chi.Mux, db *sql.DB, fallback AccessModelProvider, claimsMiddleware ...func(http.Handler) http.Handler) error {
	scope := cfg.ReBAC.PolicyScope
	if scope == "" {
		scope = "default"
	}
	repo := &resourceBoundRepository{db: db, scope: scope, router: router, basePath: cfg.Server.ContextPath, fallback: fallback, implicitCasts: cfg.General.EnableImplicitCasts}
	if err := repo.initialize(ctx, cfg.ReBAC); err != nil {
		return err
	}
	oidc, err := setupOIDC(ctx, cfg)
	if err != nil {
		return err
	}
	applySecurityMiddleware(router, oidc.Middleware, repo.middleware, claimsMiddleware...)
	return nil
}

func (repo *resourceBoundRepository) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := stripBasePath(repo.basePath, r.URL.Path)
		if repo.serveFallbackRoute(next, w, r, path) {
			return
		}
		pattern := repo.router.Find(chi.NewRouteContext(), r.Method, stripBasePath(repo.basePath, r.URL.EscapedPath()))
		if boundExcludedRoute(pattern) {
			writeBoundError(w, boundError(http.StatusNotImplemented, "UNSUPPORTED route not supported in resource-bound mode"))
			return
		}
		target, err := parseBoundTarget(r.URL.EscapedPath(), repo.basePath)
		if err != nil {
			writeBoundError(w, boundError(http.StatusNotFound, "ROUTE unsupported resource path"))
			return
		}
		if err := checkBoundWriteIdentity(r); err != nil {
			writeBoundError(w, err)
			return
		}
		if target.Access {
			resourcePath := strings.SplitN(stripBasePath(repo.basePath, r.URL.EscapedPath()), "/$access", 2)[0]
			if repo.router.Find(chi.NewRouteContext(), http.MethodGet, resourcePath) == "" {
				writeBoundError(w, boundError(http.StatusNotFound, "ROUTE unknown resource endpoint"))
				return
			}
			repo.serveAccess(w, r, target)
			return
		}
		repo.serveResource(next, w, r, target)
	})
}
func checkBoundWriteIdentity(r *http.Request) error {
	if r.Method == http.MethodGet {
		return nil
	}
	_, err := boundActor(r.Context())
	return err
}

func (repo *resourceBoundRepository) serveFallbackRoute(next http.Handler, w http.ResponseWriter, r *http.Request, path string) bool {
	if !strings.HasPrefix(path, "/security/abac/") && path != "/description" && path != "/verify" {
		return false
	}
	if repo.fallback == nil {
		writeBoundError(w, boundError(http.StatusForbidden, "FALLBACK no ABAC policy"))
		return true
	}
	ABACMiddleware(ABACSettings{Enabled: true, ModelProvider: repo.fallback, EnableImplicitCasts: repo.implicitCasts, DenyAsNotFoundPrefixes: abacDeniedAsNotFoundPrefixes(repo.basePath)})(next).ServeHTTP(w, r)
	return true
}

func boundExcludedRoute(path string) bool {
	for _, part := range []string{"/$history", "/$recent-changes", "/$signed", "/invoke-async", "/operation-status/", "/operation-results/", "/query", "/bulk", "/serialization", "/upload", "/packages"} {
		if strings.Contains(path, part) {
			return true
		}
	}
	return false
}

func (repo *resourceBoundRepository) serveResource(next http.Handler, w http.ResponseWriter, r *http.Request, target boundTarget) {
	tx, err := repo.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeBoundError(w, fmt.Errorf("REBAC-REQUEST-BEGIN %w", err))
		return
	}
	defer func() { _ = tx.Rollback() }()
	revision, err := repo.lock(r.Context(), tx, false)
	if err != nil {
		writeBoundError(w, err)
		return
	}
	state, err := repo.authorizeRequest(r, tx, target, revision)
	if err != nil {
		writeBoundError(w, err)
		return
	}
	state.target = target
	ctx := context.WithValue(common.WithWriterPostgresReads(r.Context()), boundRequestKey, state)
	ctx = WithQueryFilter(ctx, state.queryFilter())
	ctx = ContextWithAuthorizationDecision(ctx, AuthorizationDecision{Result: string(DecisionAllow), PolicyID: state.policyID})
	next.ServeHTTP(w, r.WithContext(ctx))
}

func (repo *resourceBoundRepository) authorizeRequest(r *http.Request, tx *sql.Tx, target boundTarget, revision int64) (*boundRequest, error) {
	input := EvalInput{Method: r.Method, Path: r.URL.Path, RoutePath: r.URL.EscapedPath(), Claims: FromContext(r)}
	right, exists, err := repo.requestRight(r.Context(), tx, input, target)
	if err != nil {
		return nil, err
	}
	state, err := repo.requestState(r.Context(), tx, target, input, right, revision)
	if err != nil {
		return nil, err
	}
	selected := target
	if !exists && r.Method == http.MethodPut {
		selected, err = creationBoundParent(target)
		if err != nil {
			return nil, err
		}
	}
	if right == grammar.RightsEnumCREATE {
		state.creationTarget = &selected
	}
	collectionRead := r.Method == http.MethodGet && (target.Kind == "shells" || target.Kind == "submodels")
	if !collectionRead {
		err = state.checkTarget(r.Context(), tx, selected)
	}
	if err != nil {
		return nil, err
	}
	if right == grammar.RightsEnumUPDATE || right == grammar.RightsEnumDELETE {
		if err = state.checkDescendants(r.Context(), tx, target); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func (repo *resourceBoundRepository) requestRight(ctx context.Context, db boundQueryer, input EvalInput, target boundTarget) (grammar.RightsEnum, bool, error) {
	model := &AccessModel{apiRouter: repo.router, basePath: repo.basePath}
	alternatives, mapped, found := model.mapMethodAndPathToRights(input)
	if !found {
		return "", false, boundError(http.StatusNotFound, "ROUTE not found")
	}
	if !mapped || len(alternatives) == 0 || len(alternatives[0]) == 0 {
		return "", false, boundError(http.StatusForbidden, "RIGHT unmapped route")
	}
	right := alternatives[0][0]
	exists := true
	if input.Method == http.MethodPut && (right == grammar.RightsEnumCREATE || right == grammar.RightsEnumUPDATE) {
		lookup := target
		if lookup.Submodel != "" {
			lookup.AAS = ""
		}
		_, err := repo.load(ctx, db, lookup)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", false, err
		}
		exists = err == nil
		right = grammar.RightsEnumCREATE
		if exists {
			right = grammar.RightsEnumUPDATE
		}
	}
	if input.Method == http.MethodGet && target.Suffix == "$reference" {
		right = grammar.RightsEnumVIEW
	}
	return right, exists, nil
}

func creationBoundParent(target boundTarget) (boundTarget, error) {
	switch target.Kind {
	case "aas":
		return boundTarget{Kind: "shells"}, nil
	case "submodel":
		if target.AAS != "" {
			return boundTarget{Kind: "aas", AAS: target.AAS}, nil
		}
		return boundTarget{Kind: "submodels"}, nil
	case "sme":
		index := strings.LastIndexAny(target.Path, ".[")
		if index < 0 {
			target.Kind = "submodel"
			target.Path = ""
		} else {
			target.Path = target.Path[:index]
		}
		return target, nil
	}
	return target, boundError(http.StatusBadRequest, "CREATE invalid creation target")
}

func boundTargetDataset(target boundTarget) (*goqu.SelectDataset, *grammar.ResolvedFieldPathCollector, error) {
	dialect := goqu.Dialect("postgres")
	var ds *goqu.SelectDataset
	var collector *grammar.ResolvedFieldPathCollector
	var err error
	switch target.Kind {
	case "aas":
		ds = dialect.From("aas").Where(goqu.Ex{"aas_id": target.AAS})
		collector, err = grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootAAS)
	case "submodel":
		ds = dialect.From("submodel").Where(goqu.Ex{"submodel_identifier": target.Submodel})
		collector, err = grammar.NewResolvedFieldPathCollectorForRoot(grammar.CollectorRootSM)
	case "sme":
		sm := dialect.From("submodel").Select("id").Where(goqu.Ex{"submodel_identifier": target.Submodel})
		ds = dialect.From(goqu.T("submodel_element").As("sme")).Where(goqu.Ex{"sme.idshort_path": target.Path}, goqu.I("sme.submodel_id").Eq(sm))
		collector, err = grammar.NewResolvedFieldPathCollectorForSMERow("sme")
	default:
		return nil, nil, fmt.Errorf("REBAC-CHECK-KIND unsupported data resource")
	}
	return ds, collector, err
}

func (state *boundRequest) checkTarget(ctx context.Context, db boundQueryer, target boundTarget) error {
	if err := validateBoundContext(ctx, db, target); err != nil {
		return err
	}
	if target.Kind == "shells" || target.Kind == "submodels" {
		return state.checkCollection(ctx, db, target)
	}
	ds, collector, err := boundTargetDataset(target)
	if err != nil {
		return err
	}
	expression, err := state.expression(collector, "")
	if err != nil {
		return err
	}
	provenance, err := state.provenance(collector)
	if err != nil {
		return err
	}
	query, args, err := ds.Select(provenance).Where(expression).Limit(1).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-CHECK-BUILD %w", err)
	}
	if err = db.QueryRowContext(ctx, query, args...).Scan(&state.policyID); errors.Is(err, sql.ErrNoRows) {
		return boundError(http.StatusForbidden, "DENIED resource action not granted")
	}
	if err != nil {
		return fmt.Errorf("REBAC-CHECK-QUERY %w", err)
	}
	return nil
}

func (state *boundRequest) checkCollection(ctx context.Context, db boundQueryer, target boundTarget) error {
	effective, err := state.repo.effective(ctx, db, target)
	if err != nil {
		return err
	}
	if effective != nil {
		for _, decision := range state.policies {
			if decision.id == effective.ID && decision.evaluation.Allowed {
				state.policyID = decision.evaluation.PolicyID
				return nil
			}
		}
	}
	if state.fallback.Allowed {
		state.policyID = state.fallback.PolicyID
		return nil
	}
	return boundError(http.StatusForbidden, "CREATE parent does not grant creation")
}

func (state *boundRequest) checkDescendants(ctx context.Context, db boundQueryer, target boundTarget) error {
	if target.Kind != "submodel" && target.Kind != "sme" {
		return nil
	}
	dialect := goqu.Dialect("postgres")
	sm := dialect.From("submodel").Select("id").Where(goqu.Ex{"submodel_identifier": target.Submodel})
	ds := dialect.From(goqu.T("submodel_element").As("sme")).Where(goqu.I("sme.submodel_id").Eq(sm))
	if target.Kind == "sme" {
		ds = ds.Where(goqu.Or(goqu.I("sme.idshort_path").Eq(target.Path), goqu.I("sme.idshort_path").Like(escapeBoundLike(target.Path)+".%"), goqu.I("sme.idshort_path").Like(escapeBoundLike(target.Path)+"[%")))
	}
	collector, err := grammar.NewResolvedFieldPathCollectorForSMERow("sme")
	if err != nil {
		return err
	}
	expression, err := state.expression(collector, "")
	if err != nil {
		return err
	}
	query, args, err := ds.Select(goqu.L("1")).Where(goqu.L("NOT (?)", expression)).Limit(1).Prepared(true).ToSQL()
	if err != nil {
		return fmt.Errorf("REBAC-DESCENDANTS-BUILD %w", err)
	}
	var denied int
	err = db.QueryRowContext(ctx, query, args...).Scan(&denied)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("REBAC-DESCENDANTS-QUERY %w", err)
	}
	return boundError(http.StatusForbidden, "DESCENDANTS mutation includes inaccessible descendants")
}
func escapeBoundLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(value)
}
