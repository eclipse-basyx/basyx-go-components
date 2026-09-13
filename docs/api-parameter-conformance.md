# API parameter conformance

The registry, repository, Discovery, DTR, and AAS Environment boundaries use IDTA Part 2 v3.2 and the [versioned OpenAPI schemas](https://raw.githubusercontent.com/admin-shell-io/aas-specs-api/v3.2.0/Part2-API-Schemas/openapi.yaml). This change concerns identifier/filter parameters and pagination, not a claim of complete API conformance.

## Client-visible corrections

- Send nonempty UTF-8 identifiers as canonical unpadded base64url. Padding, the ordinary base64 alphabet, and whitespace in the encoded token produce HTTP 400. Identifier character limits count Unicode characters. The general binary decoder remains unchanged.
- Serialize `semanticId`, `isCaseOf`, and `dataSpecificationRef` as JSON Reference objects before base64url encoding. JSON property order and formatting whitespace do not affect matching. `semanticId` has a 3072-character encoded limit.
- Reference matching is exact: type, ordered keys (including type and value), and optional `referredSemanticId`. The semantic filter matches either the primary reference or any supplemental reference. These follow [Part 2 HTTP rules](https://industrialdigitaltwin.io/aas-specifications/IDTA-01002/v3.2/http-rest-api/http-rest-api.html) and [Part 1 default reference matching](https://industrialdigitaltwin.io/aas-specifications/IDTA-01001/v3.2/annex/general.html).
- Send each `assetIds` entry as an individually encoded SpecificAssetId using repeated query parameters: `?assetIds=<first>&assetIds=<second>`. Every name/value pair must match; name and value must belong to the same entry. Explicit empty entries and comma-packed tokens are rejected. Existing external-subject authorization remains active.
- Omit `cursor` for the first page. Explicit empty cursors are invalid (AASa-001). Echo the server-issued cursor unchanged and omit it again to restart. All selected APIs, including `QuerySubmodels`, emit unpadded base64url. Path-list cursor state is internal JSON inside that token. Old raw query cursors and old path cursor formats have no fallback.
- Explicit `limit` must be an integer from 1 through 2147483647. Omission retains the default of 100. Minimum 1 comes from the OpenAPI definition; the prose's non-negative wording conflicts with that definition. The upper bound is the existing implementation's int32 range, not an IDTA requirement.
- `paging_metadata` remains present and its cursor is omitted at the end. Endpoint-specific result shapes, DTR's optional empty result, and extension metadata remain intact.

Base64url cursor encoding and rejection of noncanonical pad bits are implementation policies; the specification permits other opaque cursor formats. These corrections are intentionally breaking, with no compatibility switch or token migration.

## Requirement-to-test map

| Behavior | Coverage |
| --- | --- |
| Strict encoding, UTF-8, Unicode length boundaries, JSON objects | `internal/common/api_parameters_test.go` |
| Empty/omitted parameters, limits, encoded filters and identifiers | Shared `RunAPIParameterConformance` matrix in all six standalone suites, DTR and AAS Environment |
| Exact references, key order/count, primary and supplemental matches, nested references, paginated results | Shared `RunSubmodelReferenceConformance`; Submodel Repository and AAS Environment |
| Query cursor round-trip and no duplicated results | `requireQueryCursorRoundTrip` in the same suites |
| Complete Concept Description references | Shared `RunConceptDescriptionReferenceConformance` and updated full-reference integration fixtures |
| All asset pairs, order independence, pair correlation before pagination | `RunAssetPairConformance`; AAS Registry, AAS Repository and AAS Environment |
| Hidden semantic references cannot determine membership | `TestAPIReferenceFilterVisibility` in the Submodel query/security integration suite |
| Immutable typed predicates | `TestReferenceSelectorSnapshotsInput` |
| Invalid wrapper writes leave repository and descriptor unchanged | `TestInvalidSubmodelIdentifierDoesNotMutateEnvironmentOrDescriptor` |
| DTR early returns, discovery continuation and custom visibility | Shared matrix plus complete DTR integration, security and Tractus-X suites |

All SQL predicates run before pagination, using GOQU and the existing authorization context. There are no schema migrations or regenerated API packages.
