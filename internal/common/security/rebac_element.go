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
	"context"
	"database/sql"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"net/http"
	"net/url"
)

// AuthorizeReBACReadElement resolves an element's own additive permissions.
func AuthorizeReBACReadElement(ctx context.Context, submodelID, path string) (context.Context, error) {
	return authorizeReBACReadPath(ctx, "/submodels/"+common.EncodeString(submodelID)+"/submodel-elements/"+url.PathEscape(path))
}

// AuthorizeReBACExecuteElement authorizes the operation target with current scope state.
func AuthorizeReBACExecuteElement(ctx context.Context, submodelID, path string) (context.Context, error) {
	return authorizeReBACPath(ctx, "/submodels/"+common.EncodeString(submodelID)+"/submodel-elements/"+url.PathEscape(path)+"/invoke", http.MethodPost)
}

// AuthorizeReBACMutationElement authorizes one element mutation within its writer transaction.
func AuthorizeReBACMutationElement(ctx context.Context, tx *sql.Tx, submodelID, path, method string) (context.Context, error) {
	return authorizeReBACMutationPath(ctx, tx, "/submodels/"+common.EncodeString(submodelID)+"/submodel-elements/"+url.PathEscape(path), method)
}
