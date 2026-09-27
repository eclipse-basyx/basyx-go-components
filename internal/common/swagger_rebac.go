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

package common

import (
	"strings"
)

// reBACAccessBase describes one $access sub-resource documented in a
// service specification.
type reBACAccessBase struct {
	marker      string
	path        string
	parameters  string
	tag         string
	inheritance bool
}

var reBACAccessBases = []reBACAccessBase{
	{marker: "  /shells/{aasIdentifier}:\n", path: "/shells/{aasIdentifier}/$access", tag: "Shell",
		parameters: "        - $ref: '#/components/parameters/ReBACAASIdentifier'\n"},
	{marker: "  /submodels/{submodelIdentifier}:\n", path: "/submodels/{submodelIdentifier}/$access", tag: "Submodel", inheritance: true,
		parameters: "        - $ref: '#/components/parameters/ReBACSubmodelIdentifier'\n"},
	{marker: "  /submodels/{submodelIdentifier}/submodel-elements/{idShortPath}:\n",
		path: "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$access", tag: "SubmodelElement",
		parameters: "        - $ref: '#/components/parameters/ReBACSubmodelIdentifier'\n        - $ref: '#/components/parameters/ReBACIdShortPath'\n"},
	{marker: "  /concept-descriptions/{cdIdentifier}:\n", path: "/concept-descriptions/{cdIdentifier}/$access", tag: "ConceptDescription",
		parameters: "        - $ref: '#/components/parameters/ReBACCDIdentifier'\n"},
	{marker: "  /shell-descriptors/{aasIdentifier}:\n", path: "/shell-descriptors/{aasIdentifier}/$access", tag: "ShellDescriptor",
		parameters: "        - $ref: '#/components/parameters/ReBACAASIdentifier'\n"},
	{marker: "  /submodel-descriptors/{submodelIdentifier}:\n", path: "/submodel-descriptors/{submodelIdentifier}/$access", tag: "SubmodelDescriptor",
		parameters: "        - $ref: '#/components/parameters/ReBACSubmodelIdentifier'\n"},
	{marker: "  /lookup/shells/{aasIdentifier}:\n", path: "/lookup/shells/{aasIdentifier}/$access", tag: "AssetLinks",
		parameters: "        - $ref: '#/components/parameters/ReBACAASIdentifier'\n"},
	{marker: "  /packages/{packageId}:\n", path: "/packages/{packageId}/$access", tag: "AASXPackage",
		parameters: "        - $ref: '#/components/parameters/ReBACPackageIdentifier'\n"},
}

