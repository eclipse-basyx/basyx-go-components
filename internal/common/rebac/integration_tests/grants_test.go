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

package integration_tests

import (
	"context"
	"errors"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
	"github.com/google/uuid"
)

func TestGrantRevisionAndOwnership(t *testing.T) {
	db := testDatabase(t)
	state := &rebac.StateStore{DB: db, Scope: "grant-" + uuid.NewString()}
	if err := state.Initialize(t.Context(), "config"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := state.CreateResource(t.Context(), tx, "aas", "machine", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	service := rebac.GrantService{State: state, Checker: managerChecker{}}
	alice := rebac.Principal{Kind: "user", Issuer: "https://issuer.example", ID: "alice"}
	revision, err := service.Replace(t.Context(), rebac.GrantActor{Administrator: true}, resource, 0, []rebac.Grant{{Principal: alice, Role: "owner"}})
	if err != nil || revision != 1 {
		t.Fatalf("bootstrap: %d %v", revision, err)
	}
	if _, err = service.Replace(t.Context(), rebac.GrantActor{Administrator: true}, resource, 0, nil); !errors.Is(err, rebac.ErrStaleRevision) {
		t.Fatalf("stale accepted: %v", err)
	}
	if _, err = state.ProjectNext(t.Context(), &testWriter{}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Replace(t.Context(), rebac.GrantActor{User: "user:alice"}, resource, revision, nil); !errors.Is(err, rebac.ErrLastOwner) {
		t.Fatalf("last owner removed: %v", err)
	}
	service.Checker = deniedManager{}
	if _, err = service.Replace(t.Context(), rebac.GrantActor{User: "user:other"}, resource, revision, []rebac.Grant{{Principal: alice, Role: "owner"}}); !errors.Is(err, rebac.ErrForbidden) {
		t.Fatalf("unauthorized sharing: %v", err)
	}
}

type managerChecker struct{}

func (managerChecker) Check(context.Context, string, string, string, []rebac.Tuple) (bool, error) {
	return true, nil
}

type deniedManager struct{}

func (deniedManager) Check(context.Context, string, string, string, []rebac.Tuple) (bool, error) {
	return false, nil
}
