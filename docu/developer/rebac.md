# ReBAC Developer Guide

This guide explains how relationship-based access control (ReBAC) is
implemented and how to extend it. For behavior and concepts see the
[overview](../security/rebac/README.md), for the HTTP contract the
[Sharing API](../security/rebac/sharing-api.md).

## Design principles

- **Union with ABAC.** ReBAC only adds access; it never restricts what ABAC
  allows. ABAC code paths stay unchanged when ReBAC is disabled.
- **PostgreSQL only.** Relationships live in the BaSyx database and are
  evaluated as SQL subqueries of the backend queries. There is no cache and
  no projection, so changes take effect at commit.
- **Fail closed.** If ReBAC is needed but cannot decide, the request fails
  with `503 SECURITY-REBAC-UNAVAILABLE`; there is no ABAC-only fallback.
- **No existence oracle.** Denials look exactly like ABAC denials, and the
  management API answers `404` to non-managers.
- **Per-resource grants.** A grant widens exactly one resource kind of one
  request and never another table touched by the same request.

## Code map

| Path | Content |
| --- | --- |
| `internal/common/rebac_config.go` | `rebac.*` configuration and validation |
| `internal/common/service_profiles.go` | Optional service profiles such as the ReBAC profile in `/description` |
| `internal/common/security/rebac_middleware.go` | `ReBACResolver` hook of the ABAC middleware, `503` handling |
| `internal/common/security/rebac_grants.go` | `ReBACGrantSet` and its translation into SQL predicates |
| `internal/common/security/rebac_state.go` | `ReBACState` and the `RecordReBAC*` hooks called by backends |
| `internal/common/security/rebac/setup.go` | Startup, prerequisites, reconciliation, readiness |
| `internal/common/security/rebac/coordinator.go` | `Coordinator`: decisions per covered route |
| `internal/common/security/rebac/routes.go` | Route matrix: which route needs which permission on which resource |
| `internal/common/security/rebac/permissions.go` | Relations, permissions and the SQL that selects permitted objects |
| `internal/common/security/rebac/identity.go` | Principals, subject and object keys |
| `internal/common/security/rebac/store*.go`, `state.go`, `derivation.go` | Grant storage, recording of ownership, deletion and derivations |
| `internal/common/security/rebac/management*.go` | `$access`, repository, invitation, administration, audit and principal endpoints |
| `internal/common/security/rebac/audit*.go` | Hash-chained audit trail and its evidence |
| `internal/common/security/rebac/reconcile.go` | Removal of orphaned state |
| `internal/common/security/rebac/telemetry.go` | Spans and metrics |
| `database/patches/1_2_2.sql` | `auth_uuid` columns and `rebac_*` tables |

## Request flow

1. The OIDC middleware authenticates the caller. Anonymous requests are
   ABAC-only.
2. The ABAC middleware evaluates the request as before.
3. If ABAC unconditionally allows the required rights (formula `true`, no
   fragment filters), the request proceeds without ReBAC.
4. Otherwise, for covered routes, the middleware calls
   `ReBACResolver.Resolve` with the route, claims and pending rights.
5. The `Coordinator` looks up the `routeSpec` of the route, derives the
   principal from the claims (`rebac.subjectClaim`, `rebac.groupClaim`) and
   fills a `ReBACGrantSet`: concrete authorization UUIDs, all of a kind for
   repository admins, or subqueries such as "every Submodel the caller may
   read".
6. The grant set is stored in the request context. When a backend turns the
   ABAC QueryFilter into SQL, `reBACGrantPredicate` ORs the grant predicate
   of the matching resource kind and right into the condition. Backends need
   no ReBAC-specific read code.
7. An empty grant set keeps the ABAC decision; an error becomes `503`.

Because grants are subqueries, every backend query of a request evaluates
them again. Asynchronous work that captured the request context therefore
sees revocations.

## Data model

| Table | Content |
| --- | --- |
| `rebac_grant` | Direct grants: object key, type, `auth_uuid`, element path, relation, subject key, issuer, subject |
| `rebac_object_revision` | Access revision per object; serializes changes and is the `ETag` |
| `rebac_submodel_link` | Approved shell links of Submodels |
| `rebac_derivation` | Derived objects and their source |
| `rebac_invitation` | Invitations with SHA-256 token hashes |
| `rebac_audit_event` | Hash-chained audit trail (append-only under the history guard) |

Resources are addressed by the `auth_uuid` column of their row, never by
their identifier, so a resource recreated with the same identifier never
inherits old grants.

Keys:

