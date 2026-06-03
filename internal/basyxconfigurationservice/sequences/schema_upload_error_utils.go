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
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (su *SchemaUpload) applyRc01Compatibility(schemaToLoad string) error {
	compatibilitySchemaPath := filepath.Join(filepath.Dir(schemaToLoad), rc01CompatibilitySchemaFileName)
	//nolint:gosec // only set by config service, not set via user input/config files
	compatibilitySQL, err := os.ReadFile(compatibilitySchemaPath)
	if err != nil {
		return fmt.Errorf("BASYXCFG-SCHEMA-RC01READFILE: %w", err)
	}
	if _, err = su.ctx.DB.Exec(string(compatibilitySQL)); err != nil {
		return fmt.Errorf("BASYXCFG-SCHEMA-RC01EXECUTE: %w", err)
	}
	return nil
}

func isRc01UpgradeError(err error) bool {
	return strings.HasSuffix(err.Error(), errCaseRc01UpgradeFieldErr)
}
