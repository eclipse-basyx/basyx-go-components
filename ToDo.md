# Open ToDo List for ABAC and Audit Improvements

## 1. Generate request/correlation IDs when missing

In AuditContextMiddleware, if request_id is empty, generate a random request id.
If correlation_id is empty, set it to the request id.
Also write both to response headers so clients/operators can trace the request.
This is safe because request IDs are not identity. We are not inventing a user.

## 2. Add deterministic ABAC rule IDs

Extend the ABAC evaluator to return a stable matched rule id, for example rule:<index>:<sha256-prefix>.
Store that in AuthorizationDecision.MatchedRuleID.
Keep policy_id as the hash of the whole access policy. Together, policy_id + matched_rule_id gives good audit traceability.
