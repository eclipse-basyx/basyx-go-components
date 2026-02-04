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

// Package bench provides benchmarking utilities for submodel repository tests.
//
//nolint:all
package bench

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	testenv "github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

func mustHaveCompose(t testing.TB) {
	t.Helper()
	if _, _, err := testenv.FindCompose(); err != nil {
		t.Skipf("docker/podman compose not found: %v", err)
	}
}

func waitUntilHealthy(b *testing.B) {
	b.Helper()

	wd, _ := os.Getwd()
	root := filepath.Dir(filepath.Dir(filepath.Dir(wd)))

	composeFile := filepath.Join(root, "examples", "BaSyxMinimalExample", "docker-compose.yml")
	if _, err := os.Stat(composeFile); os.IsNotExist(err) {
		b.Fatalf("docker-compose.yml not found at %s", composeFile)
	}

	bin, args, err := testenv.FindCompose()
	if err != nil {
		b.Fatalf("compose not found: %v", err)
	}

	ctx := context.Background()

	upArgs := append(args, "-f", composeFile, "up", "-d")
	b.Logf("Starting services: %s %v", bin, upArgs)
	if err := testenv.RunCompose(ctx, bin, upArgs...); err != nil {
		b.Fatalf("failed to start compose: %v", err)
	}

	healthURL := fmt.Sprintf("%s/health", testenv.BaseURL)
	b.Logf("Waiting for service to be healthy at %s", healthURL)
	testenv.WaitHealthy(b, healthURL, 2*time.Minute)
	b.Logf("Service is healthy and ready for benchmarking")
}
