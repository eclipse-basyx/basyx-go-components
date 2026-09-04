# AAS Hierarchy Query Authorization Plan

Status: proposed architecture; not implemented yet.

## Security invariant

An AAS query may evaluate a referenced Submodel or Submodel Element only when the requesting principal is allowed to read the data being evaluated. A reference from an allowed AAS does not grant access to the referenced resource.

The query must behave as if unauthorized related resources do not exist. This applies to every operator, including equality, inequality, prefix, regular expression, numeric comparison, casts, negation, and `$match`.

## Recommended policy semantics

The effective result is the intersection of independently evaluated permissions:

1. The outer AAS must satisfy the effective AAS READ policy.
2. A `$sm` predicate may inspect only referenced Submodels that satisfy the effective Submodel READ policy.
3. A `$sme` predicate may inspect only elements and fields visible under the effective Submodel/SME READ policy, including fragment filters.

These permissions do not have to be declared in one access rule. Separate rules are preferable when AAS and Submodel access have different conditions. A rule containing both `$aas` and `$sm` objects can express the same grant when both objects intentionally share rights, attributes, and formulas; the object entries remain alternative targets and must still be evaluated once for each resource scope.

A dedicated `$sme` object rule is required only when access is granted at Referable level. Unrestricted Submodel READ grants visibility of its elements; a Submodel grant with element filters exposes only the filtered element view.

Conceptually:

```text
returned AAS
  = AAS allowed by AAS READ
  AND caller hierarchy predicate matches at least one related row
      visible through Submodel/SME READ
```

## Required architecture changes

### 1. Preserve one authorization snapshot for the request

The ABAC middleware should place a request-scoped authorization session in the context. It should contain the active compiled access-model snapshot, claims, trusted globals, simplification options, and policy identifier used for the initial decision.

All related-resource evaluations must use this same session. Reading the active model from the provider a second time could combine an AAS decision from one policy version with a Submodel decision from another version.

Relevant components:

- `internal/common/security/authorize.go`
- `internal/common/security/abac_engine.go`

### 2. Compile effective READ views by resource scope

Add a security-layer operation that compiles the effective READ view for a semantic target such as `$aas`, `$sm`, or `$sme`. It should reuse the existing rule processing for rights, claims, objects, formulas, and fragment filters, but it must not depend only on the incoming HTTP route.

The result should be a query-filter-shaped value:

- a row formula for the requested scope;
- fragment filters for fields or child resources;
- an explicit unrestricted or denied result;
- policy and matched-rule metadata for auditing.

Existing route-only rules need a defined mapping to semantic resource scopes. A related-resource view must be rejected when a route grant cannot be translated without ambiguity. The implementation must never interpret an AAS route grant as an implicit Submodel grant.

### 3. Determine the required related scopes before persistence

The AAS query API should inspect the validated query roots:

- no `$sm` or `$sme`: keep the existing AAS-only authorization path;
- `$sm`: request the effective Submodel READ view;
- `$sme`: request both the parent Submodel READ view and effective element visibility.

When ABAC is enabled and a required view cannot be compiled, fail closed before executing SQL. Until related-scope enforcement is implemented, the safe transitional behavior is to reject hierarchy predicates under ABAC while continuing to allow them when ABAC is disabled.

Relevant components:

- `internal/aasrepository/api/api_asset_administration_shell_repository_api_service.go`
- `internal/common/model/grammar/query_field_roots.go`

### 4. Inject visibility guards into hierarchy planning

Pass the compiled related-resource views to the hierarchy query planner as typed expressions or callbacks. Keep the grammar package independent from the security package; authorization code may produce grammar expressions, while the planner only consumes generic guards.

The Submodel authorization formula must be evaluated inside the same correlated `EXISTS` as the caller's `$sm` predicate:

```sql
EXISTS (
  SELECT 1
  FROM referenced_submodel
  WHERE referenced_submodel belongs to outer_aas
    AND submodel_read_policy(referenced_submodel)
    AND caller_submodel_predicate(referenced_submodel)
)
```

For `$sme`, the Submodel guard and applicable element/fragment guard must constrain the same Submodel and SME rows as the caller predicate. Two independent `EXISTS` clauses are insufficient because one could authorize a different row from the row containing the matched protected value.

Authorization expressions must remain structured goqu expressions throughout planning. They must not be rendered and concatenated as SQL text.

Relevant components:

- `internal/common/model/grammar/logical_expression_to_sql.go`
- `internal/aasrepository/persistence`

### 5. Preserve query correlation semantics

Authorization guards must be added to every hierarchy scope produced by the planner:

- separate predicates outside `$match` may use separate authorized `EXISTS` scopes;
- predicates grouped by `$match` must share one authorized related-resource scope;
- negation must operate on the authorized view, so denied rows cannot affect its result;
- fragment filters must remain bound to the same array element or SME instance.

## Test plan

Add ABAC integration coverage with an allowed AAS referencing both allowed and denied Submodels. Verify that denied data never changes the returned AAS set for:

- equality and inequality;
- starts-with and regular-expression predicates;
- numeric comparisons and casts;
- `$match` and non-`$match` combinations;
- negated hierarchy predicates;
- an allowed Submodel with denied or filtered SMEs;
- multiple referenced Submodels where authorization and the caller predicate match different rows;
- a policy activation during a request, proving one policy snapshot is used;
- missing or untranslatable related-resource authorization, proving fail-closed behavior.

Positive cases must show that an AAS is returned only when the same referenced row is both readable and matches the caller predicate. Response bodies and status behavior must not reveal whether a denied related resource exists.

## Implementation order

1. Define and store the request authorization session.
2. Extract resource-scope READ compilation from the current route authorization flow.
3. Add hierarchy guard inputs to the collector/planner.
4. Apply Submodel and SME guards within correlated `EXISTS` scopes.
5. Add unit tests for guard placement and correlation.
6. Add the ABAC integration matrix.
7. Remove the transitional rejection only after all security tests pass.

## Acceptance criteria

- AAS authorization alone can never make denied Submodel or SME data observable through a query predicate.
- Policies can grant AAS and Submodel/SME access in separate rules.
- All related checks use the same principal, trusted globals, simplification options, and active policy snapshot.
- Missing, denied, or ambiguous related-resource authorization fails closed.
- The grammar and SQL planner remain authorization-provider agnostic.
- No policy expression is injected through raw SQL string composition.

## Specification reference

The design follows the resource separation and least-privilege intent of the [IDTA AAS Specification Part 4: Security](https://industrialdigitaltwin.org/en/wp-content/uploads/sites/2/2025/06/IDTA-01004-3-0-2_AAS-Specification_Part4_Security.pdf). The specification defines authorization targets for Identifiables and Referables and requires protected information to remain unavailable; the exact database enforcement mechanism remains an implementation concern.
