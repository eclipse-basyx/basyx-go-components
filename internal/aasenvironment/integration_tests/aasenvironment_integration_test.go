package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

const composeFilePath = "./docker_compose/docker_compose.yml"
const integrationTestDSN = "host=127.0.0.1 port=6432 user=admin password=admin123 dbname=basyxTestDB sslmode=disable"

var allowedIntegrationPackages = map[string]struct{}{
	"github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/integration_tests":                  {},
	"github.com/eclipse-basyx/basyx-go-components/internal/smregistry/integration_tests":                   {},
	"github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/integration_tests":                {},
	"github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/integration_tests":           {},
	"github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/integration_tests": {},
	"github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/integration_tests":             {},
}

func TestMain(m *testing.M) {
	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     composeFilePath,
		PreDownBeforeUp: true,
		HealthURL:       "http://127.0.0.1:6004/health",
		HealthTimeout:   3 * time.Minute,
	}))
}

func TestIntegration(t *testing.T) {
	packages := []string{
		"github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/integration_tests",
		"github.com/eclipse-basyx/basyx-go-components/internal/smregistry/integration_tests",
		"github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/integration_tests",
		"github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/integration_tests",
		"github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/integration_tests",
		"github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/integration_tests",
	}

	for _, pkg := range packages {
		pkg := pkg
		t.Run(strings.ReplaceAll(pkg, "/", "_"), func(t *testing.T) {
			t.Helper()
			resetDatabase(t)
			_, ok := allowedIntegrationPackages[pkg]
			require.True(t, ok, "unsupported integration package: %s", pkg)

			// #nosec G204 -- pkg is validated against a static allow-list above.
			cmd := exec.Command("go", "test", "-v", "-count=1", pkg)
			cmd.Env = append(os.Environ(), "BASYX_EXTERNAL_COMPOSE=1")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			require.NoError(t, cmd.Run(), "failed integration package: %s", pkg)
		})
	}
}

func resetDatabase(t *testing.T) {
	t.Helper()

	db, err := sql.Open("postgres", integrationTestDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	rows, err := db.Query("SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	tables := make([]string, 0, 64)
	for rows.Next() {
		var table string
		require.NoError(t, rows.Scan(&table))
		tables = append(tables, table)
	}
	require.NoError(t, rows.Err())

	for _, table := range tables {
		truncateQuery := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", quoteIdentifier(table))
		_, execErr := db.Exec(truncateQuery)
		require.NoErrorf(t, execErr, "failed to truncate table %s", table)
	}
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
