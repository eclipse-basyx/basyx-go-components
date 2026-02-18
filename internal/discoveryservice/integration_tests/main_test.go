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

//nolint:all
package bench

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

const (
	BaseURL         = "http://127.0.0.1:5004"
	ComposeFilePath = "./docker_compose/docker_compose.yml"
)

var (
	composeAvailable bool
	composeEngine    string
	composeArgsBase  []string
)

// TestMain runs for integration and benchmark tests
func TestMain(m *testing.M) {
	eng, baseArgs, err := testenv.FindCompose()
	if err != nil {
		_, _ = fmt.Println("compose engine not found:", err)
		composeAvailable = false
		os.Exit(m.Run())
	}
	composeAvailable = true
	composeEngine = eng
	composeArgsBase = baseArgs

	//nolint:all
	upArgs := append(composeArgsBase, "-f", ComposeFilePath, "up", "-d", "--build")
	if v := getenv("DISC_TEST_BUILD"); v == "1" {
		upArgs = append(upArgs, "--build")
	}

	//nolint:all
	ctxUp, cancelUp := context.WithTimeout(context.Background(), 10*time.Minute)
	if err := testenv.RunCompose(ctxUp, composeEngine, upArgs...); err != nil {
		_, _ = fmt.Println("failed to start compose:", err)
		composeAvailable = false
	}
	cancelUp()

	code := m.Run()

	//nolint:all
	ctxDown, cancelDown := context.WithTimeout(context.Background(), 10*time.Minute)
	_ = testenv.RunCompose(ctxDown, composeEngine, append(composeArgsBase, "-f", ComposeFilePath, "down")...)
	cancelDown()

	os.Exit(code)
}

func mustHaveCompose(tb testing.TB) {
	tb.Helper()
	if !composeAvailable {
		tb.Skip("compose not available in this environment")
	}
}

func waitUntilHealthy(tb testing.TB) {
	tb.Helper()
	testenv.WaitHealthy(tb, BaseURL+"/health", 2*time.Minute)
}

func getenv(k string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return ""
}
