# Resource-bound authorization validation

Validated on 2026-09-08 with PostgreSQL 18. The test cache was cleared before the final suite runs. No test-selection flags were used.

| Check | Result |
|---|---|
| `go test -v ./internal/common/security/...` with `BASYX_REBAC_TEST_DSN` pointing to the migrated test database | PASS, including 14 new access integration tests and all alias subtests |
| Common, AAS persistence, Submodel persistence, and SubmodelElement persistence suites | PASS |
| `go test -v ./internal/aasrepository/integration_tests` | PASS on the final run |
| `go test -v ./internal/submodelrepository/integration_tests` | Two existing failures, listed below |
| `go test -v ./internal/aasenvironment/integration_tests` | Fails through its embedded Submodel history suite |
| `bash scripts/lint.sh` | PASS, zero issues; golangci-lint v2.13.2 rebuilt with the repository's Go toolchain |
| Cognitive complexity of new implementation functions | All at or below 15 |
| `git diff --check` | PASS |
| Bridge Docker Compose configuration and Python syntax | PASS |

The remaining integration failures are `TestDelegationOperation` (the test endpoint's resolved address is rejected by the delegation trust configuration) and `TestSubmodelRepositoryHistoryTracksSubmodelElementChangesAndRecentDeletes` (history returns the earlier value). Both also fail in an isolated, unchanged checkout of baseline commit `d8443a9c`. An AAS history assertion also failed intermittently on that baseline and earlier runs; it passed on the final run.

The bridge example requires deployment-specific OIDC identities and tokens. Its complete authenticated Docker workflow was not run against a live identity provider. The access integration tests use real repository controllers and PostgreSQL with test-only identity injection; the existing OIDC suites also pass.

Configuration, supported routes, and the conservative rules for anonymous list identities and inheritance-edge changes are documented in [Resource-bound access](RESOURCE_BOUND_ACCESS.md).
