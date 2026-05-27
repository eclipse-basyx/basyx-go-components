package persistence

import (
	"os"
	"path/filepath"
	"testing"

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

	tempFile, err := os.CreateTemp("", "upload-file.aasx.*")
	require.NoError(t, err)
	// #nosec G703 -- tempFile.Name() is provided by os.CreateTemp in this test.
	defer func() { _ = os.Remove(tempFile.Name()) }()
	defer func() { _ = tempFile.Close() }()

	require.Equal(t, "provided.aasx", normalizeFileName("  provided.aasx ", tempFile))
	require.Equal(t, "upload-file.aasx", normalizeFileName("", tempFile))
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

			resolved, err := detectAASXEnvironmentContentType(tempFile)
			require.NoError(t, err)
			require.Equal(t, tt.expected, resolved)
		})
	}
}
