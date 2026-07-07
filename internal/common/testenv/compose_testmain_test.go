package testenv

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComposeCommandArgsIncludesProjectName(t *testing.T) {
	args := composeCommandArgs([]string{"compose"}, "docker_compose/docker_compose.yml", "basyx-it-123", "up", "-d")

	require.Equal(t, []string{
		"compose",
		"-f", "docker_compose/docker_compose.yml",
		"-p", "basyx-it-123",
		"up", "-d",
	}, args)
}

func TestNormalizeComposeTestMainOptionsDefaultsDownCleanup(t *testing.T) {
	options := normalizeComposeTestMainOptions(ComposeTestMainOptions{})

	require.Equal(t, []string{"down", "-v", "--remove-orphans"}, options.DownArgs)
	require.NotEmpty(t, options.ProjectName)
}

func TestNormalizeComposeTestMainOptionsAddsMissingDownCleanupArgs(t *testing.T) {
	options := normalizeComposeTestMainOptions(ComposeTestMainOptions{
		DownArgs: []string{"down", "--timeout", "1"},
	})

	require.Contains(t, options.DownArgs, "-v")
	require.Contains(t, options.DownArgs, "--remove-orphans")
}

func TestNewComposeProjectNameIsUniqueAndValid(t *testing.T) {
	first, err := NewComposeProjectName("Internal/AAS Registry Integration Tests")
	require.NoError(t, err)
	second, err := NewComposeProjectName("Internal/AAS Registry Integration Tests")
	require.NoError(t, err)

	require.NotEqual(t, first, second)
	require.True(t, first[0] >= 'a' && first[0] <= 'z')
	require.NotContains(t, first, "/")
	require.Equal(t, strings.ToLower(first), first)
}
