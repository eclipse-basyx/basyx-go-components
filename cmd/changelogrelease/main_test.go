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
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const commandChangelogFixture = `# Changelog

## Unreleased

<!-- changelog-unreleased:start -->

| Type | Change | Pull request | Security impact |
| --- | --- | --- | --- |
| Bugfix | Fixed. | [#1](https://github.com/eclipse-basyx/basyx-go-components/pull/1) | None. |
<!-- changelog-unreleased:end -->

## v1.0.9

Previous release.
`

func TestRunValidate(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "CHANGELOG.md")
	require.NoError(t, os.WriteFile(inputPath, []byte(commandChangelogFixture), 0o600))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"validate", "--changelog", inputPath}, &stdout, &stderr)
	require.Equal(t, 0, exitCode, stderr.String())
	require.Equal(t, "Changelog is valid.\n", stdout.String())
}

func TestRunPrepare(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "CHANGELOG.md")
	outputPath := filepath.Join(directory, "prepared.md")
	require.NoError(t, os.WriteFile(inputPath, []byte(commandChangelogFixture), 0o600))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{
		"prepare",
		"--version", "v1.0.10",
		"--changelog", inputPath,
		"--output", outputPath,
	}, &stdout, &stderr)
	require.Equal(t, 0, exitCode, stderr.String())

	prepared, err := os.ReadFile(outputPath) // #nosec G304 -- outputPath is created under t.TempDir by this test.
	require.NoError(t, err)
	require.Contains(t, string(prepared), "## v1.0.10")
	require.Empty(t, stdout.String())
}

func TestRunExtractToStandardOutput(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "CHANGELOG.md")
	input := `# Changelog

## Unreleased

<!-- changelog-unreleased:start -->

| Type | Change | Pull request | Security impact |
| --- | --- | --- | --- |
<!-- changelog-unreleased:end -->

## v1.0.10

Changes since [v1.0.9](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.0.9...v1.0.10).

| Type | Change | Pull request | Security impact |
| --- | --- | --- | --- |
| Bugfix | Fixed. | [#1](https://github.com/eclipse-basyx/basyx-go-components/pull/1) | None. |

## v1.0.9

Previous release.
`
	require.NoError(t, os.WriteFile(inputPath, []byte(input), 0o600))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{
		"extract",
		"--version", "v1.0.10",
		"--changelog", inputPath,
		"--output", "-",
	}, &stdout, &stderr)
	require.Equal(t, 0, exitCode, stderr.String())
	require.Contains(t, stdout.String(), "## Changelog")
}

func TestRunReportsCodedErrors(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"unknown"}, &stdout, &stderr)
	require.Equal(t, 1, exitCode)
	require.Contains(t, stderr.String(), "CHLOG-MAIN-PARSECOMMAND")
}
