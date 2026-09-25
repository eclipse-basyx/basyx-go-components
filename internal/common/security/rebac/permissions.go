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

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
)

// permissionRelations lists the granted relations that imply a permission.
var permissionRelations = map[string][]string{
	PermissionRead:    {RelationViewer, RelationEditor, RelationOwner},
	PermissionUpdate:  {RelationEditor, RelationOwner},
	PermissionDelete:  {RelationOwner},
	PermissionManage:  {RelationOwner},
	PermissionExecute: {RelationExecutor, RelationOwner},
}

// linkedPermissions are inherited from a shell through an approved link.
// Management and deletion are never inherited.
var linkedPermissions = map[string]bool{
	PermissionRead:    true,
	PermissionUpdate:  true,
	PermissionExecute: true,
}

// grantedObjects selects the authorization UUIDs of objectType on which any
// of subjectKeys holds a relation implying permission.
func grantedObjects(objectType string, subjectKeys []string, permission string) *goqu.SelectDataset {
	return dialect.From(goqu.T(grantTable).As("g")).
		Select(goqu.I("g.object_uuid")).
		Where(
			goqu.I("g.object_type").Eq(objectType),
			goqu.I("g.subject_key").In(subjectKeys),
			goqu.I("g.relation").In(permissionRelations[permission]),
		)
}

// linkedSubmodels selects Submodels that inherit permission from a shell the
// subjects hold it on. A link only counts while the shell still references
// the Submodel, so removed references never leave stale inheritance.
func linkedSubmodels(subjectKeys []string, permission string) *goqu.SelectDataset {
	return dialect.From(goqu.T("rebac_submodel_link").As("link")).
		Select(goqu.I("link.submodel_uuid")).
		Where(
			goqu.I("link.aas_uuid").In(grantedObjects(TypeAAS, subjectKeys, permission)),
			goqu.L("EXISTS (?)", submodelReferenceExists("link")),
		)
}

// permittedObjects selects every object of kind the subjects hold a
// relation implying permission on, including approved links.
func permittedObjects(kind ResourceKind, subjectKeys []string, permission string) *goqu.SelectDataset {
	direct := grantedObjects(kind.ObjectType, subjectKeys, permission)
	if kind.ObjectType == TypeSubmodel && linkedPermissions[permission] {
		return direct.Union(linkedSubmodels(subjectKeys, permission))
	}
	return direct
}

// liveObjects selects every object of kind the subjects hold permission on:
// through relations, repository administration or the source of a derived
// object. Grants built from it are re-evaluated by every backend query, so
// revocations also stop asynchronous work that captured the request context.
func liveObjects(kind ResourceKind, subjectKeys []string, permission string) *goqu.SelectDataset {
	admin := dialect.From(kind.Rows().As("admin_scope")).
		Select(goqu.I("admin_scope.object_uuid")).
		Where(existsQuery(repositoryGrant(kind, subjectKeys, RelationAdmin)))
	live := permittedObjects(kind, subjectKeys, permission).Union(admin)
	if source, derived := derivationSource(kind); derived {
		live = live.Union(derivedObjects(kind, liveObjects(source, subjectKeys, permission)))
	}
	return live
}

// liveObject selects authUUID when the subjects hold permission on it.
func liveObject(kind ResourceKind, subjectKeys []string, permission string, authUUID string) *goqu.SelectDataset {
	target := goqu.L("?::uuid", authUUID)
	return dialect.Select(target.As("object_uuid")).
		Where(permissionCondition(kind, subjectKeys, permission, target))
}

// permissionCondition matches when the subjects hold permission on the
// object whose authorization UUID is target. It is correlated, so single
// objects never scan a whole repository.
func permissionCondition(kind ResourceKind, subjectKeys []string, permission string, target exp.Expression) exp.Expression {
	conditions := []exp.Expression{
		goqu.L("? IN (?)", target, permittedObjects(kind, subjectKeys, permission)),
		existsQuery(repositoryGrant(kind, subjectKeys, RelationAdmin)),
	}
	if source, derived := derivationSource(kind); derived {
		alias := "derivation_" + kind.ObjectType
		sourceUUID := goqu.I(alias + ".source_uuid")
		conditions = append(conditions, existsQuery(dialect.From(goqu.T(derivationTable).As(alias)).
			Select(goqu.L("1")).
			Where(
				goqu.I(alias+".object_uuid").Eq(target),
				goqu.I(alias+".object_type").Eq(kind.ObjectType),
				permissionCondition(source, subjectKeys, permission, sourceUUID),
			)))
	}
	return goqu.Or(conditions...)
}

// derivedObjects selects the objects of kind derived from the sources query.
func derivedObjects(kind ResourceKind, sources *goqu.SelectDataset) *goqu.SelectDataset {
	alias := "derived_" + kind.ObjectType
	return dialect.From(goqu.T(derivationTable).As(alias)).
		Select(goqu.I(alias+".object_uuid")).
		Where(
			goqu.I(alias+".object_type").Eq(kind.ObjectType),
			goqu.I(alias+".source_uuid").In(sources),
		)
}

