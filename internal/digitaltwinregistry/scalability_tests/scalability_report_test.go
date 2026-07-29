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
	rows        []scenarioReportRow
	failures    []string
}

func newScalabilityReport(startedAt time.Time) *scalabilityReport {
	return &scalabilityReport{startedAt: startedAt}
}

func (r *scalabilityReport) configure(fixtures []fixture, users []testUser, pageLimit, repetitions, concurrency int) {
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

	if err := os.MkdirAll(scalabilityResultsDirectory, 0o755); err != nil {
		return "", fmt.Errorf("DTRSCALE-REPORT-WRITE-CREATEDIRECTORY: %w", err)
	}
	createdAt := time.Now().UTC()
	fileName := fmt.Sprintf("scalability-%s-%d.md", createdAt.Format("20060102T150405.000000000Z"), os.Getpid())
	path := filepath.Join(scalabilityResultsDirectory, fileName)
	content := r.markdown(createdAt, exitCode)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
	builder.WriteString("# DTR scalability result\n\n")
	fmt.Fprintf(&builder, "- Started: `%s`\n", r.startedAt.UTC().Format(time.RFC3339Nano))
	fmt.Fprintf(&builder, "- Report created: `%s`\n", createdAt.Format(time.RFC3339Nano))
	fmt.Fprintf(&builder, "- Result: **%s** (exit code `%d`)\n", result, exitCode)
	fmt.Fprintf(&builder, "- Request timeout: `%s`\n", requestTimeout)
	fmt.Fprintf(&builder, "- Page limit: `%d`\n", r.pageLimit)
	fmt.Fprintf(&builder, "- Repetitions per scenario: `%d`\n", r.repetitions)
	fmt.Fprintf(&builder, "- Maximum concurrent requests per scenario: `%d`\n", r.concurrency)
	fmt.Fprintf(&builder, "- Users: `%s`\n", strings.Join(r.users, ", "))

	builder.WriteString("\n## Fixtures\n\n")
	builder.WriteString("| # | AAS ID | Submodel ID |\n| --- | --- | --- |\n")
	for index, item := range r.fixtures {
		fmt.Fprintf(&builder, "| %d | `%s` | `%s` |\n", index+1, item.aasID, item.submodelID)
	}

	builder.WriteString("\n## Scenario results\n\n")
	builder.WriteString("| Fixture | User | Scenario | Requests | HTTP statuses | Total response bytes | Avg. response bytes | p50 | p95 | Max |\n| --- | --- | --- | ---: | --- | ---: | ---: | --- | --- | --- |\n")
	for _, row := range r.rows {
		averageBodyBytes := int64(0)
		if row.requests > 0 {
			averageBodyBytes = row.bodyBytes / int64(row.requests)
		}
		fmt.Fprintf(&builder, "| %d | `%s` | `%s` | %d | `%s` | %d | %d | `%s` | `%s` | `%s` |\n", row.context.fixtureIndex, row.context.user, row.scenario, row.requests, row.statusCounts, row.bodyBytes, averageBodyBytes, row.p50, row.p95, row.maximum)
	}

	if len(r.failures) > 0 {
		builder.WriteString("\n## Failures\n\n")
		for _, failure := range r.failures {
			fmt.Fprintf(&builder, "- %s\n", failure)
		}
	}
	return builder.String()
}
