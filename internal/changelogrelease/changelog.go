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

// Package changelogrelease prepares and renders versioned changelog sections.
package changelogrelease

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	releaseChangelogStartMarker = "<!-- release-changelog:start -->"
	releaseChangelogEndMarker   = "<!-- release-changelog:end -->"
	unreleasedStartMarker       = "<!-- changelog-unreleased:start -->"
	unreleasedEndMarker         = "<!-- changelog-unreleased:end -->"
	unreleasedGuidance          = "<!-- Add entries as table rows. Keep exactly four columns and escape literal pipes as \\|. -->"
	unreleasedHeading           = "## Unreleased"
	headingPrefix               = "## "
	tableHeader                 = "| Type | Change | Pull request | Security impact |"
	tableDelimiter              = "| --- | --- | --- | --- |"
)

var (
	versionPattern     = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$`)
	pullRequestPattern = regexp.MustCompile(`^\[#([1-9][0-9]*)\]\(https://github\.com/eclipse-basyx/basyx-go-components/pull/([1-9][0-9]*)\)$`)
)

type markdownDocument struct {
	lines   []string
	newline string
}

type markdownSection struct {
	start int
	end   int
}

type markerPair struct {
	start int
	end   int
}

type changeTable struct {
	rows []string
}

type releaseSection struct {
	tag     string
	version semanticVersion
	section markdownSection
}

type changelogStructure struct {
	document   markdownDocument
	unreleased markdownSection
	table      changeTable
	releases   []releaseSection
}

type semanticVersion struct {
	core       [3]int
	prerelease []string
}

// Validate verifies the contributor-managed changelog structure and entries.
func Validate(content []byte) error {
	_, err := inspectChangelog(content)
	return err
}

// Prepare promotes the Unreleased entries to version and creates a new empty Unreleased section.
func Prepare(content []byte, version string) ([]byte, error) {
	targetVersion, err := parseVersion(version)
	if err != nil {
		return nil, err
	}
	structure, err := inspectChangelog(content)
	if err != nil {
		return nil, err
	}
	if len(structure.table.rows) == 0 {
		return nil, fmt.Errorf("CHLOG-PREP-VALIDATEENTRIES: Unreleased change table contains no entries")
	}

	previousRelease := structure.releases[0]
	if findReleaseByTag(structure.releases, version) >= 0 {
		return nil, fmt.Errorf("CHLOG-PREP-FINDVERSION: changelog already contains a section for %s", version)
	}
	if compareVersions(targetVersion, previousRelease.version) <= 0 {
		return nil, fmt.Errorf("CHLOG-PREP-COMPAREVERSION: target version %s must be newer than %s", version, previousRelease.tag)
	}

	preparedLines := prepareLines(structure, previousRelease.tag, version)
	return structure.document.render(preparedLines), nil
}

// Extract renders one versioned changelog section for a GitHub release body.
func Extract(content []byte, version string) ([]byte, error) {
	if _, err := parseVersion(version); err != nil {
		return nil, err
	}
	structure, err := inspectChangelog(content)
	if err != nil {
		return nil, err
	}
	releaseIndex := findReleaseByTag(structure.releases, version)
	if releaseIndex < 0 {
		return nil, fmt.Errorf("CHLOG-EXTRACT-FINDVERSION: expected exactly one section for %s, found 0", version)
	}

	section := structure.releases[releaseIndex].section
	body := trimBlankLines(structure.document.lines[section.start+1 : section.end])
	if len(body) == 0 {
		return nil, fmt.Errorf("CHLOG-EXTRACT-VALIDATESECTION: section for %s is empty", version)
	}
	headerIndex := findExactLine(body, tableHeader)
	if headerIndex < 0 {
		return nil, fmt.Errorf("CHLOG-EXTRACT-TABLEHEADER: expected table header %q", tableHeader)
	}
	if _, err = locateChangeTable(body, headerIndex, len(body), false, "CHLOG-EXTRACT"); err != nil {
		return nil, err
	}

	notes := append([]string{"## Changelog", ""}, body...)
	return []byte(strings.Join(notes, "\n") + "\n"), nil
}

// Compose adds or replaces the generated changelog block in an existing release body.
func Compose(existingBody []byte, changelog []byte) ([]byte, error) {
	normalizedBody := normalizeNewlines(string(existingBody))
	normalizedChangelog := strings.TrimSpace(normalizeNewlines(string(changelog)))
	if normalizedChangelog == "" {
		return nil, fmt.Errorf("CHLOG-COMPOSE-VALIDATECHANGELOG: generated changelog is empty")
	}
	if strings.Contains(normalizedChangelog, releaseChangelogStartMarker) || strings.Contains(normalizedChangelog, releaseChangelogEndMarker) {
		return nil, fmt.Errorf("CHLOG-COMPOSE-VALIDATECHANGELOG: generated changelog contains reserved markers")
	}

	block := releaseChangelogStartMarker + "\n" + normalizedChangelog + "\n" + releaseChangelogEndMarker
	composed, err := replaceMarkedBlock(normalizedBody, block)
	if err != nil {
		return nil, err
	}
	return []byte(composed + "\n"), nil
}

func inspectChangelog(content []byte) (changelogStructure, error) {
	document := parseDocument(content)
	unreleased, err := findUnreleasedSection(document.lines)
	if err != nil {
		return changelogStructure{}, err
	}
	markers, err := findUnreleasedMarkers(document.lines, unreleased)
	if err != nil {
		return changelogStructure{}, err
	}
	if err = validateUnreleasedBoundaries(document.lines, unreleased, markers); err != nil {
		return changelogStructure{}, err
	}
	table, err := locateChangeTable(document.lines, markers.start+1, markers.end, true, "CHLOG-VALIDATE")
	if err != nil {
		return changelogStructure{}, err
	}
	releases, err := findReleaseSections(document.lines, unreleased.end)
	if err != nil {
		return changelogStructure{}, err
	}
	return changelogStructure{
		document:   document,
		unreleased: unreleased,
		table:      table,
		releases:   releases,
	}, nil
}

func parseDocument(content []byte) markdownDocument {
	newline := "\n"
	if bytes.Contains(content, []byte("\r\n")) {
		newline = "\r\n"
	}
	normalized := strings.TrimRight(normalizeNewlines(string(content)), "\n")
	if normalized == "" {
		return markdownDocument{newline: newline}
	}
	return markdownDocument{lines: strings.Split(normalized, "\n"), newline: newline}
}

func (document markdownDocument) render(lines []string) []byte {
	return []byte(strings.Join(lines, document.newline) + document.newline)
}

func normalizeNewlines(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func findUnreleasedSection(lines []string) (markdownSection, error) {
	start := -1
	count := 0
	for index, line := range lines {
		if !strings.HasPrefix(line, headingPrefix+"Unreleased") {
			continue
		}
		count++
		if line != unreleasedHeading {
			return markdownSection{}, fmt.Errorf("CHLOG-VALIDATE-FINDUNRELEASED: heading must be exactly %q", unreleasedHeading)
		}
		start = index
	}
	if count != 1 {
		return markdownSection{}, fmt.Errorf("CHLOG-VALIDATE-FINDUNRELEASED: expected exactly one Unreleased section, found %d", count)
	}
	return markdownSection{start: start, end: findSectionEnd(lines, start)}, nil
}

func findUnreleasedMarkers(lines []string, section markdownSection) (markerPair, error) {
	start := findUniqueLine(lines, unreleasedStartMarker)
	end := findUniqueLine(lines, unreleasedEndMarker)
	if start < 0 || end < 0 {
		return markerPair{}, fmt.Errorf("CHLOG-VALIDATE-FINDMARKERS: expected exactly one complete Unreleased marker pair")
	}
	if start <= section.start || end <= start || end >= section.end {
		return markerPair{}, fmt.Errorf("CHLOG-VALIDATE-FINDMARKERS: Unreleased markers must be ordered inside the Unreleased section")
	}
	return markerPair{start: start, end: end}, nil
}

func findUniqueLine(lines []string, expected string) int {
	match := -1
	for index, line := range lines {
		if line != expected {
			continue
		}
		if match >= 0 {
			return -1
		}
		match = index
	}
	return match
}

func findExactLine(lines []string, expected string) int {
	for index, line := range lines {
		if line == expected {
			return index
		}
	}
	return -1
}

func validateUnreleasedBoundaries(lines []string, section markdownSection, markers markerPair) error {
	if !containsOnlyBlankLines(lines[section.start+1 : markers.start]) {
		return fmt.Errorf("CHLOG-VALIDATE-SECTIONBOUNDARY: content before the Unreleased start marker is not allowed")
	}
	if !containsOnlyBlankLines(lines[markers.end+1 : section.end]) {
		return fmt.Errorf("CHLOG-VALIDATE-SECTIONBOUNDARY: content after the Unreleased end marker is not allowed")
	}
	return nil
}

func containsOnlyBlankLines(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return false
		}
	}
	return true
}