| Key | Format |
| --- | --- |
| User | `user:<b64url(issuer)>.<b64url(subject)>` |
| Group | `group:<b64url(issuer)>.<b64url(name)>` |
| Resource | `<objectType>:<auth_uuid>` |
| Element | `element:<submodel auth_uuid>.<first 32 hex chars of sha256(idShortPath)>` |
| Repository | `repository:<objectType>` |

Overlong keys use a digest form. Group memberships are never stored; the
subject keys of a request are the user key plus one key per group in the
token.

## Permissions

`permissionRelations` maps relations to permissions (`can_read`,
`can_update`, `can_delete`, `can_execute`, `can_manage`).
`liveObjects(kind, subjectKeys, permission)` selects every object of a kind
the subjects may use, as the union of:

- direct grants (`grantedObjects`),
- Submodels linked to a shell the subjects hold the permission on
  (`linkedSubmodels`, only read, update and execute, and only while the
  shell references the Submodel),
- every object of the kind for repository admins,
- objects derived from a source the subjects hold the permission on
  (`derivedObjects`, recursively: discovery entry → shell descriptor →
  shell).

`permissionCondition` is the correlated form for one object, so single
resources never scan a repository. All queries use the index
`ix_rebac_grant_permission (subject_key, object_type, relation)`.

## Recording state

Backends call the `RecordReBAC*` helpers of `rebac_state.go` inside the
transaction of a mutation. They are no-ops while ReBAC is disabled.

- `RecordReBACResourceCreated` makes an authenticated creator owner and
  records derivations of synchronized descriptors and discovery entries.
- `RecordReBACResourceDeleted` removes grants, invitations, links and
  derivations of the resource.
- `RecordReBACSubmodelReferenceRemoved` ends shell links whose reference is
  gone.
- `RecordReBACSubmodelCreatedWithShell` links Submodels created with a
  passport to its shell.

Ownership assigned at creation is part of the resource history; the audit
trail records access changes only.

## Management API

`RegisterManagementRoutes` mounts the `$access` sub-resources of the kinds a
service serves and the `/security/rebac` endpoints.
`ExemptManagementMutationRoutes` exempts them from the history mutation
guard, because access changes are not resource mutations. The routes sit
behind OIDC but outside ABAC route evaluation and authorize with
`can_manage` or administrator status.

Changes lock the object revision (`LockObjectRevision`), compare it with
`If-Match`, apply the grant diff, append an audit event and bump the
revision in one transaction.

## Audit trail

`Coordinator.audit` serializes writers with a transaction-scoped advisory
lock, reads the head hash, computes the event hash over a canonical JSON
document (version, id, time, type, actor, object, details, previous hash),
archives the event in the WORM store when evidence is enabled, and inserts
it. Element events carry their idShort path in `details`, because the object
key only contains a digest. When listing, `resolveAuditResources` adds the
current identifier of each object with one query per object type; this field
is not part of the hash.

`VerifyAuditRange` verifies a range after an optional checkpoint;
`VerifyAuditTrail` verifies the whole chain for the evidence verifier CLI.

## Reconciliation

`ReconcileOrphans` runs at every startup before the service becomes ready
and on `POST /security/rebac/admin/reconcile`. It deletes grants and
invitations of objects whose row is gone, links whose reference is gone, and
derivations whose object or source is gone. It only removes state.

## Extending ReBAC

### Covering a new route

1. Add the route to the matrix in `routes.go` with a `routeSpec`: the target
   (`targetResource`, `targetList`, `targetElement`, `targetSubmodelElements`,
   `targetJob`, `targetAggregate`), the resource kind and the required
   permission. Superpath routes also set `aasRelation`.
2. Make sure the backend applies the ABAC QueryFilter to its queries, so
   the grant predicate takes effect.
3. Routes that must stay ABAC-only belong to `isExcludedRoute`.

### Adding a resource kind

1. Add an `auth_uuid` column and unique index to its table in the current
   schema patch.
2. Define a `ResourceKind` in `store.go` (semantic kind, object type, table,
   route prefix and parameter, rows query) and add it to `AllKinds`.
3. Call the `RecordReBAC*` helpers in the create and delete transactions.
4. Extend the object type checks in `1_2_2.sql` and `ValidateRelation`.
5. Mount its `$access` routes through `RegisterManagementRoutes` in the
   service's `main.go`.

### Tests

- Unit tests live next to the code in `internal/common/security/rebac`.
- Integration tests in `internal/common/security/rebac/integration_tests`
  start PostgreSQL, Keycloak and the services with Docker Compose:

  ```bash
  go clean -testcache
  go test -v ./internal/common/security/rebac/integration_tests
  ```

- The example `examples/BaSyxReBACExample` has a smoke test (`smoke.sh`)
  against a running stack.
