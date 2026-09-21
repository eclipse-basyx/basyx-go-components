# Changelog

All notable user-visible and security-relevant changes are documented here.
Changes are collected as reviewed fragments and batched into this file when a
release is created.

Each entry states whether users need to take action and records the security
consequence separately. High-impact entries require an API, configuration,
policy, or deployment update. Low-impact entries do not require migration.

## v1.0.12 (2026-09-10)

Changes since [v1.0.11](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.0.11...v1.0.12).

### Added

* **High impact** — Synchronize DPP API writes with AAS and Submodel registries and product discovery. ([#650](https://github.com/eclipse-basyx/basyx-go-components/pull/650))
  * **Security:** DPP write authorization now also covers the configured internal AAS Registry, Submodel Registry, and Discovery mutations; their independent read policies continue to control visibility.

* **Low impact** — AAS Environment Service shell queries can evaluate `$sm` and `$sme` field conditions against Submodels referenced by each Asset Administration Shell, with logical `$match` correlating all enclosed conditions to the same referenced Submodel hierarchy. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** Cross-resource query fields use the authorization view of their corresponding semantic resource.

* **Low impact** — Querying the text of a MultiLanguageProperty ([#659](https://github.com/eclipse-basyx/basyx-go-components/pull/659))
  * **Security:** None

* **Low impact** — The AASX File Server now supports durable asynchronous package uploads through `POST /packages-async`. ([#609](https://github.com/eclipse-basyx/basyx-go-components/pull/609))
  * **Security:** Async uploads use existing create/read authorization; status and results are restricted to the submitting owner.


### Changed

* **High impact** — ABAC claims retain their JWT JSON types. `CLAIM` selects a top-level value and `CLAIMPATH` selects a nested value using a non-empty RFC 6901 JSON Pointer. Every declared claim must be present before its rule can become active; values are resolved and type-checked when formulas evaluate them. Uncast scalar comparisons require strings; claim casts accept only the corresponding JSON type. Exact string-array membership uses `$contains` with `CLAIMPATH` first. The branch-only `$in` operator has been removed, so policies must migrate from `{"$in":[scalar,CLAIMPATH]}` to `{"$contains":[CLAIMPATH,scalar]}`. Formula evaluation uses true/false/indeterminate logic, and CREATE/UPDATE checks are performed on staged state before commit, with UPDATE requiring both current and prospective state. ([#616](https://github.com/eclipse-basyx/basyx-go-components/pull/616))
  * **Security:** Claim evaluation now follows the documented JSON types and paths. Missing declared claims make the rule ineligible. Present `null`, wrong-type, object, mixed-array, and unsupported values are indeterminate and fail closed when evaluated; empty string arrays evaluate false.

* **High impact** — Expose AAS-managed File attachments as AAS Environment attachment URLs in DPP representations; GENERAL_EXTERNALURL is required to serialize these URLs even when registry synchronization is disabled. ([#650](https://github.com/eclipse-basyx/basyx-go-components/pull/650))
  * **Security:** None.

* **Low impact** — Query-capable repository, registry, and discovery endpoints now represent caller conditions, public list selectors, projections, and authorization constraints in an immutable request-scoped semantic access-view intermediate representation. Responses for authorized resources remain compatible. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** The policy version, claims, trusted global attributes, and per-resource access views remain consistent throughout each request.

* **High impact** — Database schema `v1.1.19` requires PostgreSQL 16 or newer and installs `basyx_safe_regex_pattern` for total regular-expression evaluation and `basyx_validated_cast_input` for safe nested query casts. Run the configuration service before starting updated services. Deployments using an older PostgreSQL version must upgrade before applying this schema patch. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** Invalid regular-expression and cast inputs use consistent no-match semantics. Nested casts validate and convert the same textual value without duplicating inner SQL expressions.


### Fixed

* **Low impact** — Invalid ABAC claim comparisons now retain their indeterminate result through `$not`, `$and`, `$or`, and `$match` instead of being converted to Boolean values. ([#616](https://github.com/eclipse-basyx/basyx-go-components/pull/616))
  * **Security:** Invalid or unusable claim values now consistently fail closed.

* **Low impact** — Reduce database round trips and redundant root writes when updating AAS and Submodel Descriptors. ([#658](https://github.com/eclipse-basyx/basyx-go-components/pull/658))
  * **Security:** None.

* **Low impact** — Query conditions now evaluate fields and related resources through their effective semantic access view, including negated conditions, nested `$match`, response filters, list selectors, casts, and regular expressions. AAS object grants no longer authorize nested Submodel data, Fragment objects constrain exact SME fields, and list selectors are emitted only once. Incompatible cast values and invalid regular expressions return no match without hiding PostgreSQL's regex operator from index planning, and Basic Discovery global asset identifier lookups use the Basic Discovery semantic field. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** Caller and policy expressions now use consistent per-resource authorization and query-evaluation semantics. Nested AAS Environment Submodel routes require dedicated Submodel or Submodel Element grants.

* **High impact** — Nested query casts preserve authorization checks without exponential SQL expansion or unsupported PostgreSQL casts. Policy routes containing parent segments derive query permissions from the same mounted path as direct authorization. Queries and policy expressions now enforce 64 JSON container levels and 8,192 JSON tokens before recursive decoding. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** Bounds expression complexity and SQL expansion, prevents nested cast database errors, and prevents semantic permissions from escaping the configured route mount. Existing oversized expressions must be reduced.

* **High impact** — Compiled query policies are copied structurally so many individually valid access rules retain their grants when combined. External query and policy-expression limits remain enforced. ([#654](https://github.com/eclipse-basyx/basyx-go-components/pull/654))
  * **Security:** Prevents valid combined allow rules from silently becoming deny-all while preserving fragment scope, indeterminate conditions, and request policy isolation.
## v1.0.11 (2026-08-31)

Changes since [v1.0.10](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.0.10...v1.0.11).

### Changed

* **Low impact** — AAS create and delete operations avoid separate root and lookup executions for batches within the existing size limits, reducing writer connection occupation under concurrent load. ([#645](https://github.com/eclipse-basyx/basyx-go-components/pull/645))
  * **Security:** Existing duplicate visibility, ABAC, history, row-locking, not-found, and managed-thumbnail cleanup behavior remains unchanged.

* **Low impact** — Shell descriptor creates avoid rewriting newly inserted embedded Submodel descriptors while synchronizing administration timestamps. ([#646](https://github.com/eclipse-basyx/basyx-go-components/pull/646))
  * **Security:** Descriptor validation, authorization, history, and stored graph semantics remain unchanged.

* **Low impact** — Nested SubmodelElement creates combine child position, duplicate, and identifier allocation state into one database statement. ([#646](https://github.com/eclipse-basyx/basyx-go-components/pull/646))
  * **Security:** Existing parent locking and ABAC-aware duplicate visibility checks remain unchanged.


### Fixed

* **Low impact** — The release workflow can inspect its newly created draft release and continue directly into artifact publication. ([#637](https://github.com/eclipse-basyx/basyx-go-components/pull/637))
  * **Security:** The guard receives contents write permission only because GitHub requires push access to view drafts; its operations remain read-only.

* **Low impact** — Docker release verification now authenticates every Docker Hub access and avoids redundant multi-platform registry reads that could trigger rate limits. ([#638](https://github.com/eclipse-basyx/basyx-go-components/pull/638))
  * **Security:** Docker Hub credentials remain limited to release jobs and are used only for registry operations; immutable digest, signature, attestation, and image-label verification remain enforced.

* **Low impact** — Existing Submodel PUT requests reuse generic PostgreSQL plans while loading persisted SubmodelElement state, avoiding repeated planning overhead on large databases. ([#640](https://github.com/eclipse-basyx/basyx-go-components/pull/640))
  * **Security:** Authorization and transaction isolation remain unchanged; persisted replacement semantics are unchanged.

* **Low impact** — Registry PUT operations preserve descriptor root rows and rewrite only changed descriptor fields and child collections. ([#641](https://github.com/eclipse-basyx/basyx-go-components/pull/641))
  * **Security:** Existing pre-update and post-update ABAC checks remain in place; restricted reads use a complete internal snapshot only after authorization succeeds.

* **Low impact** — Concept Description PUT operations update existing rows in place and reject mismatching path and body identifiers. ([#642](https://github.com/eclipse-basyx/basyx-go-components/pull/642))
  * **Security:** Prevents PUT authorization and history from being split across different Concept Description identifiers.

* **Low impact** — SubmodelElement writes no longer maintain a duplicate path index already covered by the stable path-order index. ([#643](https://github.com/eclipse-basyx/basyx-go-components/pull/643))
  * **Security:** Submodel persistence and authorization behavior remain unchanged; the retained index covers the same Submodel and idShort-path lookup prefix.

* **Low impact** — The configuration service skips the baseline schema for initialized databases, preventing later configuration runs from recreating objects removed by versioned patches. ([#644](https://github.com/eclipse-basyx/basyx-go-components/pull/644))
  * **Security:** None.
## v1.0.10 (2026-08-29)

Changes since [v1.0.9](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.0.9...v1.0.10).

### Changed

* **Low impact** — Every declared `CLAIM` is mandatory, including when combined with `GLOBAL=ANONYMOUS`; unsupported attributes fail closed. ([#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573))
  * **Security:** More restrictive: missing claims and unsupported attributes deny access.

* **Low impact** — Anonymous time-based rules can grant access when `GLOBAL=ANONYMOUS` is explicitly configured. ([#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573))
  * **Security:** Potentially broader, but only for policies that explicitly allow anonymous access.

* **Low impact** — Existing AAS and Submodel `PUT` requests now reconcile only changed persisted rows, preserve unchanged persistence identities and managed binary references, and synchronize registries only when the derived descriptor content changes. Semantic no-op replacements skip the live-model write but still record the acknowledged update when history or WORM evidence is enabled. ([#591](https://github.com/eclipse-basyx/basyx-go-components/pull/591))
  * **Security:** Improves audit completeness for acknowledged no-op `PUT` requests; authorization semantics are unchanged.

* **Low impact** — AAS Repository, Submodel Repository, and registry `getAll` endpoints now use bounded, set-based database reads whose query count is independent of the requested page size. Pagination, ordering, representations, and filters remain compatible. ([#606](https://github.com/eclipse-basyx/basyx-go-components/pull/606))
  * **Security:** Authorization behavior is unchanged; existing ABAC filters and fragment masks remain enforced.


### Fixed

* **High impact** — ABAC rules containing only `UTCNOW`, `LOCALNOW`, or `CLIENTNOW` now require a `CLAIM`, or `GLOBAL=ANONYMOUS` for intentionally public access. ([#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573))
  * **Security:** More restrictive: existing time-only rules no longer grant access.

* **High impact** — `CLIENTNOW` is no longer generated or overwritten by the server and must come from the verified access token. Use `UTCNOW` or `LOCALNOW` when server time is intended. ([#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573))
  * **Security:** More restrictive: rules requiring `CLIENTNOW` no longer match without the token claim.

* **Low impact** — `UTCNOW` and `LOCALNOW` now use the trusted server clock instead of same-named JWT claims. ([#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573))
  * **Security:** JWT values can no longer control server-time authorization decisions.

* **Low impact** — Concurrent top-level and nested SubmodelElement `POST` requests now allocate unique sibling positions without deadlocking or timing out on a second database connection. These writes honor request cancellation, and nested inserts persist the correct tree depth. ([#599](https://github.com/eclipse-basyx/basyx-go-components/pull/599))
  * **Security:** Authorization and information-disclosure behavior are unchanged.
## v1.0.9 (2026-08-04)

See the [v1.0.9 GitHub release](https://github.com/eclipse-basyx/basyx-go-components/releases/tag/v1.0.9).
