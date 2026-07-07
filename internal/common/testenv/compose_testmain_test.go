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

package testenv

import (
	"os"
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

func TestReserveLockedLocalPortPreventsDuplicateLock(t *testing.T) {
	port, lock, err := reserveLockedLocalPort(nil)
	require.NoError(t, err)
	t.Cleanup(lock.Release)

	duplicate, err := acquirePortLock(port)
	if err == nil {
		duplicate.Release()
	}
	require.Error(t, err)
}

func TestNewComposeRuntimeRegistersLocksForRelease(t *testing.T) {
	runtime, err := NewComposeRuntime("lock-release", []PortBinding{
		{Name: "api", EnvVar: "BASYX_TESTENV_LOCK_RELEASE_PORT"},
	})
	require.NoError(t, err)
	t.Cleanup(ReleaseReservedLocalPorts)

	require.Len(t, runtime.portLocks, 1)
	lockPath := runtime.portLocks[0].path
	require.DirExists(t, lockPath)

	ReleaseReservedLocalPorts()

	require.NoDirExists(t, lockPath)
}

func TestApplyProcessEnvSetsValidEntries(t *testing.T) {
	t.Cleanup(func() { _ = os.Unsetenv("BASYX_TESTENV_PROCESS_ENV") })

	require.NoError(t, applyProcessEnv([]string{
		"BASYX_TESTENV_PROCESS_ENV=expected",
		"not-an-env-entry",
		"=missing-key",
	}))

	require.Equal(t, "expected", os.Getenv("BASYX_TESTENV_PROCESS_ENV"))
}

func TestSetEnvDefaultsDoesNotOverrideExistingValues(t *testing.T) {
	t.Setenv("BASYX_TESTENV_DEFAULT", "existing")
	t.Cleanup(func() { _ = os.Unsetenv("BASYX_TESTENV_ADDED") })

	require.NoError(t, SetEnvDefaults(map[string]string{
		"BASYX_TESTENV_DEFAULT": "new",
		"BASYX_TESTENV_ADDED":   "added",
	}))

	require.Equal(t, "existing", os.Getenv("BASYX_TESTENV_DEFAULT"))
	require.Equal(t, "added", os.Getenv("BASYX_TESTENV_ADDED"))
}
