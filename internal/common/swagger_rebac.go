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
}

const reBACAccessPathTemplate = `  {path}:
    get:
      tags: [ReBAC Access Management]
      summary: Gets the direct grants of a {tag}
      operationId: GetReBACAccess{tag}
      parameters:
{parameters}      responses:
        '200':
          description: Direct grants, approved inheritance links and projection state. The ETag is the access revision.
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
        '503':
          description: OpenFGA unavailable
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
          description: Grants replaced and applied to OpenFGA
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ReBACAccess'
        '202':
          description: Grants stored; the Location header points to the projection operation
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
          description: Missing resource or caller without any right
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
          description: Links replaced and applied
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
          description: A direct grant for the caller was created and applied
        '202':
          description: Grant stored; projection pending
        '404':
          description: Unknown, expired, revoked or exhausted invitation
  /security/rebac/status:
    get:
      tags: [ReBAC Administration]
      summary: Gets scope, store, model and projection state
      operationId: GetReBACStatus
      responses:
        '200':
          description: ReBAC status for administrators
        '404':
          description: Caller is not a configured administrator
  /security/rebac/operations/{operationId}:
    get:
      tags: [ReBAC Access Management]
      summary: Gets the projection state of an accepted access change
      operationId: GetReBACOperation
      parameters:
        - name: operationId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        '200':
          description: applied or pending
        '404':
          description: Unknown operation
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
      summary: Removes orphaned desired state and repairs OpenFGA drift
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
            enum: [aas, submodel, concept_description]
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
        sync:
          type: object
          properties:
            pendingOperations:
              type: integer
            revocationPending:
              type: boolean
    ReBACEffectiveRights:
      type: object
      properties:
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
        enum: [aas, submodel, concept_description]
    ReBACIfMatch:
      name: If-Match
      in: header
      required: true
      description: ETag of the access resource as returned by GET
      schema:
        type: string
`

// injectReBACManagementAPI documents the $access sub-resources of the
// resource families present in a service specification and the global
// ReBAC management routes.
func injectReBACManagementAPI(specContent []byte) []byte {
	content := string(specContent)
	if strings.Contains(content, "  /security/rebac/status:") {
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
