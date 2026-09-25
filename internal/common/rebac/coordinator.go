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

package rebac

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel"
	"time"
)

// Checker checks a single relationship against the pinned model.
type Checker interface {
	Check(context.Context, string, string, string, []Tuple) (bool, error)
}

// AccessRequest identifies one principal, permission, and resource.
type AccessRequest struct {
	User             string
	Relation         string
	Object           string
	ContextualTuples []Tuple
}

// Decision records an effective authorization result and its source.
type Decision struct {
	Allowed bool   `json:"allowed"`
	Source  string `json:"source"`
	Partial bool   `json:"partial,omitempty"`
}

// Fallback evaluates the existing ABAC policy only after a ReBAC denial.
type Fallback func(context.Context) (Decision, error)

// DecisionRecorder persists an authorization decision before data release.
type DecisionRecorder func(context.Context, AccessRequest, Decision, error) error

// Coordinator combines independent grants without altering ABAC evaluation.
// Record must durably persist the decision before the caller releases data.
type Coordinator struct {
	Checker Checker
	Record  DecisionRecorder
}

// Authorize checks ReBAC first and audits the effective decision.
func (c Coordinator) Authorize(ctx context.Context, request AccessRequest, fallback Fallback) (decision Decision, err error) {
	started := time.Now()
	ctx, span := otel.Tracer("basyx/rebac").Start(ctx, "authorization.coordinate")
	defer func() {
		observeCoordination(ctx, started, decision, err)
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}()
	if c.Checker == nil || fallback == nil {
		return Decision{}, fmt.Errorf("REBAC-COORDINATOR-CONFIG checker and ABAC fallback are required")
	}
	decision, err = c.evaluate(ctx, request, fallback)
	if c.Record != nil {
		if auditErr := c.Record(ctx, request, decision, err); auditErr != nil {
			return Decision{}, fmt.Errorf("REBAC-COORDINATOR-AUDIT: %w", auditErr)
		}
	}
	return decision, err
}

func (c Coordinator) evaluate(ctx context.Context, request AccessRequest, fallback Fallback) (Decision, error) {
	allowed, err := c.Checker.Check(ctx, request.User, request.Relation, request.Object, request.ContextualTuples)
	if err != nil {
		return Decision{Source: "rebac"}, fmt.Errorf("REBAC-COORDINATOR-CHECK: %w", err)
	}
	if allowed {
		return Decision{Allowed: true, Source: "rebac"}, nil
	}
	decision, err := fallback(ctx)
	decision.Source = "abac"
	return decision, err
}