// derivationSource returns the kind that objects of kind can be derived
// from: registry descriptors generated from repository resources and
// discovery entries generated from descriptors.
func derivationSource(kind ResourceKind) (ResourceKind, bool) {
	switch kind.ObjectType {
	case TypeAASDescriptor:
		return KindAAS, true
	case TypeSubmodelDescriptor:
		return KindSubmodel, true
	case TypeAssetLinks:
		return KindAASDescriptor, true
	default:
		return ResourceKind{}, false
	}
}

// derivedKinds returns the kinds whose objects can be derived from kind.
func derivedKinds(kind ResourceKind) []ResourceKind {
	var kinds []ResourceKind
	for _, candidate := range AllKinds {
		if source, derived := derivationSource(candidate); derived && source.ObjectType == kind.ObjectType {
			kinds = append(kinds, candidate)
		}
	}
	return kinds
}

// liveElements selects the granted element subtrees of one Submodel.
func liveElements(subjectKeys []string, permission string, submodelUUID string) *goqu.SelectDataset {
	return grantedElements(subjectKeys, permission, submodelUUID)
}

// grantedElements selects (submodel_uuid, element_path) pairs of element
// grants implying permission, optionally restricted to one Submodel.
func grantedElements(subjectKeys []string, permission string, submodelUUID string) *goqu.SelectDataset {
	ds := dialect.From(goqu.T(grantTable).As("g")).
		Select(goqu.I("g.object_uuid").As("submodel_uuid"), goqu.I("g.element_path").As("element_path")).
		Where(
			goqu.I("g.object_type").Eq(TypeElement),
			goqu.I("g.subject_key").In(subjectKeys),
			goqu.I("g.relation").In(permissionRelations[permission]),
		)
	if submodelUUID != "" {
		ds = ds.Where(goqu.I("g.object_uuid").Eq(goqu.L("?::uuid", submodelUUID)))
	}
	return ds
}

// repositoryGrant selects the repository relations of the subjects.
func repositoryGrant(kind ResourceKind, subjectKeys []string, relations ...string) *goqu.SelectDataset {
	return dialect.From(goqu.T(grantTable).As("g")).
		Select(goqu.L("1")).
		Where(
			goqu.I("g.object_key").Eq(RepositoryKey(kind.ObjectType)),
			goqu.I("g.subject_key").In(subjectKeys),
			goqu.I("g.relation").In(relations),
		)
}

func exists(ctx context.Context, q Queryer, code string, conditions ...exp.Expression) (bool, error) {
	ds := dialect.Select(goqu.Or(conditions...)).Prepared(true)
	var allowed bool
	if _, err := queryRowDataset(ctx, q, code, ds, &allowed); err != nil {
		return false, err
	}
	return allowed, nil
}

func existsQuery(ds *goqu.SelectDataset) exp.Expression {
	return goqu.L("EXISTS (?)", ds)
}

// isRepositoryAdmin reports whether the subjects administer a repository
// family. Repository admins hold every permission on every object of it.
func isRepositoryAdmin(ctx context.Context, q Queryer, kind ResourceKind, subjectKeys []string) (bool, error) {
	return exists(ctx, q, "REBAC-ISREPOSITORYADMIN", existsQuery(repositoryGrant(kind, subjectKeys, RelationAdmin)))
}

// isRepositoryCreator reports whether the subjects may create top-level
// objects of a repository family.
func isRepositoryCreator(ctx context.Context, q Queryer, kind ResourceKind, subjectKeys []string) (bool, error) {
	return exists(ctx, q, "REBAC-ISREPOSITORYCREATOR", existsQuery(repositoryGrant(kind, subjectKeys, RelationCreator, RelationAdmin)))
}

// hasPermission reports whether the subjects hold permission on one object,
// directly, through an approved link, as repository admin or through the
// source of a derived object.
func hasPermission(ctx context.Context, q Queryer, kind ResourceKind, authUUID string, subjectKeys []string, permission string) (bool, error) {
	if _, known := permissionRelations[permission]; !known {
		return false, fmt.Errorf("REBAC-HASPERMISSION-UNKNOWN permission %q", permission)
	}
	return exists(ctx, q, "REBAC-HASPERMISSION", permissionCondition(kind, subjectKeys, permission, goqu.L("?::uuid", authUUID)))
}

// hasElementPermission reports whether the subjects hold permission on an
// element path: through an element grant on the path or an ancestor, or
// through the Submodel. An empty path addresses the Submodel itself.
func hasElementPermission(ctx context.Context, q Queryer, submodelUUID string, path string, subjectKeys []string, permission string) (bool, error) {
	if path == "" {
		return hasPermission(ctx, q, KindSubmodel, submodelUUID, subjectKeys, permission)
	}
	onPath := grantedElements(subjectKeys, permission, submodelUUID).
		Where(goqu.I("g.element_path").In(ElementPathChain(path)))
	return exists(ctx, q, "REBAC-HASELEMENTPERMISSION",
		existsQuery(onPath),
		permissionCondition(KindSubmodel, subjectKeys, permission, goqu.L("?::uuid", submodelUUID)),
	)
}
