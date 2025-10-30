# Builder Unit Tests

This directory contains comprehensive unit tests for the builder packages used in the BaSyx Go Components project.

## Test Files

### 1. `reference_builder_test.go`
Tests for the `ReferenceBuilder` which constructs Reference objects with nested ReferredSemanticId structures.

**Test Coverage:**
- Creating new reference builders
- Adding keys to references
- Duplicate key prevention
- Setting and creating referred semantic IDs
- Building nested reference hierarchies (2-3 levels deep)
- Error handling for missing parent references
- Complete reference hierarchy integration tests

### 2. `sql_to_go_builder_test.go`
Tests for SQL-to-Go parsing functions that convert database JSON results into Go structures.

**Test Coverage:**
- Parsing references from JSON (empty, single, multiple)
- Handling multiple keys per reference
- Parsing referred references with hierarchies
- Null key handling
- Multi-level reference hierarchies
- Missing parent reference error cases
- Parsing language strings (LangStringNameType, LangStringTextType)
- Complete integration tests simulating real database query results

### 3. `embedded_data_specification_builder_test.go`
Tests for the `EmbeddedDataSpecificationsBuilder` which constructs EmbeddedDataSpecification objects with their references.

**Test Coverage:**
- Creating new builders
- Building single and multiple EDS
- Handling multiple keys per EDS reference
- EDS with referred references (nested hierarchies)
- Empty JSON handling
- Invalid JSON error handling
- Validation that each EDS has exactly one root reference
- Complex integration tests with multiple EDS and multi-level hierarchies

## Running the Tests

### Run All Tests
```bash
cd /Users/fried/Documents/Projekte/BaSyx_Kern/basyx-go-components
go test ./internal/common/builder/unit_test/...
```

### Run Tests with Verbose Output
```bash
go test -v ./internal/common/builder/unit_test/...
```

### Run a Specific Test File
```bash
go test ./internal/common/builder/unit_test/reference_builder_test.go
go test ./internal/common/builder/unit_test/sql_to_go_builder_test.go
go test ./internal/common/builder/unit_test/embedded_data_specification_builder_test.go
```

### Run a Specific Test Function
```bash
go test -v ./internal/common/builder/unit_test/... -run TestNewReferenceBuilder
go test -v ./internal/common/builder/unit_test/... -run TestParseReferences
go test -v ./internal/common/builder/unit_test/... -run TestEmbeddedDataSpecificationsBuilder
```

### Run Tests with Coverage
```bash
go test -cover ./internal/common/builder/unit_test/...
```

### Generate Coverage Report
```bash
go test -coverprofile=coverage.out ./internal/common/builder/unit_test/...
go tool cover -html=coverage.out
```

## Test Patterns and Best Practices

### 1. Shared Builder Maps
Tests demonstrate the importance of using shared `referenceBuilderRefs` maps when parsing references:

```go
referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)
builder.ParseReferences(rootJSON, referenceBuilderRefs)
builder.ParseReferredReferences(referredJSON, referenceBuilderRefs)
```

### 2. Two-Phase Reference Building
Tests follow the two-phase pattern used in production:

```go
// Phase 1: Parse references
refs, _ := builder.ParseReferences(json, builderMap)
builder.ParseReferredReferences(referredJSON, builderMap)

// Phase 2: Build nested structures
for _, rb := range builderMap {
    rb.BuildNestedStructure()
}
```

### 3. Error Handling
Tests validate proper error handling for:
- Missing parent references
- Invalid JSON
- Null values in required fields
- Multiple references where only one is expected

### 4. Integration Tests
Each test file includes comprehensive integration tests that simulate real-world usage:
- `TestIntegration_CompleteReferenceHierarchy` - Complete reference parsing flow
- `TestCompleteReferenceHierarchy` - Multi-level reference builder usage
- `TestEmbeddedDataSpecificationsBuilder_Integration_CompleteHierarchy` - Complex EDS scenarios

## Test Data Format

### Reference JSON Format
```json
[
  {
    "reference_id": 100,
    "reference_type": "ExternalReference",
    "key_id": 1,
    "key_type": "GlobalReference",
    "key_value": "https://example.com/concept"
  }
]
```

### Referred Reference JSON Format
```json
[
  {
    "reference_id": 101,
    "reference_type": "ModelReference",
    "parentReference": 100,
    "rootReference": 100,
    "key_id": 2,
    "key_type": "ConceptDescription",
    "key_value": "0173-1#01-ABC123#001"
  }
]
```

### Language String JSON Format
```json
[
  {
    "id": 1,
    "language": "en",
    "text": "Example Text"
  }
]
```

## Common Test Scenarios

### Testing Single-Level References
```go
ref, rb := builder.NewReferenceBuilder("ExternalReference", 100)
rb.CreateKey(1, "GlobalReference", "https://example.com/concept")
// Verify: ref has 1 key, correct type and value
```

### Testing Multi-Level Hierarchies
```go
ref, rb := builder.NewReferenceBuilder("ExternalReference", 100)
rb.CreateKey(1, "GlobalReference", "root")
rb.CreateReferredSemanticId(101, 100, "ModelReference")
rb.CreateReferredSemanticIdKey(101, 2, "ConceptDescription", "child")
rb.CreateReferredSemanticId(102, 101, "ExternalReference")
rb.CreateReferredSemanticIdKey(102, 3, "GlobalReference", "grandchild")
rb.BuildNestedStructure()
// Verify: ref.ReferredSemanticId.ReferredSemanticId exists with correct structure
```

### Testing JSON Parsing
```go
referenceBuilderRefs := make(map[int64]*builder.ReferenceBuilder)
refs, err := builder.ParseReferences(jsonData, referenceBuilderRefs)
// Verify: correct number of references, no errors
```

## Continuous Integration

These tests should be run as part of CI/CD pipeline to ensure:
1. Reference hierarchy building works correctly
2. JSON parsing handles all edge cases
3. Error conditions are properly handled
4. No regressions in builder functionality

## Maintenance

When adding new features to builders:
1. Add corresponding unit tests
2. Include both positive and negative test cases
3. Add integration tests for complex scenarios
4. Update this README with new test coverage information

## Troubleshooting

### Import Issues
If you encounter import errors, ensure you're running tests from the project root:
```bash
cd /Users/fried/Documents/Projekte/BaSyx_Kern/basyx-go-components
```

### Test Failures
- Check that `go.mod` dependencies are up to date: `go mod tidy`
- Ensure the builder code hasn't changed in incompatible ways
- Verify test data format matches current implementation

### Coverage Gaps
To identify untested code paths:
```bash
go test -coverprofile=coverage.out ./internal/common/builder/unit_test/...
go tool cover -func=coverage.out
```

## Related Documentation

- [Reference Hierarchy Guide](../../../../REFERENCE_HIERARCHY_GUIDE.md) - Detailed explanation of reference fetching and rebuilding
- Builder GoDoc - Run `godoc -http=:6060` and navigate to package documentation
