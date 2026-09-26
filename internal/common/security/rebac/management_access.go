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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

const maxGrantsPerObject = 1000

var (
	errPreconditionRequired = errors.New("REBAC-MANAGEMENT-IFMATCHREQUIRED If-Match header is required")
	errPreconditionFailed   = errors.New("REBAC-MANAGEMENT-IFMATCHSTALE access revision changed")
	errTargetGone           = errors.New("REBAC-MANAGEMENT-TARGETGONE resource no longer exists")
)

// accessTarget is the resource or element path a management call manages.
type accessTarget struct {
	kind        ResourceKind
	identifier  string
	authUUID    string
	elementPath string
}

// kindRepository marks repository targets; their identifier is the object
// type of the repository family.
var kindRepository = ResourceKind{ObjectType: TypeRepository}

func (t accessTarget) objectType() string {
	if t.elementPath != "" {
		return TypeElement
	}
	return t.kind.ObjectType
}

func (t accessTarget) objectKey() string {
	switch {
	case t.elementPath != "":
		return ElementKey(t.authUUID, t.elementPath)
	case t.kind.ObjectType == TypeRepository:
		return RepositoryKey(t.identifier)
	default:
		return ResourceKey(t.kind.ObjectType, t.authUUID)
	}
}

type accessObject struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	IDShortPath string `json:"idShortPath,omitempty"`
}

type inheritanceLink struct {
	AASID      string    `json:"aasId"`
	ApprovedBy string    `json:"approvedBy"`
	ApprovedAt time.Time `json:"approvedAt"`
}

type accessDocument struct {
	Object      accessObject      `json:"object"`
	Revision    int64             `json:"revision"`
	Grants      []Grant           `json:"grants"`
	Inheritance []inheritanceLink `json:"inheritance,omitempty"`
	DerivedFrom *accessObject     `json:"derivedFrom,omitempty"`
}

type grantInput struct {
	Relation    string `json:"relation"`
	SubjectType string `json:"subjectType"`
	Issuer      string `json:"issuer"`
	Subject     string `json:"subject"`
}

type grantsDocument struct {
	Grants []grantInput `json:"grants"`
}

type inheritanceDocument struct {
	AASIDs []string `json:"aasIds"`
}

func revisionETag(revision int64) string {
	return `"` + strconv.FormatInt(revision, 10) + `"`
}

// requireRevision validates If-Match against the locked object revision.
func requireRevision(r *http.Request, current int64) error {
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	if ifMatch == "" {
		return errPreconditionRequired
	}
	if ifMatch != revisionETag(current) && ifMatch != "*" {
		return errPreconditionFailed
	}
	return nil
}

func (c *Coordinator) handleGetAccess(w http.ResponseWriter, r *http.Request, request accessRequest) {
	document, err := c.accessDocument(r.Context(), c.db, request.target)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	w.Header().Set("ETag", revisionETag(document.Revision))
	writeJSON(w, http.StatusOK, document)
}

func (c *Coordinator) accessDocument(ctx context.Context, q Queryer, target accessTarget) (accessDocument, error) {
	document := accessDocument{Object: accessObject{Type: target.objectType(), ID: target.identifier, IDShortPath: target.elementPath}}
	var err error
	if document.Revision, err = ObjectRevision(ctx, q, target.objectKey()); err != nil {
		return document, err
	}
	if document.Grants, err = ListGrants(ctx, q, target.objectKey()); err != nil {
		return document, err
	}
	if document.Grants == nil {
		document.Grants = []Grant{}
	}
	if target.kind.ObjectType == TypeSubmodel && target.elementPath == "" {
		if document.Inheritance, err = c.inheritanceLinks(ctx, q, target.authUUID); err != nil {
			return document, err
		}
	}
	if _, derivable := derivationSource(target.kind); derivable {
		document.DerivedFrom, err = derivedFromObject(ctx, q, target.authUUID)
	}
	return document, err
}

