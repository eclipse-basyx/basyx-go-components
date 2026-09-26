# Relationship-Based Access Control (ReBAC)

> **Experimental.** The ReBAC integration, its management API and its
> database tables may change incompatibly in future releases. Configuration
> keys carry no experimental marker; this document is the only place that
> declares the status.

ReBAC lets resource owners share Asset Administration Shells, Submodels,
SubmodelElements, Concept Descriptions, registry descriptors, discovery
entries and AASX packages with other users or groups without changing the
ABAC policy. It runs next to ABAC as a **strict union**:

```text
access = ABAC allows  OR  ReBAC allows
```

ABAC syntax, evaluator semantics, policy management and existing
configurations are unchanged. With `rebac.enabled=false` (the default) no
ReBAC code is wired into the routers and every service behaves exactly as
before.

Relationships are stored in the BaSyx PostgreSQL database and evaluated there
as part of each query. There is no external authorization service. All
services sharing one database share the same relationships.

## Contents

- [Covered services and routes](#covered-services-and-routes)
- [ReBAC and ABAC interplay (read this first)](#rebac-and-abac-interplay-read-this-first)
- [Evaluation order](#evaluation-order)
- [Roles and relations](#roles-and-relations)
- [Identities](#identities)
- [SubmodelElement grants](#submodelelement-grants)
- [AAS-to-Submodel inheritance](#aas-to-submodel-inheritance)
- [Registries, discovery and synchronized descriptors](#registries-discovery-and-synchronized-descriptors)
- [Aggregates, packages and passports](#aggregates-packages-and-passports)
- [Ownership and administrators](#ownership-and-administrators)
- [Management API](#management-api)
- [Invitation links](#invitation-links)
- [Audit trail and evidence](#audit-trail-and-evidence)
- [Telemetry](#telemetry)
- [Configuration](#configuration)
- [Consistency and revocation](#consistency-and-revocation)
- [Reconciliation](#reconciliation)
- [Limits and known restrictions](#limits-and-known-restrictions)
- [Upgrade notes](#upgrade-notes)

## Covered services and routes

| Service | Covered resources |
| --- | --- |
| AAS Repository | Shells, asset information, thumbnails, Submodel references, Submodel superpaths, `/serialization` |
| Submodel Repository | Submodels and all representations, SubmodelElements, attachments, operations (sync, async, status, results) |
| Concept Description Repository | Concept Descriptions |
| AAS Registry, Submodel Registry, Digital Twin Registry | Shell descriptors including embedded Submodel descriptors, standalone Submodel descriptors, bulk API |
| Discovery | Discovery entries (the asset links of one shell) and lookups |
| AASX File Server | AASX packages, asynchronous uploads |
| AAS Environment | All of the above except AASX packages, plus `/upload` and `/serialization` |
| DPP API | Current state of Digital Product Passports |

The following stay **ABAC-only**; a ReBAC grant never gives access to them:
history endpoints (`$history`), `$recent-changes`, event feeds, `$signed`
representations, `/verify`, historical passports (`/v1/dppsByIdAndDate`),
the ABAC policy management API and the Company Lookup service.

## ReBAC and ABAC interplay (read this first)

ReBAC is a full union with ABAC. A ReBAC grant on a resource is **not limited
by ABAC fragment filters, masks or update conditions** for that resource:

- ABAC rules may hide individual SubmodelElements (for example with a `FILTER`
  on `$sme`) or mask fields. When the owner shares the Submodel with a user
  through ReBAC, that user sees the complete Submodel, including elements the
  ABAC filter would hide.
- ABAC update conditions (formulas on the written state) do not constrain a
  ReBAC editor of the resource.
- ABAC cannot cap what owners share through ReBAC. Anyone who owns or manages
  a resource can share it with any user or group.

Example: the policy lets role `editor` read Submodels with idShort `public`
but hides elements with idShort `secret`. User `userx` has that role.

| Request by `userx` | Result |
| --- | --- |
| `GET /submodels/{public}` | `200`, element `secret` hidden (ABAC) |
| `GET /submodels/{private}` without grant | `403` |
| `GET /submodels/{private}` after the owner granted `viewer` | `200`, **all** elements including `secret` |
| `GET /submodels/{public}` after that grant | `200`, `secret` still hidden (grants are per resource) |

Guidance for administrators:

- Keep sensitive content in **separate resources** (separate Submodels or
  Submodel elements with their own owners) instead of relying on ABAC filters
  inside a resource that will be shared.
- Grant ownership deliberately. Owners can share, invite and delete.
- Review grants through `GET …/$access` and the audit trail.

A grant only ever widens access to the granted resource. It never widens
other tables touched by the same request: a Submodel grant does not make the
enclosing shell of a superpath visible, and caller query conditions over
related resources (for example `$sm` fields inside an AAS query) are still
evaluated with ABAC visibility only.

## Evaluation order

1. The OIDC middleware authenticates the caller. Anonymous requests are
   ABAC-only.
2. ABAC evaluates the request exactly as before.
3. If ABAC unconditionally allows the required right (formula `true`, no
   fragment filters), the request proceeds without ReBAC.
4. Otherwise BaSyx resolves the addressed resource and decides in one SQL
   query whether the caller holds the required permission:
   - **allowed** – the request proceeds. The backend query is widened for the
     granted resources only. The grant is a subquery that is evaluated again
     by every backend query of the request.
   - **denied** – the ABAC result stands, including today's `403`/`404` and
     deny-as-not-found behavior.
   - **error** – `503 SECURITY-REBAC-UNAVAILABLE`. There is never a silent
     ABAC-only fallback when ReBAC was needed.

Unknown identifiers behave exactly as before; ReBAC never reveals whether a
resource exists. Lists are filtered in SQL, so they need no allowlist and
have no size limit.

## Roles and relations

| Role | Grants |
| --- | --- |
| `viewer` | read |
| `editor` | read, update, create children |
| `executor` | invoke operations and read their status and results |
| `owner` | everything above, plus delete and manage access |

`executor` exists for shells, Submodels and SubmodelElements only. Each
repository family (`aas`, `submodel`, `concept_description`,
`aas_descriptor`, `submodel_descriptor`, `asset_links`, `aasx_package`) has
two further relations:

| Repository relation | Meaning |
| --- | --- |
| `creator` | may create top-level resources of the family and becomes their owner |
| `admin` | has every right on every resource of the family and may manage repository grants |

Right mapping: `READ` → read, `UPDATE` and child `CREATE` → update,
top-level `DELETE` → delete, `EXECUTE` → execute. Deleting SubmodelElements,
thumbnails or Submodel references edits the parent and therefore needs update
on the parent.

## Identities

- Users are issuer-scoped: `user:<b64url(iss)>.<b64url(sub)>`. The same `sub`
  from another issuer is a different user.
- Groups are issuer-scoped: `group:<b64url(iss)>.<b64url(name)>`. Group names
  come from the normalized claim configured in `rebac.groupClaim`
  (default `groups`; use the OIDC `claimMappings` to normalize provider
  claims). Memberships are **never stored**; each decision uses the caller's
  current groups, so membership changes take effect with the next token.
- Resources use the persistent `auth_uuid` of their row. A resource that is
  deleted and recreated with the same identifier gets a new `auth_uuid` and
  never inherits old grants.

## SubmodelElement grants

Grants on SubmodelElements are keyed by the Submodel and the idShortPath:

- A grant covers the element and all descendants, never ancestors, siblings or
  the containing Submodel. `GET /submodels/{id}` stays denied for users with
  element grants only.
- Grants follow the **path**, not the element instance. They survive PUT and
  PATCH reconciliation; an element deleted and recreated at the same path gets
  its grant back. A grant on a list item `list[2]` follows the index, not the
  item.
- `GET …/submodel-elements` for users with element grants only returns the
  granted subtrees.
- ABAC fragment filters on **ancestors** of a granted element still apply.
- Deleting a Submodel deletes all of its element grants in the same
  transaction.

## AAS-to-Submodel inheritance

A shell's grants reach a Submodel only through an explicitly approved link:

- `PUT /submodels/{id}/$access/inheritance` with `{"aasIds": [...]}` replaces
  the approved shells. The caller needs manage on the Submodel and on each
  shell, and each shell must reference the Submodel.
- A link carries read, update, create-children and execute, **never** manage
  or delete.
- A link only counts while the shell references the Submodel. Removing the
  reference ends the inheritance at once; the link row is removed as well.
- Semantic IDs, other references and Concept Description references never
  create inheritance.

Superpath routes (`/shells/{aas}/submodels/{sm}/…`) keep the existing
reference check: the caller needs read access to the shell (update for
superpath PUT and DELETE) and the required right on the Submodel.

## Registries, discovery and synchronized descriptors

Shell descriptors, standalone Submodel descriptors and discovery entries are
ReBAC resources of their own, with `$access` sub-resources and repository
relations.

- A Submodel descriptor embedded in a shell descriptor is part of that shell
  descriptor. Its routes need the corresponding right on the shell
  descriptor.
- A caller who registers a descriptor or creates a discovery entry through
  the API becomes its owner.
- **Derived resources.** Descriptors that the AAS Environment or the DPP API
  synchronize from a shell or Submodel are *derived* from that source. So are
  discovery entries that a registry's discovery integration creates for a new
  shell descriptor. A derived resource has no owner of its own. It inherits
  every permission of its source, including manage, and may carry additional
  direct grants. `GET …/$access` reports the source as `derivedFrom`.
- Derivation is recorded only when the synchronization creates the resource.
  Matching identifiers alone never grant anything. If someone else already
  registered a descriptor with the identifier of a shell, the owner of the
  shell does not gain access to it, and synchronizing the shell over it fails.
- Requests that synchronize derived resources are authorized for them through
  their source. For example, an editor of a shell updates its descriptor
  through `PUT /shells/{id}`. A Submodel update refreshes the embedded
  descriptors in the descriptors of shells referencing the Submodel, without
  granting read access to those descriptors.
- Resources created before ReBAC was enabled have no derivation. Assign
  owners through ownership recovery if needed.

## Aggregates, packages and passports

- `/serialization` returns only resources the caller may read.
- `/upload` creates resources for creators of the respective families, who
  become their owners, and updates resources the caller may update. As with
  ABAC, the items of one upload are processed one by one.
- The registry bulk API creates, updates and deletes descriptors with the
  same rights as the single-item API. Bulk jobs and asynchronous package
  uploads stay bound to the caller who started them.
- AASX packages belong to their uploader. The package list shows ReBAC-only
  callers exactly the packages they may read. The AAS identifiers stored with
  a package are claims of the uploader and grant nothing.
- The DPP API authorizes a passport through its shell: reading, updating and
  deleting a passport needs the right on the shell, and each Submodel of the
  passport is authorized by its own relations. Submodels created with a
  passport are linked to the passport shell, so sharing the shell shares the
  whole passport (read, update and execute; delete and manage stay with the
  owners). Use the `$access` routes of the AAS and Submodel repositories to
  share passports.
- Asynchronous work that runs after the request, such as operation jobs and
  bulk jobs, evaluates the grants again with each query. A revocation
  therefore also stops work that is already queued.

## Ownership and administrators

- An authenticated creator becomes `owner` in the same transaction.
  Anonymous creation that ABAC allows assigns no owner. Updates never
  transfer ownership.
- Top-level creation needs `creator` on the repository family or an ABAC
  `CREATE` right.
- Existing resources and resources created while ReBAC was disabled have no
  owner. Administrators assign owners through ownership recovery.
- `rebac.administrators` lists bootstrap and recovery principals as
  `issuer|subject` or `issuer|group:<name>`. Administrators may manage every
  `$access` resource, repository grants, reconciliation, ownership recovery
  and the audit trail, and may remove the last owner.

Bootstrap example (administrator grants creators and a repository admin):

```http
GET /security/rebac/repositories/submodel/$access
→ ETag: "0"

PUT /security/rebac/repositories/submodel/$access/grants
If-Match: "0"
{"grants": [
  {"relation": "creator", "subjectType": "group", "issuer": "https://idp/realms/basyx", "subject": "engineering"},
  {"relation": "admin",   "subjectType": "user",  "issuer": "https://idp/realms/basyx", "subject": "8d1c…"}
]}
```

## Management API

All management routes sit behind OIDC but outside ABAC route evaluation, so a
missing ABAC rule never hides them. They authorize with the manage permission
(or administrator status); ABAC data rights never grant sharing. Denials
answer `404`, exactly like missing resources. Responses carry
`Cache-Control: no-store` and `Referrer-Policy: no-referrer`.

Every managed resource has an `$access` sub-resource:
`/shells/{id}/$access`, `/submodels/{id}/$access`,
`/submodels/{id}/submodel-elements/{idShortPath}/$access`,
`/concept-descriptions/{id}/$access`, `/shell-descriptors/{id}/$access`,
`/submodel-descriptors/{id}/$access`, `/lookup/shells/{id}/$access` and
`/packages/{packageId}/$access`. Each service mounts the sub-resources of
the resources it serves.

| Operation | Contract |
| --- | --- |
| `GET …/$access` | Direct grants, approved inheritance links and the source of derived resources. `ETag` is the access revision. |
| `PUT …/$access/grants` | Replaces the direct grants. `If-Match` required: missing → `428`, stale → `412`. Removing the last owner → `409` unless the caller is an administrator. |
| `GET …/$access/effective` | The caller's own rights per action with source `abac`, `abac-conditional`, `rebac`, `administrator` or `none`. Callers without any right get `404`. |
| `PUT /submodels/{id}/$access/inheritance` | Replaces approved AAS links (`If-Match` required). |
| `GET/POST …/$access/invitations`, `DELETE …/$access/invitations/{id}` | Invitation links, see below. |
| `GET/PUT /security/rebac/repositories/{kind}/$access[/grants]` | Creator and admin grants of a repository family. |
| `POST /security/rebac/admin/reconcile` | Removes orphaned state (administrators). |
| `PUT /security/rebac/admin/owners/{type}/{base64 id}` | Ownership recovery (administrators, `If-Match` required). |
| `GET /security/rebac/admin/audit?afterId=&limit=` | Pages through the audit trail (administrators). |
| `GET /security/rebac/admin/audit/verify?expectedHead=` | Verifies the audit trail (administrators). |

Grant changes take effect when their transaction commits.

Services with ReBAC enabled list the profile
`https://basyx.org/aas/API/3/2/RelationshipBasedAccessControl/1.0` in
`GET /description`. Clients such as the BaSyx UI show sharing only for
services that announce it. CORS responses expose `ETag` for browser clients.
The idShort path of an element `$access` route may be URL encoded, for
example `Markings%5B0%5D` for `Markings[0]`.

```http
PUT /submodels/{id}/$access/grants
If-Match: "3"
Content-Type: application/json

{"grants": [
  {"relation": "owner",  "subjectType": "user",  "issuer": "https://idp/realms/basyx", "subject": "8d1c…"},
  {"relation": "viewer", "subjectType": "group", "issuer": "https://idp/realms/basyx", "subject": "engineering"}
]}
```

There are no public or wildcard grants.

## Invitation links

- `POST …/$access/invitations` (needs manage) with
  `{"relation": "viewer|editor|executor", "expiresAt": "<RFC 3339>", "maxUses": 1}`
  creates an invitation. `expiresAt` is required and at most 90 days ahead;
  `maxUses` defaults to 1. An optional `expectedPrincipal`
  (`{"issuer": …, "subject": …}`) restricts redemption to one user. The
  response carries a one-time-visible `token`. Invitations never grant
  `owner`.
- `POST /security/rebac/invitations/accept` with `{"token": "…"}` requires an
  authenticated caller and creates a normal direct grant for the caller's own
  issuer and subject. The token itself never authorizes data access. The token
  is sent in the body so it never appears in URLs, access logs or traces.
- Tokens are 256-bit random values stored only as SHA-256 hashes. Redemption
  is atomic, so concurrent accepts never exceed `maxUses`.
- `DELETE …/$access/invitations/{id}` revokes a pending invitation. Grants
  that were already redeemed stay until they are removed individually.

## Audit trail and evidence

Every access change is appended to the audit trail `rebac_audit_event` in
the transaction of the change. This covers grant changes, invitations
(created, revoked, redeemed), inheritance changes and reconciliations that
removed state. Each event records its actor, the object key and the change.

- Events form a hash chain. Each event hash covers the event, its id and the
  hash of its predecessor. Writers are serialized, so the chain never forks.
- The table is append-only while the history guard is enabled, like the
  history tables.
- With history evidence enabled (`history.evidence.*`), each event is
  archived in the WORM store before its transaction commits. The receipt is
  stored with the event. If the store is unavailable, the change fails with
  `503` instead of being applied unaudited.
- `GET /security/rebac/admin/audit/verify` recomputes the chain. With an
  evidence store it also verifies every archived event against its WORM
  object. Pass a head hash retained outside the database as `expectedHead` to
  detect removed trailing events. The evidence verifier CLI offers the same
  check:

  ```bash
  historyevidenceverifier -config config.yaml -rebac-audit -expected-head-hash <hash>
  ```

Ownership assigned at creation and derivations are part of the resource
history, not of the audit trail. Denied data requests are not audited in the
database; they appear in the decision metrics and in the service logs.

## Telemetry

ReBAC uses the service's OpenTelemetry configuration:

- Span `rebac.decision` for every decision, with attributes `http.route` and
  `rebac.outcome` (`granted`, `none`, `uncovered`, `unavailable`).
- Counter `basyx.rebac.decisions` and histogram
  `basyx.rebac.decision.duration` (seconds), by `rebac.outcome` and
  `http.route`.
- Counter `basyx.rebac.access.changes` by `rebac.event`.
- Counter `basyx.rebac.management.denials` by `http.route`.

## Configuration

| Key | Env | Default | Purpose |
| --- | --- | --- | --- |
| `rebac.enabled` | `REBAC_ENABLED` | `false` | Master switch |
| `rebac.groupClaim` | `REBAC_GROUP_CLAIM` | `groups` | Normalized claim with group names |
| `rebac.administrators` | `REBAC_ADMINISTRATORS` (comma-separated) | `[]` | `issuer\|subject` or `issuer\|group:<name>` |

`rebac.enabled=true` requires `abac.enabled=true` and an OIDC trustlist.
Enable ReBAC consistently in all services that share a database.

See `examples/BaSyxReBACExample` for a complete compose setup.

## Consistency and revocation

The relationships (`rebac_grant`, `rebac_submodel_link`,
`rebac_derivation`, `rebac_invitation`) live in the database of the
resources. Decisions and grants are SQL over these tables.

- A grant change takes effect when its transaction commits. There is no
  cache and no background projection.
- Resource mutations write the owner grant, derivations and the removal of
  state of deleted resources in their own transaction.
- Changes of one object serialize on its access revision, which is also the
  `ETag` of `$access`.

## Reconciliation

While ReBAC is disabled, nothing tracks deletions or removed references.
Every startup with `rebac.enabled=true` removes grants, element grants,
invitations, derivations and links of resources that no longer exist and
links whose reference is gone. The service only becomes ready afterwards.
Administrators can run the same step through
`POST /security/rebac/admin/reconcile`. Reconciliation only removes state:
authorization UUIDs are never reused and links are checked against live
references, so orphaned state never grants access.

## Limits and known restrictions

- Repository admins see every resource of the family.
- `$signed`, history, feeds and historical passports stay ABAC-only (see
  above).
- Caller query conditions on related resources (`$sm`/`$sme` inside other
  resource queries) only see ABAC-visible related rows.
- Items of `/upload` and of bulk requests are authorized one by one; an
  upload can therefore be applied partially, as with ABAC.

## Upgrade notes

Database schema `v1.2.2` adds `auth_uuid` columns to `aas`, `submodel`,
`concept_description`, `descriptor`, `aas_identifier` and `aasx_package` and
assigns a UUID to every existing row. PostgreSQL rewrites these tables while
holding an exclusive lock; plan a maintenance window for large
installations. Run the configuration service before upgrading services. The
patch also adds the `rebac_*` tables, which stay empty while ReBAC is
disabled.
