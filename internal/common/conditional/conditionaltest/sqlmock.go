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

// Package conditionaltest provides sqlmock expectations for the resource
// revision statements that write transactions run before commit.
package conditionaltest

import (
	"github.com/DATA-DOG/go-sqlmock"
)

// ExpectRevisionUpsert expects the revision write of a created or changed
// resource immediately before commit.
func ExpectRevisionUpsert(mock sqlmock.Sqlmock, kind string, identifier string) {
	mock.ExpectQuery(`INSERT INTO "resource_revision" .* ON CONFLICT \(kind, identifier\) DO UPDATE`).
		WithArgs(identifier, kind).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "identifier", "revision"}).AddRow(kind, identifier, 1))
}

// ExpectRevisionDelete expects the revision removal of a deleted resource
// immediately before commit.
func ExpectRevisionDelete(mock sqlmock.Sqlmock, kind string, identifier string) {
	mock.ExpectExec(`DELETE FROM "resource_revision"`).
		WithArgs(identifier, kind).
		WillReturnResult(sqlmock.NewResult(0, 1))
}
