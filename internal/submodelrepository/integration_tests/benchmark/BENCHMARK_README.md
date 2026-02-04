# Submodel Repository Benchmark

This benchmark tests the performance of POST or GET operations for the Submodel Repository service running on native performance (without Docker overhead).

## Overview

The benchmark can be configured to run either:
- **POST operations**: Creates new submodel instances with sequential IDs
- **GET operations**: Retrieves submodels by ID

All parameters are fully configurable via command-line flags.

## Prerequisites

- Go 1.21 or later
- Running Submodel Repository service (start manually on configured port)
- Python 3.8+ with matplotlib and numpy (for chart generation - optional)

## Running the Benchmark

### 1. Start the Service

Start your Submodel Repository service manually on the desired port:

```bash
# Example: Start service on default port 5004
go run ./cmd/submodelrepositoryservice -config ./cmd/submodelrepositoryservice/config.yaml
```

### 2. Execute the Benchmark

Run the benchmark with configurable parameters:

```bash
# Basic POST benchmark (default: 10 threads, 1000 submodels)
go test -v -run TestBenchmarkSubmodelRepo

# POST benchmark with custom parameters
go test -v -run TestBenchmarkSubmodelRepo \
  -operation=post \
  -threads=20 \
  -count=5000 \
  -baseurl=http://localhost:5004 \
  -idprefix="benchmark_sm_%d" \
  -logfailures=true

# GET benchmark (retrieve previously created submodels)
go test -v -run TestBenchmarkSubmodelRepo \
  -operation=get \
  -threads=50 \
  -count=5000 \
  -baseurl=http://localhost:5004 \
  -idprefix="benchmark_sm_%d"

# High-load benchmark
go test -v -run TestBenchmarkSubmodelRepo \
  -operation=post \
  -threads=100 \
  -count=10000 \
  -logfailures=false
```

### 3. Configuration Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-operation` | `post` | Operation type: `post` or `get` |
| `-baseurl` | `http://127.0.0.1:5004` | Base URL of the service |
| `-threads` | `10` | Number of concurrent threads |
| `-count` | `1000` | Total number of submodels to process |
| `-idprefix` | `submodelID_%d` | ID template (%d = auto-increment 0 to count-1) |
| `-logfailures` | `true` | Log failed requests with error details |

### 4. ID Prefix Examples

The `-idprefix` flag supports `%d` for auto-incrementing numbers:

```bash
# Simple numeric IDs: 0, 1, 2, ...
-idprefix="%d"

# Prefixed IDs: submodel_0, submodel_1, ...
-idprefix="submodel_%d"

# Complex IDs: benchmark_sm_00001, benchmark_sm_00002, ...
-idprefix="benchmark_sm_%05d"

# URL-style IDs: https://example.org/sm/0, https://example.org/sm/1, ...
-idprefix="https://example.org/sm/%d"
```

## Understanding the Results

The benchmark outputs comprehensive statistics:

```
====================================================================================
BENCHMARK RESULTS
====================================================================================
Operation:           POST
Total Requests:      1000
Successful:          998
Failed:              2
Success Rate:        99.80%
Total Time:          5.234s
Average Latency:     52.340 µs
Throughput:          191.07 ops/sec
====================================================================================
```

### Key Metrics

- **Total Requests**: Number of operations executed
- **Successful**: Successfully completed operations
- **Failed**: Failed operations (check logs for details)
- **Success Rate**: Percentage of successful operations
- **Total Time**: Total execution time
- **Average Latency**: Mean time per operation
- **Throughput**: Operations per second

### Failure Logging

When `-logfailures=true` (default), failed requests are logged with details:

```
[FAILED] POST #42 | ID: submodelID_42 | Status: 409 | Duration: 15.234ms
[FAILED] GET #123 | ID: submodelID_123 | Error: connection refused | Duration: 2.001s
```

## Typical Usage Workflows

### 1. Test POST Performance