func findReleaseSections(lines []string, start int) ([]releaseSection, error) {
	releases := make([]releaseSection, 0)
	for index, line := range lines {
		if !strings.HasPrefix(line, headingPrefix+"v") {
			continue
		}
		tag := strings.TrimPrefix(line, headingPrefix)
		version, err := parseVersion(tag)
		if err != nil {
			return nil, fmt.Errorf("CHLOG-VALIDATE-PARSERELEASE: invalid release heading %q: %w", line, err)
		}
		if index < start {
			return nil, fmt.Errorf("CHLOG-VALIDATE-ORDERRELEASES: release heading %q appears before Unreleased", line)
		}
		releases = append(releases, releaseSection{
			tag:     tag,
			version: version,
			section: markdownSection{start: index, end: findSectionEnd(lines, index)},
		})
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("CHLOG-VALIDATE-FINDRELEASE: at least one previous release heading is required")
	}
	for index := 1; index < len(releases); index++ {
		if compareVersions(releases[index-1].version, releases[index].version) <= 0 {
			return nil, fmt.Errorf("CHLOG-VALIDATE-ORDERRELEASES: release headings must be unique and ordered newest first")
		}
	}
	return releases, nil
}

func findSectionEnd(lines []string, start int) int {
	for index := start + 1; index < len(lines); index++ {
		if strings.HasPrefix(lines[index], headingPrefix) {
			return index
		}
	}
	return len(lines)
}

