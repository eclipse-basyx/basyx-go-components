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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultCheckDBIsEmptyExcludedTablesIncludesPersistentSchemaTables(t *testing.T) {
	excluded := defaultCheckDBIsEmptyExcludedTables([]string{"AAS_Identifier", " "})

	for _, table := range []string{
		"basyxsystem",
		"history_guard_config",
		"aas_history",
		"aas_history_payload",
		"submodel_history",
		"submodel_history_payload",
		"concept_description_history",
		"concept_description_history_payload",
		"descriptor_history",
		"descriptor_history_payload",
		"aas_identifier",
	} {
		_, ok := excluded[table]
		require.Truef(t, ok, "expected %s to be excluded", table)
	}
}
