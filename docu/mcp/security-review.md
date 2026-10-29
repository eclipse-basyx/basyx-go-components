# MVP security review

This is an implementation self-review, not an independent security approval.

| Boundary | Control | Evidence |
| --- | --- | --- |
| Caller identity | Shared OIDC trustlist, required audience, original-token in-process dispatch | OIDC/common suites, dispatch unit tests, secured live sandbox |
| Tool access | Shared profile registry for listing/invocation; explicit write mode | Protocol/profile tests |
| Aggregates | Only REST-visible projections; fixed GOQU SQL, typed parameters, bounded complete scans | Three-policy live aggregate parity tests and denied user |
| Human approval | Browser OAuth with PKCE, owner binding, CSRF/origin checks; no confirmation tool | Real browser sign-in plus automated approval flow |
| Confirmation replay | Hash binding to plan/issuer/subject/action/payload, expiry, optimistic consumption | Approval unit tests and live replay rejection |
| Draft quality | Strict SDK metamodel verification plus allowlisted structural templates | Validation tests and live creation |
| Crash window | Revision receipt and resource mutation share a transaction | Revision-observer failure test and saga integration tests |
| Compensation | Original revision checks, reverse order, current caller permissions | Duplicate-ID compensation and changed-resource rollback tests |
| Audit | Transactional plan events plus per-resource receipts; no saved bearer tokens | Store audit failure test, sandbox audit inspection |
| Prompt injection | Retrieved content remains data; writes require separate approval | Injection unit case and absent/forged-token tests |
| Resource exhaustion | Request/depth/response/page/scan/fan-out/rate/concurrency/deadline budgets | Limit and protocol tests; bounded query paths |

## Operational limits and remaining qualification

- Compensating writes cannot guarantee immediate cleanup during outages, permission revocation or concurrent mutation. Inspect `recovery_required` and `conflict`; do not treat them as completion.
- A user changing policy between preflight and execution is handled by current authorization on each API request. Cleanup may require that user's permissions to be restored before explicit rollback.
- A field-dependent policy that cannot prove full create/read/delete rights is rejected for creation rather than bypassed.
- Browser sign-in state is process-local. A deployment with multiple replicas needs sticky routing for the short OAuth browser flow or must repeat login after landing on another instance.
- Qualified IDTA/submodel_info release artifacts were not supplied. The included templates are sandbox examples; qualification of production template versions remains an operator/release responsibility.
- L3 semantics, arbitrary SQL, external registry resolution and operation invocation are intentionally absent.
- Plan/provenance and audit retention need an operator policy. Operational audit is not tamper-evident evidence.
- Human acceptance of all scenarios in the chosen production clients and an independent security review remain release activities.

The mandatory Submodel Repository suite was run in full on this change and on an untouched HEAD snapshot. Both exhibited the same three failures: `TestDelegationOperation` (Colima delegation address allowlist), `TestSubmodelRepositoryHistoryExtentAndCoreLevel`, and `TestSubmodelRepositoryHistoryTracksSubmodelElementChangesAndRecentDeletes` (historical read returned 404). These failures are not waived or reported as passing.