// derivedFromObject names the source a derived object inherits access from.
func derivedFromObject(ctx context.Context, q Queryer, authUUID string) (*accessObject, error) {
	sourceType, sourceUUID, found, err := derivationSourceOf(ctx, q, authUUID)
	if err != nil || !found {
		return nil, err
	}
	sourceKind, _ := KindForObjectType(sourceType)
	identifier, exists, err := IdentifierByAuthUUID(ctx, q, sourceKind, sourceUUID)
	if err != nil || !exists {
		return nil, err
	}
	return &accessObject{Type: sourceType, ID: identifier}, nil
}

func (c *Coordinator) inheritanceLinks(ctx context.Context, q Queryer, submodelUUID string) ([]inheritanceLink, error) {
	links, err := ListSubmodelLinks(ctx, q, submodelUUID)
	if err != nil {
		return nil, err
	}
	result := make([]inheritanceLink, 0, len(links))
	for _, link := range links {
		aasID, found, lookupErr := IdentifierByAuthUUID(ctx, q, KindAAS, link.AASUUID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if found {
			result = append(result, inheritanceLink{AASID: aasID, ApprovedBy: link.ApprovedBy, ApprovedAt: link.ApprovedAt})
		}
	}
	return result, nil
}

func (c *Coordinator) handlePutGrants(w http.ResponseWriter, r *http.Request, request accessRequest) {
	var body grantsDocument
	if err := decodeBody(r, &body); err != nil {
		writeManagementError(w, r, err)
		return
	}
	desired, err := buildGrants(request, body.Grants)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	c.replaceGrants(w, r, request, desired)
}

// replaceGrants replaces the direct grants of the request target.
func (c *Coordinator) replaceGrants(w http.ResponseWriter, r *http.Request, request accessRequest, desired []Grant) {
	var document accessDocument
	err := common.ExecuteInTransaction(c.db, "REBAC-PUTGRANTS-STARTTX", "REBAC-PUTGRANTS-COMMIT", func(tx *sql.Tx) error {
		if err := c.lockTarget(r, tx, request.target); err != nil {
			return err
		}
		current, err := ListGrants(r.Context(), tx, request.target.objectKey())
		if err != nil {
			return err
		}
		if !request.admin && countOwners(current) > 0 && countOwners(desired) == 0 {
			return common.NewErrConflict("REBAC-PUTGRANTS-LASTOWNER removing the last owner requires an administrator")
		}
		if err = c.applyGrantDiff(r.Context(), tx, request.target, current, desired); err != nil {
			return err
		}
		document, err = c.accessDocument(r.Context(), tx, request.target)
		return err
	})
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	respondAccess(w, document)
}

// respondAccess answers with the committed access document and its ETag.
func respondAccess(w http.ResponseWriter, document accessDocument) {
	w.Header().Set("ETag", revisionETag(document.Revision))
	writeJSON(w, http.StatusOK, document)
}

// lockTarget serializes access changes of the target, validates If-Match and
// ensures the resource still exists.
func (c *Coordinator) lockTarget(r *http.Request, tx *sql.Tx, target accessTarget) error {
	revision, err := LockObjectRevision(r.Context(), tx, target.objectKey())
	if err != nil {
		return err
	}
	if err = requireRevision(r, revision); err != nil {
		return err
	}
	if target.kind.ObjectType == TypeRepository {
		return nil
	}
	authUUID, found, err := LookupAuthUUID(r.Context(), tx, target.kind, target.identifier)
	if err != nil {
		return err
	}
	if !found || authUUID != target.authUUID {
		return errTargetGone
	}
	return nil
}

// applyGrantDiff stores the desired grants of one object. Changes take
// effect when the transaction commits.
func (c *Coordinator) applyGrantDiff(ctx context.Context, tx *sql.Tx, target accessTarget, current []Grant, desired []Grant) error {
	removed, added := diffGrants(current, desired)
	if len(removed) == 0 && len(added) == 0 {
		return nil
	}
	details := map[string]any{"added": auditedGrants(added), "removed": auditedGrants(removed)}
	if err := c.auditTarget(ctx, tx, AuditGrantsChanged, target, details); err != nil {
		return err
	}
	for _, grant := range removed {
		if err := DeleteGrant(ctx, tx, grant); err != nil {
			return err
		}
	}
	for _, grant := range added {
		if _, err := InsertGrant(ctx, tx, grant); err != nil {
			return err
		}
	}
	_, err := BumpObjectRevision(ctx, tx, target.objectKey())
	return err
}

// auditedGrants lists relations and subject keys of grants for the audit.
func auditedGrants(grants []Grant) []map[string]string {
	audited := make([]map[string]string, 0, len(grants))
	for _, grant := range grants {
		audited = append(audited, map[string]string{"relation": grant.Relation, "subject": grant.SubjectKey})
	}
	return audited
}

func diffGrants(current []Grant, desired []Grant) ([]Grant, []Grant) {
	var removed, added []Grant
	for _, grant := range current {
		if !containsGrant(desired, grant) {
			removed = append(removed, grant)
		}
	}
	for _, grant := range desired {
		if !containsGrant(current, grant) {
			added = append(added, grant)
		}
	}
	return removed, added
}

func containsGrant(grants []Grant, candidate Grant) bool {
	for _, grant := range grants {
		if grant.SameRelationship(candidate) {
			return true
		}
	}
	return false
}

func countOwners(grants []Grant) int {
	count := 0
	for _, grant := range grants {
		if grant.Relation == RelationOwner {
			count++
		}
	}
	return count
}

// buildGrants validates the requested grants of a target.
func buildGrants(request accessRequest, inputs []grantInput) ([]Grant, error) {
	if len(inputs) > maxGrantsPerObject {
		return nil, common.NewErrBadRequest(fmt.Sprintf("REBAC-PUTGRANTS-TOOMANY at most %d grants per object", maxGrantsPerObject))
	}
	grants := make([]Grant, 0, len(inputs))
	for _, input := range inputs {
		grant, err := buildGrant(request, input)
		if err != nil {
			return nil, err
		}
		if !containsGrant(grants, grant) {
			grants = append(grants, grant)
		}
	}
	return grants, nil
}

func buildGrant(request accessRequest, input grantInput) (Grant, error) {
	target := request.target
	relation := strings.TrimSpace(input.Relation)
	if err := ValidateRelation(target.objectType(), relation); err != nil {
		return Grant{}, common.NewErrBadRequest(err.Error())
	}
	issuer, subject := strings.TrimSpace(input.Issuer), strings.TrimSpace(input.Subject)
	if issuer == "" || subject == "" || len(issuer) > 2048 || len(subject) > 2048 {
		return Grant{}, common.NewErrBadRequest("REBAC-PUTGRANTS-SUBJECT issuer and subject are required and limited to 2048 characters")
	}
	grant := Grant{
		ObjectKey: target.objectKey(), ObjectType: target.objectType(), ObjectUUID: target.authUUID,
		ElementPath: target.elementPath, Relation: relation, SubjectType: input.SubjectType,
		SubjectIssuer: issuer, SubjectName: subject, CreatedBy: request.principal.UserKey(),
	}
	switch input.SubjectType {
	case TypeUser:
		grant.SubjectKey = UserKey(issuer, subject)
	case TypeGroup:
		grant.SubjectKey = GroupKey(issuer, subject)
	default:
		return Grant{}, common.NewErrBadRequest("REBAC-PUTGRANTS-SUBJECTTYPE subjectType must be user or group")
	}
	return grant, nil
}

func (c *Coordinator) handlePutInheritance(w http.ResponseWriter, r *http.Request, request accessRequest) {
	var body inheritanceDocument
	if err := decodeBody(r, &body); err != nil {
		writeManagementError(w, r, err)
		return
	}
	aasUUIDs, err := c.approvedAAS(r.Context(), request, body.AASIDs)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	var document accessDocument
	err = common.ExecuteInTransaction(c.db, "REBAC-PUTINHERITANCE-STARTTX", "REBAC-PUTINHERITANCE-COMMIT", func(tx *sql.Tx) error {
		if lockErr := c.lockTarget(r, tx, request.target); lockErr != nil {
			return lockErr
		}
		if txErr := c.replaceLinks(r.Context(), tx, request, aasUUIDs); txErr != nil {
			return txErr
		}
		var txErr error
		document, txErr = c.accessDocument(r.Context(), tx, request.target)
		return txErr
	})
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	respondAccess(w, document)
}

// approvedAAS resolves the AAS to link. Every AAS must reference the
// Submodel and be manageable by the caller; the error does not reveal which
// condition failed.
func (c *Coordinator) approvedAAS(ctx context.Context, request accessRequest, identifiers []string) ([]string, error) {
	aasUUIDs := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		identifier = strings.TrimSpace(identifier)
		authUUID, found, err := LookupAuthUUID(ctx, c.db, KindAAS, identifier)
		if err != nil {
			return nil, err
		}
		rejected := common.NewErrBadRequest(fmt.Sprintf("REBAC-PUTINHERITANCE-AAS AAS %q is unknown, does not reference the Submodel, or is not manageable", identifier))
		if !found {
			return nil, rejected
		}
		manageable, err := c.canManage(ctx, accessRequest{principal: request.principal, admin: request.admin, target: accessTarget{kind: KindAAS, identifier: identifier, authUUID: authUUID}})
		if err != nil {
			return nil, err
		}
		referenced, err := SubmodelReferenced(ctx, c.db, authUUID, request.target.authUUID)
		if err != nil {
			return nil, err
		}
		if !manageable || !referenced {
			return nil, rejected
		}
		aasUUIDs = append(aasUUIDs, authUUID)
	}
	return uniqueStrings(aasUUIDs), nil
}

