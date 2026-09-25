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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package auth

import (
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

// ABACRightAccess classifies how ABAC grants one right of a route.
type ABACRightAccess string

// ABAC right access classes.
const (
	ABACAccessNone          ABACRightAccess = "none"
	ABACAccessConditional   ABACRightAccess = "abac-conditional"
	ABACAccessUnconditional ABACRightAccess = "abac"
)

// ABACModelBinder is implemented by extensions that need the active ABAC
// policy, for example to report effective rights.
type ABACModelBinder interface {
	BindABAC(provider AccessModelProvider, enableImplicitCasts bool)
}

// EvaluateABACRight evaluates the active policy for one route path and right
// without executing the request. requestPath includes the context path.
func EvaluateABACRight(
	provider AccessModelProvider,
	enableImplicitCasts bool,
	method string,
	requestPath string,
	claims Claims,
	right grammar.RightsEnum,
) ABACRightAccess {
	if provider == nil {
		return ABACAccessNone
	}
	model := provider.ActiveAccessModel()
	if model == nil {
		return ABACAccessNone
	}
	options := grammar.DefaultSimplifyOptions()
	options.EnableImplicitCasts = enableImplicitCasts
	session := newAuthorizationSession(model, claims, nil, options)
	evaluation := session.evaluate(method, requestPath)
	if !evaluation.Allowed {
		return ABACAccessNone
	}
	if evaluation.QueryFilter == nil {
		return ABACAccessUnconditional
	}
	formula, ok := evaluation.QueryFilter.FormulasByRight[right]
	if !ok || formula.Boolean != nil && !*formula.Boolean {
		return ABACAccessNone
	}
	if formula.Boolean != nil && len(evaluation.QueryFilter.Filters) == 0 {
		return ABACAccessUnconditional
	}
	return ABACAccessConditional
}
