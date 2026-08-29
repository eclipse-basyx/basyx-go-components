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

package changelogrelease

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const changelogFixture = `# Changelog

Introduction.

## Unreleased

<!-- changelog-unreleased:start -->
<!-- Add entries as table rows. Keep exactly four columns and escape literal pipes as \|. -->

| Type | Change | Pull request | Security impact |
| --- | --- | --- | --- |
| Bugfix | Fixed a release issue. | [#1](https://github.com/eclipse-basyx/basyx-go-components/pull/1) | None. |
<!-- changelog-unreleased:end -->

## v1.0.9

Previous release.
`

func TestValidate(t *testing.T) {
	require.NoError(t, Validate([]byte(changelogFixture)))

	empty := strings.Replace(changelogFixture, "| Bugfix | Fixed a release issue. | [#1](https://github.com/eclipse-basyx/basyx-go-components/pull/1) | None. |\n", "", 1)
	require.NoError(t, Validate([]byte(empty)))

	escapedPipe := strings.Replace(changelogFixture, "Fixed a release issue.", `Fixed input \| output handling.`, 1)
	require.NoError(t, Validate([]byte(escapedPipe)))
}

func TestValidateRejectsMalformedContributorEntries(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		errorCode string
	}{
		{
			name:      "missing marker",
			input:     strings.Replace(changelogFixture, "<!-- changelog-unreleased:end -->", "", 1),
			errorCode: "CHLOG-VALIDATE-FINDMARKERS",
		},
		{
			name:      "unexpected prose outside markers",
			input:     strings.Replace(changelogFixture, "## Unreleased\n", "## Unreleased\n\nEditable prose.\n", 1),
			errorCode: "CHLOG-VALIDATE-SECTIONBOUNDARY",
		},
		{
			name:      "wrong column count",
			input:     strings.Replace(changelogFixture, "Fixed a release issue.", "Fixed input | output handling.", 1),
			errorCode: "CHLOG-VALIDATE-TABLEROW",
		},
		{
			name:      "unsupported type",
			input:     strings.Replace(changelogFixture, "| Bugfix |", "| Feature |", 1),
			errorCode: "CHLOG-VALIDATE-ROWTYPE",
		},
		{
			name:      "mismatched pull request",
			input:     strings.Replace(changelogFixture, "pull/1)", "pull/2)", 1),
			errorCode: "CHLOG-VALIDATE-ROWPULLREQUEST",
		},
		{
			name:      "empty security impact",
			input:     strings.Replace(changelogFixture, "| None. |", "| |", 1),
			errorCode: "CHLOG-VALIDATE-ROWSECURITY",
		},
		{
			name:      "duplicate release version",
			input:     changelogFixture + "\n## v1.0.9\n\nDuplicate.\n",
			errorCode: "CHLOG-VALIDATE-ORDERRELEASES",
		},
		{
			name:      "ascending release versions",
			input:     changelogFixture + "\n## v1.0.10\n\nOut of order.\n",
			errorCode: "CHLOG-VALIDATE-ORDERRELEASES",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate([]byte(test.input))
			require.ErrorContains(t, err, test.errorCode)
		})
	}
}

func TestPrepareCreatesReleaseAndNextUnreleasedSection(t *testing.T) {
	prepared, err := Prepare([]byte(changelogFixture), "v1.0.10")
	require.NoError(t, err)

	expected := `# Changelog

Introduction.

## Unreleased

<!-- changelog-unreleased:start -->
<!-- Add entries as table rows. Keep exactly four columns and escape literal pipes as \|. -->

| Type | Change | Pull request | Security impact |
| --- | --- | --- | --- |
<!-- changelog-unreleased:end -->

## v1.0.10

Changes since [v1.0.9](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.0.9...v1.0.10).

| Type | Change | Pull request | Security impact |
| --- | --- | --- | --- |
| Bugfix | Fixed a release issue. | [#1](https://github.com/eclipse-basyx/basyx-go-components/pull/1) | None. |

## v1.0.9

Previous release.
`
	require.Equal(t, expected, string(prepared))
	require.NoError(t, Validate(prepared))
}

func TestPreparePreservesCRLF(t *testing.T) {
	input := strings.ReplaceAll(changelogFixture, "\n", "\r\n")

	prepared, err := Prepare([]byte(input), "v1.0.10")
	require.NoError(t, err)
	require.NotContains(t, strings.ReplaceAll(string(prepared), "\r\n", ""), "\n")
}

