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
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// maxContextualTuples bounds the contextual tuples of one OpenFGA request.
const maxContextualTuples = 100

var (
	// ErrRevocationPending reports that a committed revocation has not reached
	// OpenFGA yet, so a ReBAC allow cannot be trusted.
	ErrRevocationPending = errors.New("REBAC-BARRIER-PENDING revocation not yet applied")
	// ErrNotReady reports that startup reconciliation has not completed.
	ErrNotReady = errors.New("REBAC-COORDINATOR-NOTREADY relationship-based authorization is starting")
	// ErrTooManyCandidates reports a list that exceeds rebac.maxScanCandidates.
	ErrTooManyCandidates = errors.New("REBAC-LIST-TOOMANY visible objects exceed rebac.maxScanCandidates")
	// ErrTooManyContextualTuples reports a check that exceeds the OpenFGA limit.
	ErrTooManyContextualTuples = errors.New("REBAC-CHECK-CONTEXTUALTUPLES too many groups or element levels for one check")
)

// Coordinator resolves ReBAC grants for covered routes and records desired
// state for resource mutations. It implements auth.ReBACResolver and
// auth.ReBACState.
type Coordinator struct {
	db             *sql.DB
	client         Client
	projector      *Projector
	scope          string
	groupClaim     string
	listMax        int
	scanMax        int
	routes         routeMatrix
	management     map[string]struct{}
	administrators []common.ReBACAdministrator
	ready          atomic.Bool

	abacProvider      auth.AccessModelProvider
	abacImplicitCasts bool
}

// CoordinatorOptions configures a Coordinator.
type CoordinatorOptions struct {
	DB             *sql.DB
	Client         Client
	Projector      *Projector
	Config         common.ReBACConfig
	Administrators []common.ReBACAdministrator
}

