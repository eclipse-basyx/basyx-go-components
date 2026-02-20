# `testenv` Quick Guide

Shared helpers for integration tests:

- `RunComposeTestMain(...)` for compose setup/teardown.
- `RunJSONSuite(...)` for config-driven HTTP test steps.

## Quick Start

```go
func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:          "docker_compose/docker_compose.yml",
		FailIfComposeMissing: true,
		HealthURL:            "http://localhost:6004/health",
	}))
}

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		DefaultExpectedStatus: http.StatusOK,
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet),
	})
}
```

## `RunComposeTestMain`

Runs compose `up`, optional readiness checks, tests, then compose `down`.
Compose engine is auto-detected via `FindCompose()` (`docker compose` first, then `podman compose`).

`ComposeTestMainOptions`:

- `ComposeFile` default: `docker_compose/docker_compose.yml`
- `UpArgs` default: `["up", "-d", "--build"]`
- `DownArgs` default: `["down"]`
- `PreDownBeforeUp`: run `down` before `up`
- `SkipDownAfterTests`: keep stack running after tests
- `FailIfComposeMissing`: fail when neither docker nor podman exists
- `HealthURL`: wait for HTTP 200 before running tests
- `HealthTimeout`: timeout for `HealthURL` (default `2m` when set)
- `WaitForReady`: optional custom readiness callback

## `RunJSONSuite`

Loads `it_config.json` (or `ConfigPath`) and executes each step as a subtest.

Step fields (`JSONSuiteStep`):

- `context`
- `method`
- `endpoint`
- `data` (path to body file)
- `shouldMatch` (path to expected JSON file)
- `expectedStatus`
- `action`
- `headers`
- `token` (`user`/`password`)

`JSONSuiteOptions` (most-used):

- `ConfigPath` default: `it_config.json`
- `LogsDir` default: `logs`
- `InitialDelay`
- `RequestTimeout` default: `10s`
- `DefaultExpectedStatus` (fallback is 200)
- `ShouldCompareResponse`
- `ActionHandlers` for `action` steps
- `TokenProvider` for token-based steps
- `EnableRequestLog` and `EnableRawDump`

### Example `RunJSONSuite` + `it_config.json`

```go
func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		DefaultExpectedStatus: http.StatusOK,
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet),
		ActionHandlers: map[string]testenv.JSONStepAction{
			"DELETE_ALL_ITEMS": func(t *testing.T, runner *testenv.JSONSuiteRunner, _ testenv.JSONSuiteStep, stepNumber int) {
				_, err := runner.RunStep(testenv.JSONSuiteStep{
					Method:         http.MethodDelete,
					Endpoint:       "http://localhost:6004/items",
					ExpectedStatus: http.StatusNoContent,
				}, stepNumber)
				require.NoError(t, err)
			},
		},
	})
}
```

```json
[
  {
    "context": "Create item",
    "method": "POST",
    "endpoint": "http://localhost:6004/items",
    "data": "postBody/item.json",
    "expectedStatus": 201
  },
  {
    "context": "Get all items",
    "method": "GET",
    "endpoint": "http://localhost:6004/items",
    "shouldMatch": "expected/items_all.json"
  },
  {
    "context": "Cleanup",
    "action": "DELETE_ALL_ITEMS"
  }
]
```

## Logging Behavior

By default, logs are written on failures:

- status mismatch: `logs/STEP_<n>.log`
- JSON mismatch: `logs/STEP_<n>.log`
- request error: `logs/REQUEST_STEP_<n>.error.log`

If `EnableRequestLog` is true, request logs are written for all steps.
If `EnableRawDump` is also true, raw HTTP dumps are written too.

## Useful Helpers

- `CompareMethods("GET", ...)` to compare `shouldMatch` only for chosen methods
- `NewPasswordGrantTokenProvider(tokenURL, clientID, timeout)` for OIDC password grant tokens
