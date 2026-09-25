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
	"errors"
	"testing"
)

type fixedChecker struct {
	allowed bool
	err     error
}

func (f fixedChecker) Check(context.Context, string, string, string, []Tuple) (bool, error) {
	return f.allowed, f.err
}

func TestCoordinatorIndependentGrants(t *testing.T) {
	cases := []struct {
		name        string
		rebac, abac bool
		wantSource  string
	}{
		{"relationship grant", true, false, "rebac"}, {"attribute fallback", false, true, "abac"}, {"deny", false, false, "abac"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := Coordinator{Checker: fixedChecker{allowed: tc.rebac}}
			decision, err := c.Authorize(context.Background(), AccessRequest{}, func(context.Context) (Decision, error) { calls++; return Decision{Allowed: tc.abac}, nil })
			if err != nil || decision.Allowed != (tc.rebac || tc.abac) || decision.Source != tc.wantSource {
				t.Fatalf("unexpected decision %+v: %v", decision, err)
			}
			if tc.rebac && calls != 0 {
				t.Fatal("ABAC must not veto a relationship grant")
			}
		})
	}
}

func TestCoordinatorOutageDoesNotFallBack(t *testing.T) {
	outage := errors.New("unavailable")
	c := Coordinator{Checker: fixedChecker{err: outage}}
	decision, err := c.Authorize(context.Background(), AccessRequest{}, func(context.Context) (Decision, error) {
		t.Fatal("fallback after outage")
		return Decision{Allowed: true}, nil
	})
	if !errors.Is(err, outage) || decision.Allowed {
		t.Fatalf("outage allowed: %+v %v", decision, err)
	}
}

func TestCoordinatorAuditFailurePreventsRelease(t *testing.T) {
	auditErr := errors.New("audit unavailable")
	c := Coordinator{Checker: fixedChecker{allowed: true}, Record: func(context.Context, AccessRequest, Decision, error) error { return auditErr }}
	decision, err := c.Authorize(context.Background(), AccessRequest{}, func(context.Context) (Decision, error) { return Decision{}, nil })
	if !errors.Is(err, auditErr) || decision.Allowed {
		t.Fatalf("unaudited grant: %+v %v", decision, err)
	}
}

func TestIdentityNamespacesDoNotCollide(t *testing.T) {
	a, _ := (Principal{Kind: "user", Issuer: "a", ID: "b:c"}).Object()
	b, _ := (Principal{Kind: "user", Issuer: "a:b", ID: "c"}).Object()
	if a == b {
		t.Fatal("issuer and subject collision")
	}
	group, _ := (Principal{Kind: "group", Issuer: "a", ID: "b:c"}).Object()
	if a == group {
		t.Fatal("user/group collision")
	}
	tuples, err := MembershipTuples(Principal{Kind: "user", Issuer: "a", ID: "u"}, []string{"g", "g"})
	if err != nil || len(tuples) != 1 {
		t.Fatalf("deduplication: %v %v", tuples, err)
	}
}