// NewCoordinator creates a coordinator. It rejects requests needing ReBAC
// with 503 until MarkReady is called.
func NewCoordinator(options CoordinatorOptions) *Coordinator {
	return &Coordinator{
		db:             options.DB,
		client:         options.Client,
		projector:      options.Projector,
		scope:          options.Config.Scope,
		groupClaim:     options.Config.GroupClaim,
		listMax:        options.Config.ListObjectsMaxResults,
		scanMax:        options.Config.MaxScanCandidates,
		routes:         newRouteMatrix(),
		management:     map[string]struct{}{},
		administrators: options.Administrators,
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
// unconditionally.
func (c *Coordinator) Resolve(ctx context.Context, request auth.ReBACRequest) (*auth.ReBACGrantSet, error) {
	spec, covered := c.routes.lookup(request.Route.Method, request.Route.Pattern)
	principal, authenticated := PrincipalFromClaims(request.Claims, c.groupClaim)
	if !covered || !authenticated {
		return auth.NewReBACGrantSet(request.Route.Rights...), nil
	}
	if !c.Ready() {
		return nil, ErrNotReady
	}
	grants, err := c.resolveGrants(ctx, spec, principal, request)
	if err != nil || grants.IsEmpty() {
		return grants, err
	}
	drained, err := c.verifyNoPendingRevocation(ctx, true)
	if err != nil || !drained {
		return grants, err
	}
	grants, err = c.resolveGrants(ctx, spec, principal, request)
	if err != nil || grants.IsEmpty() {
		return grants, err
	}
	_, err = c.verifyNoPendingRevocation(ctx, false)
	return grants, err
}

func (c *Coordinator) resolveGrants(ctx context.Context, spec routeSpec, principal Principal, request auth.ReBACRequest) (*auth.ReBACGrantSet, error) {
	grants := auth.NewReBACGrantSet(request.Route.Rights...)
	resolution := &resolution{coordinator: c, principal: principal, route: request.Route, grants: grants}
	if err := resolution.resolve(ctx, spec); err != nil {
		return nil, err
	}
	return grants, nil
}

// verifyNoPendingRevocation implements the barrier. It runs after OpenFGA
// answered, so an allow is never served from a graph that misses a
// committed revocation. When drain is set, committed revocations are
// applied first, bounded by a short wait; drained reports that the caller
// must decide again because the graph changed.
func (c *Coordinator) verifyNoPendingRevocation(ctx context.Context, drain bool) (drained bool, err error) {
	pending, err := RevocationPending(ctx, c.db, c.scope)
	if err != nil || !pending {
		return false, err
	}
	if !drain {
		return false, ErrRevocationPending
	}
	applied, drainErr := c.projector.DrainRevocations(ctx)
	if drainErr != nil || !applied {
		c.projector.Notify()
		return false, firstError(drainErr, ErrRevocationPending)
	}
	return true, nil
}

// resolution evaluates one request.
type resolution struct {
	coordinator *Coordinator
	principal   Principal
	route       auth.ReBACRoute
	grants      *auth.ReBACGrantSet
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
	return LookupAuthUUID(ctx, r.coordinator.db, kind, identifier)
}

func (r *resolution) resolveSuperpathAAS(ctx context.Context, relation string) error {
	authUUID, found, err := r.lookup(ctx, KindAAS)
	if err != nil || !found {
		return err
	}
	allowed, err := r.checkResource(ctx, KindAAS, relation, ResourceObject(TypeAAS, authUUID), nil)
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
	relation, rights := spec.relation, r.rights()
	if spec.action == actionUpsert {
		relation, rights = RelationCanUpdate, []grammar.RightsEnum{grammar.RightsEnumUPDATE}
	}
	allowed, err := r.checkResource(ctx, spec.kind, relation, ResourceObject(spec.kind.ObjectType, authUUID), nil)
	if err != nil || !allowed {
		return err
	}
	return r.grants.AllowResources(spec.kind.Semantic, []string{authUUID}, rights...)
}

func (r *resolution) resolveCreate(ctx context.Context, kind ResourceKind) error {
	allowed, err := r.check(ctx, CheckItem{
		User: r.principal.UserObject(), Relation: RelationCreator, Object: RepositoryObject(kind.ObjectType),
	})
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
	relation, target, rights := spec.relation, path, r.rights()
	switch spec.action {
	case actionParentUpdate:
		relation, target = RelationCanUpdate, ParentElementPath(path)
	case actionUpsert:
		exists, existsErr := elementExists(ctx, r.coordinator.db, submodelUUID, path)
		if existsErr != nil {
			return existsErr
		}
		relation, rights = RelationCanUpdate, []grammar.RightsEnum{grammar.RightsEnumUPDATE}
		if !exists {
			target, rights = ParentElementPath(path), []grammar.RightsEnum{grammar.RightsEnumCREATE}
		}
	}
	allowed, err := r.checkElementOrSubmodel(ctx, submodelUUID, target, relation)
	if err != nil || !allowed {
		return err
	}
	return r.grants.AllowSubmodelElement(submodelUUID, path, rights...)
}

// checkElementOrSubmodel checks relation on an element path, or on the
// Submodel itself when path is empty.
func (r *resolution) checkElementOrSubmodel(ctx context.Context, submodelUUID string, path string, relation string) (bool, error) {
	if path == "" {
		return r.checkResource(ctx, KindSubmodel, relation, ResourceObject(TypeSubmodel, submodelUUID), nil)
	}
	return r.checkResource(ctx, KindSubmodel, relation, ElementObject(submodelUUID, path), ElementAncestry(submodelUUID, path))
}

func (r *resolution) resolveSubmodelElements(ctx context.Context) error {
	submodelUUID, found, err := r.lookup(ctx, KindSubmodel)
	if err != nil || !found {
		return err
	}
	allowed, err := r.checkResource(ctx, KindSubmodel, RelationCanRead, ResourceObject(TypeSubmodel, submodelUUID), nil)
	if err != nil {
		return err
	}
	if allowed {
		return r.grants.AllowResources(auth.SemanticResourceSM, []string{submodelUUID}, r.rights()...)
	}
	return r.resolveElementGrants(ctx, submodelUUID)
}

// resolveElementGrants grants the element subtrees a caller can read when it
// has no access to the Submodel itself.
func (r *resolution) resolveElementGrants(ctx context.Context, submodelUUID string) error {
	paths, err := ElementGrantPaths(ctx, r.coordinator.db, submodelUUID, r.principal.SubjectKeys())
	if err != nil || len(paths) == 0 {
		return err
	}
	items := make([]CheckItem, 0, len(paths))
	for _, path := range paths {
		item, itemErr := r.checkItem(RelationCanRead, ElementObject(submodelUUID, path), ElementAncestry(submodelUUID, path))
		if itemErr != nil {
			return itemErr
		}
		items = append(items, item)
	}
	allowed, err := r.coordinator.client.BatchCheck(ctx, items)
	if err != nil {
		return err
	}
	for index, path := range paths {
		if !allowed[index] {
			continue
		}
		if err = r.grants.AllowSubmodelElement(submodelUUID, path, r.rights()...); err != nil {
			return err
		}
	}
	return nil
}

func (r *resolution) checkItem(relation string, object string, structure []Tuple) (CheckItem, error) {
	contextual := append(r.principal.GroupTuples(), structure...)
	if len(contextual) > maxContextualTuples {
		return CheckItem{}, ErrTooManyContextualTuples
	}
	return CheckItem{User: r.principal.UserObject(), Relation: relation, Object: object, Contextual: contextual}, nil
}

func (r *resolution) check(ctx context.Context, item CheckItem) (bool, error) {
	checked, err := r.checkItem(item.Relation, item.Object, item.Contextual)
	if err != nil {
		return false, err
	}
	return r.coordinator.client.Check(ctx, checked)
}

// checkResource checks relation on object and, in the same round trip,
// whether the caller administers the repository of kind.
func (r *resolution) checkResource(ctx context.Context, kind ResourceKind, relation string, object string, structure []Tuple) (bool, error) {
	resourceItem, err := r.checkItem(relation, object, structure)
	if err != nil {
		return false, err
	}
	adminItem, err := r.checkItem(RelationAdmin, RepositoryObject(kind.ObjectType), nil)
	if err != nil {
		return false, err
	}
	results, err := r.coordinator.client.BatchCheck(ctx, []CheckItem{resourceItem, adminItem})
	if err != nil {
		return false, err
	}
	return results[0] || results[1], nil
}

func (r *resolution) resolveList(ctx context.Context, kind ResourceKind) error {
	admin, err := r.check(ctx, CheckItem{User: r.principal.UserObject(), Relation: RelationAdmin, Object: RepositoryObject(kind.ObjectType)})
	if err != nil {
		return err
	}
	if admin {
		return r.grants.AllowAllOfKind(kind.Semantic, r.rights()...)
	}
	objects, err := r.coordinator.client.ListObjects(ctx, r.principal.UserObject(), RelationCanRead, kind.ObjectType, r.principal.GroupTuples())
	if err != nil {
		return err
	}
	var authUUIDs []string
	if len(objects) < r.coordinator.listMax {
		authUUIDs = objectUUIDs(kind.ObjectType, objects)
	} else if authUUIDs, err = r.scanCandidates(ctx, kind); err != nil {
		return err
	}
	return r.grants.AllowResources(kind.Semantic, authUUIDs, r.rights()...)
}

// scanCandidates is the fallback when ListObjects reaches its cap. Every
// object a caller can read carries a direct grant or, for Submodels, an
// approved link to an AAS with a grant; these candidates are verified with
// BatchCheck.
func (r *resolution) scanCandidates(ctx context.Context, kind ResourceKind) ([]string, error) {
	limit := r.coordinator.scanMax + 1
	candidates, err := CandidateObjects(ctx, r.coordinator.db, kind.ObjectType, r.principal.SubjectKeys(), limit)
	if err != nil {
		return nil, err
	}
	if kind.ObjectType == TypeSubmodel {
		aasUUIDs, aasErr := CandidateObjects(ctx, r.coordinator.db, TypeAAS, r.principal.SubjectKeys(), limit)
		if aasErr != nil {
			return nil, aasErr
		}
		linked, linkErr := LinkedSubmodels(ctx, r.coordinator.db, aasUUIDs)
		if linkErr != nil {
			return nil, linkErr
		}
		candidates = uniqueStrings(append(candidates, linked...))
	}
	if len(candidates) > r.coordinator.scanMax {
		return nil, ErrTooManyCandidates
	}
	items := make([]CheckItem, len(candidates))
	for index, candidate := range candidates {
		item, itemErr := r.checkItem(RelationCanRead, ResourceObject(kind.ObjectType, candidate), nil)
		if itemErr != nil {
			return nil, itemErr
		}
		items[index] = item
	}
	allowed, err := r.coordinator.client.BatchCheck(ctx, items)
	if err != nil {
		return nil, err
	}
	visible := make([]string, 0, len(candidates))
	for index, candidate := range candidates {
		if allowed[index] {
			visible = append(visible, candidate)
		}
	}
	return visible, nil
}

func objectUUIDs(objectType string, objects []string) []string {
	uuids := make([]string, 0, len(objects))
	for _, object := range objects {
		if authUUID, ok := strings.CutPrefix(object, objectType+":"); ok {
			uuids = append(uuids, authUUID)
		}
	}
	return uuids
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := values[:0]
	for _, value := range values {
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
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
	found, err := queryRowDataset(ctx, q, "REBAC-ELEMENTEXISTS", ds, &marker)
	if err != nil {
		return false, fmt.Errorf("REBAC-ELEMENTEXISTS: %w", err)
	}
	return found, nil
}
