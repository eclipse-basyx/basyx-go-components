#!/bin/bash
# Benchmark runner script for common scenarios

BASEURL="${BASEURL:-http://127.0.0.1:5004}"
THREADS="${THREADS:-10}"
COUNT="${COUNT:-1000}"
IDPREFIX="${IDPREFIX:-TDSM2%d}"

echo "==================================================="
echo "Submodel Repository Benchmark Runner"
echo "==================================================="
echo "Base URL:   $BASEURL"
echo "Threads:    $THREADS"
echo "Count:      $COUNT"
echo "ID Prefix:  $IDPREFIX"
echo "==================================================="
echo ""

# Check if service is running
echo "Checking if service is running at $BASEURL/health..."
if ! curl -s -f "$BASEURL/health" > /dev/null 2>&1; then
    echo "WARNING: Service is not responding at $BASEURL"
    echo "Please ensure the service is started:"
    echo "  go run ./cmd/submodelrepositoryservice -config ./cmd/submodelrepositoryservice/config.yaml"
    echo ""
    echo "Continuing anyway (benchmark may fail)..."
else
    echo "✓ Service is running"
fi
echo ""

# Track exit codes
EXIT_CODE=0

# Run benchmark based on argument
case "${1:-}" in
    post)
        echo "Running POST benchmark..."
        go test -v -run TestBenchmarkSubmodelRepo \
            -operation=post \
            -baseurl="$BASEURL" \
            -threads="$THREADS" \
            -count="$COUNT" \
            -idprefix="$IDPREFIX" || EXIT_CODE=$?
        ;;
    get)
        echo "Running GET benchmark..."
        go test -v -run TestBenchmarkSubmodelRepo \
            -operation=get \
            -baseurl="$BASEURL" \
            -threads="$THREADS" \
            -count="$COUNT" \
            -idprefix="$IDPREFIX" || EXIT_CODE=$?
        ;;
    both)
        echo "Running POST benchmark..."
        go test -v -run TestBenchmarkSubmodelRepo \
            -operation=post \
            -baseurl="$BASEURL" \
            -threads="$THREADS" \
            -count="$COUNT" \
            -idprefix="$IDPREFIX" || POST_EXIT=$?
        
        echo ""
        if [ "${POST_EXIT:-0}" -ne 0 ]; then
            echo "WARNING: POST benchmark failed with exit code $POST_EXIT"
        fi
        
        echo "Waiting 2 seconds..."
        sleep 2
        
        echo ""
        echo "Running GET benchmark..."
        go test -v -run TestBenchmarkSubmodelRepo \
            -operation=get \
            -baseurl="$BASEURL" \
            -threads="$THREADS" \
            -count="$COUNT" \
            -idprefix="$IDPREFIX" || GET_EXIT=$?
        
        if [ "${GET_EXIT:-0}" -ne 0 ]; then
            echo "WARNING: GET benchmark failed with exit code $GET_EXIT"
        fi
        
        # Set exit code if either failed
        if [ "${POST_EXIT:-0}" -ne 0 ] || [ "${GET_EXIT:-0}" -ne 0 ]; then
            EXIT_CODE=1
        fi
        ;;
    *)
        echo "Usage: $0 {post|get|both}"
        echo ""
        echo "Environment variables:"
        echo "  BASEURL    - Base URL (default: http://127.0.0.1:5004)"
        echo "  THREADS    - Number of threads (default: 10)"
        echo "  COUNT      - Number of operations (default: 1000)"
        echo "  IDPREFIX   - ID prefix template (default: submodelID_%d)"
        echo ""
        echo "Examples:"
        echo "  $0 post"
        echo "  THREADS=20 COUNT=5000 $0 both"
        echo "  IDPREFIX='benchmark_%05d' $0 post"
        exit 1
        ;;
esac

echo ""
echo "==================================================="
if [ $EXIT_CODE -eq 0 ]; then
    echo "Benchmark completed successfully"
else
    echo "Benchmark completed with errors (exit code: $EXIT_CODE)"
fi
echo "==================================================="

exit $EXIT_CODE
