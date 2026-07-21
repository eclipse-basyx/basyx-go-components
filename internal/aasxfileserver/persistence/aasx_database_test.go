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

package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/stretchr/testify/require"
)

func TestParseCursorID(t *testing.T) {
	t.Parallel()

	cursor, err := ParseCursorID("")
	require.NoError(t, err)
	require.Equal(t, int64(0), cursor)

	cursor, err = ParseCursorID("42")
	require.NoError(t, err)
	require.Equal(t, int64(42), cursor)

	_, err = ParseCursorID("-1")
	require.Error(t, err)

	_, err = ParseCursorID("abc")
	require.Error(t, err)
}

func TestNormalizeAASIDs(t *testing.T) {
	t.Parallel()

	result := normalizeAASIDs([]string{"id1,id2", " id2 ", "", "id3", "id1"})
	require.Equal(t, []string{"id1", "id2", "id3"}, result)
}

func TestNormalizeFileName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "provided.aasx", normalizeFileName("  provided.aasx ", "upload-file.aasx"))
	require.Equal(t, "upload-file.aasx", normalizeFileName("", "upload-file.aasx"))
}

func TestDetectAASXEnvironmentContentType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		relativeAASX string
		expected     string
	}{
		{
			name:         "xml environment aasx",
			relativeAASX: "../../aasenvironment/integration_tests/testdata/IESEDriveMotorDM3000.aasx",
			expected:     "application/aasx+xml",
		},
		{
			name:         "json environment aasx",
			relativeAASX: "../../aasenvironment/integration_tests/testdata/ProductionPlanSFKL.aasx",
			expected:     "application/aasx+json",
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filePath := filepath.Clean(tt.relativeAASX)
			tempFile, err := os.Open(filePath)
			require.NoError(t, err)
			defer func() { _ = tempFile.Close() }()

			resolved, err := detectAASXEnvironmentContentType(tempFile, common.AASXLimitsFromConfig(nil))
			require.NoError(t, err)
			require.Equal(t, tt.expected, resolved)
		})
	}
}
