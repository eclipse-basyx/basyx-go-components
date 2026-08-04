# DTR scalability tests

This suite starts a local Keycloak and local Keycloak PostgreSQL database and builds both the BaSyx configuration service and DTR from the current repository code. The configuration service and DTR both use the external PostgreSQL database configured in `.env`, so the schema and service binaries always come from the same revision.

The default workload is read-only after configuration-service schema initialization. It obtains specific asset-link values through `GET /lookup/shells/{aasIdentifier}` and the global asset ID through `GET /shell-descriptors/{aasIdentifier}`; neither is hard-coded.

The optional async bulk workload is not read-only. Enable it only for a scalability database by setting `DTR_SCALE_ASYNC_BULK_ENABLED=true`. It creates uniquely identified temporary shell descriptors and removes the current descriptor rows after each sample. Descriptor history and audit records created by these operations are permanent, and an interrupted process can leave temporary descriptors behind. Do not enable this workload against a production database. The reference user must be allowed to create, update, delete, and read bulk operation status and results.

## Configure and sample fixtures

From the repository root:

```powershell
Copy-Item .\internal\digitaltwinregistry\scalability_tests\.env.example .\internal\digitaltwinregistry\scalability_tests\.env
notepad .\internal\digitaltwinregistry\scalability_tests\.env
go run .\scripts\dtr_scalability_fixtures\main.go -env-file .\internal\digitaltwinregistry\scalability_tests\.env -count 5
```

The fixture script samples shell/submodel pairs with indexed primary-key range probes. A selected shell must have a global asset ID and both configured generic asset-ID names, so every fixture can exercise specific- and global-asset lookup requests. It does not use `ORDER BY random()` and makes no changes to PostgreSQL.

## Run the suite

```powershell
go test -v .\internal\digitaltwinregistry\scalability_tests
```

Each regular request has a strict 10-second context timeout. Configure `DTR_SCALE_REQUEST_REPETITIONS` and `DTR_SCALE_REQUEST_CONCURRENCY` in `.env` to control load. The test logs the returned status distribution, total response-body bytes, average response-body bytes, p50, p95, and maximum duration for each endpoint, fixture, and user.

Async bulk scenarios use `DTR_SCALE_ASYNC_BULK_TIMEOUT_SECONDS` as the lifecycle timeout and `DTR_SCALE_ASYNC_BULK_POLL_INTERVAL_MILLISECONDS` between running-status polls. `DTR_SCALE_ASYNC_BULK_SIZE` controls the number of descriptors in each atomic operation. Each measured sample starts with the mutation request and ends after the terminal result has been retrieved; preparation and cleanup are excluded. The response-byte count includes the initial `202`, all status responses, and the result response.

The Compose setup reuses the DTR integration-test Keycloak realm and access rules, with a rewritten local issuer URL. `DTR_SCALE_KEYCLOAK_USERS` therefore defaults to `admin`, `usera`, `userb`, `no_bpn_viewer`, `userx`, and `usery`. `DTR_SCALE_INCLUDE_ANONYMOUS` defaults to `true`, adding an unauthenticated `anonymous` workload user that sends no Authorization header. For the regular read scenarios, all HTTP responses below 500, including permission-filtered `403` and `404` responses, are reported as scalability results. Async bulk scenarios require a successful terminal `204`. A request timeout, transport error, or HTTP 5xx response fails the test.

## Result document

Every run writes a timestamped Markdown document to `results/`, for example `results/scalability-20260729T114535.123456789Z-32232.md`. The directory is ignored by Git. A result document contains the run timestamps and outcome, workload configuration, fixture AAS/submodel IDs, and one row per fixture/user/scenario with HTTP status counts, p50, p95, and maximum duration. Async bulk rows use `-` in the fixture column because that workload does not depend on sampled records. The report is also written when Compose startup fails, so the developer has a dated record of unsuccessful runs.

## Exact request sequence and data used

`.env.example` contains five concrete sample fixtures. Regenerate fixtures from the same PostgreSQL database that DTR uses before relying on them for a different target database. For each generated fixture `n`, the sampler writes these exact values into `.env`:

```text
DTR_SCALE_AAS_ID_n=<aas.id selected from PostgreSQL>
DTR_SCALE_SUBMODEL_ID_n=<submodel.id belonging to that AAS>
DTR_SCALE_P_n=<Base64URL JSON asset link used for P>
DTR_SCALE_Q_n=<Base64URL JSON asset link used for Q>
DTR_SCALE_G_n=<Base64URL JSON asset link used for G>
DTR_SCALE_PRIMARY_ASSET_ID_VALUE_n=<unencoded value used for P>
DTR_SCALE_SECONDARY_ASSET_ID_VALUE_n=<unencoded value used for Q>
DTR_SCALE_GLOBAL_ASSET_ID_n=<unencoded value used for G>
```

`P`, `Q`, and `G` are the sampled fallback values. Before the workload, the test attempts to refresh them from DTR using the reference user. If either bootstrap request returns a status below 500 other than `200`, the fallback value is used and the workload continues.

The selected AAS has a non-empty `globalAssetId`, a `manufacturerPartId`, a `customerPartId`, and a submodel descriptor:

| Symbol | Exact value used |
| --- | --- |
| `A` | Base64URL encoding without padding of `DTR_SCALE_AAS_ID_n` |
| `S` | Base64URL encoding without padding of `DTR_SCALE_SUBMODEL_ID_n` |
| `P` | Base64URL encoding without padding of the lookup link named `DTR_SCALE_ASSET_ID_NAME` (default: `manufacturerPartId`), or the sampled `DTR_SCALE_P_n` fallback |
| `Q` | Base64URL encoding without padding of the lookup link named `DTR_SCALE_SECOND_ASSET_ID_NAME` (default: `customerPartId`), or the sampled `DTR_SCALE_Q_n` fallback |
| `G` | Base64URL encoding without padding of `{"name":"globalAssetId","value":"<globalAssetId returned by DTR>"}`, or the sampled `DTR_SCALE_G_n` fallback |
| `L` | `DTR_SCALE_PAGE_LIMIT` (default: `50`) |

`P` and `Q` are Base64URL encodings of JSON such as:

```json
{"name":"manufacturerPartId","value":"<value returned by DTR lookup>"}
```

Before the workload starts for a fixture, the reference user (`DTR_SCALE_KEYCLOAK_REFERENCE_USER`, default `admin`) performs these requests in order:

```text
GET /api/v3/lookup/shells/{A}
GET /api/v3/shell-descriptors/{A}
```

The first `200` response supplies `P` and `Q`; the second `200` response supplies the `globalAssetId` used to create `G`. Both requests include `Authorization: Bearer <reference-user-token>`. A `403` or `404` uses the sampled fallback and does not abort the workload. A `5xx`, timeout, or transport error fails the test.

The following logical scenario sequence is then executed for every configured user and fixture. Authenticated users include `Authorization: Bearer <that-user-token>`; the `anonymous` user sends no Authorization header. POST requests include `Content-Type: application/json`.

```text
 1. GET  /api/v3/shell-descriptors?limit={L}
 2. GET  /api/v3/shell-descriptors/{A}
 3. GET  /api/v3/lookup/shells/{A}

 4. GET  /api/v3/shell-descriptors?limit={L}&assetIds={P}
 5. GET  /api/v3/shell-descriptors?limit={L}&assetIds={P}&assetIds={Q}
 6. GET  /api/v3/shell-descriptors?limit={L}&assetIds={G}
 7. GET  /api/v3/shell-descriptors?limit={L}&assetIds={G}&assetIds={P}

 8. GET  /api/v3/lookup/shells?limit={L}&assetIds={P}
 9. GET  /api/v3/lookup/shells?limit={L}&assetIds={P}&assetIds={Q}
10. GET  /api/v3/lookup/shells?limit={L}&assetIds={G}
11. GET  /api/v3/lookup/shells?limit={L}&assetIds={G}&assetIds={P}

12. POST /api/v3/lookup/shellsByAssetLink?limit={L}
    Body: [{"name":"manufacturerPartId","value":"<value used in P>"}]

13. POST /api/v3/lookup/shellsByAssetLink?limit={L}
    Body: [{"name":"globalAssetId","value":"<value used in G>"}]

14. GET  /api/v3/shell-descriptors/{A}/submodel-descriptors?limit={L}
15. GET  /api/v3/shell-descriptors/{A}/submodel-descriptors/{S}
```

Each scenario is repeated `DTR_SCALE_REQUEST_REPETITIONS` times with at most `DTR_SCALE_REQUEST_CONCURRENCY` simultaneous requests. Therefore, the scenarios above start in the listed order, but requests repeated within one scenario do not have a deterministic network order.

When `DTR_SCALE_ASYNC_BULK_ENABLED=true`, three additional scenarios run once per configured repetition for the reference user. Their batch size is `DTR_SCALE_ASYNC_BULK_SIZE`:

```text
16. POST /api/v3/bulk/shell-descriptors
    Poll GET /api/v3/bulk/status/{handleId} until 302.
    GET /api/v3/bulk/result/{handleId}
    Remove the temporary descriptors outside the measured interval.

17. Create temporary descriptors outside the measured interval.
    PUT /api/v3/bulk/shell-descriptors with updated versions
    Poll GET /api/v3/bulk/status/{handleId} until 302.
    GET /api/v3/bulk/result/{handleId}
    Remove the temporary descriptors outside the measured interval.

18. Create temporary descriptors outside the measured interval.
    DELETE /api/v3/bulk/shell-descriptors with their identifiers
    Poll GET /api/v3/bulk/status/{handleId} until 302.
    GET /api/v3/bulk/result/{handleId}
```

All descriptors use IDs beginning with `urn:basyx:dtr-scalability:async-bulk:`. Successful lifecycle scenarios report the terminal result status, normally `204`. Setup or cleanup failures fail the suite.

## Covered endpoints

- `GET /shell-descriptors?limit=...`
- `GET /shell-descriptors/{aasIdentifier}`
- `GET /lookup/shells/{aasIdentifier}`
- `GET /shell-descriptors?assetIds=...` with one, two, global, and global-plus-specific links
- `GET /lookup/shells?assetIds=...` with one, two, global, and global-plus-specific links
- `POST /lookup/shellsByAssetLink` with a specific and a global link
- `GET /shell-descriptors/{aasIdentifier}/submodel-descriptors?limit=...`
- `GET /shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}`
- `POST /bulk/shell-descriptors` with temporary descriptors
- `PUT /bulk/shell-descriptors` with temporary descriptors
- `DELETE /bulk/shell-descriptors` with temporary descriptor IDs
- `GET /bulk/status/{handleId}`
- `GET /bulk/result/{handleId}`

The local Keycloak database is destroyed when the test ends. The external database is never dropped or reset, but the configuration service can apply schema migrations. Use a dedicated scalability database with an appropriate backup and an account that is permitted to run the BaSyx schema configuration service and, when enabled, the async bulk mutations.