const reBACAccessPathTemplate = `  {path}:
    get:
      tags: [ReBAC Access Management]
      summary: Gets the direct grants of a {tag}
      operationId: GetReBACAccess{tag}
      parameters:
{parameters}      responses:
        '200':
          description: Direct grants and approved inheritance links. The ETag is the access revision.
          headers:
            ETag:
              schema:
                type: string
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACAccess'
        '404':
          description: Missing resource or caller without can_manage
  {path}/grants:
    put:
      tags: [ReBAC Access Management]
      summary: Replaces the direct grants of a {tag}
      operationId: PutReBACGrants{tag}
      parameters:
{parameters}        - $ref: '#/components/parameters/ReBACIfMatch'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ReBACGrants'
            example:
              grants:
                - relation: owner
                  subjectType: user
                  issuer: https://idp.example/realms/basyx
                  subject: 7b1e0f0e-8d4c-4f4e-9b43-7f3d2b1a0c11
                - relation: viewer
                  subjectType: group
                  issuer: https://idp.example/realms/basyx
                  subject: engineering
      responses:
        '200':
          description: Grants replaced; they take effect immediately
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACAccess'
        '404':
          description: Missing resource or caller without can_manage
        '409':
          description: Removing the last owner requires an administrator
        '412':
          description: The access revision changed since it was read
        '428':
          description: If-Match is required
  {path}/effective:
    get:
      tags: [ReBAC Access Management]
      summary: Gets the caller's effective rights on a {tag}
      operationId: GetReBACEffective{tag}
      parameters:
{parameters}      responses:
        '200':
          description: Effective rights with their source (abac, abac-conditional, rebac, administrator, none)
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACEffectiveRights'
        '404':
          description: Missing resource or caller without a confirmed right (conditional ABAC rights do not confirm access)
  {path}/invitations:
    get:
      tags: [ReBAC Access Management]
      summary: Lists pending invitations of a {tag}
      operationId: GetReBACInvitations{tag}
      parameters:
{parameters}      responses:
        '200':
          description: Pending invitations without tokens
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACInvitations'
        '404':
          description: Missing resource or caller without can_manage
    post:
      tags: [ReBAC Access Management]
      summary: Creates an invitation link for a {tag}
      operationId: PostReBACInvitation{tag}
      parameters:
{parameters}      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ReBACInvitationRequest'
            example:
              relation: viewer
              expiresAt: '2026-12-31T00:00:00Z'
              maxUses: 1
      responses:
        '201':
          description: Invitation created; the token is only returned in this response
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACInvitation'
        '400':
          description: Invalid relation, expiry or maxUses
        '404':
          description: Missing resource or caller without can_manage
  {path}/invitations/{invitationId}:
    delete:
      tags: [ReBAC Access Management]
      summary: Revokes a pending invitation of a {tag}
      operationId: DeleteReBACInvitation{tag}
      parameters:
{parameters}        - name: invitationId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        '204':
          description: Invitation revoked; grants already redeemed stay in place
        '404':
          description: Unknown invitation or caller without can_manage
`

const reBACInheritancePathTemplate = `  {path}/inheritance:
    put:
      tags: [ReBAC Access Management]
      summary: Replaces the approved AAS inheritance links of a Submodel
      operationId: PutReBACInheritance
      parameters:
{parameters}        - $ref: '#/components/parameters/ReBACIfMatch'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [aasIds]
              properties:
                aasIds:
                  type: array
                  items:
                    type: string
      responses:
        '200':
          description: Links replaced
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACAccess'
        '400':
          description: An AAS is unknown, does not reference the Submodel, or is not manageable by the caller
        '412':
          description: The access revision changed since it was read
        '428':
          description: If-Match is required
`

