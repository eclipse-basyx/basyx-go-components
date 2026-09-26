# ReBAC Sharing API

This guide is for people and clients who share resources through the API,
for example to automate sharing or to build a user interface. Read the
[overview](README.md) for roles and concepts. Administrators find setup and
operations in the [administration guide](administration.md).

## Contents

- [Basics](#basics)
- [Access of a resource](#access-of-a-resource)
- [SubmodelElement grants](#submodelelement-grants)
- [Shell links](#shell-links)
- [Descriptors and discovery entries](#descriptors-and-discovery-entries)
- [Lists, aggregates, packages and passports](#lists-aggregates-packages-and-passports)
- [Invitation links](#invitation-links)
- [Repository grants](#repository-grants)
- [Administration](#administration)
- [Errors](#errors)

## Basics

- **Detection.** Services with ReBAC enabled list the profile
  `https://basyx.org/aas/API/3/2/RelationshipBasedAccessControl/1.0` in
  `GET /description`. Offer sharing only for services that announce it.
- **Authentication.** All endpoints need an OIDC access token. Anonymous
  callers never manage access.
- **Identities.** Grants name a user or group by `issuer` and `subject`. For
  users, `subject` is the value of the claim the administrator configured
  (`sub` by default). `GET /security/rebac/principal` returns the caller as
  ReBAC identifies them; show this subject as the user ID others share
  with:

  ```json
  {"issuer": "https://idp.example.com/realms/basyx", "subject": "8d1c…", "groups": ["engineering"], "administrator": false}
  ```

- **Concurrency.** Every managed object has an access revision, returned as
  `ETag`. Changes need `If-Match` with the current revision.
- **Management root.** Endpoints that do not belong to one resource live
  below `/security/rebac` of any ReBAC-enabled service; all services of one
  database share the same state.
- **Browsers.** `ETag` and `Location` are exposed through CORS; requests
  send `If-Match` and `Content-Type`.

## Access of a resource

Every managed resource has an `$access` sub-resource:

| Resource | `$access` path |
| --- | --- |
| Shell | `/shells/{id}/$access` |
| Submodel | `/submodels/{id}/$access` |
| SubmodelElement | `/submodels/{id}/submodel-elements/{idShortPath}/$access` |
| Concept Description | `/concept-descriptions/{id}/$access` |
| Shell descriptor | `/shell-descriptors/{id}/$access` |
| Submodel descriptor | `/submodel-descriptors/{id}/$access` |
| Discovery entry | `/lookup/shells/{id}/$access` |
| AASX package | `/packages/{packageId}/$access` |

Identifiers are base64url encoded as in the AAS API. The idShort path may be
URL encoded, for example `Markings%5B0%5D` for `Markings[0]`.

| Operation | Contract |
| --- | --- |
| `GET …/$access` | The access document; `ETag` is the access revision. Needs manage. |
| `PUT …/$access/grants` | Replaces the direct grants. `If-Match` required. Needs manage. |
| `GET …/$access/effective` | The caller's own rights per action. Needs any right on the resource. |
| `GET/POST …/$access/invitations`, `DELETE …/$access/invitations/{id}` | [Invitation links](#invitation-links). Needs manage. |
| `PUT /submodels/{id}/$access/inheritance` | [Shell links](#shell-links). `If-Match` required. |

Access document:

```json
{
  "object": {"type": "submodel", "id": "urn:example:sm:status"},
  "revision": 3,
  "grants": [
    {"relation": "owner", "subjectType": "user", "issuer": "https://idp.example.com/realms/basyx", "subject": "8d1c…",
     "createdBy": "user:…", "createdAt": "2026-09-26T10:00:00Z"}
  ],
  "inheritance": [{"aasId": "urn:example:aas:compressor", "approvedBy": "user:…", "approvedAt": "2026-09-26T10:05:00Z"}],
  "derivedFrom": {"type": "aas", "id": "urn:example:aas:compressor"}
}
```

`inheritance` appears for Submodels, `derivedFrom` for
[derived resources](#descriptors-and-discovery-entries). Elements report
`idShortPath` in `object`.

Replacing the grants sends the complete desired list; grants missing from it
are removed. `createdBy` and `createdAt` are ignored on input. At most 1000
grants per object are allowed.

```http
PUT /submodels/{id}/$access/grants
If-Match: "3"
Content-Type: application/json

{"grants": [
  {"relation": "owner",  "subjectType": "user",  "issuer": "https://idp.example.com/realms/basyx", "subject": "8d1c…"},
  {"relation": "viewer", "subjectType": "group", "issuer": "https://idp.example.com/realms/basyx", "subject": "engineering"}
]}
```

The response is the new access document with the new `ETag`. A resource
always keeps one owner: removing the last one fails with `409` unless an
administrator does it.

Effective rights report each action with its source: `abac` (the policy
allows it), `abac-conditional` (the policy allows it for matching content),
`rebac` (shared), `administrator` or `none`.

```json
{"object": {"type": "aas", "id": "urn:example:aas:compressor"},
 "rights": [{"action": "read", "source": "rebac"}, {"action": "update", "source": "none"},
            {"action": "delete", "source": "none"}, {"action": "execute", "source": "none"},
            {"action": "manage", "source": "none"}]}
```

## SubmodelElement grants

- A grant on an element covers the element and all its descendants, never
  its ancestors, siblings or the containing Submodel. `GET /submodels/{id}`
  stays denied for users with element grants only.
- `GET …/submodel-elements` returns only the granted subtrees to such users.
- Grants follow the **path**, not the element instance. An element deleted
  and recreated at the same path gets its grant back. A grant on a list item
  `list[2]` follows the index, not the item.
- ABAC fragment filters on **ancestors** of a granted element still apply.
- Deleting a Submodel deletes its element grants.

## Shell links

A shell's grants reach a Submodel only through an explicitly approved link.

- `PUT /submodels/{id}/$access/inheritance` with `{"aasIds": [...]}` and
  `If-Match` replaces the linked shells. The caller needs manage on the
  Submodel and on each shell, and each shell must reference the Submodel.
- A link passes on read, update, create-children and execute, **never**
  manage or delete. Owners of the shell do not become owners of the
  Submodel.
- A link only counts while the shell references the Submodel. Removing the
  reference ends it at once.
- Semantic IDs and other references never create links.

Superpath routes (`/shells/{aas}/submodels/{sm}/…`) need read access to the
shell (update for superpath `PUT` and `DELETE`) and the required right on
the Submodel.

## Descriptors and discovery entries

- Registering a descriptor or creating a discovery entry through the API
  makes the caller its owner.
- Descriptors that the AAS Environment or the DPP API synchronize from a
  shell or Submodel, and discovery entries that a registry creates for a new
  shell descriptor, are **derived**. They have no owner of their own and
  follow every permission of their source, including manage. They may carry
  additional direct grants. `derivedFrom` names the source.
- Matching identifiers never grant anything. If someone else registered a
  descriptor with the identifier of a shell, the owner of the shell gains no
  access to it.
- A Submodel descriptor embedded in a shell descriptor is part of the shell
  descriptor.

## Lists, aggregates, packages and passports

- **Lists** contain exactly the resources the caller may read through ABAC
  or ReBAC. Paging works as usual.
- **Creation.** Creating a top-level resource needs `creator` on its
  repository family (or an ABAC `CREATE` right); the creator becomes owner.
  Creating children (elements, references) needs update on the parent.
- **Deletion** needs delete on the resource. Deleting SubmodelElements,
  thumbnails or Submodel references edits the parent and needs update on
  it.
- **`/serialization`** returns only readable resources. **`/upload`**
  creates resources for creators and updates resources the caller may
  update; items are processed one by one.
- The **registry bulk API** uses the same rights as the single-item API.
  Bulk jobs and asynchronous package uploads stay bound to their caller.
- **AASX packages** belong to their uploader. The AAS identifiers stored
  with a package grant nothing.
- **Passports** are authorized through their shell. Submodels created with a
  passport are linked to the passport shell, so sharing the shell shares the
  whole passport except delete and manage. Share passports through the
  `$access` routes of the AAS and Submodel repositories.
- Asynchronous work (operation jobs, bulk jobs) re-evaluates grants with
  every query, so a revocation also stops queued work.

## Invitation links

An invitation grants a role to whoever signs in and accepts it.

```http
POST /submodels/{id}/$access/invitations
Content-Type: application/json

{"relation": "editor", "expiresAt": "2026-10-01T00:00:00Z", "maxUses": 1,
 "expectedPrincipal": {"issuer": "https://idp.example.com/realms/basyx", "subject": "8d1c…"}}
```

- `relation` is `viewer`, `editor` or `executor`; invitations never grant
  `owner`. `expiresAt` is required and at most 90 days ahead. `maxUses` is
  1 to 1000 (default 1). `expectedPrincipal` optionally restricts the
  invitation to one user.
- The response contains `id`, the settings, `usedCount`, `restricted` and a
  `token`. The token is returned only once; BaSyx stores its SHA-256 hash.
- `GET …/$access/invitations` lists open invitations without tokens;
  `DELETE …/$access/invitations/{id}` revokes one. Grants created by earlier
  acceptances stay.
- The recipient accepts with `POST /security/rebac/invitations/accept` and
  `{"token": "…"}`. This creates a normal grant for the caller and returns
  `{"object": {…}, "relation": "editor"}`. Invalid, expired, used-up or
  revoked tokens, and tokens for another user, answer `404`.
- Send the token to the recipient out of band, for example in the fragment
  of a UI link, never in a query string. The BaSyx UI builds links of the
  form `https://ui.example.com/#/share-access?token=…&service=…&component=…`.

## Repository grants

Creators and repository admins are granted per repository family (`aas`,
`submodel`, `concept_description`, `aas_descriptor`,
`submodel_descriptor`, `asset_links`, `aasx_package`):

```http
GET /security/rebac/repositories/submodel/$access
→ ETag: "0"

PUT /security/rebac/repositories/submodel/$access/grants
If-Match: "0"
Content-Type: application/json

{"grants": [
  {"relation": "creator", "subjectType": "group", "issuer": "https://idp.example.com/realms/basyx", "subject": "engineering"},
  {"relation": "admin",   "subjectType": "user",  "issuer": "https://idp.example.com/realms/basyx", "subject": "8d1c…"}
]}
```

Configured administrators and repository admins of the family may read and
change these grants.

## Administration

These endpoints are for configured administrators only. Others get `404`.

| Operation | Contract |
| --- | --- |
| `PUT /security/rebac/admin/owners/{type}/{base64url id}` | Sets the owners of a resource, for example one created before ReBAC was enabled. Body `{"owners": [grant, …]}`, `If-Match` required. `{type}` is `aas`, `submodel`, `concept_description`, `aas_descriptor`, `submodel_descriptor`, `asset_links` or `aasx_package`. |
| `POST /security/rebac/admin/reconcile` | Removes grants, invitations, derivations and links of resources that no longer exist, and reports the counts. |
| `GET /security/rebac/admin/audit` | Pages through the audit trail. |
| `GET /security/rebac/admin/audit/verify` | Verifies the audit trail in ranges. |

### Audit trail

`GET /security/rebac/admin/audit` returns `{"events": [...], "hasMore": true}`,
newest events first:

| Parameter | Meaning |
| --- | --- |
| `limit` | Events per page, 1 to 1000 (default 100). |
| `beforeId` | Continue with events older than this event id. |
| `afterId` | Page forwards, oldest first, from events newer than this id (for example to export the trail). Not combinable with `beforeId`. |
| `objectType`, `objectId` | Only events of one object: `aas`, `submodel`, `concept_description`, `aas_descriptor`, `submodel_descriptor`, `asset_links` or `aasx_package` with its identifier, `element` with the Submodel identifier and `idShortPath`, or `repository` with a repository family. Unknown resources return an empty page. |
| `object` | Only events of one object key as returned in `object`, also for deleted resources. |
| `actorIssuer`, `actorSubject` | Only changes made by one user. |

Each event has `id`, `occurredAt`, `type` (`grants_changed`,
`invitation_created`, `invitation_revoked`, `invitation_redeemed`,
`inheritance_changed`, `reconciled`), `actor`, `object`, `details`,
`previousHash`, `hash`, the WORM receipt as `evidence` when archived, and
`resource` (type, identifier and, for elements, the idShort path) while the
object still exists.

`GET /security/rebac/admin/audit/verify` verifies up to `limit` events
(default 1000, at most 10000) and returns:

```json
{"valid": true, "complete": false, "checked": 1000, "lastId": 1000, "headHash": "887e…",
 "evidenceVerified": 1000, "evidenceMissing": 0}
```

Continue with `afterId=<lastId>&afterHash=<headHash>` until `complete` is
true. The same pair, kept outside the database, is a checkpoint for later
verifications; a checkpoint that does not match the trail makes the result
invalid. `expectedHead` is compared once the end of the trail is reached.
An invalid result names `firstInvalidId` and a `reason`.

## Errors

| Status | Meaning |
| --- | --- |
| `400` | Invalid request, for example an unknown relation or too many grants. |
| `404` | The resource does not exist, or the caller may not manage it. Both look the same, so ReBAC never reveals whether a resource exists. |
| `409` | The last owner would be removed. |
| `412` | The access revision changed since it was read. Read again and retry. |
| `428` | `If-Match` is missing. |
| `503` | ReBAC is unavailable, or the evidence store rejected an access change. |

Denied data requests keep the ABAC behavior (`403` or `404`), whether or not
ReBAC was involved.
