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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package scalability_tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const scalabilityResultsDirectory = "results"

type scenarioReportContext struct {
	fixtureIndex int
	user         string
}

type scenarioReportRow struct {
	context      scenarioReportContext
	scenario     string
	requests     int
	statusCounts string
	bodyBytes    int64
	p50          time.Duration
	p95          time.Duration
	maximum      time.Duration
}

type scalabilityReport struct {
	mutex       sync.Mutex
	startedAt   time.Time
	fixtures    []fixture
	users       []string
	pageLimit   int
	repetitions int
	concurrency int
	asyncBulk   asyncBulkConfig
	rows        []scenarioReportRow
	failures    []string
}

func newScalabilityReport(startedAt time.Time) *scalabilityReport {
	return &scalabilityReport{startedAt: startedAt}
}

func (r *scalabilityReport) configure(
	fixtures []fixture,
	users []testUser,
	pageLimit, repetitions, concurrency int,
	asyncBulk asyncBulkConfig,
) {
	if r == nil {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.fixtures = append([]fixture{}, fixtures...)
	r.users = make([]string, 0, len(users))
	for _, user := range users {
		r.users = append(r.users, user.name)
	}
	r.pageLimit = pageLimit
	r.repetitions = repetitions
	r.concurrency = concurrency
	r.asyncBulk = asyncBulk
}

func (r *scalabilityReport) addRow(row scenarioReportRow) {
	if r == nil {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.rows = append(r.rows, row)
}

func (r *scalabilityReport) addFailure(message string) {
	if r == nil {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.failures = append(r.failures, message)
}

func (r *scalabilityReport) write(exitCode int) (string, error) {
	if r == nil {
		return "", fmt.Errorf("DTRSCALE-REPORT-WRITE report is not initialized")
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if err := os.MkdirAll(scalabilityResultsDirectory, 0o750); err != nil {
		return "", fmt.Errorf("DTRSCALE-REPORT-WRITE-CREATEDIRECTORY: %w", err)
	}
	createdAt := time.Now().UTC()
	fileName := fmt.Sprintf("scalability-%s-%d.md", createdAt.Format("20060102T150405.000000000Z"), os.Getpid())
	path := filepath.Join(scalabilityResultsDirectory, fileName)
	content := r.markdown(createdAt, exitCode)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("DTRSCALE-REPORT-WRITE-FILE: %w", err)
	}
	return path, nil
}

func (r *scalabilityReport) markdown(createdAt time.Time, exitCode int) string {
	result := "PASSED"
	if exitCode != 0 {
		result = "FAILED"
	}

	var builder strings.Builder
	writeMarkdown(&builder, "# DTR scalability result\n\n")
	writeMarkdown(&builder, "- Started: `%s`\n", r.startedAt.UTC().Format(time.RFC3339Nano))
	writeMarkdown(&builder, "- Report created: `%s`\n", createdAt.Format(time.RFC3339Nano))
	writeMarkdown(&builder, "- Result: **%s** (exit code `%d`)\n", result, exitCode)
	writeMarkdown(&builder, "- Request timeout: `%s`\n", requestTimeout)
	writeMarkdown(&builder, "- Page limit: `%d`\n", r.pageLimit)
	writeMarkdown(&builder, "- Repetitions per scenario: `%d`\n", r.repetitions)
	writeMarkdown(&builder, "- Maximum concurrent requests per scenario: `%d`\n", r.concurrency)
	writeMarkdown(&builder, "- Users: `%s`\n", strings.Join(r.users, ", "))
	writeMarkdown(&builder, "- Async bulk enabled: `%t`\n", r.asyncBulk.enabled)
	if r.asyncBulk.enabled {
		writeMarkdown(&builder, "- Async bulk descriptors per operation: `%d`\n", r.asyncBulk.size)
		writeMarkdown(&builder, "- Async bulk lifecycle timeout: `%s`\n", r.asyncBulk.timeout)
		writeMarkdown(&builder, "- Async bulk poll interval: `%s`\n", r.asyncBulk.pollInterval)
	}

	writeMarkdown(&builder, "\n## Fixtures\n\n")
	writeMarkdown(&builder, "| # | AAS ID | Submodel ID |\n| --- | --- | --- |\n")
	for index, item := range r.fixtures {
		writeMarkdown(&builder, "| %d | `%s` | `%s` |\n", index+1, item.aasID, item.submodelID)
	}

	writeMarkdown(&builder, "\n## Scenario results\n\n")
	writeMarkdown(&builder, "| Fixture | User | Scenario | Requests | HTTP statuses | Total response bytes | Avg. response bytes | p50 | p95 | Max |\n| --- | --- | --- | ---: | --- | ---: | ---: | --- | --- | --- |\n")
	for _, row := range r.rows {
		fixture := "-"
		if row.context.fixtureIndex > 0 {
			fixture = fmt.Sprintf("%d", row.context.fixtureIndex)
		}
		averageBodyBytes := int64(0)
		if row.requests > 0 {
			averageBodyBytes = row.bodyBytes / int64(row.requests)
		}
		writeMarkdown(&builder, "| %s | `%s` | `%s` | %d | `%s` | %d | %d | `%s` | `%s` | `%s` |\n", fixture, row.context.user, row.scenario, row.requests, row.statusCounts, row.bodyBytes, averageBodyBytes, row.p50, row.p95, row.maximum)
	}

	if len(r.failures) > 0 {
		writeMarkdown(&builder, "\n## Failures\n\n")
		for _, failure := range r.failures {
			writeMarkdown(&builder, "- %s\n", failure)
		}
	}
	return builder.String()
}

func writeMarkdown(builder *strings.Builder, format string, arguments ...any) {
	_, _ = fmt.Fprintf(builder, format, arguments...)
}
