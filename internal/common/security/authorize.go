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

type ABACSettings struct {
	Enabled             bool
	ClientRolesAudience string
	Model               *AccessModel
}

type Resource struct {
	Type   string
	Tenant string
	Attrs  map[string]any
}

type Input struct {
	Subject  Claims
	Action   string
	Resource Resource
	Env      Env
}

type Env struct {
	UTCNow time.Time
}

const (
	filterKey ctxKey = "queryFilter"
)

type ResolveResource func(r *http.Request) (Resource, error)

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

func FromFilter(r *http.Request) *QueryFilter {
	if v := r.Context().Value(filterKey); v != nil {
		if f, ok := v.(*QueryFilter); ok {
			return f
		}
	}
	return nil
}

func FromFilterCtx(ctx context.Context) *QueryFilter {
	if v := ctx.Value(filterKey); v != nil {
		if f, ok := v.(*QueryFilter); ok {
			return f
		}
	}
	return nil
}

func HasRealmRole(claims Claims, role string) bool {
	ra, ok := claims["realm_access"].(map[string]any)
	if !ok {
		return false
	}
	roles, ok := ra["roles"].([]any)
	if !ok {
		return false
	}
	for _, r := range roles {
		if s, ok := r.(string); ok && s == role {
			return true
		}
	}
	return false
}

func HasClientRole(claims Claims, clientID, role string) bool {
	resAcc, ok := claims["resource_access"].(map[string]any)
	if !ok {
		return false
	}
	client, ok := resAcc[clientID].(map[string]any)
	if !ok {
		return false
	}
	roles, ok := client["roles"].([]any)
	if !ok {
		return false
	}
	for _, r := range roles {
		if s, ok := r.(string); ok && s == role {
			return true
		}
	}
	return false
}

func SimpleResolver(tenantHeaderOrQuery string) ResolveResource {
	return func(r *http.Request) (Resource, error) {
		t := r.URL.Query().Get(tenantHeaderOrQuery)
		if t == "" {
			t = r.Header.Get(tenantHeaderOrQuery)
		}
		if t == "" {
			return Resource{}, errors.New("tenant not found in request")
		}
		return Resource{
			Type:   "AAS",
			Tenant: t,
			Attrs:  map[string]any{},
		}, nil
	}
}
