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
	"slices"
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

func (r *resolution) lookup(ctx context.Context, kind ResourceKind) (string, bool, error) {
	identifier, ok := r.identifier(kind.Param)
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
	return r.allowObject(KindAAS, permission, authUUID, r.rights())
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
	return r.allowObject(spec.kind, permission, authUUID, rights)
}

// allowObject grants one object as a live query, together with the objects
// derived from it that the same request synchronizes.
func (r *resolution) allowObject(kind ResourceKind, permission string, authUUID string, rights []grammar.RightsEnum) error {
	if err := r.grants.AllowQueriedResources(kind.Semantic, liveObject(kind, r.keys, permission, authUUID), rights...); err != nil {
		return err
	}
	synchronized := r.synchronizedRights(rights)
	if err := r.allowDerived(kind, uuidQuery(authUUID), synchronized); err != nil {
		return err
	}
	identifier, known := r.identifier(kind.Param)
	if kind.ObjectType != TypeSubmodel || !known || !writes(synchronized) {
		return nil
	}
	return r.allowDerived(KindAAS, shellsReferencing(identifier), synchronized)
}

// synchronizedRights are the rights of objects that a request synchronizes
// with its target. Synchronization runs with the route's rights, which may
// be wider than the right selected for the target, for example CREATE and
// UPDATE of an upsert.
func (r *resolution) synchronizedRights(rights []grammar.RightsEnum) []grammar.RightsEnum {
	union := slices.Clone(rights)
	for _, right := range r.rights() {
		if !slices.Contains(union, right) {
			union = append(union, right)
		}
	}
	return union
}

// allowDerived grants the objects derived from the sources query, so that
// registry and discovery entries synchronized by the same request follow
// their source. A derived object inherits every permission of its source.
func (r *resolution) allowDerived(kind ResourceKind, sources *goqu.SelectDataset, rights []grammar.RightsEnum) error {
	for _, derived := range derivedKinds(kind) {
		objects := derivedObjects(derived, sources)
		if err := r.grants.AllowQueriedResources(derived.Semantic, objects, rights...); err != nil {
			return err
		}
		if err := r.allowDerived(derived, objects, rights); err != nil {
			return err
		}
	}
	return nil
}

func (r *resolution) resolveCreate(ctx context.Context, kind ResourceKind) error {
	allowed, err := isRepositoryCreator(ctx, r.db(), kind, r.keys)
	if err != nil || !allowed {
		return err
	}
	if err = r.grants.AllowAllOfKind(kind.Semantic, grammar.RightsEnumCREATE); err != nil {
		return err
	}
	rights := r.synchronizedRights([]grammar.RightsEnum{grammar.RightsEnumCREATE})
	sources := liveObjects(kind, r.keys, PermissionUpdate)
	if err = r.allowDerived(kind, sources, rights); err != nil || kind.ObjectType != TypeSubmodel {
		return err
	}
	return r.allowDerived(KindAAS, shellsReferencing(submodelIdentifiers(sources)), rights)
}

// writes reports rights of mutating requests. Submodel mutations
// synchronize the Submodel descriptors embedded in referencing shell
// descriptors, which therefore follow the Submodel in such requests.
func writes(rights []grammar.RightsEnum) bool {
	for _, right := range rights {
		if right == grammar.RightsEnumCREATE || right == grammar.RightsEnumUPDATE || right == grammar.RightsEnumDELETE {
			return true
		}
	}
	return false
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
	return r.allowLiveElements(submodelUUID, permission, rights)
}

// allowLiveElements grants the Submodel when the caller holds permission on
// it, and every element subtree the caller holds permission on, both as live
// queries. The preceding decision guarantees that the target is covered.
func (r *resolution) allowLiveElements(submodelUUID string, permission string, rights []grammar.RightsEnum) error {
	if err := r.allowObject(KindSubmodel, permission, submodelUUID, rights); err != nil {
		return err
	}
	return r.grants.AllowQueriedSubmodelElements(liveElements(r.keys, permission, submodelUUID), rights...)
}

// resolveSubmodelElements grants the element list of a Submodel: the whole
// Submodel when the caller can read it and the granted subtrees otherwise.
func (r *resolution) resolveSubmodelElements(ctx context.Context) error {
	submodelUUID, found, err := r.lookup(ctx, KindSubmodel)
	if err != nil || !found {
		return err
	}
	allowed, err := exists(ctx, r.db(), "REBAC-RESOLVEELEMENTS-EXISTS",
		goqu.L("?::uuid IN (?)", submodelUUID, liveObjects(KindSubmodel, r.keys, PermissionRead)),
		existsQuery(liveElements(r.keys, PermissionRead, submodelUUID)))
	if err != nil || !allowed {
		return err
	}
	return r.allowLiveElements(submodelUUID, PermissionRead, r.rights())
}

// resolveList grants the objects of a list route. The grant is a SQL
// subquery evaluated by the backend, so lists need no allowlist and no cap.
// Callers without any readable object keep today's ABAC decision.
func (r *resolution) resolveList(ctx context.Context, kind ResourceKind) error {
	readable := liveObjects(kind, r.keys, PermissionRead)
	readableExists, err := exists(ctx, r.db(), "REBAC-RESOLVELIST-EXISTS", existsQuery(readable))
	if err != nil || !readableExists {
		return err
	}
	return r.grants.AllowQueriedResources(kind.Semantic, readable, r.rights()...)
}

func uuidQuery(authUUID string) *goqu.SelectDataset {
	return dialect.Select(goqu.L("?::uuid", authUUID).As("object_uuid"))
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
