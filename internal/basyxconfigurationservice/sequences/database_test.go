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

package sequences

import (
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

func TestDatabaseConnectionGetDescription(t *testing.T) {
	step := NewDatabaseConnection(&ExecutionContext{})
	description := step.GetDescription(3)
	if description != "[Step 3] Connecting to Database" {
		t.Fatalf("unexpected description: %q", description)
	}
}

func TestDatabaseConnectionExecuteRequiresPreloadedConfig(t *testing.T) {
	step := NewDatabaseConnection(&ExecutionContext{})
	statusCode, err := step.Execute(1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if statusCode != 1 {
		t.Fatalf("expected status code 1, got %d", statusCode)
	}
	if !strings.Contains(err.Error(), "BASYXCFG-DB-NOCONFIG") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatabaseConnectionRetainsPreloadedConfigWhenConnectionFails(t *testing.T) {
	cfg := &common.Config{}
	ctx := &ExecutionContext{Config: cfg}
	step := NewDatabaseConnection(ctx)

	statusCode, err := step.Execute(1)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
	if statusCode != 1 {
		t.Fatalf("expected status code 1, got %d", statusCode)
	}
	if ctx.Config != cfg {
		t.Fatal("expected the preloaded configuration to remain in the execution context")
	}
}
