# Changelog

All notable user-visible and security-relevant changes are documented here.
Changes are collected as reviewed fragments and batched into this file when a
release is created.

Each entry states whether users need to take action and records the security
consequence separately. High-impact entries require an API, configuration,
policy, or deployment update. Low-impact entries do not require migration.

## v1.1.1 (2026-10-08)

Changes since [v1.1.0](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.1.0...v1.1.1).

### Changed

* **High impact** — All paginated endpoints now share one configurable page size policy. `server.pagination.defaultLimit` (default `100`, env `SERVER_PAGINATION_DEFAULT_LIMIT`) is applied when a request omits `limit`, and `server.pagination.maxLimit` (default `1000`, env `SERVER_PAGINATION_MAX_LIMIT`) is the largest accepted `limit`; a larger `limit` is rejected with HTTP 400 instead of being clamped. The policy covers the AAS, Submodel, SubmodelElement, Concept Description, registry, discovery, Company Lookup, AASX File Server, DPP product id search and Event Feed endpoints. The AAS Repository list endpoints no longer return unbounded pages when `limit` is omitted, and the AASX File Server maximum of `500` is replaced by the shared maximum. ([#735](https://github.com/eclipse-basyx/basyx-go-components/pull/735))
  * **Security:** Every paginated endpoint now applies a default page size and enforces a maximum, so a single request can no longer load an arbitrarily large result set into memory. This mitigates memory exhaustion and denial-of-service through oversized or omitted `limit` values.


### Removed

* **High impact** — The setting `eventing.feed.maxPageSize` and its environment variable `BASYX_EVENTING_FEED_MAX_PAGE_SIZE` have been removed. The Event Feed now uses `server.pagination.defaultLimit` and `server.pagination.maxLimit`, so its default and maximum page sizes are `100` and `1000` unless those settings are changed. The removed key is ignored if it is still present in a configuration. ([#735](https://github.com/eclipse-basyx/basyx-go-components/pull/735))
  * **Security:** None.


### Fixed

* **Low impact** — The Digital Twin Registry now reports all supported AAS Registry and Discovery service profiles through GET /description. ([#725](https://github.com/eclipse-basyx/basyx-go-components/pull/725))
  * **Security:** None.

* **Low impact** — The Asset Administration Shell Repository now exposes GET /serialization and returns 501 Not Implemented until serialization is supported. ([#732](https://github.com/eclipse-basyx/basyx-go-components/pull/732))
  * **Security:** None.

* **Low impact** — `server.pagination.maxLimit` must now not exceed `2147483646`. A configured maximum of `2147483647` left no room for the extra row that page queries fetch to detect a following page, so a request with that `limit` overflowed the query limit and failed with HTTP 500 or a panic. Page buffers are also no longer sized from the requested `limit`, so a large `limit` does not reserve memory before any row is read. ([#736](https://github.com/eclipse-basyx/basyx-go-components/pull/736))
  * **Security:** Prevents a configured maximum page size from causing integer overflow in page queries, and from reserving memory proportional to the requested limit before any rows exist.
## v1.1.0 (2026-09-28)

Changes since [v1.0.12](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.0.12...v1.1.0).

### Added

* **Low impact** — Experimental Event Feed API for AAS, Submodel, Asset, and PCN change notifications, including opt-in configuration, served JSON Schemas with optional Submodel semantic IDs, the feed_events schema migration, and an interactive example covered by CI smoke tests. ([#647](https://github.com/eclipse-basyx/basyx-go-components/pull/647))
  * **Security:** Event Feed inherits OIDC and ABAC. Records require access to the referenced model and all AAS ownership data disclosed by the payload. Remaining query filters or unknown ownership fail closed.

* **High impact** — Experimental MQTT 5 CloudEvents publishing for existing AAS, asset, Submodel and PCN mutations, independently of the HTTP feed and history, with a transactional per-sink outbox, retries, ordering, TLS authentication and telemetry. Apply database schema v1.2.1 before upgrading services. ([#678](https://github.com/eclipse-basyx/basyx-go-components/pull/678))
  * **Security:** MQTT supports authenticated, verified TLS connections. Broker ACLs govern subscribers; HTTP caller-specific ABAC remains on the feed. Effective configuration output omits credentials.

* **Low impact** — Add experimental Kafka CloudEvents eventing with a Kafka playground, using the existing event contract and outbox. ([#679](https://github.com/eclipse-basyx/basyx-go-components/pull/679))
  * **Security:** Kafka supports TLS, mutual TLS and SASL authentication. Broker ACLs control consumer access; HTTP authorization does not filter Kafka subscribers.

* **Low impact** — Add experimental AMQP 1.0 CloudEvents eventing with a RabbitMQ playground using the existing event contract and outbox. ([#680](https://github.com/eclipse-basyx/basyx-go-components/pull/680))
  * **Security:** AMQP supports TLS and mutual TLS with optional SASL PLAIN authentication. Broker ACLs control subscriber access; HTTP authorization does not filter AMQP subscribers.

* **Low impact** — Implement value-only synchronous and asynchronous operation invocation and result retrieval for the Submodel and AAS Repository APIs, with typed numeric and boolean values, empty structured arguments, shared persistent async handles, and delegated-operation example coverage. ([#685](https://github.com/eclipse-basyx/basyx-go-components/pull/685))
  * **Security:** None. Existing OIDC, ABAC, async-job owner and path validation, and request security contexts are preserved.

* **Low impact** — Configurable response size limit for delegated Submodel Operations, allowing larger synchronous and asynchronous results while retaining the 1 MiB default. ([#707](https://github.com/eclipse-basyx/basyx-go-components/pull/707))
  * **Security:** Delegated responses remain size-limited, and invalid limits are rejected at startup. Raising the limit increases peak memory use per request.

* **Low impact** — Experimental relationship-based access control (ReBAC) for the AAS, Submodel and Concept Description repositories, the AAS and Submodel registries, the Digital Twin Registry, discovery, the AASX file server, the AAS Environment and the current state of Digital Product Passports. When `rebac.enabled` is set, access is granted when ABAC or ReBAC allows it. Creators own new resources, and owners share Shells, Submodels, single SubmodelElement paths, Concept Descriptions, descriptors, discovery entries and AASX packages with users and groups as viewer, editor, executor or owner through `$access` sub-resources with optimistic concurrency (`If-Match`). Further features are approved AAS-to-Submodel inheritance links, invitation links, and administrator bootstrap and recovery. Users are identified by issuer and the claim configured in `rebac.subjectClaim` (default `sub`, for example `oid` for Microsoft Entra ID), and `GET /security/rebac/principal` returns the identity grants are matched against. Descriptors synchronized from Shells and Submodels, and discovery entries created by the discovery integration, inherit the access of their source. Relationships are stored and evaluated in the BaSyx PostgreSQL database, including lists, `/serialization`, `/upload` and the registry bulk API. Services with ReBAC enabled announce the profile `https://basyx.org/aas/API/3/2/RelationshipBasedAccessControl/1.0` in `/description`, and CORS responses of all services expose the `ETag` and `Location` headers so web clients such as the BaSyx UI can manage access. ReBAC is disabled by default and otherwise leaves all existing behavior unchanged. The example `examples/BaSyxReBACExample` includes the BaSyx UI. See docu/security/rebac/README.md. ([#712](https://github.com/eclipse-basyx/basyx-go-components/pull/712))
  * **Security:** ReBAC is only evaluated when ABAC does not allow unconditionally; if it cannot decide, requests fail with 503 instead of falling back to ABAC-only decisions. A ReBAC grant widens only the granted resource and, by design, is not limited by ABAC fragment filters or update conditions of that resource, so administrators should keep sensitive content in separate resources. Denials without grants look exactly like ABAC denials, management routes answer 404 to non-managers, and anonymous callers stay ABAC-only. Grants are re-evaluated by every query, so revocations take effect at commit, also for queued asynchronous work. Matching identifiers never grant access; only resources created by synchronization inherit from their source. Invitation tokens are stored only as SHA-256 hashes and accepted in the request body. Access changes check the permission of the caller again while the object is locked, so a revocation always wins over a pending change, and access ETags are bound to their object. Upserts enforce the create or update right for what they actually write, so a creation right never replaces a resource created concurrently by someone else. `/$access/effective` answers only for confirmed rights and treats conditional ABAC rights as unconfirmed.

* **Low impact** — With ReBAC enabled, every access change (grants, invitations, inheritance links and reconciliations) is recorded in a hash-chained, append-only audit trail. With history evidence enabled, each audit event is archived in the WORM evidence store. Administrators page through the trail newest first through `/security/rebac/admin/audit`, filtered by resource or by the user who made the change, and each event names its resource while it exists. `/security/rebac/admin/audit/verify` verifies the chain in ranges and from checkpoints (event id and hash) retained outside the database, and `historyevidenceverifier -rebac-audit` verifies it offline against an independently retained head hash. ReBAC decisions and access changes are reported as OpenTelemetry spans and metrics. ([#712](https://github.com/eclipse-basyx/basyx-go-components/pull/712))
  * **Security:** Access changes are attributable and tamper-evident. With history evidence enabled, a change fails with 503 instead of being applied without its WORM audit record. The audit trail is readable by configured ReBAC administrators only.


### Changed

* **High impact** — Database schema v1.2.2 adds a persistent `auth_uuid` to `aas`, `submodel`, `concept_description`, `descriptor`, `aas_identifier` and `aasx_package`, and adds the `rebac_*` tables. Applying the patch rewrites these tables under an exclusive lock; plan a maintenance window for large installations and apply schema v1.2.2 with the configuration service before upgrading services. ([#712](https://github.com/eclipse-basyx/basyx-go-components/pull/712))
  * **Security:** None.

* **Low impact** — `PATCH /submodels/{submodelIdentifier}` and `PUT` on a SubmodelElementCollection, SubmodelElementList, Entity or AnnotatedRelationshipElement now write only the differences to the stored state, like Submodel `PUT` already does. The Submodel and unchanged elements keep their database rows instead of being deleted and inserted again, which reduces write load for large Submodels. `PATCH` with `statements` or `annotations` also reconciles the child elements. Removing many elements in one write no longer takes time quadratic in their number, neither when planning nor in the database. ([#716](https://github.com/eclipse-basyx/basyx-go-components/pull/716))
  * **Security:** The ReBAC authorization identity of a Submodel stays on its row instead of being copied during a replacement, so grants on the Submodel and on element paths are kept by construction. ABAC checks of the existing and the prospective state are unchanged.

* **Low impact** — The secured examples and the integration test setups now use Keycloak 26.7.4 instead of 26.0.6. Their realm imports only contain the BaSyx-specific clients, roles, groups, users and user profile attributes, so Keycloak supplies its current built-in defaults. The Compose files use `KC_BOOTSTRAP_ADMIN_USERNAME` and `KC_BOOTSTRAP_ADMIN_PASSWORD` and no longer set removed hostname v1 options. Existing example Keycloak databases are migrated by Keycloak on startup; an already imported realm is not re-imported. ([#717](https://github.com/eclipse-basyx/basyx-go-components/pull/717))
  * **Security:** Moves the example and test identity provider from Keycloak 26.0.6 to 26.7.4, which includes the upstream security fixes released in between. The committed realm files no longer contain realm signing and encryption private keys, so each fresh import generates its own keys instead of reusing keys published in the repository.

* **High impact** — Signed-in callers without ReBAC grants receive empty lists instead of `403` on the list routes ReBAC covers, so ABAC policies no longer need list rules for them. The ReBAC example and the administration guide drop their list rules, which compared identifiers with a placeholder value. ([#718](https://github.com/eclipse-basyx/basyx-go-components/pull/718))
  * **Security:** The removed placeholder rules exposed every resource created with the placeholder identifier to all signed-in users, including its content; policies that copied them should remove them. Anonymous callers and single resources keep the ABAC denial, and backends without formula support, such as the AASX file server, return no row to callers without grants.

* **Low impact** — Lists that only ReBAC grants make visible are read from the granted rows instead of testing every row of the table. ([#718](https://github.com/eclipse-basyx/basyx-go-components/pull/718))
  * **Security:** None.


### Removed

* **Low impact** — The Digital Twin Registry does not offer ReBAC: it authorizes with ABAC only, ignores `rebac.*` settings, and neither provides, documents nor announces the ReBAC management API or profile. ([#718](https://github.com/eclipse-basyx/basyx-go-components/pull/718))
  * **Security:** ReBAC grants no longer give access to the Digital Twin Registry; ABAC alone decides there.


### Fixed

* **High impact** — AAS Environment and Submodel Repository writes reject semantically invalid boolean and temporal Property or Range values with HTTP 400 instead of exposing PostgreSQL conversion errors as HTTP 500 responses. ([#649](https://github.com/eclipse-basyx/basyx-go-components/pull/649))
  * **Security:** None.

* **Low impact** — Reuse PostgreSQL plans for unrestricted shell descriptor updates to reduce repeated query planning on the writer database. ([#669](https://github.com/eclipse-basyx/basyx-go-components/pull/669))
  * **Security:** None. Existing authorization checks and writer transaction consistency are preserved.

* **Low impact** — Correct registry synchronization descriptor interface to use API version 3.2. ([#670](https://github.com/eclipse-basyx/basyx-go-components/pull/670))
  * **Security:** None.

* **Low impact** — Run registry benchmark compose commands with literal arguments and report process-launch failures with BENCH-RUN-EXEC. ([#681](https://github.com/eclipse-basyx/basyx-go-components/pull/681))
  * **Security:** Benchmark hardening: compose paths are passed as literal arguments without shell interpretation; no demonstrated production vulnerability.

* **High impact** — Align encoded API parameters and exact reference and asset filtering across registries, repositories, Discovery, DTR and AAS Environment. Accept valid padded and unpadded base64url for Tractus-X conformance and require nonempty supplied pagination parameters; standardize query and path cursors without legacy fallback. Use the standalone registry and discovery contracts in the joined AAS Environment OpenAPI, keeping DTR extensions exclusive to the DTR specification. Correct DTR asset-link schema references and Company Lookup reference, required-name, and error-response schemas; require clientTimeoutDuration only for asynchronous value-only invocation and return HTTP 400 when missing. ([#683](https://github.com/eclipse-basyx/basyx-go-components/pull/683))
  * **Security:** Rejects malformed API parameters and prevents reference filters from matching supplemental semantic IDs hidden by collection-level authorization masks.

* **High impact** — Require ID-based DPP lookup to match both the owning AAS identifier and its DppMetadata.digitalProductPassportId; a requested ID returns 404 when either stored value differs. Historical lookup applies the same check, while product-based retrieval remains available through uniqueProductIdentifier. Generate a bounded, valid DPP AAS idShort independently of potentially long or punctuated identifiers while preserving the exact identifier as the AAS ID. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Preserves request authorization and applies metadata value visibility before returning product-search DPP IDs or pagination cursors; mismatched AAS and metadata IDs return 404 for ID-based requests. The generated idShort is non-authoritative display metadata; authorization and lookup continue to use the exact AAS/DPP identifier.

* **High impact** — Download attachments with missing filenames using the managed reference or file path, and return 404 when attachment bytes are absent. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Normalizes fallback download filenames.

* **High impact** — Apply DPP merge patches only to affected resources, preserving existing AAS and Submodel identities and metadata. Reuse the loaded passport state while preparing updates and reject generated-ID collisions when creating new sections. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Prevents unintended overwrite or deletion of unrelated and shared Submodels during DPP updates.

* **High impact** — Delete a DPP's owning AAS, DppMetadata and their descriptors while retaining all content Submodels, their descriptors and managed attachments. Removing a content section also retains its Submodel and attachments. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Revalidates the matching DPP ID and ownership under transaction row locks before deletion while preserving request authorization; content Submodels and attachments are retained instead of being removed as a side effect of DPP deletion or section removal.

* **High impact** — Preserve collection field names and shapes in compressed DPP representations while converting only typed multilingual properties and files. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** None. Serialization remains subject to the existing authorization and field-visibility checks.

* **High impact** — Include DPP content only for Submodels whose semantic IDs are declared in contentSpecificationIds, with empty or absent declarations selecting no content. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Prevents undeclared AAS Submodels from being exposed through current, historical, full, compressed, or fine-grained DPP reads.

* **High impact** — Serialize AAS Entity elements in full DPP read representations as EN 18223 DataElementCollection values, preserving entity identity, specific asset references, nested statements, multilingual values, and file resources. Unsupported full element conversions now return 422 instead of being reported as missing DPPs. ([#691](https://github.com/eclipse-basyx/basyx-go-components/pull/691))
  * **Security:** Existing authorization and content-selection filtering remain unchanged; the fix only corrects authorized DPP read serialization and error classification.

* **Low impact** — Pull current SNAPSHOT images when starting the MQTT, Kafka, and AMQP examples, avoiding startup failures caused by outdated cached images. ([#698](https://github.com/eclipse-basyx/basyx-go-components/pull/698))
  * **Security:** None.

* **Low impact** — Declare AAS Environment health checks in the Event Feed, MQTT, Kafka, and AMQP examples so Podman Compose can start their dependent Web UI services. ([#699](https://github.com/eclipse-basyx/basyx-go-components/pull/699))
  * **Security:** None.

* **Low impact** — Allow Submodel creation with field-based ABAC CREATE rules when Submodel Registry synchronization is enabled. ([#700](https://github.com/eclipse-basyx/basyx-go-components/pull/700))
  * **Security:** Submodel CREATE authorization remains enforced; only the internal descriptor synchronization excludes the Submodel-specific field filter.

* **Low impact** — The guarded History Audit example and the Submodel repository benchmark start again: MinIO no longer publishes container images, so they now use pinned community builds of MinIO and its client (`pgsty/minio`, `pgsty/mc`) with unchanged object lock settings. ([#712](https://github.com/eclipse-basyx/basyx-go-components/pull/712))
  * **Security:** The WORM evidence store of the example keeps compliance-mode object lock. The images are pinned to release tags instead of `latest`.

* **Low impact** — `PATCH` on a SubmodelElementCollection or SubmodelElementList with a `value` now replaces the child elements with the given ones: omitted children are deleted and `"value": []` removes all children. Before, the given children were inserted next to the existing ones, which failed with a conflict, and an empty array changed nothing. Clients that sent only new children or an empty array must now send the complete list. A `$metadata` `PATCH` on a container with children no longer fails, and a `PATCH` without `value`, `statements` or `annotations` leaves the child elements untouched. ([#716](https://github.com/eclipse-basyx/basyx-go-components/pull/716))
  * **Security:** A `PATCH` without child elements no longer rewrites the children of an Entity or AnnotatedRelationshipElement from the caller's filtered view, so children the caller cannot see are no longer removed.

* **Low impact** — A Submodel `PUT` no longer fails with a conflict when an element was deleted from a SubmodelElementCollection, Entity or AnnotatedRelationshipElement before and the new state adds elements to it. Reconciliation now compares against the stored sibling positions instead of assuming gapless positions. ([#716](https://github.com/eclipse-basyx/basyx-go-components/pull/716))
  * **Security:** None.

* **Low impact** — The OpenAPI specification of the Company Lookup service no longer documents ReBAC endpoints when `rebac.enabled` is set; the service does not offer ReBAC. ([#718](https://github.com/eclipse-basyx/basyx-go-components/pull/718))
  * **Security:** None.

* **Medium impact** — Allow Submodel deletion and descriptor-changing Submodel updates (PUT, PATCH, PATCH $metadata) with field-based ABAC rules when Submodel Registry synchronization is enabled; these requests previously failed with HTTP 500. ([#719](https://github.com/eclipse-basyx/basyx-go-components/pull/719))
  * **Security:** Submodel and AAS mutation authorization remains enforced by the repositories; internal descriptor synchronization no longer evaluates the caller's repository-specific field filter against registry tables.


### Security

* **Low impact** — An AASX package `PUT` no longer replaces an existing package with only the right to create packages, or creates one with only the right to update packages. ([#718](https://github.com/eclipse-basyx/basyx-go-components/pull/718))
  * **Security:** Callers with only one of the create and update rights could write packages beyond that right.
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
