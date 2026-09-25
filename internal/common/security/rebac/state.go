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
	"log/slog"

	"github.com/doug-martin/goqu/v9"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// ResourceCreated makes an authenticated creator the owner of a new
// identifiable. The owner tuple is written to OpenFGA before commit: the
// object is new and its UUID is never reused, so a rolled-back creation only
// leaves an inert tuple. Anonymous creation assigns no owner.
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
		ObjectKey: ResourceObject(kind.ObjectType, authUUID), ObjectType: kind.ObjectType, ObjectUUID: authUUID,
		Relation: RelationOwner, SubjectType: TypeUser, SubjectKey: principal.UserObject(),
		SubjectIssuer: principal.Issuer, SubjectName: principal.Subject, CreatedBy: principal.UserObject(),
	}
	if _, err = InsertGrant(ctx, tx, grant); err != nil {
		return fmt.Errorf("REBAC-RESOURCECREATED-GRANT: %w", err)
	}
	if _, err = LockObjectRevision(ctx, tx, grant.ObjectKey); err != nil {
		return err
	}
	if _, err = BumpObjectRevision(ctx, tx, grant.ObjectKey); err != nil {
		return err
	}
	mode := OutboxApplied
	if err = c.client.Write(ctx, []Tuple{grant.Tuple()}, nil); err != nil {
		slog.WarnContext(ctx, "ReBAC owner tuple deferred to outbox", "error.code", "REBAC-RESOURCECREATED-DIRECTWRITE", "error", err)
		mode = OutboxPending
		c.projector.Notify()
	}
	_, err = EnqueueOutbox(ctx, tx, c.scope, []OutboxOperation{{Tuple: grant.Tuple()}}, mode)
	return err
}

// ResourceDeleted removes the grants, element grants, links and invitations
// of a deleted identifiable. Removing links revokes access to live Submodels
// and therefore engages the barrier until applied.
func (c *Coordinator) ResourceDeleted(ctx context.Context, tx *sql.Tx, resource auth.SemanticResourceKind, identifier string) error {
	kind, covered := KindForSemantic(resource)
	if !covered {
		return nil
	}
	authUUID, found, err := LookupAuthUUID(ctx, tx, kind, identifier)
	if err != nil || !found {
		return err
	}
	resourceKey := ResourceObject(kind.ObjectType, authUUID)
	if _, err = LockObjectRevision(ctx, tx, resourceKey); err != nil {
		return err
	}
	links, err := deleteLinksOfResource(ctx, tx, kind, authUUID)
	if err != nil {
		return err
	}
	if err = lockLinkedObjects(ctx, tx, links); err != nil {
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
	operations := make([]OutboxOperation, 0, len(grants)+len(links))
	for _, grant := range grants {
		objectKeys = append(objectKeys, grant.ObjectKey)
		operations = append(operations, OutboxOperation{Delete: true, Tuple: grant.Tuple()})
	}
	if err = DeleteObjectRevisions(ctx, tx, uniqueStrings(objectKeys)); err != nil {
		return err
	}
	mode := OutboxPending
	for _, link := range links {
		operations = append(operations, OutboxOperation{Delete: true, Tuple: link.Tuple()})
		mode = OutboxRevocation
	}
	_, err = EnqueueOutbox(ctx, tx, c.scope, operations, mode)
	c.projector.Notify()
	return err
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

// SubmodelReferenceRemoved removes the approved link between an AAS and a
// Submodel whose reference was removed from the AAS.
func (c *Coordinator) SubmodelReferenceRemoved(ctx context.Context, tx *sql.Tx, aasIdentifier string, submodelIdentifier string) error {
	aasAuthUUID, found, err := LookupAuthUUID(ctx, tx, KindAAS, aasIdentifier)
	if err != nil || !found {
		return err
	}
	submodelUUID, found, err := LookupAuthUUID(ctx, tx, KindSubmodel, submodelIdentifier)
	if err != nil || !found {
		return err
	}
	links, err := DeleteSubmodelLinks(ctx, tx, LinkBetween(aasAuthUUID, submodelUUID))
	if err != nil {
		return err
	}
	return c.revokeLinks(ctx, tx, links)
}

func (c *Coordinator) revokeLinks(ctx context.Context, tx *sql.Tx, links []SubmodelLink) error {
	if len(links) == 0 {
		return nil
	}
	if err := lockLinkedObjects(ctx, tx, links); err != nil {
		return err
	}
	operations := make([]OutboxOperation, len(links))
	for index, link := range links {
		operations[index] = OutboxOperation{Delete: true, Tuple: link.Tuple()}
	}
	_, err := EnqueueOutbox(ctx, tx, c.scope, operations, OutboxRevocation)
	c.projector.Notify()
	return err
}

// lockLinkedObjects serializes link changes with grant changes of the linked
// Submodels and bumps their access revision.
func lockLinkedObjects(ctx context.Context, tx *sql.Tx, links []SubmodelLink) error {
	for _, link := range links {
		object := link.Tuple().Object
		if _, err := LockObjectRevision(ctx, tx, object); err != nil {
			return err
		}
		if _, err := BumpObjectRevision(ctx, tx, object); err != nil {
			return err
		}
	}
	return nil
}

// DeleteInvitationsOfResource removes pending invitations of a deleted
// identifiable, including invitations to its element paths.
func DeleteInvitationsOfResource(ctx context.Context, tx *sql.Tx, authUUID string) error {
	ds := dialect.Delete(invitationTable).
		Where(goqu.C("object_uuid").Eq(goqu.L("?::uuid", authUUID))).Prepared(true)
	_, err := execDataset(ctx, tx, "REBAC-DELETEINVITATIONS", ds)
	return err
}
