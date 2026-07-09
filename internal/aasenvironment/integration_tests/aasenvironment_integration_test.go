/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

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
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

const composeFilePath = "./docker_compose/docker_compose.yml"

var aasEnvBaseURL = testenv.LocalURLFromEnv("BASYX_IT_API_PORT", 6004)
var aasEnvSyncOffBaseURL = testenv.LocalURLFromEnv("BASYX_IT_SYNC_OFF_API_PORT", 6005)
var integrationTestDSN = testenv.PostgresKeywordDSNFromEnv("BASYX_IT_DB_PORT", 6432, "basyxTestDB")

var allowedIntegrationPackages = map[string]struct{}{
	"github.com/eclipse-basyx/basyx-go-components/internal/aasregistry/integration_tests":                  {},
	"github.com/eclipse-basyx/basyx-go-components/internal/smregistry/integration_tests":                   {},
	"github.com/eclipse-basyx/basyx-go-components/internal/aasrepository/integration_tests":                {},
	"github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/integration_tests":           {},
	"github.com/eclipse-basyx/basyx-go-components/internal/conceptdescriptionrepository/integration_tests": {},
	"github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/integration_tests":             {},
}

func TestMain(m *testing.M) {
	runtime := testenv.NewComposeRuntimeOrExit("aasenvironment-it", []testenv.PortBinding{
		{Name: "api", EnvVar: "BASYX_IT_API_PORT"},
		{Name: "db", EnvVar: "BASYX_IT_DB_PORT"},
		{Name: "sync-off-api", EnvVar: "BASYX_IT_SYNC_OFF_API_PORT"},
		{Name: "sync-off-db", EnvVar: "BASYX_IT_SYNC_OFF_DB_PORT"},
	})
	aasEnvBaseURL = runtime.LocalURL("api")
	aasEnvSyncOffBaseURL = runtime.LocalURL("sync-off-api")
	integrationTestDSN = runtime.PostgresKeywordDSN("db", "basyxTestDB")
	serializationBaseURL = runtime.LocalURL("api")
	serializationIntegrationDSN = runtime.PostgresKeywordDSN("db", "basyxTestDB")
	uploadIntegrationDSN = runtime.PostgresKeywordDSN("db", "basyxTestDB")
	uploadSyncDisabledIntegrationDSN = runtime.PostgresKeywordDSN("sync-off-db", "basyxTestDBSyncOff")

	os.Exit(testenv.RunComposeTestMain(m, testenv.ComposeTestMainOptions{
		ComposeFile:     composeFilePath,
		ProjectName:     runtime.ProjectName,
		Env:             runtime.Env(),
		PreDownBeforeUp: true,
		HealthURL:       aasEnvBaseURL + "/health",
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
			cmd.Env = append(os.Environ(),
				"BASYX_EXTERNAL_COMPOSE=1",
				"BASYX_AASENVIRONMENT_SKIP_IMPORTED_DESCRIPTION=1",
			)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			require.NoError(t, cmd.Run(), "failed integration package: %s", pkg)
		})
	}
}

func resetDatabase(t *testing.T) {
	t.Helper()

	db, err := sql.Open("pgx", integrationTestDSN)
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