const reBACGlobalPathsYAML = `  /security/rebac/invitations/accept:
    post:
      tags: [ReBAC Access Management]
      summary: Redeems an invitation for the authenticated caller
      operationId: AcceptReBACInvitation
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [token]
              properties:
                token:
                  type: string
      responses:
        '200':
          description: A direct grant for the caller was created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACAcceptedInvitation'
        '404':
          description: Unknown, expired, revoked, exhausted or foreign invitation
  /security/rebac/principal:
    get:
      tags: [ReBAC Access Management]
      summary: Gets the caller as ReBAC identifies them
      description: The subject is the value of the configured subject claim (rebac.subjectClaim); share with this user ID.
      operationId: GetReBACPrincipal
      responses:
        '200':
          description: Issuer, subject, current groups and administrator status of the caller
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACPrincipal'
        '404':
          description: Anonymous caller
  /security/rebac/repositories/{repositoryKind}/$access:
    get:
      tags: [ReBAC Administration]
      summary: Gets creator and admin grants of a repository family
      operationId: GetReBACRepositoryAccess
      parameters:
        - $ref: '#/components/parameters/ReBACRepositoryKind'
      responses:
        '200':
          description: Repository grants
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACAccess'
        '404':
          description: Caller is neither administrator nor repository admin
  /security/rebac/repositories/{repositoryKind}/$access/grants:
    put:
      tags: [ReBAC Administration]
      summary: Replaces creator and admin grants of a repository family
      operationId: PutReBACRepositoryGrants
      parameters:
        - $ref: '#/components/parameters/ReBACRepositoryKind'
        - $ref: '#/components/parameters/ReBACIfMatch'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ReBACGrants'
      responses:
        '200':
          description: Repository grants replaced
        '404':
          description: Caller is neither administrator nor repository admin
  /security/rebac/admin/reconcile:
    post:
      tags: [ReBAC Administration]
      summary: Removes grants, links and invitations of deleted resources
      operationId: PostReBACReconcile
      responses:
        '200':
          description: Reconciliation report
        '404':
          description: Caller is not a configured administrator
  /security/rebac/admin/owners/{objectType}/{identifier}:
    put:
      tags: [ReBAC Administration]
      summary: Replaces the owners of a resource (ownership recovery)
      operationId: PutReBACOwners
      parameters:
        - name: objectType
          in: path
          required: true
          schema:
            type: string
            enum: [aas, submodel, concept_description, aas_descriptor, submodel_descriptor, asset_links, aasx_package]
        - name: identifier
          in: path
          required: true
          description: base64url-encoded identifier
          schema:
            type: string
        - $ref: '#/components/parameters/ReBACIfMatch'
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                owners:
                  type: array
                  items:
                    $ref: '#/components/schemas/ReBACGrant'
      responses:
        '200':
          description: Owners replaced
        '404':
          description: Caller is not a configured administrator
  /security/rebac/admin/audit:
    get:
      tags: [ReBAC Administration]
      summary: Pages through the audit trail of access changes
      description: Newest events first; afterId pages forwards from the oldest events instead.
      operationId: GetReBACAudit
      parameters:
        - name: limit
          in: query
          schema:
            type: integer
            minimum: 1
            maximum: 1000
            default: 100
        - name: beforeId
          in: query
          description: Continue with events older than this event id
          schema:
            type: integer
            minimum: 0
        - name: afterId
          in: query
          description: Page forwards from events newer than this event id; not combinable with beforeId
          schema:
            type: integer
            minimum: 0
        - name: objectType
          in: query
          description: Object type of the filter, together with objectId
          schema:
            type: string
            enum: [aas, submodel, element, concept_description, aas_descriptor, submodel_descriptor, asset_links, aasx_package, repository]
        - name: objectId
          in: query
          description: Identifier of the object; the Submodel identifier for elements, the repository family for repositories
          schema:
            type: string
        - name: idShortPath
          in: query
          description: idShort path, only with objectType element
          schema:
            type: string
        - name: object
          in: query
          description: Object key as returned in events, also for deleted resources
          schema:
            type: string
        - name: actorIssuer
          in: query
          description: Issuer of the user who made the change, together with actorSubject
          schema:
            type: string
        - name: actorSubject
          in: query
          description: User ID of the user who made the change
          schema:
            type: string
      responses:
        '200':
          description: One page of audit events
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACAuditPage'
        '400':
          description: Invalid page or filter
        '404':
          description: Caller is not a configured administrator
  /security/rebac/admin/audit/verify:
    get:
      tags: [ReBAC Administration]
      summary: Verifies a range of the hash-chained audit trail
      description: Continue with afterId=lastId and afterHash=headHash until complete is true.
      operationId: GetReBACAuditVerification
      parameters:
        - name: limit
          in: query
          description: Events to verify in this range
          schema:
            type: integer
            minimum: 1
            maximum: 10000
            default: 1000
        - name: afterId
          in: query
          description: Checkpoint event id; verification starts after it
          schema:
            type: integer
            minimum: 0
        - name: afterHash
          in: query
          description: Hash of the checkpoint event, required with afterId
          schema:
            type: string
        - name: expectedHead
          in: query
          description: Head hash retained outside the database, compared once the end of the trail is reached
          schema:
            type: string
      responses:
        '200':
          description: Verification report of the range
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACAuditVerification'
        '400':
          description: Invalid range or checkpoint
        '404':
          description: Caller is not a configured administrator
`

