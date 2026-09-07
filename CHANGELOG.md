# Changelog

All notable user-visible and security-relevant changes are documented here.
Changes are collected as reviewed fragments and batched into this file when a
release is created.

Each entry states whether users need to take action and records the security
consequence separately. High-impact entries require an API, configuration,
policy, or deployment update. Low-impact entries do not require migration.

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
