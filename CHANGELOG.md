# Changelog

All notable user-visible changes are documented here.

## Terminology

The `Type` column uses the following values:

| Type | Usage |
| --- | --- |
| High impact | Requires users to update an API integration, configuration, policy, or deployment. |
| Low impact | Adds, extends, or hardens behavior without requiring migration. |
| Bugfix | Corrects behavior that did not work as intended. |

Security consequences are documented separately in the `Security impact`
column and do not determine the change type.

## Unreleased (after v1.0.9)

These changes affect upgrades from `v1.0.9` to the next release.

| Type | Change | Pull request | Security impact |
| --- | --- | --- | --- |
| High impact | ABAC `CLAIM` resolution no longer discards all but the first value of JWT arrays. String claims remain unchanged; numbers, booleans, arrays, and nested objects are exposed to formulas as deterministic JSON text. Policies that compare non-string claims may need to match the new complete JSON representation. | — | Authorization decisions involving non-string claims can change. Missing, `null`, and unsupported claim values remain unresolved and fail closed. |
| Bugfix | ABAC rules containing only `UTCNOW`, `LOCALNOW`, or `CLIENTNOW` now require a `CLAIM`, or `GLOBAL=ANONYMOUS` for intentionally public access. | [#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573) | More restrictive: existing time-only rules no longer grant access. |
| Bugfix | `CLIENTNOW` is no longer generated or overwritten by the server and must come from the verified access token. Use `UTCNOW` or `LOCALNOW` when server time is intended. | [#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573) | More restrictive: rules requiring `CLIENTNOW` no longer match without the token claim. |
| Bugfix | `UTCNOW` and `LOCALNOW` now use the trusted server clock instead of same-named JWT claims. | [#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573) | JWT values can no longer control server-time authorization decisions. |
| Low impact | Every declared `CLAIM` is mandatory, including when combined with `GLOBAL=ANONYMOUS`; unsupported attributes fail closed. | [#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573) | More restrictive: missing claims and unsupported attributes deny access. |
| Low impact | Anonymous time-based rules can grant access when `GLOBAL=ANONYMOUS` is explicitly configured. | [#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573) | Potentially broader, but only for policies that explicitly allow anonymous access. |
| Low impact | Existing AAS and Submodel `PUT` requests now reconcile only changed persisted rows, preserve unchanged persistence identities and managed binary references, and synchronize registries only when the derived descriptor content changes. Semantic no-op replacements skip the live-model write but still record the acknowledged update when history or WORM evidence is enabled. | [#591](https://github.com/eclipse-basyx/basyx-go-components/pull/591) | Improves audit completeness for acknowledged no-op `PUT` requests; authorization semantics are unchanged. |
| Bugfix | Concurrent top-level and nested SubmodelElement `POST` requests now allocate unique sibling positions without deadlocking or timing out on a second database connection. These writes honor request cancellation, and nested inserts persist the correct tree depth. | [#599](https://github.com/eclipse-basyx/basyx-go-components/pull/599) | Authorization and information-disclosure behavior are unchanged. |
