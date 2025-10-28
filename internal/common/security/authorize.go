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
// Package auth provides ABAC (Attribute-Based Access Control) middleware and
// helper utilities to enforce fine-grained authorization rules in BaSyx
// services.
// Author: Martin Stemmer ( Fraunhofer IESE )
package auth

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	openapi "github.com/eclipse-basyx/basyx-go-components/pkg/discoveryapi"
)

// ABACSettings defines the configuration used to enable and control
// Attribute-Based Access Control.
//
// Enabled: toggles ABAC enforcement.
// ClientRolesAudience: defines the expected client audience for role validation.
// Model: provides the AccessModel that evaluates authorization rules.
type ABACSettings struct {
	Enabled             bool
	ClientRolesAudience string
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

// Input bundles subject claims, action, and resource for ABAC evaluation.
type Input struct {
	Subject  Claims
	Action   string
	Resource Resource
	Env      Env
}

// Env represents environmental authorization context (e.g., current UTC).
type Env struct {
	UTCNow time.Time
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
				ok, reason, qf := settings.Model.AuthorizeWithFilter(EvalInput{
					Method:    r.Method,
					Path:      r.URL.Path,
					Claims:    claims,
					IssuedUTC: time.Now().UTC(),
				})
				if !ok {
					log.Printf("❌ ABAC(model): %s", reason)

					resp := common.NewErrorResponse(errors.New("access denied"), http.StatusForbidden, "Middleware", "Rules", "Denied")
					openapi.EncodeJSONResponse(resp.Body, &resp.Code, w)
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

// FromFilter returns a QueryFilter previously stored in the request context.
// It is used to enforce policy-induced restrictions on downstream queries.
// TODO: use this helper in backend if you need further restriction
func FromFilter(r *http.Request) *QueryFilter {
	if v := r.Context().Value(filterKey); v != nil {
		if f, ok := v.(*QueryFilter); ok {
			return f
		}
	}
	return nil
}

// FromFilterCtx extracts a QueryFilter directly from a context instance.
// TODO: use this helper in backend if you need further restriction
func FromFilterCtx(ctx context.Context) *QueryFilter {
	if v := ctx.Value(filterKey); v != nil {
		if f, ok := v.(*QueryFilter); ok {
			return f
		}
	}
	return nil
}
