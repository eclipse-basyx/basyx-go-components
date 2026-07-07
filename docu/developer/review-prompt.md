# Senior Engineering PR Review Prompt

Review this Pull Request as a Senior Software Engineer for the BaSyx Go components repository.

Follow the repository instructions in `AGENTS.md` and review the actual diff, not just the PR description. Focus only on actionable issues that should be addressed before merging.

## Review Priorities

### Correctness

Check for:

- Functional bugs and behavioral regressions
- Edge cases, nil handling, empty inputs, cancellation paths, and shutdown paths
- Race conditions, goroutine leaks, resource leaks, and unbounded waits
- Broken context propagation, especially use of `context.Background()` in live code
- Error handling that drops root causes or returns uncoded errors
- Breaking changes to public APIs, config keys, OpenAPI behavior, routes, response shapes, or database behavior

### Security

Check for:

- Missing authentication or authorization checks
- ABAC/OIDC/security middleware accidentally bypassed
- Insecure HTTP/client/server defaults, missing timeouts, unbounded request bodies, unsafe redirects, SSRF risks
- Sensitive data exposure in logs, responses, config printing, errors, or tests
- SQL injection or unsafe query construction
- Use of plain SQL where GOQU should be used
- Unsafe file/path handling, archive extraction, upload/download behavior, or trust boundary mistakes

### Database And Persistence

Check for:

- Schema changes not reflected in `basyxschema.sql`
- Queryable columns added to `*_payload` tables
- Missing transactions, incorrect rollback/commit behavior, lock ordering issues
- Inefficient queries, N+1 patterns, missing pagination, unbounded scans
- Incorrect history/audit/recent-changes behavior

### Clean Code And Architecture

Check for:

- Duplicated logic that should live in `internal/common` or existing local helpers
- Large or complex functions, deep nesting, unclear names, dead code
- Unnecessary abstractions or coupling across components
- Code that violates established package boundaries or local patterns
- Comments used to explain unclear code instead of simplifying the code
- Missing or weak GoDoc on exported functions, types, constants, or variables

### Configuration And Compatibility

Check for:

- New config fields missing defaults, validation, env support, docs/templates, or printed config behavior
- Changed defaults that may break deployments
- Service-specific config drift across equivalent components
- Mismatches between command services that should follow the same lifecycle/security pattern

### Testing

Check for:

- Missing regression tests for the changed behavior
- Missing integration tests where service behavior, DB behavior, or HTTP behavior changes
- Tests that use caches, narrow flags, brittle sleeps, leaked containers, or non-deterministic ports
- Insufficient negative tests for invalid config, malformed input, unauthorized access, cancellation, or failure paths

## Repo-Specific Rules To Enforce

- Go code must use GOQU for SQL query construction. Do not accept new plain-text SQL query construction unless there is an established exception.
- Errors should include coded prefixes in the project style, for example `SMREPO-UPDPROP-EXECDBQUERY`.
- Live code must not use `context.Background()`; pass a caller/request/signal-aware context. Unit tests may create their own context.
- Own `.go` files must include the project license header. Generated files may follow their generator's established header pattern.
- Public Go symbols must have useful GoDoc. For exported functions, check that docs explain parameters, return values, and include a short example snippet when it materially improves correct usage.
- Database schema changes must update `basyxschema.sql`.
- Queryable columns must not be added to `*_payload` tables.
- Service changes should stay aligned across equivalent production services.
- Prefer existing helpers and patterns over new one-off implementations.
- Pay special attention to security, scalability, and performance.

## Suggested Multi-Pass Review

If subagents or parallel review passes are available, split the review into:

1. Security and lifecycle pass
   - Auth, ABAC/OIDC, context propagation, HTTP timeouts, graceful shutdown, logging, sensitive data, request limits.
2. Data and API pass
   - DB/schema/query behavior, GOQU usage, transactions, API compatibility, config compatibility, OpenAPI/routes.
3. Maintainability and test pass
   - DRY, package boundaries, complexity, naming, missing tests, flaky tests, integration test coverage.

The final reviewer should merge duplicate findings and report only issues with concrete evidence.

## Output Format

Only report actionable findings. Do not praise code. Do not summarize what is already good.

For each finding, provide:

- **Severity:** Critical / High / Medium / Low
- **Location:** file and line
- **Explanation:** why this is a real issue
- **Suggested fix:** concrete remediation

Order findings by severity. If there are no actionable findings, say:

> No actionable findings found.

Then list any residual test gaps or review limitations briefly.
