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

package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestBundledAccessRuleFilesParse(t *testing.T) {
	t.Parallel()

	patterns := []string{
		filepath.Join("..", "..", "..", "cmd", "*", "config", "access_rules", "access-rules.json"),
		filepath.Join("..", "..", "..", "examples", "*", "security_env", "access-rules.json"),
	}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %q failed: %v", pattern, err)
		}
		for _, file := range files {
			file := file
			t.Run(file, func(t *testing.T) {
				t.Parallel()
				//nolint:gosec // test reads repository-local access-rule fixtures discovered by glob.
				data, err := os.ReadFile(file)
				if err != nil {
					t.Fatalf("read access-rule file failed: %v", err)
				}
				if _, err = ParseAccessModel(data, chi.NewRouter(), ""); err != nil {
					t.Fatalf("parse access-rule file failed: %v", err)
				}
			})
		}
	}
}
