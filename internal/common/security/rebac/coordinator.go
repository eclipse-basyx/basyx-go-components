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
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// ErrNotReady reports that startup reconciliation has not completed.
var ErrNotReady = errors.New("REBAC-COORDINATOR-NOTREADY relationship-based authorization is starting")

// Coordinator resolves ReBAC grants for covered routes, records desired
// state for resource mutations and serves the management API. It implements
// auth.ReBACResolver and auth.ReBACState. All relationships live in
// PostgreSQL and are evaluated there, so changes take effect at commit.
type Coordinator struct {
	db             *sql.DB
	groupClaim     string
	routes         routeMatrix
	management     map[string]struct{}
	administrators []common.ReBACAdministrator
	ready          atomic.Bool

	abacProvider      auth.AccessModelProvider
	abacImplicitCasts bool
}

// NewCoordinator creates a coordinator. It rejects requests needing ReBAC
// with 503 until MarkReady is called.
func NewCoordinator(db *sql.DB, cfg common.ReBACConfig, administrators []common.ReBACAdministrator) *Coordinator {
	return &Coordinator{
		db:             db,
		groupClaim:     cfg.GroupClaim,
		routes:         newRouteMatrix(),
		management:     map[string]struct{}{},
		administrators: administrators,
	}
}

// MarkReady enables ReBAC decisions after startup reconciliation.
func (c *Coordinator) MarkReady() {
	c.ready.Store(true)
}

// Ready reports whether startup reconciliation completed.
func (c *Coordinator) Ready() bool {
	return c.ready.Load()
}

// Covers reports whether a route participates in ReBAC.
func (c *Coordinator) Covers(route auth.ReBACRoute) bool {
	_, covered := c.routes.lookup(route.Method, route.Pattern)
	return covered
}

// IsManagementRoute reports routes of the ReBAC management API.
func (c *Coordinator) IsManagementRoute(route auth.ReBACRoute) bool {
	_, managed := c.management[routeKey(route.Method, route.Pattern)]
	return managed
}

func (c *Coordinator) registerManagementRoute(method string, pattern string) {
	c.management[routeKey(method, pattern)] = struct{}{}
}

// Resolve returns the grants of a covered request that ABAC does not allow
// unconditionally. An empty set keeps the ABAC decision.
func (c *Coordinator) Resolve(ctx context.Context, request auth.ReBACRequest) (*auth.ReBACGrantSet, error) {
	grants := auth.NewReBACGrantSet(request.Route.Rights...)
	spec, covered := c.routes.lookup(request.Route.Method, request.Route.Pattern)
	principal, authenticated := PrincipalFromClaims(request.Claims, c.groupClaim)
	if !covered || !authenticated {
		return grants, nil
	}
	if !c.Ready() {
		return nil, ErrNotReady
	}
	resolution := &resolution{coordinator: c, keys: principal.SubjectKeys(), route: request.Route, grants: grants}
	if err := resolution.resolve(ctx, spec); err != nil {
		return nil, err
	}
	return grants, nil
}

// resolution evaluates one request.
type resolution struct {
	coordinator *Coordinator
	keys        []string
	route       auth.ReBACRoute
	grants      *auth.ReBACGrantSet
}

func (r *resolution) db() Queryer {
	return r.coordinator.db
}

func (r *resolution) resolve(ctx context.Context, spec routeSpec) error {
	if spec.aasRelation != "" {
		if err := r.resolveSuperpathAAS(ctx, spec.aasRelation); err != nil {
			return err
		}
	}
	switch spec.target {
	case targetList:
		return r.resolveList(ctx, spec.kind)
	case targetElement:
		return r.resolveElement(ctx, spec)
	case targetSubmodelElements:
		return r.resolveSubmodelElements(ctx)
	default:
		return r.resolveIdentifiable(ctx, spec)
	}
}

func (r *resolution) rights() []grammar.RightsEnum {
	return r.route.Rights
}

func (r *resolution) identifier(param string) (string, bool) {
	raw, ok := r.route.Params[param]
	if !ok {
		return "", false
	}
	decoded, err := common.DecodeString(raw)
	if err != nil || strings.TrimSpace(decoded) == "" {
		return "", false
	}
	return decoded, true
}

func identifierParam(kind ResourceKind) string {
	switch kind.ObjectType {
	case TypeAAS:
		return paramAAS
	case TypeConceptDescription:
		return paramCD
	default:
		return paramSubmodel
	}
}

func (r *resolution) lookup(ctx context.Context, kind ResourceKind) (string, bool, error) {
	identifier, ok := r.identifier(identifierParam(kind))
	if !ok {
		return "", false, nil
	}
	return LookupAuthUUID(ctx, r.db(), kind, identifier)
}

func (r *resolution) resolveSuperpathAAS(ctx context.Context, permission string) error {
	authUUID, found, err := r.lookup(ctx, KindAAS)
	if err != nil || !found {
		return err
	}
	allowed, err := hasPermission(ctx, r.db(), KindAAS, authUUID, r.keys, permission)
	if err != nil || !allowed {
		return err
	}
	return r.grants.AllowResources(auth.SemanticResourceAAS, []string{authUUID}, r.rights()...)
}

