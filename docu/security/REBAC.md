# Relationship-Based Access Control (ReBAC) with OpenFGA

> **Experimental.** The ReBAC integration, its management API and its
> database tables may change incompatibly in future releases. Configuration
> keys carry no experimental marker; this document is the only place that
> declares the status.

ReBAC lets resource owners share Asset Administration Shells, Submodels,
SubmodelElements and Concept Descriptions with other users or groups without
changing the ABAC policy. It runs next to ABAC as a **strict union**:

```text
access = ABAC allows  OR  ReBAC allows
```

ABAC syntax, evaluator semantics, policy management and existing
configurations are unchanged. With `rebac.enabled=false` (the default) no
ReBAC code is wired into the routers and every service behaves exactly as
before.

## Contents

- [Covered services and routes](#covered-services-and-routes)
- [ReBAC and ABAC interplay (read this first)](#rebac-and-abac-interplay-read-this-first)
- [Evaluation order](#evaluation-order)
- [Roles and relations](#roles-and-relations)
- [Identities](#identities)
- [SubmodelElement grants](#submodelelement-grants)
- [AAS-to-Submodel inheritance](#aas-to-submodel-inheritance)
- [Ownership and administrators](#ownership-and-administrators)
- [Management API](#management-api)
- [Invitation links](#invitation-links)
- [Deployment](#deployment)
- [Configuration](#configuration)
- [Consistency, outbox and revocation barrier](#consistency-outbox-and-revocation-barrier)
- [Reconciliation](#reconciliation)
- [Limits and known restrictions](#limits-and-known-restrictions)
- [Upgrade notes](#upgrade-notes)

## Covered services and routes

| Service | Covered resources |
| --- | --- |
| AAS Repository | Shells, asset information, thumbnails, Submodel references, Submodel superpaths |
| Submodel Repository | Submodels and all representations, SubmodelElements, attachments, operations (sync, async, status, results) |
| Concept Description Repository | Concept Descriptions |
| AAS Environment | All of the above |

The following stay **ABAC-only**; a ReBAC grant never gives access to them:
history endpoints (`$history`), `$recent-changes`, event feeds, `$signed`
representations, `/verify`, the ABAC policy management API, AASX packages,
`/upload`, `/serialization`, registries, discovery, DPP and the Company Lookup
service. Registries, discovery and aggregates follow in later releases.

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
- Monitor grants through `GET …/$access` and reconcile regularly.

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
   fragment filters), the request proceeds. OpenFGA is not called.
4. Otherwise BaSyx resolves the concrete resource and asks OpenFGA:
   - **allowed** – the request proceeds; the granted resource is widened as
     described above,
   - **denied** – the ABAC result stands, including today's `403`/`404` and
     deny-as-not-found behavior,
   - **error or timeout** – `503 SECURITY-REBAC-UNAVAILABLE`. There is never a
     silent ABAC-only fallback when ReBAC was needed.
5. After OpenFGA allowed, BaSyx checks the revocation barrier (see below).

Unknown identifiers behave exactly as before; ReBAC never reveals whether a
resource exists.

## Roles and relations

| Role | Grants |
| --- | --- |
| `viewer` | read |
| `editor` | read, update, create children |
| `executor` | invoke operations and read their status and results |
| `owner` | everything above, plus delete and manage access |

Concept Descriptions have no `executor` role. Repository families
(`aas`, `submodel`, `concept_description`) have two further relations:

| Repository relation | Meaning |
| --- | --- |
| `creator` | may create top-level resources of the family and becomes their owner |
| `admin` | has every right on every resource of the family and may manage repository grants |

Right mapping: `READ` → `can_read`, `UPDATE` and child `CREATE` →
`can_update`, top-level `DELETE` → `can_delete`, `EXECUTE` → `can_execute`.
Deleting SubmodelElements, thumbnails or Submodel references edits the parent
and therefore needs `can_update` on the parent.

The authorization model is embedded in the release
(`internal/common/security/rebac/model/model.fga`) and tested with
`fga model test` (`model.fga.yaml`).

## Identities

- Users are issuer-scoped: `user:<b64url(iss)>.<b64url(sub)>`. The same `sub`
  from another issuer is a different user.
- Groups are issuer-scoped: `group:<b64url(iss)>.<b64url(name)>`. Group names
  come from the normalized claim configured in `rebac.groupClaim`
  (default `groups`; use the OIDC `claimMappings` to normalize provider
  claims). Memberships are **never stored**; each check sends the caller's
  current groups as contextual tuples, so membership changes take effect with
  the next token.
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
  granted top-level elements. Deeper grants are reachable through their path
  routes.
- ABAC fragment filters on **ancestors** of a granted element still apply.
- Deleting a Submodel deletes all of its element grants in the same
  transaction.

## AAS-to-Submodel inheritance

A shell's grants reach a Submodel only through an explicitly approved link:

- `PUT /submodels/{id}/$access/inheritance` with `{"aasIds": [...]}` replaces
  the approved shells. The caller needs `can_manage` on the Submodel and on
  each shell, and each shell must reference the Submodel.
- A link carries read, update, create-children and execute, **never** manage
  or delete.
- Removing the reference from the shell (DELETE submodel-ref, superpath
  DELETE or a shell update) removes the link in the same transaction.
- Semantic IDs, other references and Concept Description references never
  create inheritance.

Superpath routes (`/shells/{aas}/submodels/{sm}/…`) keep the existing
reference check: the caller needs read access to the shell (update for
superpath PUT and DELETE) and the required right on the Submodel.

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
  `$access` resource, repository grants, reconciliation and ownership
  recovery, and may remove the last owner.

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
missing ABAC rule never hides them. They authorize with ReBAC `can_manage`
(or administrator status); ABAC data rights never grant sharing. Denials
answer `404`, exactly like missing resources.

Every managed resource has an `$access` sub-resource:
`/shells/{id}/$access`, `/submodels/{id}/$access`,
`/submodels/{id}/submodel-elements/{idShortPath}/$access`,
`/concept-descriptions/{id}/$access`.

| Operation | Contract |
| --- | --- |
| `GET …/$access` | Direct grants, approved inheritance links, projection state. `ETag` is the access revision. |
| `PUT …/$access/grants` | Replaces the direct grants. `If-Match` required: missing → `428`, stale → `412`. Removing the last owner → `409` unless the caller is an administrator. |
| `GET …/$access/effective` | The caller's own rights per action with source `abac`, `abac-conditional`, `rebac`, `administrator` or `none`. Callers without any right get `404`. |
| `PUT /submodels/{id}/$access/inheritance` | Replaces approved AAS links (`If-Match` required). |
| `GET/POST …/$access/invitations`, `DELETE …/$access/invitations/{id}` | Invitation links, see below. |
| `GET /security/rebac/operations/{operationId}` | State (`applied`/`pending`) of an accepted change. |
| `GET/PUT /security/rebac/repositories/{kind}/$access[/grants]` | Creator and admin grants of a repository family. |
| `GET /security/rebac/status` | Scope, store, model, backlog and barrier state (administrators). |
| `POST /security/rebac/admin/reconcile` | Removes orphaned state and repairs OpenFGA drift (administrators). |
| `PUT /security/rebac/admin/owners/{type}/{base64 id}` | Ownership recovery (administrators, `If-Match` required). |

Grant changes answer `200` once they reached OpenFGA. If OpenFGA does not
confirm within a few seconds, they answer `202` with a `Location` of the
operation; the change is applied by the outbox worker.

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

- `POST …/$access/invitations` (needs `can_manage`) with
  `{"relation": "viewer|editor|executor", "expiresAt": "<RFC 3339>", "maxUses": 1}`
  creates an invitation. `expiresAt` is required and at most 90 days ahead;
  `maxUses` defaults to 1. The response carries a one-time-visible `token`.
  Invitations never grant `owner`.
- `POST /security/rebac/invitations/accept` with `{"token": "…"}` requires an
  authenticated caller and creates a normal direct grant for the caller's own
  issuer and subject. The token itself never authorizes data access. The token
  is sent in the body so it never appears in URLs, access logs or traces.
- Tokens are 256-bit random values stored only as SHA-256 hashes. Redemption
  is atomic, so concurrent accepts never exceed `maxUses`.
- `DELETE …/$access/invitations/{id}` revokes a pending invitation. Grants
  that were already redeemed stay until they are removed individually.

## Deployment

OpenFGA runs as its own service; BaSyx talks to it over HTTP with the OpenFGA
Go SDK. Pin OpenFGA **1.18.x** (minimum 1.10.0 for idempotent writes).

- OpenFGA's datastore is PostgreSQL. Its schema is migrated by
  `openfga migrate`, run as a one-shot container before `openfga run`.
  BaSyx never manages that schema.
- Use a separate database and user for OpenFGA (same server or another one).
  BaSyx and OpenFGA tables never share a database.
- Do not publish the OpenFGA port. Only BaSyx services and the provisioning
  step need write access.
- `basyxconfigurationservice` provisions the store (`basyx-<scope>`) and the
  embedded model when `rebac.provisionModel=true` and records the binding in
  `rebac_model_activation`. Services read the binding at startup.
- Every service verifies at startup that ABAC is enabled, the OIDC trustlist
  is readable, the bound store and model exist, and the model content equals
  the model of the release. Any mismatch aborts startup.
- One PostgreSQL database is bound to exactly one scope, store and model.
- Set `OPENFGA_LIST_OBJECTS_MAX_RESULTS` to at least
  `rebac.listObjectsMaxResults` and keep `OPENFGA_LIST_OBJECTS_DEADLINE`
  generous.

See `examples/BaSyxReBACExample` for a complete compose setup.

## Configuration

| Key | Env | Default | Purpose |
| --- | --- | --- | --- |
| `rebac.enabled` | `REBAC_ENABLED` | `false` | Master switch |
| `rebac.scope` | `REBAC_SCOPE` | `default` | Deployment scope, bound to one store |
| `rebac.openfga.apiUrl` | `REBAC_OPENFGA_API_URL` | – | OpenFGA HTTP endpoint (required) |
| `rebac.openfga.storeId` | `REBAC_OPENFGA_STORE_ID` | bound store | Optional; must match the binding |
| `rebac.openfga.authorizationModelId` | `REBAC_OPENFGA_AUTHORIZATION_MODEL_ID` | bound model | Optional pin; never `latest` |
| `rebac.openfga.credentials.method` | `REBAC_OPENFGA_CREDENTIALS_METHOD` | `none` | `none`, `apiToken` or `clientCredentials` |
| `rebac.openfga.credentials.apiToken` | `REBAC_OPENFGA_CREDENTIALS_API_TOKEN` | – | Pre-shared key |
| `rebac.openfga.credentials.clientId` / `clientSecret` / `apiTokenIssuer` / `apiAudience` | `REBAC_OPENFGA_CREDENTIALS_*` | – | Client credentials |
| `rebac.openfga.timeoutMillis` | `REBAC_OPENFGA_TIMEOUT_MILLIS` | `2000` | Per-call timeout |
| `rebac.openfga.consistency` | `REBAC_OPENFGA_CONSISTENCY` | `HIGHER_CONSISTENCY` | Check consistency; no decision cache |
| `rebac.openfga.batchCheckMaxItems` | `REBAC_OPENFGA_BATCH_CHECK_MAX_ITEMS` | `50` | BatchCheck chunk size (1–50) |
| `rebac.listObjectsMaxResults` | `REBAC_LIST_OBJECTS_MAX_RESULTS` | `1000` | Above this, lists use the candidate scan |
| `rebac.maxScanCandidates` | `REBAC_MAX_SCAN_CANDIDATES` | `5000` | Upper bound of the candidate scan |
| `rebac.groupClaim` | `REBAC_GROUP_CLAIM` | `groups` | Normalized claim with group names |
| `rebac.administrators` | `REBAC_ADMINISTRATORS` (comma-separated) | `[]` | `issuer\|subject` or `issuer\|group:<name>` |
| `rebac.provisionModel` | `REBAC_PROVISION_MODEL` | `false` | Configuration service provisions store and model |

`rebac.enabled=true` requires `abac.enabled=true` and an OIDC trustlist.

## Consistency, outbox and barrier

PostgreSQL holds the desired authorization state (`rebac_grant`,
`rebac_submodel_link`, `rebac_invitation`); OpenFGA is a projection that only
BaSyx writes.

- Resource mutations, grant changes and outbox rows are written in the same
  database transaction. Changes of one object serialize on its revision row,
  so the outbox order equals the commit order.
- Owner grants of new resources are applied before commit (the object is new,
  so this is always safe). Other changes are applied right after commit; a
  background worker, elected through a PostgreSQL advisory lock, drains
  anything left in order with exponential backoff.
- Pending **additions** are fail-safe: they only deny until applied.
- Pending **revocations** trip the barrier: after OpenFGA allowed a request,
  BaSyx checks for unapplied revocations. While one exists, ReBAC allows are
  not trusted and the request gets `503`. ABAC allows are unaffected.
- Removing an approved AAS link (including through reference removal) is a
  revocation.

## Reconciliation

- **Re-enable reconciliation.** While ReBAC is disabled nothing tracks
  deletions or removed references. Every startup with `rebac.enabled=true`
  removes grants, element grants, invitations and links of resources that no
  longer exist and links whose reference is gone, and queues the tuple
  deletions. The service only becomes ready afterwards.
- **Drift repair.** At startup and through
  `POST /security/rebac/admin/reconcile`, BaSyx compares the desired state
  with the tuples stored in OpenFGA, writes missing tuples and deletes
  unexpected ones. Stored group memberships are always unexpected. The report
  lists how many tuples were repaired.

## Limits and known restrictions

- At most 100 contextual tuples per check: the caller's groups plus the depth
  of an element path. Callers exceeding it get `503`.
- Lists: when a caller can see `rebac.listObjectsMaxResults` or more objects
  of a kind, BaSyx verifies direct and linked grants with BatchCheck. More
  than `rebac.maxScanCandidates` candidates answer `503`.
- Repository admins see every resource of the family.
- `$signed`, history and feeds stay ABAC-only (see above).
- Caller query conditions on related resources (`$sm`/`$sme` inside other
  resource queries) only see ABAC-visible related rows.

## Upgrade notes

Database schema `v1.2.2` adds `auth_uuid` columns to `aas`, `submodel` and
`concept_description` and assigns a UUID to every existing row. PostgreSQL
rewrites these tables while holding an exclusive lock; plan a maintenance
window for large installations. Run the configuration service before
upgrading services. The patch also adds the `rebac_*` tables, which stay
empty while ReBAC is disabled.
