# Resource-bound authorization validation

Validated through 2026-09-10 with PostgreSQL 18. The test cache was cleared before the mandatory Submodel Repository suite, and no test-selection flags were used.

| Check | Result |
|---|---|
| Fresh migration from `database/base.sql` through schema v1.1.19 | PASS |
| Database-backed `go test -v ./internal/common/security/integration_tests` | PASS, including ABAC collection admission, filtered collection reads, ownership, inheritance, aliases, concurrent mutation, ABAC fallback, input validation, and Discovery POST creation |
| `go test -v ./internal/aasrepository/integration_tests` | PASS |
| `go test -v ./internal/aasregistry/integration_tests` | PASS |
| `go test -v ./internal/smregistry/integration_tests` | PASS |
| `go test -v ./internal/discoveryservice/integration_tests` | PASS |
| `go test -v ./internal/conceptdescriptionrepository/integration_tests` | PASS |
| `go test -v ./internal/submodelrepository/integration_tests` after `go clean -testcache` | Two unrelated pre-existing failures; all remaining tests pass |
| Focused grammar, security, descriptor, registry, discovery, and Concept Description persistence tests | PASS |
| `golangci-lint run ./...` | Not rerun: the installed binary was built with Go 1.26 while the project requires Go 1.27.1 |
| `go vet ./...` and formatting check | PASS |
| Secured example Compose validation and live six-component startup | PASS; all six services healthy |
| Authenticated live CRUD and `/$access` smoke tests | PASS for AAS, Submodel, AAS Descriptor, Submodel Descriptor, Discovery record, and Concept Description |
| Collection authorization regression | PASS: collection access is ABAC-only, concrete ReBAC grants filter results, creators become direct owners, and collection `/$access` routes return 404 |
| Anonymous live collection reads | PASS for all six resource collections |

The broad-suite failures do not exercise resource-bound authorization. `TestDelegationOperation` resolves its test endpoint through `host.docker.internal` to an address outside the configured trusted delegation allowlist and returns HTTP 502. `TestSubmodelRepositoryHistoryTracksSubmodelElementChangesAndRecentDeletes` observes an unexpected historical value ordering. The remaining API contract, persistence, concurrency, and startup checks complete successfully.

The secured example was migrated in place from v1.1.18 to v1.1.19, rebuilt from this checkout, and left running. Temporary smoke-test resources were deleted after verification. The live checks use the example's Keycloak-issued administrator token and verify both data operations and the access overview.

Configuration, supported routes, ABAC migration, creation rights, registry inheritance, and access administration are documented in [Resource-bound access](RESOURCE_BOUND_ACCESS.md) and the [secured example](../../examples/BaSyxSecuredExample/README.md).
