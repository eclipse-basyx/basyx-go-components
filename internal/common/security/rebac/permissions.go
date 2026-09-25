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

// permittedObjects selects every object of kind the subjects hold permission
// on, including permissions inherited through approved links.
func permittedObjects(kind ResourceKind, subjectKeys []string, permission string) *goqu.SelectDataset {
	direct := grantedObjects(kind.ObjectType, subjectKeys, permission)
	if kind.ObjectType == TypeSubmodel && linkedPermissions[permission] {
		return direct.Union(linkedSubmodels(subjectKeys, permission))
	}
	return direct
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
// directly, through an approved link or as repository admin.
func hasPermission(ctx context.Context, q Queryer, kind ResourceKind, authUUID string, subjectKeys []string, permission string) (bool, error) {
	if _, known := permissionRelations[permission]; !known {
		return false, fmt.Errorf("REBAC-HASPERMISSION-UNKNOWN permission %q", permission)
	}
	target := goqu.L("?::uuid", authUUID)
	return exists(ctx, q, "REBAC-HASPERMISSION",
		goqu.L("? IN (?)", target, permittedObjects(kind, subjectKeys, permission)),
		existsQuery(repositoryGrant(kind, subjectKeys, RelationAdmin)),
	)
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
	target := goqu.L("?::uuid", submodelUUID)
	return exists(ctx, q, "REBAC-HASELEMENTPERMISSION",
		existsQuery(onPath),
		goqu.L("? IN (?)", target, permittedObjects(KindSubmodel, subjectKeys, permission)),
		existsQuery(repositoryGrant(KindSubmodel, subjectKeys, RelationAdmin)),
	)
}