func findReleaseByTag(releases []releaseSection, tag string) int {
	for index, release := range releases {
		if release.tag == tag {
			return index
		}
	}
	return -1
}

func locateChangeTable(lines []string, start int, end int, allowEmpty bool, errorPrefix string) (changeTable, error) {
	headerIndex := findTableHeader(lines, start, end)
	if headerIndex < 0 {
		return changeTable{}, fmt.Errorf("%s-TABLEHEADER: expected table header %q", errorPrefix, tableHeader)
	}
	if headerIndex+1 >= end || lines[headerIndex+1] != tableDelimiter {
		return changeTable{}, fmt.Errorf("%s-TABLEDELIMITER: expected table delimiter %q", errorPrefix, tableDelimiter)
	}

	rows, err := validateTableBody(lines, headerIndex+2, end, errorPrefix)
	if err != nil {
		return changeTable{}, err
	}
	if !allowEmpty && len(rows) == 0 {
		return changeTable{}, fmt.Errorf("%s-VALIDATETABLE: change table contains no entries", errorPrefix)
	}
	return changeTable{rows: rows}, nil
}

func findTableHeader(lines []string, start int, end int) int {
	for index := start; index < end; index++ {
		trimmed := strings.TrimSpace(lines[index])
		if trimmed == "" || isHTMLComment(trimmed) {
			continue
		}
		if lines[index] == tableHeader {
			return index
		}
		return -1
	}
	return -1
}