```bash
# First, clean the database
# Then start the service
go run ./cmd/submodelrepositoryservice -config ./cmd/submodelrepositoryservice/config.yaml

# In another terminal, run POST benchmark
cd internal/submodelrepository/integration_tests/benchmark
go test -v -run TestBenchmarkSubmodelRepo \
  -operation=post \
  -threads=20 \
  -count=10000 \
  -idprefix="perf_test_%d"
```

### 2. Test GET Performance

```bash
# After running POST benchmark, test GET with same IDs
go test -v -run TestBenchmarkSubmodelRepo \
  -operation=get \
  -threads=50 \
  -count=10000 \
  -idprefix="perf_test_%d"
```

### 3. Stress Testing

```bash
# High concurrency stress test
go test -v -run TestBenchmarkSubmodelRepo \
  -operation=post \
  -threads=200 \
  -count=50000 \
  -logfailures=false
```

### 4. Latency Profiling

```bash
# Low concurrency for accurate latency measurement
go test -v -run TestBenchmarkSubmodelRepo \
  -operation=post \
  -threads=1 \
  -count=1000
```

## Performance Expectations

### Good Performance Indicators

- **POST**: Throughput > 500 ops/sec, Success Rate > 99%
- **GET**: Throughput > 2000 ops/sec, Success Rate > 99%
- **Latency**: Average < 50ms under moderate load

### Warning Signs

- Success Rate < 95%: Service instability or overload
- Increasing failures over time: Memory leak or resource exhaustion
- High latency variance: Database performance issues

## Troubleshooting

### Service Not Responding

```bash
# Check if service is running
curl http://localhost:5004/health

# Check service logs
# Look for errors or resource exhaustion
```

### Connection Refused

- Verify the service is running
- Check the port matches `-baseurl` flag
- Ensure firewall allows connections

### High Failure Rate

- Reduce `-threads` to decrease load
- Check service logs for errors
- Verify database is not overloaded
- Increase service resource limits

### ID Conflicts (409 Status)

- When running multiple POST benchmarks, use different `-idprefix` values
- Or clean the database between runs

## Best Practices

1. **Baseline Testing**: Run with low thread count first (1-5 threads)
2. **Incremental Load**: Gradually increase threads to find limits
3. **Separate Concerns**: Test POST and GET separately
4. **Clean State**: Reset database between major test runs
5. **Consistent IDs**: Use same `-idprefix` for POST/GET pairs
6. **Monitor Resources**: Watch CPU, memory, and disk I/O during tests
7. **Disable Logging**: Use `-logfailures=false` for high-volume tests

## Comparing Results

To compare different configurations:

```bash
# Test 1: Low concurrency
go test -v -run TestBenchmarkSubmodelRepo -threads=10 -count=1000 2>&1 | tee results_10threads.txt

# Test 2: High concurrency  
go test -v -run TestBenchmarkSubmodelRepo -threads=50 -count=1000 2>&1 | tee results_50threads.txt

# Test 3: Very high concurrency
go test -v -run TestBenchmarkSubmodelRepo -threads=100 -count=1000 2>&1 | tee results_100threads.txt

# Compare throughput trends
grep "Throughput" results_*.txt
```

## Example Output

```bash
$ go test -v -run TestBenchmarkSubmodelRepo -threads=20 -count=5000

2026/02/04 10:15:32 Starting post benchmark with 20 threads, 5000 submodels, baseURL=http://127.0.0.1:5004, idPrefix=submodelID_%d
====================================================================================
BENCHMARK RESULTS
====================================================================================
Operation:           POST
Total Requests:      5000
Successful:          4998
Failed:              2
Success Rate:        99.96%
Total Time:          12.456s
Average Latency:     49.824 µs
Throughput:          401.42 ops/sec
====================================================================================
PASS
ok      github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/integration_tests/benchmark   12.567s
```

## Notes

- The benchmark no longer uses Docker Compose - you must start the service manually
- Chart generation scripts are not updated for the new format (can be enhanced if needed)
- For production testing, ensure the database is properly tuned
- Use `-logfailures=false` for very high-volume tests to reduce overhead
