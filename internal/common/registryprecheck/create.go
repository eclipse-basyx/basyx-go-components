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

// Package registryprecheck contains registry API precheck helpers.
package registryprecheck

import (
	"context"
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

// ExistsFunc checks raw descriptor existence without visibility filtering.
type ExistsFunc func(context.Context) (bool, error)

// ReadFunc performs a visibility-aware descriptor read.
type ReadFunc func(context.Context) error

// EnsureVisibleCreate verifies that a create request does not disclose hidden duplicates.
func EnsureVisibleCreate(
	ctx context.Context,
	exists ExistsFunc,
	read ReadFunc,
	conflictMessage string,
	deniedMessage string,
) error {
	if exists == nil {
		return common.NewInternalServerError("REGPRECHECK-CREATE-EXISTSCALLBACK existence callback must not be nil")
	}
	if read == nil {
		return common.NewInternalServerError("REGPRECHECK-CREATE-READCALLBACK read callback must not be nil")
	}

	descriptorExists, err := exists(auth.WithoutQueryFilter(ctx))
	if err != nil {
		return err
	}
	if !descriptorExists {
		return nil
	}
	if auth.GetQueryFilter(ctx) == nil {
		return common.NewErrConflict(conflictMessage)
	}

	readCtx := auth.SelectFormulaForRight(ctx, grammar.RightsEnumREAD)
	if err = read(readCtx); err != nil {
		if common.IsErrNotFound(err) || common.IsErrDenied(err) {
			return common.NewErrDenied(deniedMessage)
		}
		return err
	}
	return common.NewErrConflict(conflictMessage)
}

// ResponseStatus maps create precheck errors to an HTTP status and response step.
func ResponseStatus(err error) (int, string) {
	switch {
	case common.IsErrConflict(err):
		return http.StatusConflict, "Conflict-Exists"
	case common.IsErrDenied(err):
		return http.StatusForbidden, "Denied-Exists"
	case common.IsErrBadRequest(err):
		return http.StatusBadRequest, "BadRequest-Precheck"
	default:
		return http.StatusInternalServerError, "Unhandled-Precheck"
	}
}

// ReturnError keeps internal precheck failures visible to generated service callers.
func ReturnError(err error) error {
	statusCode, _ := ResponseStatus(err)
	if statusCode >= http.StatusInternalServerError {
		return err
	}
	return nil
}
