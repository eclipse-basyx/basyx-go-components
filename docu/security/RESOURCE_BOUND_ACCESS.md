# Resource-bound access

The implementation targets Part 4 [PR #108, commit 07c8bb6](https://github.com/admin-shell-io/aas-specs-security/pull/108). Ownership, management APIs, and ABAC fallback are BaSyx extensions to that unmerged proposal.

## Configuration

Set `security.authorizationMode: resource-bound-first` to enable the resource-bound path. Omitting the setting, or selecting `legacy-abac`, preserves existing ABAC behavior. `abac.enabled` controls the optional object-based fallback. OIDC is required in resource-bound mode, including when ABAC fallback is disabled.

| Configuration | Environment variable |
|---|---|
| `security.authorizationMode` | `SECURITY_AUTHORIZATION_MODE` |
| `rebac.policyScope` | `REBAC_POLICY_SCOPE` |
| `rebac.modelPath` | `REBAC_MODEL_PATH` |
| `rebac.groupsClaim` | `REBAC_GROUPS_CLAIM` |
| `rebac.bootstrapOwner.type` | `REBAC_BOOTSTRAP_OWNER_TYPE` |
| `rebac.bootstrapOwner.issuer` | `REBAC_BOOTSTRAP_OWNER_ISSUER` |
| `rebac.bootstrapOwner.subject` | `REBAC_BOOTSTRAP_OWNER_SUBJECT` |

`BASYX_`-prefixed variants are accepted. `groupsClaim` defaults to `groups`. Components sharing resources must use the same writer database and policy scope. Bootstrap adopts existing resources without installing individual policies; it does not restore removed grants on restart. The bootstrap owner may be a `user` (default for backward compatibility) or a `group`. Initial policies are imported once per scope. Apply schema migrations with the configuration service before starting components.

When resource-bound authorization is enabled, the service `/description` response includes `https://basyx.org/aas/API/3/2/ResourceBoundAccessControl/1.0` in its `profiles` array. The profile is omitted in `legacy-abac` mode so clients can detect ReBAC support without probing administrative endpoints.

## Decisions

Each concrete resource uses its own policy or the first policy found walking upward through its operation-context AAS hierarchy. A local policy, including an empty policy, replaces the entire inherited model. A Submodel with multiple associated AAS needs explicit AAS context to inherit from one of them; directly bound policies remain usable without that context. Inheritance stops before a top-level collection.

Top-level collection operations are ABAC-only. For example, ABAC decides whether a caller may invoke `GET` or `POST /shells`; ReBAC `CREATE` or `READ` grants on `/shells` do not exist. A collection read is then filtered per result: a resource is included when ownership, effective ReBAC, or an object-based ABAC rule grants it. A route-based ABAC allow for `/shells` is only the collection admission check and does not expose every AAS by itself.

Resource-bound rules are evaluated first. Object-based ABAC is evaluated when they do not grant the action. Consequently, an ABAC grant can allow access past a resource-bound override. Evaluation/storage errors fail closed. Filters attached to a successful resource-bound grant remain mandatory and cannot trigger fallback to restore excluded content.

### Migrating an existing ABAC deployment

Keep the existing `ABAC_ENABLED=true` and ABAC model configuration, then set `SECURITY_AUTHORIZATION_MODE=resource-bound-first` on each component. Give all components the same `REBAC_POLICY_SCOPE`, configure one trusted bootstrap user or group, and run the configuration service so schema v1.1.20 is applied before the components start. Existing resources are adopted without replacing their access settings. This makes ReBAC the first decision for concrete resources while ABAC remains a compatibility fallback.

Migrate incrementally by keeping collection access in ABAC and moving resource-specific access to `/$access`. Persisted collection policies from an older deployment are ignored. Remove equivalent ABAC object rules only after the ReBAC policy has been verified. If ABAC remains able to grant an operation, a ReBAC denial alone does not make that resource exclusive.

Composite reads authorize descendants separately. Hidden branches are omitted using the existing structured filtering machinery. Data UPDATE and DELETE check affected subtrees. Technical snapshots retain resource authorization even when ABAC query projections are suppressed. AAS Submodel references require VIEW or READ on their targets and are filtered before pagination. Full SubmodelElement representations omit VIEW-only elements because a reference cannot replace an element in that representation.

Changing AAS-to-Submodel associations requires administration of the affected local Submodel; unresolved external references cannot establish inheritance. This BaSyx restriction prevents linking a resource beneath a permissive ancestor to obtain access.

Database identities retain ownership and policy across ordinary reconciliation and explicit list-position shifts. Whole-Submodel PUT cannot distinguish edits from replacement of anonymous list entries: changed entries, or entries in a resized list, receive fresh identities and require CREATE plus DELETE. Unchanged entries in an unchanged list retain their metadata. Use element-specific updates when preserving a list entry's identity.

## Resource APIs

Append `/$access` to a concrete resource path. Supported resource types are AAS, Submodels, Submodel Elements, AAS and Submodel Descriptors, Concept Descriptions, and Discovery records, including supported nested aliases. Identifiers retain the existing base64url encoding. Top-level collection `/$access` endpoints are not available and return 404.

| Method and suffix | Body / result |
|---|---|
| `GET /$access` | Direct owners, managers and grants; local and effective policy |
| `GET /$access/policy` | Direct policy; 404 when absent |
| `PUT /$access/policy` | Single PR #108 model with matching RESOURCE |
| `DELETE /$access/policy` | Restore inheritance; remove local manager and grant metadata |
| `POST /$access/grants` | `{ "principal": { "type": "user|group", "issuer": "…", "subject": "…" }, "rights": ["READ"] }` |
| `PUT /$access/grants/{grantId}` | Replace an API-managed grant |
| `DELETE /$access/grants/{grantId}` | Remove an API-managed grant |
| `PUT /$access/managers` | Array of user or group principals |
| `PUT /$access/owners` | Nonempty array of user or group principals |

Mutations require the ETag from the access overview in `If-Match`: missing headers return 428; stale revisions return 412. ETags include the persistent binding identity, scope revision, and effective policy. They change on resource recreation and may differ across aliases with different inheritance contexts. Scope revisions conservatively invalidate ETags after any access change. First-time sharing requires explicit local-policy creation (otherwise 409). A full policy replacement must retain protected manager rules; change those through `/managers`.

### AAS update capability preflight

In `resource-bound-first` mode, an authenticated client can check its current permission to update one visible AAS without administering its policy:

```http
GET /shells/{base64url-aas-identifier}/$access/capabilities
Authorization: Bearer <current-user-token>
```

A visible, existing AAS returns exactly `{"canUpdate":true}` or `{"canUpdate":false}` with `Cache-Control: no-store`. The decision uses the same ownership, managers, effective ReBAC policy, inheritance, filters, and configured ABAC fallback as an AAS UPDATE. It does not expose policy data or return an ETag. Missing authentication returns 401. An unknown AAS and an AAS hidden from the caller both return the same neutral 404 response. Evaluation failures return a generic 500 response and always fail closed.

An AAS editor should call this endpoint immediately before its first write in a workflow that creates and then attaches a Submodel. When the result is false, the response is 404, or the check fails, the editor must not create the Submodel. A true result does not reserve the permission and does not guarantee later reference, subtree, validation, or concurrency checks; authorization is enforced again on the actual write. An atomic server operation remains the stronger solution for creating and attaching a Submodel together.

Successful grant creation returns the new grant URL in `Location`. Request bodies are limited to 1 MiB and accept exactly one JSON value. Principal lists reject unknown fields, blank identities, and duplicates; managed grants also reject duplicate rights. Rejected mutations leave the policy and ETag unchanged.

Creators become direct owners transactionally, without a policy override. Direct ownership implicitly grants `ALL` on that resource and bypasses inherited data filters. Only direct owners change owners, and one must remain. Managers receive full data grants plus policy administration, but inherited management stops at overrides. Ordinary data grants, including ALL, do not permit access administration. Access administration and writes require a verified issuer and subject. Read policies can retain the existing ANONYMOUS attribute semantics.

### Group principals

A principal without `type` remains a user. A group principal uses the token issuer as its namespace and the exact value from the configured, verified group claim as its subject:

```json
{
  "principal": {
    "type": "group",
    "issuer": "https://issuer.example/realms/production",
    "subject": "/bridge-inspectors"
  },
  "rights": ["READ"]
}
```

The group claim may be a string or a string array. Group membership is never accepted from request parameters or an unverified token. Unknown or malformed group-claim entries grant nothing and do not alter the independent ABAC result. Groups can be grants, managers, and direct owners. A bootstrap group can own every resource that exists when a new policy scope is initialized:

```yaml
rebac:
  groupsClaim: groups
  bootstrapOwner:
    type: group
    issuer: https://issuer.example/realms/production
    subject: /aas-admins
```

New resources still make the authenticated creating user their direct owner. Add a group owner explicitly when new resources should also be administered by a team.

To let a user create an AAS, grant `CREATE` on the `/shells` route in the ABAC model:

```json
{
  "ACL": {
    "ATTRIBUTES": [{"CLAIM":"role"}],
    "RIGHTS": ["CREATE"],
    "ACCESS": "ALLOW"
  },
  "FORMULA": {"$eq":[{"$attribute":{"CLAIM":"role"}},{"$strVal":"creator"}]},
  "OBJECTS": [{"ROUTE":"/shells"}]
}
```

The ABAC rule may use stable roles, tenants, or other IdP-neutral claims. Granting `CREATE` on `/shells` permits AAS creation; it does not make the user a manager or grant access to AAS resources created by others. A successful creation transaction records the caller's stable `iss` and `sub` as direct owner with implicit `ALL` on the new AAS.

## Registry and discovery inheritance

- An AAS Descriptor inherits from the AAS with the same identifier. Without a matching AAS, only its own policy, ownership, or ABAC fallback applies.
- A Submodel Descriptor inherits from the Submodel with the same identifier. Without a matching Submodel, only its own policy, ownership, or ABAC fallback applies.
- A Discovery record inherits from the AAS with the same identifier. Without a matching AAS, only its own policy, ownership, or ABAC fallback applies.
- Concept Descriptions use their own policy, ownership, or ABAC fallback.
- A nested Submodel Descriptor under an AAS Descriptor requires access through both dimensions: the containing AAS Descriptor and the matching Submodel policy when that Submodel exists.

Local policies on descriptors or discovery records override only their inheritance path. Descriptor synchronization performed during repository writes runs in the same database transaction and enforces those local policies; a denied registry update rolls the repository write back.

## Supported services and checks

AAS Environment, standalone AAS/Submodel repositories, AAS Registry, Submodel Registry, Discovery, and Concept Description Repository support their core CRUD, list, search, and query routes. Repository representations, attachments, and synchronous invocation are also supported. History, bulk, import/export, signed representations, and asynchronous routes are rejected in resource-bound-first mode. Legacy ABAC retains its existing surface. OpenAPI adds resource-access paths when the mode is enabled.

Run the database-backed access tests against a migrated isolated database:

```sh
BASYX_REBAC_TEST_DSN='postgres://user:password@localhost/database?sslmode=disable' go test -v ./internal/common/security/integration_tests
```

The integration suite uses test-only identity injection after OIDC middleware and the real repository controllers and PostgreSQL. Run the existing OIDC security suites separately to verify token handling. Without the database variable these additional tests report a skip. The mandatory regression command remains `go clean -testcache` followed by `go test -v ./internal/submodelrepository/integration_tests`.

See the activated [secured example](../../examples/BaSyxSecuredExample/README.md).
