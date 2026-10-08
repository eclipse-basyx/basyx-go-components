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
	"sync"
)

// Mode describes how conditional headers apply to a route.
type Mode int

// Route modes.
const (
	// ModeExcluded ignores conditional headers. Writes are still tracked.
	ModeExcluded Mode = iota
	// ModeCollection evaluates conditional headers against a collection,
	// which always exists and has no entity tag.
	ModeCollection
	// ModeResource evaluates conditional headers against one resource.
	ModeResource
	// ModeVerifyOnly checks If-Match before an action on a resource
	// without changing the resource revision.
	ModeVerifyOnly
)

type stateContextKey struct{}

// State is the conditional request state of one request.
type State struct {
	mu             sync.Mutex
	method         string
	mode           Mode
	kind           Kind
	target         *ResourceRef
	conds          conditions
	requireIfMatch bool

	observed     int64
	observedSet  bool
	inconsistent bool
	compositeTag string

	evaluated bool
	writeETag string
	failure   error

	addressedExists *bool
}

func stateFromContext(ctx context.Context) *State {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(stateContextKey{}).(*State)
	return state
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// evaluatesWrites reports whether writes of this request must evaluate the
// target precondition.
func (s *State) evaluatesWrites() bool {
	if s == nil || s.mode != ModeResource || s.target == nil || isSafeMethod(s.method) {
		return false
	}
	return s.conds.present() || s.requireIfMatch
}

func (s *State) observesReads() bool {
	return s != nil && s.mode == ModeResource && s.target != nil && isSafeMethod(s.method)
}

func (s *State) isTarget(ref ResourceRef) bool {
	return s != nil && s.target != nil && *s.target == ref
}

func (s *State) observe(revision int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observedSet && s.observed != revision {
		s.inconsistent = true
	}
	s.observed = revision
	s.observedSet = true
}

// readValidator returns the concurrency validator observed for the target.
func (s *State) readValidator() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inconsistent {
		return ""
	}
	if s.compositeTag != "" {
		return s.compositeTag
	}
	if !s.observedSet || s.target == nil {
		return ""
	}
	return concurrencyValidator(*s.target, s.observed)
}

func (s *State) setFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure == nil {
		s.failure = err
	}
}

func (s *State) currentFailure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failure
}

func (s *State) isEvaluated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evaluated
}

func (s *State) recordCommit(evaluated bool, writeETag string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if evaluated {
		s.evaluated = true
	}
	if writeETag != "" {
		s.writeETag = writeETag
	}
}

func (s *State) currentWriteETag() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeETag
}

func (s *State) observeComposite(validator string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.compositeTag != "" && s.compositeTag != validator {
		s.inconsistent = true
	}
	s.compositeTag = validator
}

// SetAddressedExistence records whether the part of the request target that
// the request URL addresses existed, such as a Submodel element of a
// Submodel. If-Match: * and If-None-Match: * then refer to that part, while
// entity tags are still compared with the target's revision.
func SetAddressedExistence(ctx context.Context, ref ResourceRef, exists bool) {
	state := stateFromContext(ctx)
	if !state.isTarget(ref) {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.addressedExists = &exists
}

func (s *State) addressedExistence() *bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addressedExists
}
