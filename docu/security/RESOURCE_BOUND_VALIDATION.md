# Resource-bound authorization validation

Validated on 2026-09-09 with PostgreSQL 18. The test cache was cleared before the mandatory Submodel Repository suite, and no test-selection flags were used.

| Check | Result |
|---|---|
| Fresh migration from `database/base.sql` through schema v1.1.19 | PASS |
| Database-backed `go test -v ./internal/common/security/integration_tests` | PASS, including lifecycle, ownership, inheritance, aliases, concurrent mutation, ABAC fallback, input validation, and Discovery POST creation policy |
| `go test -v ./internal/aasrepository/integration_tests` | PASS |
| `go test -v ./internal/aasregistry/integration_tests` | PASS |
| `go test -v ./internal/smregistry/integration_tests` | PASS |
| `go test -v ./internal/discoveryservice/integration_tests` | PASS |
| `go test -v ./internal/conceptdescriptionrepository/integration_tests` | PASS |
| `go test -v ./internal/submodelrepository/integration_tests` after `go clean -testcache` | One environment-sensitive delegation failure; all remaining tests pass |
| Focused grammar, security, descriptor, registry, discovery, and Concept Description persistence tests | PASS |
| `golangci-lint run ./...` with v2.13.2 built by the repository Go toolchain | PASS, zero issues |
| `go vet ./...` and formatting check | PASS |
| Secured example Compose validation and live six-component startup | PASS; all six services healthy |
| Authenticated live CRUD and `/$access` smoke tests | PASS for AAS, Submodel, AAS Descriptor, Submodel Descriptor, Discovery record, and Concept Description |
| Live collection grant workflow | PASS: administrator granted `usera` AAS CREATE, `usera` created an AAS and became its direct owner, then the temporary resource and grant were removed |
| Anonymous live collection reads | PASS for all six resource collections |

The single broad-suite failure is `TestDelegationOperation`: the test endpoint resolves through `host.docker.internal` to an address outside that test's trusted delegation allowlist, producing HTTP 502 instead of 200. It does not exercise resource-bound authorization. The same run completed the remaining API contract, persistence, history, concurrency, and startup checks successfully.

The secured example was migrated in place from v1.1.18 to v1.1.19, rebuilt from this checkout, and left running. Temporary smoke-test resources were deleted after verification. The live checks use the example's Keycloak-issued administrator token and verify both data operations and the access overview.

Configuration, supported routes, ABAC migration, creation grants, registry inheritance, and access administration are documented in [Resource-bound access](RESOURCE_BOUND_ACCESS.md) and the [secured example](../../examples/BaSyxSecuredExample/README.md).
