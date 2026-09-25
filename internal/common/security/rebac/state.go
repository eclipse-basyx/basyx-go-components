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
	"fmt"

	"github.com/doug-martin/goqu/v9"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// ResourceCreated makes an authenticated creator the owner of a new
// identifiable in the creating transaction. Anonymous creation assigns no
// owner.
func (c *Coordinator) ResourceCreated(ctx context.Context, tx *sql.Tx, resource auth.SemanticResourceKind, identifier string) error {
	kind, covered := KindForSemantic(resource)
	if !covered || !auth.IsAuthenticated(ctx) {
		return nil
	}
	principal, ok := PrincipalFromClaims(auth.ClaimsFromContext(ctx), c.groupClaim)
	if !ok {
		return nil
	}
	authUUID, found, err := LookupAuthUUID(ctx, tx, kind, identifier)
	if err != nil || !found {
		return firstError(err, fmt.Errorf("REBAC-RESOURCECREATED-LOOKUP created resource not found"))
	}
	grant := Grant{
		ObjectKey: ResourceKey(kind.ObjectType, authUUID), ObjectType: kind.ObjectType, ObjectUUID: authUUID,
		Relation: RelationOwner, SubjectType: TypeUser, SubjectKey: principal.UserKey(),
		SubjectIssuer: principal.Issuer, SubjectName: principal.Subject, CreatedBy: principal.UserKey(),
	}
	if _, err = InsertGrant(ctx, tx, grant); err != nil {
		return fmt.Errorf("REBAC-RESOURCECREATED-GRANT: %w", err)
	}
	if _, err = LockObjectRevision(ctx, tx, grant.ObjectKey); err != nil {
		return err
	}
	_, err = BumpObjectRevision(ctx, tx, grant.ObjectKey)
	return err
}

// ResourceDeleted removes the grants, element grants, links, invitations and
// revisions of an identifiable in the deleting transaction.
func (c *Coordinator) ResourceDeleted(ctx context.Context, tx *sql.Tx, resource auth.SemanticResourceKind, identifier string) error {
	kind, covered := KindForSemantic(resource)
	if !covered {
		return nil
	}
	authUUID, found, err := LookupAuthUUID(ctx, tx, kind, identifier)
	if err != nil || !found {
		return err
	}
	resourceKey := ResourceKey(kind.ObjectType, authUUID)
	if _, err = LockObjectRevision(ctx, tx, resourceKey); err != nil {
		return err
	}
	if _, err = deleteLinksOfResource(ctx, tx, kind, authUUID); err != nil {
		return err
	}
	grants, err := DeleteGrantsOfResource(ctx, tx, authUUID)
	if err != nil {
		return err
	}
	if err = DeleteInvitationsOfResource(ctx, tx, authUUID); err != nil {
		return err
	}
	objectKeys := []string{resourceKey}
	for _, grant := range grants {
		objectKeys = append(objectKeys, grant.ObjectKey)
	}
	return DeleteObjectRevisions(ctx, tx, uniqueStrings(objectKeys))
}

func deleteLinksOfResource(ctx context.Context, tx *sql.Tx, kind ResourceKind, authUUID string) ([]SubmodelLink, error) {
	switch kind.ObjectType {
	case TypeAAS:
		return DeleteSubmodelLinks(ctx, tx, LinkOfAAS(authUUID))
	case TypeSubmodel:
		return DeleteSubmodelLinks(ctx, tx, LinkOfSubmodel(authUUID))
	default:
		return nil, nil
	}
}

// SubmodelReferenceRemoved removes the approved link between a shell and a
// Submodel whose reference the shell dropped. Links are also validated
// against live references when evaluated, so this only keeps state tidy.
func (c *Coordinator) SubmodelReferenceRemoved(ctx context.Context, tx *sql.Tx, aasIdentifier string, submodelIdentifier string) error {
	aasUUID, found, err := LookupAuthUUID(ctx, tx, KindAAS, aasIdentifier)
	if err != nil || !found {
		return err
	}
	submodelUUID, found, err := LookupAuthUUID(ctx, tx, KindSubmodel, submodelIdentifier)
	if err != nil || !found {
		return err
	}
	links, err := DeleteSubmodelLinks(ctx, tx, LinkBetween(aasUUID, submodelUUID))
	if err != nil || len(links) == 0 {
		return err
	}
	submodelKey := ResourceKey(TypeSubmodel, submodelUUID)
	if _, err = LockObjectRevision(ctx, tx, submodelKey); err != nil {
		return err
	}
	_, err = BumpObjectRevision(ctx, tx, submodelKey)
	return err
}

// DeleteInvitationsOfResource removes pending invitations of a deleted
// identifiable, including invitations to its element paths.
func DeleteInvitationsOfResource(ctx context.Context, tx *sql.Tx, authUUID string) error {
	ds := dialect.Delete(invitationTable).
		Where(goqu.C("object_uuid").Eq(goqu.L("?::uuid", authUUID))).Prepared(true)
	_, err := execDataset(ctx, tx, "REBAC-DELETEINVITATIONS", ds)
	return err
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

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
