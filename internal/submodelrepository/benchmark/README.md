# Submodel Evidence Performance Benchmark

This manual benchmark measures client-visible Submodel mutation and attachment latency against PostgreSQL and MinIO Object Lock. It compares real evidence-disabled and evidence-enabled requests; it is not a CI timing gate.

Install Python 3, then run both profiles from this directory. The benchmark uses only the Python standard library. Each run starts with an empty database so the results are comparable.

```sh
docker compose down -v --remove-orphans
BASYX_BENCH_EVIDENCE_ENABLED=false \
  BASYX_BENCH_FULL_SNAPSHOT_INTERVAL=5 \
  docker compose up -d --build

python3 post_submodel.py \
  --label evidence-off-interval-5 \
  --snapshot-interval 5 \
  --output evidence-off.json

docker compose down -v --remove-orphans
BASYX_BENCH_EVIDENCE_ENABLED=true \
  BASYX_BENCH_FULL_SNAPSHOT_INTERVAL=5 \
  docker compose up -d --build

python3 post_submodel.py \
  --label evidence-on-interval-5 \
  --evidence-enabled \
  --snapshot-interval 5 \
  --output evidence-on.json

docker compose down -v --remove-orphans
```

Use `--snapshot-interval 1` with the matching Compose environment variable to measure all-snapshot evidence. The defaults create and update Submodels containing 100, 1,000, and 10,000 Properties, then upload 1 MiB and 32 MiB payloads twice. `--concurrency 8` additionally measures independent model updates and identical uploads to separate owners.

The JSON report includes attempts, successful requests, failures, payload size, p50, p95, and maximum latency. New-content and identical-repeat attachment uploads are reported separately. Attachment scenarios verify managed-path rotation and downloaded bytes before reporting success. Compare results produced on the same host and object-storage topology; shared-runner timing is not stable enough for pass/fail thresholds.
