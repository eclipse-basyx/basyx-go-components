# PostgreSQL Maintenance for BaSyx Repositories

BaSyx uses PostgreSQL for repository data. Updates and deletes leave obsolete
row versions until vacuum can reclaim them. On large Submodel repositories,
waiting for the server-wide autovacuum threshold can also leave obsolete index
entries that make otherwise small reads expensive to plan. Keep autovacuum
enabled, monitor its progress, and size its workers for the write rate of the
deployment. This guide applies to PostgreSQL 16 and newer.

## Repository Table Policy

The BaSyx Configuration Service's v1.2.2 migration sets
`autovacuum_vacuum_scale_factor = 0.002` and `vacuum_index_cleanup = on` on
these high-churn Submodel Repository tables during schema initialization or
upgrade:

- `submodel_element`
- `submodel_element_payload`
- `submodel_element_semantic_id_reference_payload`

The migration skips all three tables when
`pg_settings.source` for `autovacuum_vacuum_scale_factor` is not `default`.
PostgreSQL accepts this server-wide setting from `postgresql.conf` or the
server command line, including configuration managed through `ALTER SYSTEM`.
Otherwise, it checks each table and its TOAST relation. If either has any
explicit `autovacuum_*` or `vacuum_*` storage option, the migration leaves
that table unchanged. It sets both options only on tables without any such
option. Check `pg_settings.source` for `autovacuum_vacuum_scale_factor` and the
`reloptions` query below to see which policy applies. The migration does not
vacuum immediately or rewrite data. It takes a `SHARE UPDATE EXCLUSIVE` lock
while checking each table, so it can wait for another maintenance operation
even when it preserves an existing table policy. PostgreSQL applies these
table settings to its TOAST table unless a separate TOAST option is set.

To inspect the effective server-wide policy, connect to the writer and run:

```sql
SELECT name, setting, source
FROM pg_settings
WHERE name IN (
    'autovacuum',
    'autovacuum_max_workers',
    'autovacuum_vacuum_scale_factor',
    'autovacuum_vacuum_threshold'
)
ORDER BY name;
```

The 0.2% trigger starts cleanup sooner on large tables. Forcing index cleanup
prevents PostgreSQL's default `AUTO` policy from skipping an index scan when it
judges that too few dead tuples are present. This can increase vacuum I/O and
WAL, so review both settings against the deployment's write volume, worker
capacity, storage throughput, and replica lag. Operators can set table storage
parameters explicitly to tune a specific installation. Record such overrides
in deployment configuration so they can be reviewed after upgrades.

The policy was validated on a PostgreSQL 18 cluster with one million DPPs and
about 132,000 temporary write fixtures. Two fresh automatic cleanup cycles
kept sampled high-end index probes below 5,000 buffer accesses, root planning
below 100 ms, and representative AAS, Submodel, and DPP reads within the
trial's regression limits. The full trial, including its manual baseline
cleanup and write workload, generated about 19 GiB of WAL. These figures are
workload-specific and should be used to size monitoring, not as universal
performance guarantees.

Autovacuum requires the server-level `autovacuum` setting to be on. Its workers
also share server-wide limits such as `autovacuum_max_workers`,
`autovacuum_work_mem`, and vacuum cost settings. A lower per-table trigger
cannot compensate for workers that are disabled or continuously occupied.

## Monitor Cleanup and Latency

Run the following read-only query on the writer at regular intervals. It shows
estimated live and obsolete rows, the last automatic maintenance times, counts
since statistics were reset, table size including indexes, and explicit table
options. The row estimates are approximate; use trends across samples rather
than treating one sample as an exact threshold test.

```sql
SELECT s.schemaname,
       s.relname,
       s.n_live_tup,
       s.n_dead_tup,
       s.last_autovacuum,
       s.autovacuum_count,
       s.last_autoanalyze,
       s.autoanalyze_count,
       pg_size_pretty(pg_total_relation_size(s.relid)) AS total_size,
       c.reloptions,
       toast.oid::regclass AS toast_table,
       toast.reloptions AS toast_reloptions
FROM pg_stat_user_tables AS s
JOIN pg_class AS c ON c.oid = s.relid
LEFT JOIN pg_class AS toast ON toast.oid = c.reltoastrelid
WHERE s.relname IN (
    'submodel_element',
    'submodel_element_payload',
    'submodel_element_semantic_id_reference_payload'
)
ORDER BY s.schemaname, s.relname;
```

