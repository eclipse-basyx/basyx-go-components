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

// Package testenv provides testing utilities for integration tests and benchmarks.
// It includes helpers for HTTP requests, component benchmarking, Docker Compose management,
// and health checking of services.
// nolint:all
package testenv

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// HTTPClient returns a configured HTTP client with a 20-second timeout.
func HTTPClient() *http.Client { return &http.Client{Timeout: 20 * time.Second} }

// FindCompose searches for docker or podman on the PATH and returns the binary name and compose subcommand.
// Returns an error if neither docker nor podman is found.
func FindCompose() (bin string, args []string, err error) {
	if _, e := exec.LookPath("docker"); e == nil {
		return "docker", []string{"compose"}, nil
	}
	if _, e := exec.LookPath("podman"); e == nil {
		return "podman", []string{"compose"}, nil
	}
	return "", nil, errors.New("neither docker nor podman found on PATH")
}

// RunCompose executes a Docker Compose command with the given base command and arguments.
// Streams stdout and stderr to the current process's output streams.
func RunCompose(ctx context.Context, base string, args ...string) error {
	cmd := exec.CommandContext(ctx, base, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// WaitHealthy polls the given URL until it returns HTTP 200 or the timeout is reached.
// Uses exponential backoff (starting at 1 second, max 5 seconds) between attempts.
// Fails the test if the service is not healthy within maxWait duration.
func WaitHealthy(t testing.TB, url string, maxWait time.Duration) {
	t.Helper()

	if err := WaitHealthyURL(url, maxWait); err != nil {
		t.Fatalf("%v", err)
	}
}

// WaitHealthyURL polls the given URL until it returns HTTP 200 or the timeout is reached.
// Returns a detailed timeout error containing the last received HTTP status or request error.
func WaitHealthyURL(url string, maxWait time.Duration) error {
	if maxWait <= 0 {
		maxWait = 2 * time.Minute
	}

	deadline := time.Now().Add(maxWait)
	backoff := time.Second
	lastStatus := -1
	var lastErr error

	for {
		resp, err := HTTPClient().Get(url)
		if err != nil {
			lastStatus = -1
			lastErr = err
		} else {
			lastStatus = resp.StatusCode
			_ = resp.Body.Close()
			lastErr = nil
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		if time.Now().After(deadline) {
			return healthTimeoutError(url, maxWait, lastStatus, lastErr)
		}

		time.Sleep(backoff)
		if backoff < 5*time.Second {
			backoff += 500 * time.Millisecond
		}
	}
}

func healthTimeoutError(url string, maxWait time.Duration, lastStatus int, lastErr error) error {
	lastStatusText := "n/a"
	if lastStatus >= 0 {
		lastStatusText = strconv.Itoa(lastStatus)
	}
	if lastErr != nil {
		return fmt.Errorf(
			"TESTENV-WAITHEALTH-TIMEOUT: service not healthy at %s within %s (last_status=%s, last_error=%v)",
			url,
			maxWait,
			lastStatusText,
			lastErr,
		)
	}
	return fmt.Errorf(
		"TESTENV-WAITHEALTH-TIMEOUT: service not healthy at %s within %s (last_status=%s)",
		url,
		maxWait,
		lastStatusText,
	)
}
