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
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
)

// ABACSettings defines the configuration used to enable and control
// Attribute-Based Access Control.
//
// Enabled: toggles ABAC enforcement.
// Model: provides the AccessModel that evaluates authorization rules.
type ABACSettings struct {
	Enabled             bool
	EnableImplicitCasts bool
	Model               *AccessModel
}

// Resource represents the target object of an authorization request.
//
// Type: describes the kind of resource (e.g., "AAS").
// Tenant: identifies the organization or owner of the resource.
// Attrs: contains arbitrary key-value pairs used during policy evaluation.
type Resource struct {
	Type   string
	Tenant string
	Attrs  map[string]any
}

const (
	// filterKey stores query filter restrictions inside the request context.
	filterKey ctxKey = "queryFilter"
)

// ResolveResource extracts a Resource from an HTTP request.
type ResolveResource func(r *http.Request) (Resource, error)

// ABACMiddleware returns an HTTP middleware handler that enforces attribute-based
// authorization based on the provided ABACSettings.
//
// If ABAC is disabled, the next handler is executed without checks.
// If enabled, Claims must be present in context or the request is rejected.
// If the model denies access, a 403 Forbidden is returned.
func ABACMiddleware(settings ABACSettings) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !settings.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			claims := FromContext(r)
			if claims == nil {
				http.Error(w, "missing claims context", http.StatusUnauthorized)
				return
			}

			if settings.Model != nil {
				opts := grammar.DefaultSimplifyOptions()
				opts.EnableImplicitCasts = settings.EnableImplicitCasts
				ok, reason, qf := settings.Model.AuthorizeWithFilterWithOptions(EvalInput{
					Method: r.Method,
					Path:   r.URL.Path,
					Claims: claims,
				}, opts)
				if !ok {
					log.Printf("❌ ABAC(model): %s", reason)

					resp := common.NewErrorResponse(errors.New("access denied"), http.StatusForbidden, "Middleware", "Rules", "Denied")
					err := openapi.EncodeJSONResponse(resp.Body, &resp.Code, w)
					if err != nil {
						log.Printf("❌ Failed to encode error response: %v", err)
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					}
					return
				}

				ctx := r.Context()
				if qf != nil {
					ctx = context.WithValue(ctx, filterKey, qf)
				}

				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			http.Error(w, "resource resolution failed", http.StatusForbidden)
		})
	}
}

// GetQueryFilter extracts a *QueryFilter from the provided context.
// It returns nil if no QueryFilter is stored under the filterKey.
//
// This helper can be used from any point in the codebase where the
// QueryFilter is needed. The returned filter may still require additional
// processing (e.g., building the actual AASQL expression) depending on the
// specific component using it.
func GetQueryFilter(ctx context.Context) *QueryFilter {
	if v := ctx.Value(filterKey); v != nil {
		if f, ok := v.(*QueryFilter); ok {
			return f
		}
	}
	return nil
}

// MergeQueryFilter combines an existing QueryFilter with a user query.
// It guards nils and merges conditions and filter fragments using logical AND.
func MergeQueryFilter(ctx context.Context, query grammar.Query) context.Context {
	qf := GetQueryFilter(ctx)
	if qf == nil {
		qf = &QueryFilter{}
	}

	if query.Condition != nil {
		if qf.Formula != nil {
			combinedQuery := grammar.LogicalExpression{And: []grammar.LogicalExpression{*qf.Formula, *query.Condition}}
			combinedQuery, _ = combinedQuery.SimplifyForBackendFilterNoResolver()
			qf.Formula = &combinedQuery
		} else {
			simplifiedQuery, _ := query.Condition.SimplifyForBackendFilterNoResolver()
			qf.Formula = &simplifiedQuery
		}
	}

	for _, filterCond := range query.FilterConditions {
		if filterCond.Fragment == nil || filterCond.Condition == nil {
			continue
		}
		if qf.Filters == nil {
			qf.Filters = make(FragmentFilters)
		}
		if existing, ok := qf.Filters[*filterCond.Fragment]; ok {
			combinedQuery := grammar.LogicalExpression{And: []grammar.LogicalExpression{existing, *filterCond.Condition}}
			combinedQuery, _ = combinedQuery.SimplifyForBackendFilterNoResolver()
			qf.Filters[*filterCond.Fragment] = combinedQuery
		} else {
			simplifiedQuery, _ := filterCond.Condition.SimplifyForBackendFilterNoResolver()
			qf.Filters[*filterCond.Fragment] = simplifiedQuery
		}
	}

	return context.WithValue(ctx, filterKey, qf)
}
