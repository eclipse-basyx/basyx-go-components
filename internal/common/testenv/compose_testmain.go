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
// Author: Martin Stemmer ( Fraunhofer IESE )

//nolint:all
package testenv

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

type ComposeTestMainOptions struct {
	ComposeFile string
	ProjectName string
	Env         []string

	UpArgs   []string
	DownArgs []string

	UpTimeout   time.Duration
	DownTimeout time.Duration

	PreDownBeforeUp    bool
	SkipDownAfterTests bool

	HealthURL     string
	HealthTimeout time.Duration
	WaitForReady  func() error
}

func RunComposeTestMain(m *testing.M, options ComposeTestMainOptions) int {
	opts := normalizeComposeTestMainOptions(options)
	if err := applyProcessEnv(opts.Env); err != nil {
		fmt.Printf("Failed to apply Docker Compose test environment: %v\n", err)
		ReleaseReservedLocalPorts()
		return 1
	}

	engine, baseArgs, err := FindCompose()
	if err != nil {
		fmt.Println("compose engine not found:", err)
		ReleaseReservedLocalPorts()
		return 1
	}

	runWithLimit := func(timeout time.Duration, args ...string) error {
		return runWithTimeout(engine, baseArgs, opts.ComposeFile, opts.ProjectName, opts.Env, timeout, args...)
	}

	if opts.PreDownBeforeUp {
		_ = runWithLimit(opts.DownTimeout, opts.DownArgs...)
	}

	fmt.Println("Starting Docker Compose...")
	if err := runWithLimit(opts.UpTimeout, opts.UpArgs...); err != nil {
		fmt.Printf("Failed to start Docker Compose: %v\n", err)
		if !opts.SkipDownAfterTests {
			_ = runWithLimit(opts.DownTimeout, opts.DownArgs...)
		}
		ReleaseReservedLocalPorts()
		return 1
	}
	ReleaseReservedLocalPorts()

	if opts.WaitForReady != nil {
		if err := opts.WaitForReady(); err != nil {
			fmt.Printf("Service readiness check failed: %v\n", err)
			if !opts.SkipDownAfterTests {
				_ = runWithLimit(opts.DownTimeout, opts.DownArgs...)
			}
			return 1
		}
	}
	if opts.HealthURL != "" {
		if err := WaitHealthyURL(opts.HealthURL, opts.HealthTimeout); err != nil {
			fmt.Printf("Health check failed: %v\n", err)
			if !opts.SkipDownAfterTests {
				_ = runWithLimit(opts.DownTimeout, opts.DownArgs...)
			}
			return 1
		}
	}

	code := m.Run()

	if !opts.SkipDownAfterTests {
		fmt.Println("Stopping Docker Compose...")
		if err := runWithLimit(opts.DownTimeout, opts.DownArgs...); err != nil {
			fmt.Printf("Failed to stop Docker Compose: %v\n", err)
		}
	}

	return code
}

func applyProcessEnv(env []string) error {
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("TESTENV-COMPOSEMAIN-SETENV: %w", err)
		}
	}
	return nil
}

func runWithTimeout(engine string, baseArgs []string, composeFile string, projectName string, env []string, timeout time.Duration, args ...string) error {
	ctx := context.Background()
	cancel := func() {}
	if timeout > 0 {
		ctxWithTimeout, cancelFn := context.WithTimeout(context.Background(), timeout)
		ctx = ctxWithTimeout
		cancel = cancelFn
	}
	defer cancel()

	cmdArgs := composeCommandArgs(baseArgs, composeFile, projectName, args...)
	return RunComposeWithEnv(ctx, engine, env, cmdArgs...)
}

func composeCommandArgs(baseArgs []string, composeFile string, projectName string, args ...string) []string {
	cmdArgs := append([]string{}, baseArgs...)
	cmdArgs = append(cmdArgs, "-f", composeFile)
	if projectName != "" {
		cmdArgs = append(cmdArgs, "-p", projectName)
	}
	cmdArgs = append(cmdArgs, args...)
	return cmdArgs
}

func normalizeComposeTestMainOptions(options ComposeTestMainOptions) ComposeTestMainOptions {
	if options.ComposeFile == "" {
		options.ComposeFile = "docker_compose/docker_compose.yml"
	}
	if options.ProjectName == "" {
		projectName, err := NewComposeProjectName(options.ComposeFile)
		if err == nil {
			options.ProjectName = projectName
		}
	}
	if len(options.UpArgs) == 0 {
		options.UpArgs = []string{"up", "-d", "--build"}
	}
	if len(options.DownArgs) == 0 {
		options.DownArgs = []string{"down", "-v", "--remove-orphans"}
	} else {
		options.DownArgs = normalizeComposeDownArgs(options.DownArgs)
	}
	if options.UpTimeout <= 0 {
		options.UpTimeout = 10 * time.Minute
	}
	if options.DownTimeout <= 0 {
		options.DownTimeout = 10 * time.Minute
	}
	if options.HealthURL != "" && options.HealthTimeout <= 0 {
		options.HealthTimeout = 2 * time.Minute
	}
	return options
}

func normalizeComposeDownArgs(args []string) []string {
	normalized := append([]string{}, args...)
	if len(normalized) == 0 || normalized[0] != "down" {
		return normalized
	}
	if !hasAnyArg(normalized, "-v", "--volumes") {
		normalized = append(normalized, "-v")
	}
	if !hasAnyArg(normalized, "--remove-orphans") {
		normalized = append(normalized, "--remove-orphans")
	}
	return normalized
}

func hasAnyArg(args []string, candidates ...string) bool {
	for _, arg := range args {
		for _, candidate := range candidates {
			if arg == candidate {
				return true
			}
		}
	}
	return false
}