func isHTMLComment(line string) bool {
	return strings.HasPrefix(line, "<!--") && strings.HasSuffix(line, "-->")
}

func validateTableBody(lines []string, start int, end int, errorPrefix string) ([]string, error) {
	rows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			continue
		}
		cells, err := splitTableRow(line)
		if err != nil {
			return nil, fmt.Errorf("%s-TABLEROW: line %d: %w", errorPrefix, index+1, err)
		}
		if err = validateChangeRow(cells, index+1, errorPrefix); err != nil {
			return nil, err
		}
		rows = append(rows, lines[index])
	}
	return rows, nil
}

func splitTableRow(line string) ([]string, error) {
	if len(line) < 2 || line[0] != '|' || line[len(line)-1] != '|' {
		return nil, fmt.Errorf("row must start and end with a pipe")
	}

	cells := make([]string, 0, 4)
	cellStart := 1
	for index := 1; index < len(line)-1; index++ {
		if line[index] != '|' || isEscaped(line, index) {
			continue
		}
		cells = append(cells, strings.TrimSpace(line[cellStart:index]))
		cellStart = index + 1
	}
	cells = append(cells, strings.TrimSpace(line[cellStart:len(line)-1]))
	if len(cells) != 4 {
		return nil, fmt.Errorf("row must contain exactly four columns, found %d", len(cells))
	}
	return cells, nil
}

func isEscaped(value string, index int) bool {
	backslashes := 0
	for position := index - 1; position >= 0 && value[position] == '\\'; position-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func validateChangeRow(cells []string, lineNumber int, errorPrefix string) error {
	if !isAllowedChangeType(cells[0]) {
		return fmt.Errorf("%s-ROWTYPE: line %d uses unsupported type %q", errorPrefix, lineNumber, cells[0])
	}
	if cells[1] == "" {
		return fmt.Errorf("%s-ROWCHANGE: line %d has an empty change description", errorPrefix, lineNumber)
	}
	if err := validatePullRequest(cells[2], lineNumber, errorPrefix); err != nil {
		return err
	}
	if cells[3] == "" {
		return fmt.Errorf("%s-ROWSECURITY: line %d has an empty security impact", errorPrefix, lineNumber)
	}
	return nil
}

func isAllowedChangeType(value string) bool {
	return value == "High impact" || value == "Low impact" || value == "Bugfix"
}

func validatePullRequest(value string, lineNumber int, errorPrefix string) error {
	matches := pullRequestPattern.FindStringSubmatch(value)
	if len(matches) != 3 || matches[1] != matches[2] {
		return fmt.Errorf("%s-ROWPULLREQUEST: line %d must contain a matching BaSyx pull-request link", errorPrefix, lineNumber)
	}
	return nil
}

func prepareLines(structure changelogStructure, previousTag string, version string) []string {
	prepared := make([]string, 0, len(structure.document.lines)+10)
	prepared = append(prepared, structure.document.lines[:structure.unreleased.start]...)
	prepared = append(prepared,
		unreleasedHeading,
		"",
		unreleasedStartMarker,
		unreleasedGuidance,
		"",
		tableHeader,
		tableDelimiter,
		unreleasedEndMarker,
		"",
		fmt.Sprintf("## %s", version),
		"",
		fmt.Sprintf(
			"Changes since [%s](https://github.com/eclipse-basyx/basyx-go-components/compare/%s...%s).",
			previousTag,
			previousTag,
			version,
		),
		"",
		tableHeader,
		tableDelimiter,
	)
	prepared = append(prepared, structure.table.rows...)
	prepared = append(prepared, "")
	prepared = append(prepared, structure.document.lines[structure.unreleased.end:]...)
	return prepared
}

func trimBlankLines(lines []string) []string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return append([]string(nil), lines[start:end]...)
}

