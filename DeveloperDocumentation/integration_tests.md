# Integration Tests

## Purpose
Houses end-to-end and scenario tests that validate the complete functionality of the system, ensuring that all components work together as expected.

## Location
- `internal/{COMPONENT_NAME}/integration_tests/`
    - e.g., `internal/submodelrepository/integration_tests/`

## How they Work
- it_config.json: Configuration file defining test scenarios, endpoints, and expected results.
- postBody/ and expectedResponse/ folders: Contain JSON files for request bodies and expected responses.
- Test files (e.g., `integration_test.go`): Implement test logic using Go's testing framework.
- Use Docker Compose to set up isolated test environments with necessary services (e.g., databases). docker_compose/ folder contains relevant files.

## How to Run
### Terminal
1. Navigate to the integration_tests directory of the component you want to test:
   ```sh
   cd internal/{COMPONENT_NAME}/integration_tests/
   ```
2. Run the tests using Go's testing tool:
   ```sh
   go test -v .
   ```
### Action Plugin
1. Ensure Docker is running on your machine.
2. Click on the "Run Integration Tests" action in your IDE or CI/CD pipeline that is configured to execute the integration tests.
### Task
1. Ensure Docker is running on your machine.
2. Open Command Palette (Ctrl+Shift+P or Cmd+Shift+P for Windows, F1 for macOS).
3. Tasks: Run Task
4. Select "Run Integration Tests" from the list.

## For new Components
When adding new components or features, create corresponding integration tests in the relevant `integration_tests/` folder to cover end-to-end scenarios.

`Note: You may reuse the integration_test.go template found in existing integration test folders.`

You must always add an Action and Task to run the integration tests for the new component in the .vscode folder.

### Best Practices
- Cover all Endpoints
- Use realistic data in postBody/ files
- Validate both success and error scenarios
- Keep it_config.json up to date with new test cases as features are added
