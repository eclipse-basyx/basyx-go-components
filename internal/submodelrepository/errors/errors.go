/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

// Package errors provides centralized error definitions for the submodel repository.
package errors

import "github.com/eclipse-basyx/basyx-go-components/internal/common"

// Transaction-related errors
var (
	// ErrTransactionCommitFailed is returned when a PostgreSQL transaction fails to commit.
	ErrTransactionCommitFailed = common.NewInternalServerError("Failed to commit PostgreSQL transaction - no changes applied - see console for details")

	// ErrTransactionBeginFailed is returned when a PostgreSQL transaction fails to begin.
	ErrTransactionBeginFailed = common.NewInternalServerError("Failed to begin PostgreSQL transaction - no changes applied - see console for details")

	// ErrTransactionRollbackFailed is returned when a PostgreSQL transaction fails to rollback.
	ErrTransactionRollbackFailed = common.NewInternalServerError("Failed to rollback PostgreSQL transaction - see console for details")
)

// Element-related errors
var (
	// ErrSubmodelNotFound is returned when the requested submodel does not exist.
	ErrSubmodelNotFound = common.NewErrNotFound("Submodel not found")

	// ErrSubmodelAlreadyExists is returned when trying to create a submodel that already exists.
	ErrSubmodelAlreadyExists = common.NewErrConflict("Submodel already exists")

	// ErrSubmodelElementNotFound is returned when the requested submodel element does not exist.
	ErrSubmodelElementNotFound = common.NewErrNotFound("Submodel element not found")

	// ErrSubmodelElementAlreadyExists is returned when trying to create a submodel element that already exists.
	ErrSubmodelElementAlreadyExists = common.NewErrConflict("Submodel element already exists")
)

// Blob-related errors
var (
	// ErrBlobTooLarge is returned when a blob value exceeds the maximum size limit.
	ErrBlobTooLarge = common.NewErrBadRequest("blob value exceeds maximum size of 1GB - for files larger than 1GB, you must use File submodel element instead - Postgres Limitation")
)

// Handler creation error messages
const (
	// HandlerCreationFailedFormat is the format string for handler creation failures.
	HandlerCreationFailedFormat = "Failed to create %s handler. See console for details."
)

// NewHandlerCreationError creates an internal server error for handler creation failures.
func NewHandlerCreationError(handlerType string) error {
	return common.NewInternalServerError("Failed to create " + handlerType + " handler. See console for details.")
}
