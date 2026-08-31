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
| High impact | ABAC claims are no longer serialized to JSON text. `CLAIM` selects a top-level scalar or string array, while `CLAIMPATH` selects a nested value with an RFC 6901 JSON Pointer. Scalar comparisons continue to use `$eq`; exact string-array membership uses `$in` with the scalar first. Policies using $contains, $regex, or $eq with actual array or object claims must migrate. These operators continue to work with scalar string claims. Single-item arrays also remain arrays and require `$in`. | — | Eliminates substring and nested-path claim confusion. Missing, `null`, empty, object, mixed-array, and unsupported claim values fail closed. |
| Bugfix | Invalid ABAC operations remain invalid through `$not`, `$and`, `$or`, and `$match`, and cannot become an allow decision through negation. | — | More restrictive: malformed or unusable claim values can no longer activate negated rules. |
| Bugfix | ABAC rules containing only `UTCNOW`, `LOCALNOW`, or `CLIENTNOW` now require a `CLAIM`, or `GLOBAL=ANONYMOUS` for intentionally public access. | [#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573) | More restrictive: existing time-only rules no longer grant access. |
| Bugfix | `CLIENTNOW` is no longer generated or overwritten by the server and must come from the verified access token. Use `UTCNOW` or `LOCALNOW` when server time is intended. | [#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573) | More restrictive: rules requiring `CLIENTNOW` no longer match without the token claim. |
| Bugfix | `UTCNOW` and `LOCALNOW` now use the trusted server clock instead of same-named JWT claims. | [#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573) | JWT values can no longer control server-time authorization decisions. |
| Low impact | Every declared `CLAIM` is mandatory, including when combined with `GLOBAL=ANONYMOUS`; unsupported attributes fail closed. | [#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573) | More restrictive: missing claims and unsupported attributes deny access. |
| Low impact | Anonymous time-based rules can grant access when `GLOBAL=ANONYMOUS` is explicitly configured. | [#573](https://github.com/eclipse-basyx/basyx-go-components/pull/573) | Potentially broader, but only for policies that explicitly allow anonymous access. |
| Low impact | Existing AAS and Submodel `PUT` requests now reconcile only changed persisted rows, preserve unchanged persistence identities and managed binary references, and synchronize registries only when the derived descriptor content changes. Semantic no-op replacements skip the live-model write but still record the acknowledged update when history or WORM evidence is enabled. | [#591](https://github.com/eclipse-basyx/basyx-go-components/pull/591) | Improves audit completeness for acknowledged no-op `PUT` requests; authorization semantics are unchanged. |
| Bugfix | Concurrent top-level and nested SubmodelElement `POST` requests now allocate unique sibling positions without deadlocking or timing out on a second database connection. These writes honor request cancellation, and nested inserts persist the correct tree depth. | [#599](https://github.com/eclipse-basyx/basyx-go-components/pull/599) | Authorization and information-disclosure behavior are unchanged. |