func replaceMarkedBlock(body string, block string) (string, error) {
	startCount := strings.Count(body, releaseChangelogStartMarker)
	endCount := strings.Count(body, releaseChangelogEndMarker)
	if startCount == 0 && endCount == 0 {
		return appendBlock(body, block), nil
	}
	if startCount != 1 || endCount != 1 {
		return "", fmt.Errorf("CHLOG-COMPOSE-VALIDATEMARKERS: expected zero or one complete marker pair")
	}

	start := strings.Index(body, releaseChangelogStartMarker)
	end := strings.Index(body, releaseChangelogEndMarker)
	if end < start {
		return "", fmt.Errorf("CHLOG-COMPOSE-VALIDATEMARKERS: end marker precedes start marker")
	}
	before := strings.TrimRight(body[:start], " \t\n")
	after := strings.TrimLeft(body[end+len(releaseChangelogEndMarker):], " \t\n")
	return joinBodyParts(before, block, after), nil
}

func appendBlock(body string, block string) string {
	trimmedBody := strings.TrimSpace(body)
	if trimmedBody == "" {
		return block
	}
	return trimmedBody + "\n\n" + block
}

func joinBodyParts(before string, block string, after string) string {
	parts := make([]string, 0, 3)
	if before != "" {
		parts = append(parts, before)
	}
	parts = append(parts, block)
	if after != "" {
		parts = append(parts, after)
	}
	return strings.Join(parts, "\n\n")
}

func parseVersion(value string) (semanticVersion, error) {
	matches := versionPattern.FindStringSubmatch(value)
	if len(matches) != 7 {
		return semanticVersion{}, fmt.Errorf("CHLOG-VERSION-PARSE: version %q must use the vX.Y.Z SemVer format", value)
	}

	var parsed semanticVersion
	for index := range parsed.core {
		number, err := strconv.Atoi(matches[index+1])
		if err != nil {
			return semanticVersion{}, fmt.Errorf("CHLOG-VERSION-PARSE: version component in %q is too large: %w", value, err)
		}
		parsed.core[index] = number
	}
	if matches[5] == "" {
		return parsed, nil
	}
	parsed.prerelease = strings.Split(matches[5], ".")
	for _, identifier := range parsed.prerelease {
		if isNumeric(identifier) && len(identifier) > 1 && identifier[0] == '0' {
			return semanticVersion{}, fmt.Errorf("CHLOG-VERSION-PARSE: numeric prerelease identifiers must not contain leading zeroes in %q", value)
		}
	}
	return parsed, nil
}

func compareVersions(left semanticVersion, right semanticVersion) int {
	for index := range left.core {
		if left.core[index] != right.core[index] {
			return compareInts(left.core[index], right.core[index])
		}
	}
	if len(left.prerelease) == 0 || len(right.prerelease) == 0 {
		return compareReleaseState(left.prerelease, right.prerelease)
	}
	return comparePrerelease(left.prerelease, right.prerelease)
}

func compareReleaseState(left []string, right []string) int {
	if len(left) == len(right) {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	return -1
}

func comparePrerelease(left []string, right []string) int {
	limit := min(len(left), len(right))
	for index := 0; index < limit; index++ {
		comparison := comparePrereleaseIdentifier(left[index], right[index])
		if comparison != 0 {
			return comparison
		}
	}
	return compareInts(len(left), len(right))
}

func comparePrereleaseIdentifier(left string, right string) int {
	leftNumeric := isNumeric(left)
	rightNumeric := isNumeric(right)
	if leftNumeric && rightNumeric {
		if len(left) != len(right) {
			return compareInts(len(left), len(right))
		}
		return strings.Compare(left, right)
	}
	if leftNumeric != rightNumeric {
		if leftNumeric {
			return -1
		}
		return 1
	}
	return strings.Compare(left, right)
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func compareInts(left int, right int) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