const reBACSchemasYAML = `    ReBACGrant:
      type: object
      required: [relation, subjectType, issuer, subject]
      properties:
        relation:
          type: string
          enum: [owner, editor, viewer, executor, creator, admin]
        subjectType:
          type: string
          enum: [user, group]
        issuer:
          type: string
        subject:
          type: string
          description: OIDC subject for users, group name for groups
        createdBy:
          type: string
          readOnly: true
        createdAt:
          type: string
          format: date-time
          readOnly: true
    ReBACGrants:
      type: object
      required: [grants]
      properties:
        grants:
          type: array
          items:
            $ref: '#/components/schemas/ReBACGrant'
    ReBACAccess:
      type: object
      properties:
        object:
          type: object
          properties:
            type:
              type: string
            id:
              type: string
            idShortPath:
              type: string
        revision:
          type: integer
        grants:
          type: array
          items:
            $ref: '#/components/schemas/ReBACGrant'
        derivedFrom:
          type: object
          description: Source whose access a synchronized descriptor or discovery entry inherits
          properties:
            type:
              type: string
            id:
              type: string
        inheritance:
          type: array
          items:
            type: object
            properties:
              aasId:
                type: string
              approvedBy:
                type: string
              approvedAt:
                type: string
                format: date-time
    ReBACObject:
      type: object
      properties:
        type:
          type: string
        id:
          type: string
        idShortPath:
          type: string
    ReBACEffectiveRights:
      type: object
      properties:
        object:
          $ref: '#/components/schemas/ReBACObject'
        rights:
          type: array
          items:
            type: object
            properties:
              action:
                type: string
                enum: [read, update, delete, execute, manage]
              source:
                type: string
                enum: [abac, abac-conditional, rebac, administrator, none]
    ReBACInvitationRequest:
      type: object
      required: [relation, expiresAt]
      properties:
        relation:
          type: string
          enum: [viewer, editor, executor]
        expiresAt:
          type: string
          format: date-time
        maxUses:
          type: integer
          minimum: 1
          maximum: 1000
          default: 1
        expectedPrincipal:
          type: object
          description: Restricts redemption to one verified user
          required: [issuer, subject]
          properties:
            issuer:
              type: string
            subject:
              type: string
    ReBACInvitation:
      type: object
      properties:
        id:
          type: string
          format: uuid
        relation:
          type: string
        expiresAt:
          type: string
          format: date-time
        maxUses:
          type: integer
        usedCount:
          type: integer
        restricted:
          type: boolean
          description: Only the expected principal may redeem the invitation
        token:
          type: string
          description: Only returned when the invitation is created
    ReBACInvitations:
      type: object
      properties:
        invitations:
          type: array
          items:
            $ref: '#/components/schemas/ReBACInvitation'
    ReBACAcceptedInvitation:
      type: object
      properties:
        object:
          $ref: '#/components/schemas/ReBACObject'
        relation:
          type: string
          enum: [viewer, editor, executor]
    ReBACPrincipal:
      type: object
      properties:
        issuer:
          type: string
        subject:
          type: string
          description: Value of the configured subject claim
        groups:
          type: array
          items:
            type: string
        administrator:
          type: boolean
    ReBACAuditEvent:
      type: object
      properties:
        id:
          type: integer
        occurredAt:
          type: string
          format: date-time
        type:
          type: string
          enum: [grants_changed, invitation_created, invitation_revoked, invitation_redeemed, inheritance_changed, reconciled]
        actor:
          type: string
        object:
          type: string
          description: Object key
        details:
          type: object
        previousHash:
          type: string
        hash:
          type: string
        evidence:
          type: object
          description: WORM evidence receipt when history evidence is enabled
        resource:
          allOf:
            - $ref: '#/components/schemas/ReBACObject'
          description: Public identity of the object while it still exists
    ReBACAuditPage:
      type: object
      properties:
        events:
          type: array
          items:
            $ref: '#/components/schemas/ReBACAuditEvent'
        hasMore:
          type: boolean
    ReBACAuditVerification:
      type: object
      properties:
        valid:
          type: boolean
        complete:
          type: boolean
          description: The range reached the end of the trail
        checked:
          type: integer
        lastId:
          type: integer
        headHash:
          type: string
        firstInvalidId:
          type: integer
        reason:
          type: string
        evidenceVerified:
          type: integer
        evidenceMissing:
          type: integer
`

