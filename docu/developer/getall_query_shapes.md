# getAll query shapes and cluster benchmark handoff

This document records the database statement and round-trip budgets for issue
[#567](https://github.com/eclipse-basyx/basyx-go-components/issues/567). Counts
refer to successful ordinary reads. Restrictive reference reconstruction is
listed separately because it is request-dependent but remains page-wide and
bounded.

## Statement budgets

| Service and representation | Baseline | Final | Final database round trips |
|---|---:|---:|---:|
| Submodel Repository, full | `1 + 3N` primary reads | 2 primary reads; up to 6 fixed bulk reference fallbacks | 2 to 8 |
| Submodel Repository, value-only | `1 + 3N` primary reads | 2 primary reads; up to 6 fixed bulk reference fallbacks | 2 to 8 |
| Submodel Repository, query | `1 + 3N` primary reads | 2 primary reads; up to 6 fixed bulk reference fallbacks | 2 to 8 |
| Submodel Repository, metadata and recent changes | 1 primary read; up to 2 fixed restrictive-reference reads | unchanged | 1 to 3 |
| Submodel Repository, reference | metadata materialization plus optional reference reads | 1 | 1 |
| Submodel Repository, path | Submodel reference pages plus 2 or 3 reads for every visited Submodel | 1 | 1 |
| AAS Repository, full, final page | page, core, Submodel references, Specific Asset IDs and their two reference reads | 4 | pgx: 2; `database/sql`: 4 |
| AAS Repository, full, saturated page | final-page reads plus cursor lookup | 5 | pgx: 2; `database/sql`: 5 |
| AAS Repository, reference | complete AAS materialization | 1 | 1 |
| Submodel Registry, global Submodel Descriptors | cursor check, ID page, base row and four child lookups | 1 | 1 |
| AAS Registry, nested Submodel Descriptors | parent lookup, unpaged base row and four child lookups | 2 | 2 |
| AAS Registry, AAS Descriptors | 1 | unchanged: 1 | 1 |
| Concept Description Repository, full and recent changes | 1 | unchanged: 1 | 1 |

`N` is the number of Submodels returned on the page. The final shapes use array
parameters, so the rendered SQL is identical for limits 1 and 100. The pgx AAS
path sends core rows, Submodel references, the combined Specific Asset ID
materializer, and the optional cursor lookup in queue order. The non-pgx path
uses the same builders, scanners, ordering, and model assembler sequentially.

The Submodel full and value-only representations intentionally retain the
existing maximum of 100 visible top-level SMEs per returned Submodel. That
possible completeness issue is outside #567.

## Three-replica cluster benchmark matrix

Run every row at limits 1 and 100 against three service replicas. Use the same
dataset, security configuration, filters, client concurrency, warm-up, and
measurement interval for baseline and candidate builds.

| Service | Representation | Limit | Status | Bytes | p50 | p95 | RPS | PostgreSQL calls | PostgreSQL time | Pod CPU | Pod memory | Pool waits |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Submodel Repository | full, core/deep, with/without blob values | 1 / 100 |  |  |  |  |  |  |  |  |  |  |
| Submodel Repository | metadata | 1 / 100 |  |  |  |  |  |  |  |  |  |  |
| Submodel Repository | value-only, core/deep, with/without blob values | 1 / 100 |  |  |  |  |  |  |  |  |  |  |
| Submodel Repository | reference | 1 / 100 |  |  |  |  |  |  |  |  |  |  |
| Submodel Repository | path, core/deep | 1 / 100 |  |  |  |  |  |  |  |  |  |  |
| AAS Repository | full | 1 / 100 |  |  |  |  |  |  |  |  |  |  |
| AAS Repository | reference | 1 / 100 |  |  |  |  |  |  |  |  |  |  |
| Submodel Registry | global descriptor | 1 / 100 |  |  |  |  |  |  |  |  |  |  |
| AAS Registry | nested Submodel Descriptor | 1 / 100 |  |  |  |  |  |  |  |  |  |  |
| AAS Registry | AAS Descriptor | 1 / 100 |  |  |  |  |  |  |  |  |  |  |
| Concept Description Repository | full | 1 / 100 |  |  |  |  |  |  |  |  |  |  |

Repeat the matrix with authorization disabled and enabled, then with identifier,
semantic, timestamp, query, and fragment-mask filters. Include empty, saturated,
and final pages. Treat a changed status, response byte count, cursor sequence, or
element ordering as a compatibility failure before comparing throughput.
