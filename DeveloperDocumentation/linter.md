# Linter
This Project uses golangci-lint as linter to ensure code quality and consistency across the codebase.

## Enabled Linters (as of November 17th 2025)
- misspell
- errcheck
- govet
- ineffassign
- staticcheck
- revive
- unused
- gocritic

## Running the Linter
To run the linter locally, ensure you have golangci-lint installed. You can install it via:
### General
```sh
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```
### macOS (Recommended)
```sh
brew install golangci-lint
```

## Usage
Navigate to the root directory of the project and execute:
```sh
golangci-lint run
```

## Rules
### Disabling Specific Linters for a Line or Block
You can disable specific linters for a single line by adding a comment before the line:
```go
//nolint:<linter1>,<linter2>
```
For example:
```go
//nolint:errcheck
someFunctionCall()
```
You should use this sparingly and only when you are certain that ignoring the linter warning is justified.

Also provide a brief comment explaining why the linter is being disabled.

`Note: This is not needed for model_*.go files as they are auto-generated and thus may not conform to all linter rules.`

## Common Issues
- **Linter has less errors locally than in CI/CD**: Ensure you are using the same version of golangci-lint as specified in the CI/CD pipeline. Additionally, clear the cache using `golangci-lint cache clean` and rerun the linter - There is a handy script `lint.sh` in the root directory that does this for you which is also available via the Actions Plugin or as a Task in your IDE.