func (c *Coordinator) replaceLinks(ctx context.Context, tx *sql.Tx, request accessRequest, aasUUIDs []string) error {
	current, err := ListSubmodelLinks(ctx, tx, request.target.authUUID)
	if err != nil {
		return err
	}
	removed, err := removeUnapprovedLinks(ctx, tx, current, aasUUIDs)
	if err != nil {
		return err
	}
	added, err := addApprovedLinks(ctx, tx, request, current, aasUUIDs)
	if err != nil || removed+added == 0 {
		return err
	}
	if err = c.auditTarget(ctx, tx, AuditInheritanceChanged, request.target, map[string]any{"linkedShells": aasUUIDs}); err != nil {
		return err
	}
	_, err = BumpObjectRevision(ctx, tx, request.target.objectKey())
	return err
}

func removeUnapprovedLinks(ctx context.Context, tx *sql.Tx, current []SubmodelLink, aasUUIDs []string) (int, error) {
	removed := 0
	for _, link := range current {
		if containsString(aasUUIDs, link.AASUUID) {
			continue
		}
		if _, err := DeleteSubmodelLinks(ctx, tx, LinkBetween(link.AASUUID, link.SubmodelUUID)); err != nil {
			return 0, err
		}
		removed++
	}
	return removed, nil
}

func addApprovedLinks(ctx context.Context, tx *sql.Tx, request accessRequest, current []SubmodelLink, aasUUIDs []string) (int, error) {
	added := 0
	for _, aasUUID := range aasUUIDs {
		if containsLink(current, aasUUID) {
			continue
		}
		link := SubmodelLink{SubmodelUUID: request.target.authUUID, AASUUID: aasUUID, ApprovedBy: request.principal.UserKey()}
		referenced, err := SubmodelReferenced(ctx, tx, aasUUID, link.SubmodelUUID)
		if err != nil {
			return 0, err
		}
		if !referenced {
			return 0, common.NewErrConflict("REBAC-PUTINHERITANCE-REFERENCE a reference was removed concurrently")
		}
		if err = InsertSubmodelLink(ctx, tx, link); err != nil {
			return 0, err
		}
		added++
	}
	return added, nil
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func containsLink(links []SubmodelLink, aasUUID string) bool {
	for _, link := range links {
		if link.AASUUID == aasUUID {
			return true
		}
	}
	return false
}
