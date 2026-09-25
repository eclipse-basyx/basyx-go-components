# OpenFGA ReBAC implementation status

This feature is experimental and explicitly enabled with `REBAC_ENABLED=true`.
The development deployment in [BaSyxOpenFGA](../examples/BaSyxOpenFGA/README.md)
runs the authorization coordinator, PostgreSQL projection worker, access APIs
and AAS Web UI. Dedicated examples cover audit/WORM storage and OpenTelemetry.
ReBAC-disabled deployments retain the existing ABAC path. Unimplemented
aggregate routes fail closed with `503`. The full requested acceptance scope is
**not yet complete**.

## Implemented foundations

- ReBAC configuration, issuer-scoped principals and administrators, strict
  enabled-state validation, a fixed OpenFGA authorization model, and conditional
  `/description` advertisement through the
  `https://basyx.org/aas/API/3/2/ResourceBoundAccessControl/1.0` profile.
- Bounded OpenFGA HTTP client, pinned-model verification, Check/BatchCheck and
  streamed ListObjects with indexed object-key resolution,
  idempotent desired relationship writes, tuple-change pagination, global
  OpenTelemetry spans and bounded-cardinality metrics.
- ReBAC-first coordinator: a denial invokes the supplied unchanged ABAC
  evaluator; an OpenFGA error never invokes that fallback. Audit failure blocks
  a grant from being returned.
- PostgreSQL scope/configuration identity, stable resource identity primitives,
  desired relationships, revision barrier, retryable projection, and direct
  grant replacement with stale-revision and last-owner protection.
- One registered schema patch, `1_2_2.sql`, and matching schema metadata.
- Global `audit` and `audit.worm` configuration, field-level legacy history
  compatibility, explicit-conflict rejection, and independent audit/WORM
  enablement. Configuration does not contain experimental naming.
- Shared evidence package with history-compatible aliases and unchanged S3
  object formats; no dependency on enabled resource history.
- Durable hash-chained audit repository, atomic append, backlog checks,
  consistent-snapshot integrity verification, WORM archiver, retention checks,
  and isolated PostgreSQL/OpenFGA/MinIO integration tests.

The runtime is mounted by DB-backed security setup. Live tests have verified
creator ownership, sharing, revocation, ABAC fallback, OpenFGA outage handling,
signed WORM archival, trace correlation, atomic JSON/XML imports and complete
serialization checks. Bulk workers now authorize individual resources, retain
submitter ownership, and refresh initiating credentials. Current DPP operations have passed live underlying-resource authorization, rollback, visibility, and historical ABAC isolation checks.

Embedded Submodel descriptor identities include the containing AAS descriptor;
matching standalone IDs do not convey access. Real PostgreSQL tests cover
independent element rows and standalone versus embedded descriptor visibility.

## Remaining work before the full plan can be accepted

- Package-only resources receive stable authorization identities and creator ownership atomically with ingestion without being published as repository data. Automatic initialization of manifests for packages stored before ReBAC remains unavailable; re-uploading an established package through PUT initializes it. Conditional ABAC package-content evaluation remains conservative; unconditional ABAC and ReBAC grants are supported.
- Local performance measurements are included, but representative production-scale benchmarks remain deployment work.
- Complete per-result ABAC audit coverage and the full asynchronous expiry/revocation/failure deployment matrix still require acceptance work. All eight effective registry/submodel-registry/discovery configuration states are covered by real PostgreSQL integration tests; the forced-on Discovery defaults are also covered by the live AAS Environment setup.
- Historical signing keys are supported through a PEM public-key bundle, but the complete storage-outage/key-rotation/restart deployment matrix is not yet covered.
- A Changie release fragment needs the actual PR number; no PR exists for this branch, and no number has been invented.

The working demo is suitable for development and evaluation. It is not a claim that all requirements in the full implementation plan are complete.

History and event-feed authorization, including historical DPP, remain ABAC-only.
Neither ABAC policies nor their evaluator should be translated or replaced.

## Validation performed

- Affected security, repository and aggregate packages compile and their focused tests pass. Full-tree lint and complexity review are repeated as implementation proceeds.
- Affected common/security/ReBAC/audit/evidence/history unit packages.
- Live PostgreSQL revision rollback, pending barrier, failed projection/retry,
  stale grant revision, last-owner protection and unauthorized management.
- Live OpenFGA fixed model, contextual groups, issuer isolation, element
  containment, explicit AAS inheritance and read-only descriptor/discovery
  source inheritance.
- Live PostgreSQL audit rollback, concurrent appends, tamper detection, backlog
  protection, independent commit and concurrent verification.
- Real MinIO Object Lock: compliance-retained versions reject deletion; later
  versions do not invalidate prior versions. Audit delivery is acknowledged
  only after actual archive and retention verification. Insufficient retention
  is rejected; retained writes explicitly record the legal-hold state.
- Full mandatory Submodel integration suite passed after `go clean -testcache`, using exactly `go test -v ./internal/submodelrepository/integration_tests` (118.186 seconds in the final run). Test fixtures resolve the Docker delegation gateway and read persisted historical timestamps; production history semantics are unchanged.
- All seven standalone service families passed creation/ownership, ReBAC sharing, revocation, and surviving ABAC fallback against isolated databases freshly migrated through v1.2.2.
- ReBAC-only AAS/Submodel creation and qualified/direct Submodel mutations passed, including preservation of inaccessible sibling descriptor metadata during internal synchronization.
- Stored-package reads passed package-only identity creation, joint package/content authorization, revocation, deleted/recreated source isolation, asynchronous ingestion completion, and submitter-only result access.
- The optional Grafana dashboard loads, Prometheus scrapes the shared Collector, and alert rules evaluate successfully. Operational metrics use the existing telemetry runtime.
- The local benchmark measured ReBAC decision p50/p95 at 18.89/25.42 ms, a five-item authorized list at 18.75/19.94 ms, a 20-member JSON import at 96.57 members/s, and a 20-member atomic asynchronous bulk request at 15.65 members/s including status polling. These figures describe the local demo, not production capacity.
- Full-tree golangci-lint v2.13.2 reported zero issues. Affected common, repository, registry, discovery, package, environment, and DPP tests passed.
- OpenAPI extension documentation is provided in [rebac-openapi.json](rebac-openapi.json).

## Running the new live tests

Provision an isolated PostgreSQL database with the registered migration and an
OpenFGA server. Set `BASYX_REBAC_TEST_DSN` and
`BASYX_REBAC_TEST_OPENFGA_URL` and, for an authenticated server, `BASYX_REBAC_TEST_OPENFGA_TOKEN`, then run:

```sh
go test -v ./internal/common/rebac/integration_tests ./internal/common/audit/integration_tests
```

For Object Lock tests, also set `TEST_S3_ENDPOINT`, `TEST_S3_ACCESS_KEY`, and
`TEST_S3_SECRET_KEY` to disposable S3-compatible storage supporting Object Lock:

```sh
go test -v ./internal/common/evidence/integration_tests ./internal/common/audit/integration_tests
```

The Object Lock tests create uniquely named buckets with one-day compliance
retention. Use disposable test storage, as retained versions cannot be deleted
through the object-store API before that retention expires. Live suites skip
when their explicit service configuration is absent; a skipped suite is not
live validation.
