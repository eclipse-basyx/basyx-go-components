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

package smregistrypostgresql

import "testing"

func TestBuildSubmodelDescriptorUpsertLockSQLUsesPostgresPlaceholders(t *testing.T) {
	t.Parallel()

	query, args, err := buildSubmodelDescriptorUpsertLockSQL("submodel-1")

	if err != nil {
		t.Fatalf("buildSubmodelDescriptorUpsertLockSQL returned error: %v", err)
	}
	if query != "SELECT pg_advisory_xact_lock(hashtextextended($1, $2))" {
		t.Fatalf("unexpected query: %s", query)
	}
	if len(args) != 2 || args[0] != "submodel_descriptor:submodel-1" || args[1] != int64(0) {
		t.Fatalf("unexpected args: %#v", args)
	}
}