func (r *resolution) resolveIdentifiable(ctx context.Context, spec routeSpec) error {
	if spec.action == actionCreate {
		return r.resolveCreate(ctx, spec.kind)
	}
	authUUID, found, err := r.lookup(ctx, spec.kind)
	if err != nil {
		return err
	}
	if spec.action == actionUpsert && !found {
		return r.resolveCreate(ctx, spec.kind)
	}
	if !found {
		return nil
	}
	permission, rights := spec.relation, r.rights()
	if spec.action == actionUpsert {
		permission, rights = PermissionUpdate, []grammar.RightsEnum{grammar.RightsEnumUPDATE}
	}
	allowed, err := hasPermission(ctx, r.db(), spec.kind, authUUID, r.keys, permission)
	if err != nil || !allowed {
		return err
	}
	return r.grants.AllowResources(spec.kind.Semantic, []string{authUUID}, rights...)
}

func (r *resolution) resolveCreate(ctx context.Context, kind ResourceKind) error {
	allowed, err := isRepositoryCreator(ctx, r.db(), kind, r.keys)
	if err != nil || !allowed {
		return err
	}
	return r.grants.AllowAllOfKind(kind.Semantic, grammar.RightsEnumCREATE)
}

func (r *resolution) resolveElement(ctx context.Context, spec routeSpec) error {
	submodelUUID, found, err := r.lookup(ctx, KindSubmodel)
	path := strings.TrimSpace(r.route.Params[paramPath])
	if err != nil || !found || path == "" {
		return err
	}
	permission, target, rights := spec.relation, path, r.rights()
	switch spec.action {
	case actionParentUpdate:
		permission, target = PermissionUpdate, ParentElementPath(path)
	case actionUpsert:
		exists, existsErr := elementExists(ctx, r.db(), submodelUUID, path)
		if existsErr != nil {
			return existsErr
		}
		permission, rights = PermissionUpdate, []grammar.RightsEnum{grammar.RightsEnumUPDATE}
		if !exists {
			target, rights = ParentElementPath(path), []grammar.RightsEnum{grammar.RightsEnumCREATE}
		}
	}
	allowed, err := hasElementPermission(ctx, r.db(), submodelUUID, target, r.keys, permission)
	if err != nil || !allowed {
		return err
	}
	return r.grants.AllowSubmodelElement(submodelUUID, path, rights...)
}

// resolveSubmodelElements grants the element list of a Submodel: the whole
// Submodel when the caller can read it, otherwise the granted subtrees.
func (r *resolution) resolveSubmodelElements(ctx context.Context) error {
	submodelUUID, found, err := r.lookup(ctx, KindSubmodel)
	if err != nil || !found {
		return err
	}
	allowed, err := hasPermission(ctx, r.db(), KindSubmodel, submodelUUID, r.keys, PermissionRead)
	if err != nil {
		return err
	}
	if allowed {
		return r.grants.AllowResources(auth.SemanticResourceSM, []string{submodelUUID}, r.rights()...)
	}
	elements := grantedElements(r.keys, PermissionRead, submodelUUID)
	granted, err := exists(ctx, r.db(), "REBAC-RESOLVEELEMENTS-EXISTS", existsQuery(elements))
	if err != nil || !granted {
		return err
	}
	return r.grants.AllowQueriedSubmodelElements(elements, r.rights()...)
}

// resolveList grants the objects of a list route. The grant is a SQL
// subquery evaluated by the backend, so lists need no allowlist and no cap.
// Callers without any readable object keep today's ABAC decision.
func (r *resolution) resolveList(ctx context.Context, kind ResourceKind) error {
	admin, err := isRepositoryAdmin(ctx, r.db(), kind, r.keys)
	if err != nil {
		return err
	}
	if admin {
		return r.grants.AllowAllOfKind(kind.Semantic, r.rights()...)
	}
	readable := permittedObjects(kind, r.keys, PermissionRead)
	readableExists, err := exists(ctx, r.db(), "REBAC-RESOLVELIST-EXISTS", existsQuery(readable))
	if err != nil || !readableExists {
		return err
	}
	return r.grants.AllowQueriedResources(kind.Semantic, readable, r.rights()...)
}

func elementExists(ctx context.Context, q Queryer, submodelUUID string, path string) (bool, error) {
	ds := dialect.From(goqu.T("submodel_element").As("sme")).
		InnerJoin(goqu.T("submodel").As("s"), goqu.On(goqu.I("s.id").Eq(goqu.I("sme.submodel_id")))).
		Select(goqu.L("1")).
		Where(
			goqu.I("s.auth_uuid").Eq(goqu.L("?::uuid", submodelUUID)),
			goqu.I("sme.idshort_path").Eq(path),
		).Limit(1).Prepared(true)
	var marker int
	return queryRowDataset(ctx, q, "REBAC-ELEMENTEXISTS", ds, &marker)
}
