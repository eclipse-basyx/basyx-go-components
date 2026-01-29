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

// Package config provides configuration constants for the submodel repository.
package config

const (
	// DefaultPageLimit is the default number of items returned per page when pagination limit is not specified.
	DefaultPageLimit int32 = 100

	// MaxPageLimit is the maximum allowed number of items per page for pagination.
	MaxPageLimit int32 = 1000

	// WorkerPoolSize is the default number of concurrent workers for parallel processing operations.
	WorkerPoolSize = 10

	// MaxBlobSizeBytes is the maximum allowed size for blob elements (1 GB).
	// This is a PostgreSQL limitation for bytea columns.
	MaxBlobSizeBytes = 1 << 30 // 1 GB
)
