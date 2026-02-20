//nolint:all
package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deleteAllInfrastructureDescriptors(t *testing.T, runner *testenv.JSONSuiteRunner, stepNumber int) {
	body, err := runner.RunStep(testenv.JSONSuiteStep{
		Method:         http.MethodGet,
		Endpoint:       "http://127.0.0.1:5004/infrastructure-descriptors",
		ExpectedStatus: http.StatusOK,
	}, stepNumber)
	require.NoError(t, err)

	var list struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &list))

	for _, item := range list.Result {
		enc := base64.RawURLEncoding.EncodeToString([]byte(item.ID))
		_, err := runner.RunStep(testenv.JSONSuiteStep{
			Method:         http.MethodDelete,
			Endpoint:       fmt.Sprintf("http://127.0.0.1:5004/infrastructure-descriptors/%s", enc),
			ExpectedStatus: http.StatusNoContent,
		}, stepNumber)
		require.NoError(t, err)
	}
}

func TestIntegration(t *testing.T) {
	testenv.RunJSONSuite(t, testenv.JSONSuiteOptions{
		ShouldCompareResponse: testenv.CompareMethods(http.MethodGet),
		EnableRequestLog:      true,
		EnableRawDump:         true,
		StepName: func(step testenv.JSONSuiteStep, stepNumber int) string {
			context := "Not Provided"
			if step.Context != "" {
				context = step.Context
			}
			return fmt.Sprintf("Step_(%s)_%d_%s_%s", context, stepNumber, step.Method, step.Endpoint)
		},
		ActionHandlers: map[string]testenv.JSONStepAction{
			"DELETE_ALL_INFRASTRUCTURE_DESCRIPTORS": func(t *testing.T, runner *testenv.JSONSuiteRunner, _ testenv.JSONSuiteStep, stepNumber int) {
				deleteAllInfrastructureDescriptors(t, runner, stepNumber)
			},
		},
	})

	t.Run("Check_DB_Empty", func(t *testing.T) {
		db, err := sql.Open("postgres", "host=127.0.0.1 port=5432 user=admin password=admin123 dbname=basyxTestDB sslmode=disable")
		require.NoError(t, err)
		defer func() { _ = db.Close() }()
		require.NoError(t, db.Ping())

		rows, err := db.Query("SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()

		nonEmpty := []string{}
		for rows.Next() {
			var table string
			require.NoError(t, rows.Scan(&table))

			var cnt int
			q := fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", table)
			err = db.QueryRow(q).Scan(&cnt)
			require.NoError(t, err)
			if cnt != 0 {
				nonEmpty = append(nonEmpty, fmt.Sprintf("%s:%d", table, cnt))
			}
		}
		require.NoError(t, rows.Err())

		assert.Empty(t, nonEmpty, "Expected all tables empty, but found rows in: %v", nonEmpty)
	})
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile: "docker_compose/docker-compose.yml",
		HealthURL:   "http://localhost:5004/health",
	}))
}