Check active work separately, including TOAST maintenance. An empty result
means no vacuum is active at that instant, not that autovacuum is disabled.

```sql
SELECT p.datname,
       p.relid::regclass AS table_name,
       p.phase,
       p.heap_blks_scanned,
       p.heap_blks_total,
       p.heap_blks_vacuumed,
       p.index_vacuum_count
FROM pg_stat_progress_vacuum AS p
ORDER BY p.datname, p.relid;
```

Track the p50 and p95 latency and request rate of representative Submodel GET,
PATCH, and DELETE operations through your ingress metrics or BaSyx request
logs. BaSyx `HTTP request completed` records include the route, status and
`duration_ms`; optional OpenTelemetry traces provide per-request timing.
Correlate latency changes with obsolete-row trends, vacuum cycles, database
CPU, disk throughput and free space. On replicated setups, watch WAL volume
and standby replay lag. Frequent vacuum work that keeps obsolete rows low but
saturates CPU or storage, or causes sustained replica lag, calls for capacity
or worker-cost tuning.

```sql
SELECT application_name,
       state,
       pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS replay_lag_bytes
FROM pg_stat_replication;
```

The replication query runs on the primary and shows no rows without connected
standbys. Byte lag and time lag are different; use your cluster's replication
metrics for alerting. For alert thresholds, establish a baseline under normal
load and verify that repeated write cycles are followed by automatic cleanup
without sustained API latency or resource pressure.

## Override and Recovery

Inspect the current per-table options before changing them. An explicit
operator override uses PostgreSQL's `ALTER TABLE ... SET` storage parameters,
for example `autovacuum_vacuum_scale_factor`, `vacuum_index_cleanup`, and
`autovacuum_vacuum_threshold`. Apply a tested value to each affected table
that needs a different policy; do not copy a scale factor from a different
workload without checking its maintenance cost.

`ALTER TABLE ... RESET (autovacuum_vacuum_scale_factor, vacuum_index_cleanup)`
removes the per-table values and inherits the server settings. It does
**not** reinstate the BaSyx policy after an override. To return a table to that
policy, explicitly set `autovacuum_vacuum_scale_factor = 0.002` and
`vacuum_index_cleanup = on` on that table. After a change, confirm
the resulting `reloptions` with the query above and observe at least several
write and automatic vacuum cycles.

If obsolete rows accumulate and API latency rises, first check that
autovacuum is enabled, workers are available, no long-running transaction is
holding back cleanup, storage has headroom, and replicas can absorb the WAL.
Once the cause is understood, ordinary manual maintenance can be performed
on the writer during an appropriate operations window:

```sql
VACUUM (ANALYZE, INDEX_CLEANUP ON, TRUNCATE OFF) submodel_element;
VACUUM (ANALYZE, INDEX_CLEANUP ON, TRUNCATE OFF) submodel_element_payload;
VACUUM (ANALYZE, INDEX_CLEANUP ON, TRUNCATE OFF)
    submodel_element_semantic_id_reference_payload;
```

Run these commands outside a transaction and monitor progress, free space,
CPU, WAL and replica lag. Ordinary vacuum does not rewrite a whole table;
`VACUUM FULL` does and requires an exclusive lock, so it is not a routine
response to this condition. Afterward, compare the same API operations and
maintenance counters, then adjust the automatic policy or resources to avoid
repeating manual intervention.

See the PostgreSQL documentation for [routine vacuuming](https://www.postgresql.org/docs/16/routine-vacuuming.html),
[statistics views](https://www.postgresql.org/docs/16/monitoring-stats.html),
and [vacuum progress](https://www.postgresql.org/docs/16/progress-reporting.html).