func TestPrepareRejectsInvalidState(t *testing.T) {
	emptyTable := strings.Replace(changelogFixture, "| Bugfix | Fixed a release issue. | [#1](https://github.com/eclipse-basyx/basyx-go-components/pull/1) | None. |\n", "", 1)
	tests := []struct {
		name      string
		input     string
		version   string
		errorCode string
	}{
		{name: "invalid target version", input: changelogFixture, version: "1.0.10", errorCode: "CHLOG-VERSION-PARSE"},
		{name: "target is not newer", input: changelogFixture, version: "v1.0.8", errorCode: "CHLOG-PREP-COMPAREVERSION"},
		{name: "release already exists", input: changelogFixture, version: "v1.0.9", errorCode: "CHLOG-PREP-FINDVERSION"},
		{name: "missing unreleased section", input: strings.Replace(changelogFixture, "## Unreleased", "## Pending", 1), version: "v1.0.10", errorCode: "CHLOG-VALIDATE-FINDUNRELEASED"},
		{name: "empty change table", input: emptyTable, version: "v1.0.10", errorCode: "CHLOG-PREP-VALIDATEENTRIES"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Prepare([]byte(test.input), test.version)
			require.ErrorContains(t, err, test.errorCode)
		})
	}
}

func TestPrepareSupportsPrereleaseProgression(t *testing.T) {
	input := strings.ReplaceAll(changelogFixture, "v1.0.9", "v1.1.0-rc.1")

	_, err := Prepare([]byte(input), "v1.1.0-rc.2")
	require.NoError(t, err)

	_, err = Prepare([]byte(input), "v1.1.0")
	require.NoError(t, err)
}

func TestExtract(t *testing.T) {
	prepared, err := Prepare([]byte(changelogFixture), "v1.0.10")
	require.NoError(t, err)

	notes, err := Extract(prepared, "v1.0.10")
	require.NoError(t, err)
	require.Equal(t, `## Changelog

Changes since [v1.0.9](https://github.com/eclipse-basyx/basyx-go-components/compare/v1.0.9...v1.0.10).

| Type | Change | Pull request | Security impact |
| --- | --- | --- | --- |
| Bugfix | Fixed a release issue. | [#1](https://github.com/eclipse-basyx/basyx-go-components/pull/1) | None. |
`, string(notes))
}

func TestExtractRejectsMissingVersionOrEmptyRelease(t *testing.T) {
	_, err := Extract([]byte(changelogFixture), "v1.0.10")
	require.ErrorContains(t, err, "CHLOG-EXTRACT-FINDVERSION")

	emptyRelease := strings.Replace(changelogFixture, "Previous release.", tableHeader+"\n"+tableDelimiter, 1)
	_, err = Extract([]byte(emptyRelease), "v1.0.9")
	require.ErrorContains(t, err, "CHLOG-EXTRACT-VALIDATETABLE")
}

func TestCompose(t *testing.T) {
	existing := []byte("Introductory release notes.\n")
	changelog := []byte("## Changelog\n\n| Type | Change |\n| --- | --- |\n| Bugfix | Fixed. |\n")
	emptyBody, err := Compose(nil, changelog)
	require.NoError(t, err)
	require.Equal(t, `<!-- release-changelog:start -->
## Changelog

| Type | Change |
| --- | --- |
| Bugfix | Fixed. |
<!-- release-changelog:end -->
`, string(emptyBody))

	composed, err := Compose(existing, changelog)
	require.NoError(t, err)
	require.Equal(t, `Introductory release notes.

<!-- release-changelog:start -->
## Changelog

| Type | Change |
| --- | --- |
| Bugfix | Fixed. |
<!-- release-changelog:end -->
`, string(composed))

	recomposed, err := Compose(composed, changelog)
	require.NoError(t, err)
	require.Equal(t, composed, recomposed)
}

func TestComposeReplacesExistingChangelog(t *testing.T) {
	existing := []byte(`Intro.

<!-- release-changelog:start -->
## Changelog

Old content.
<!-- release-changelog:end -->

Closing text.
`)

	composed, err := Compose(existing, []byte("## Changelog\n\nNew content.\n"))
	require.NoError(t, err)
	require.NotContains(t, string(composed), "Old content")
	require.Contains(t, string(composed), "New content")
	require.Contains(t, string(composed), "Closing text")
}

func TestComposeRejectsMalformedMarkers(t *testing.T) {
	_, err := Compose(
		[]byte("<!-- release-changelog:start -->\nUnclosed block.\n"),
		[]byte("## Changelog\n"),
	)
	require.ErrorContains(t, err, "CHLOG-COMPOSE-VALIDATEMARKERS")
}