const reBACParametersYAML = `    ReBACAASIdentifier:
      name: aasIdentifier
      in: path
      required: true
      schema:
        type: string
    ReBACSubmodelIdentifier:
      name: submodelIdentifier
      in: path
      required: true
      schema:
        type: string
    ReBACCDIdentifier:
      name: cdIdentifier
      in: path
      required: true
      schema:
        type: string
    ReBACPackageIdentifier:
      name: packageId
      in: path
      required: true
      schema:
        type: string
    ReBACIdShortPath:
      name: idShortPath
      in: path
      required: true
      schema:
        type: string
    ReBACRepositoryKind:
      name: repositoryKind
      in: path
      required: true
      schema:
        type: string
        enum: [aas, submodel, concept_description, aas_descriptor, submodel_descriptor, asset_links, aasx_package]
    ReBACIfMatch:
      name: If-Match
      in: header
      required: true
      description: ETag of the access resource as returned by GET; it is bound to its object. * matches any revision.
      schema:
        type: string
`

// injectReBACManagementAPI documents the $access sub-resources of the
// resource families present in a service specification and the global
// ReBAC management routes.
// injectReBACManagementAPIIf documents the ReBAC routes when enabled.
func injectReBACManagementAPIIf(enabled bool, specContent []byte) []byte {
	if !enabled {
		return specContent
	}
	return injectReBACManagementAPI(specContent)
}

// reBACEnabled reports whether ReBAC is enabled in cfg.
func reBACEnabled(cfg *Config) bool {
	return cfg != nil && cfg.ReBAC.Enabled
}

func injectReBACManagementAPI(specContent []byte) []byte {
	content := string(specContent)
	if strings.Contains(content, "  /security/rebac/invitations/accept:") {
		return specContent
	}
	fragments := make([]string, 0, 2*len(reBACAccessBases)+1)
	for _, base := range reBACAccessBases {
		if !strings.Contains(content, base.marker) {
			continue
		}
		fragments = append(fragments, expandReBACTemplate(reBACAccessPathTemplate, base))
		if base.inheritance {
			fragments = append(fragments, expandReBACTemplate(reBACInheritancePathTemplate, base))
		}
	}
	fragments = append(fragments, reBACGlobalPathsYAML)
	specContent = injectComponentSchemas(specContent, reBACSchemasYAML)
	specContent = injectComponentParameters(specContent, reBACParametersYAML)
	return injectPathFragment(specContent, strings.Join(fragments, ""))
}

func expandReBACTemplate(template string, base reBACAccessBase) string {
	return strings.NewReplacer("{path}", base.path, "{tag}", base.tag, "{parameters}", base.parameters).Replace(template)
}

// injectComponentParameters adds parameters below components.parameters.
func injectComponentParameters(specContent []byte, parameters string) []byte {
	lines := strings.SplitAfter(string(specContent), "\n")
	componentsIndex := topLevelLineIndex(lines, "components:")
	if componentsIndex < 0 {
		content := ensureTrailingNewline(string(specContent))
		return []byte(content + "components:\n  parameters:\n" + parameters)
	}
	nextTopLevel := nextTopLevelLineIndex(lines, componentsIndex+1)
	for index := componentsIndex + 1; index < nextTopLevel && index < len(lines); index++ {
		if strings.TrimRight(lines[index], "\r\n") == "  parameters:" {
			return []byte(insertLines(lines, index+1, parameters))
		}
	}
	return []byte(insertLines(lines, componentsIndex+1, "  parameters:\n"+parameters))
}
