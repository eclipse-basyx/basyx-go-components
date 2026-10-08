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

package conditional

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

// Options configure a Guard.
type Options struct {
	// RequireIfMatch answers 428 to writes of existing resources without
	// If-Match.
	RequireIfMatch bool
	// DecodeIdentifier decodes base64url encoded identifiers of route
	// parameters.
	DecodeIdentifier func(string) (string, error)
}

// Guard installs conditional request handling on routes.
type Guard struct {
	options Options
}

// NewGuard creates a Guard.
func NewGuard(options Options) *Guard {
	return &Guard{options: options}
}

// Wrap returns next with conditional request handling for the route pattern.
// It must wrap the route handler itself, so URL parameters are available.
func (g *Guard) Wrap(pattern string, next http.Handler) http.Handler {
	if g == nil {
		return next
	}
	policy := Classify(pattern)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := g.newState(r, policy)
		if policy.Mode == ModeCollection {
			if err := evaluateCollection(state); err != nil {
				writePreconditionError(w, err)
				return
			}
		}
		writer := newResponseWriter(w, state, r.URL.RequestURI())
		next.ServeHTTP(writer, r.WithContext(context.WithValue(r.Context(), stateContextKey{}, state)))
		writer.finish(r.Context())
	})
}

// WrapFunc is Wrap for handler functions.
func (g *Guard) WrapFunc(pattern string, next http.HandlerFunc) http.HandlerFunc {
	return g.Wrap(pattern, next).ServeHTTP
}

func (g *Guard) newState(r *http.Request, policy RoutePolicy) *State {
	state := &State{
		method:         r.Method,
		mode:           policy.Mode,
		kind:           policy.Kind,
		conds:          parseConditions(r.Header),
		requireIfMatch: g.options.RequireIfMatch,
	}
	if policy.Mode != ModeResource && policy.Mode != ModeVerifyOnly {
		return state
	}
	identifier, ok := g.targetIdentifier(r, policy)
	if !ok {
		state.mode = ModeExcluded
		return state
	}
	target := Ref(policy.Kind, identifier)
	state.target = &target
	return state
}

func (g *Guard) targetIdentifier(r *http.Request, policy RoutePolicy) (string, bool) {
	raw := chi.URLParam(r, policy.Param)
	if raw == "" {
		return "", false
	}
	if policy.encoding == encodingPath {
		if r.URL.RawPath == "" {
			return raw, true
		}
		decoded, err := url.PathUnescape(raw)
		return decoded, err == nil
	}
	if g.options.DecodeIdentifier == nil {
		return raw, true
	}
	decoded, err := g.options.DecodeIdentifier(raw)
	return decoded, err == nil && decoded != ""
}

// evaluateCollection evaluates conditional headers against a collection. A
// collection always exists and has no entity tag.
func evaluateCollection(state *State) error {
	if state.conds.ifMatch.present && !state.conds.ifMatch.any {
		return errIfMatchFailed()
	}
	if !isSafeMethod(state.method) && state.conds.ifNoneMatch.any {
		return errIfNoneMatchFailed()
	}
	return nil
}
