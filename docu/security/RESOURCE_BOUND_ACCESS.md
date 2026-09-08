# Resource-bound access

The implementation targets Part 4 [PR #108, commit 07c8bb6](https://github.com/admin-shell-io/aas-specs-security/pull/108). Ownership, management APIs, and ABAC fallback are BaSyx extensions to that unmerged proposal.

## Configuration

Set `security.authorizationMode: resource-bound-first` to enable the resource-bound path. Omitting the setting, or selecting `legacy-abac`, preserves existing ABAC behavior. `abac.enabled` controls the optional object-based fallback. OIDC is required in resource-bound mode, including when ABAC fallback is disabled.

| Configuration | Environment variable |
|---|---|
| `security.authorizationMode` | `SECURITY_AUTHORIZATION_MODE` |
| `rebac.policyScope` | `REBAC_POLICY_SCOPE` |
| `rebac.modelPath` | `REBAC_MODEL_PATH` |
| `rebac.bootstrapOwner.issuer` | `REBAC_BOOTSTRAP_OWNER_ISSUER` |
| `rebac.bootstrapOwner.subject` | `REBAC_BOOTSTRAP_OWNER_SUBJECT` |

`BASYX_`-prefixed variants are accepted. Components sharing resources must use the same writer database and policy scope. Bootstrap adopts existing resources without installing individual policies; it does not restore removed grants on restart. Initial policies are imported once per scope. Apply schema migrations with the configuration service before starting components.

## Decisions

Each resource uses its own policy or the first policy found walking upward through its operation-context AAS hierarchy. A local policy, including an empty policy, replaces the entire inherited model. A Submodel with multiple associated AAS needs explicit AAS context to inherit from one of them; directly bound policies remain usable without that context. Unassociated Submodels use the repository collection as their parent.

Resource-bound rules are evaluated first. Object-based ABAC is evaluated when they do not grant the action. Consequently, an ABAC grant can allow access past a resource-bound override. Evaluation/storage errors fail closed. Filters attached to a successful resource-bound grant remain mandatory and cannot trigger fallback to restore excluded content.

Composite reads authorize descendants separately. Hidden branches are omitted using the existing structured filtering machinery. Data UPDATE and DELETE check affected subtrees. Technical snapshots retain resource authorization even when ABAC query projections are suppressed. AAS Submodel references require VIEW or READ on their targets and are filtered before pagination. Full SubmodelElement representations omit VIEW-only elements because a reference cannot replace an element in that representation.

Changing AAS-to-Submodel associations requires administration of the affected local Submodel; unresolved external references cannot establish inheritance. This BaSyx restriction prevents linking a resource beneath a permissive ancestor to obtain access.

Database identities retain ownership and policy across ordinary reconciliation and explicit list-position shifts. Whole-Submodel PUT cannot distinguish edits from replacement of anonymous list entries: changed entries, or entries in a resized list, receive fresh identities and require CREATE plus DELETE. Unchanged entries in an unchanged list retain their metadata. Use element-specific updates when preserving a list entry's identity.

## Resource APIs

Append `/$access` to `/shells/{aasIdentifier}`, `/submodels/{submodelIdentifier}`, or a SubmodelElement path. AAS-nested aliases address the same policy while supplying AAS context. Identifiers retain the existing base64url encoding. `/shells/$access` and `/submodels/$access` govern collection creation rights.

| Method and suffix | Body / result |
|---|---|
| `GET /$access` | Direct owners, managers and grants; local and effective policy |
| `GET /$access/policy` | Direct policy; 404 when absent |
| `PUT /$access/policy` | Single PR #108 model with matching RESOURCE |
| `DELETE /$access/policy` | Restore inheritance; remove local manager and grant metadata |
| `POST /$access/grants` | `{ "principal": { "issuer": "…", "subject": "…" }, "rights": ["READ"] }` |
| `PUT /$access/grants/{grantId}` | Replace an API-managed grant |
| `DELETE /$access/grants/{grantId}` | Remove an API-managed grant |
| `PUT /$access/managers` | Array of issuer/subject principals |
| `PUT /$access/owners` | Nonempty array of issuer/subject principals |

Mutations require the ETag from the access overview in `If-Match`: missing headers return 428; stale revisions return 412. ETags include the persistent binding identity, scope revision, and effective policy. They change on resource recreation and may differ across aliases with different inheritance contexts. Scope revisions conservatively invalidate ETags after any access change. First-time sharing requires explicit local-policy creation (otherwise 409). A full policy replacement must retain protected manager rules; change those through `/managers`.

Creators become direct owners transactionally, without a policy override or implicit data READ. Only direct owners change owners, and one must remain. Managers receive full data grants plus policy administration, but inherited management stops at overrides. Ordinary data grants, including ALL, do not permit access administration. Access administration and writes require a verified issuer and subject. Read policies can retain the existing ANONYMOUS attribute semantics.

## Supported services and checks

AAS Environment and standalone AAS/Submodel repositories support core resource APIs, representations, attachments, and synchronous invocation. Query, history, bulk, import/export, signed representations, and asynchronous routes are rejected in resource-bound-first mode. Legacy ABAC retains its existing surface. OpenAPI adds resource-access paths when the mode is enabled.

Run the database-backed access tests against a migrated isolated database:

```sh
BASYX_REBAC_TEST_DSN='postgres://user:password@localhost/database?sslmode=disable' go test -v ./internal/common/security/integration_tests
```

The integration suite uses test-only identity injection after OIDC middleware and the real repository controllers and PostgreSQL. Run the existing OIDC security suites separately to verify token handling. Without the database variable these additional tests report a skip. The mandatory regression command remains `go clean -testcache` followed by `go test -v ./internal/submodelrepository/integration_tests`.

See the [bridge example](../../examples/BaSyxResourceBoundExample/README.md).
